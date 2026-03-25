// Package agent provides an OpenAI-compatible LLM client with streaming support.
package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Message roles.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message is a single turn in the conversation.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall is a tool invocation requested by the model.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Tool defines a callable function exposed to the model.
type Tool struct {
	Type     string   `json:"type"` // always "function"
	Function ToolSpec `json:"function"`
}

// ToolSpec describes the function signature.
type ToolSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// LLMConfig holds provider settings.
type LLMConfig struct {
	Provider string // "openai" | "ollama"
	APIKey   string
	BaseURL  string // e.g. https://api.openai.com or http://localhost:11434
	Model    string
}

// Client is an OpenAI-compatible LLM HTTP client.
// It works with OpenAI and Ollama (which exposes /v1/chat/completions).
type Client struct {
	cfg    LLMConfig
	client *http.Client
}

// NewClient builds a ready-to-use LLM client.
func NewClient(cfg LLMConfig) *Client {
	if cfg.BaseURL == "" {
		if cfg.Provider == "ollama" {
			cfg.BaseURL = "http://localhost:11434"
		} else {
			cfg.BaseURL = "https://api.openai.com"
		}
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	return &Client{
		cfg:    cfg,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// ── Non-streaming ────────────────────────────────────────────────────────────

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat sends a non-streaming chat completion request.
func (c *Client) Chat(ctx context.Context, messages []Message, tools []Tool) (*Message, error) {
	body, _ := json.Marshal(chatRequest{
		Model:    c.cfg.Model,
		Messages: messages,
		Tools:    tools,
		Stream:   false,
	})

	req, err := c.newRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		var errBody struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errBody); err == nil && errBody.Error != nil {
			return nil, fmt.Errorf("llm api error %d: %s", resp.StatusCode, errBody.Error.Message)
		}
		return nil, fmt.Errorf("llm api error: HTTP %d", resp.StatusCode)
	}

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("llm decode: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("llm error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("llm: no choices returned")
	}
	msg := out.Choices[0].Message
	return &msg, nil
}

// ── Streaming ────────────────────────────────────────────────────────────────

// StreamChunk is one piece of a streaming response.
type StreamChunk struct {
	Content string // text token
	Done    bool   // stream ended
	Err     error  // error that stopped the stream
}

// ChatStream sends a streaming chat completion and returns a channel of chunks.
// Only use after tool-calling is complete (no tools slice).
func (c *Client) ChatStream(ctx context.Context, messages []Message) (<-chan StreamChunk, error) {
	body, _ := json.Marshal(chatRequest{
		Model:    c.cfg.Model,
		Messages: messages,
		Stream:   true,
	})

	req, err := c.newRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm stream: %w", err)
	}

	ch := make(chan StreamChunk, 32)
	go func() {
		defer func() { _ = resp.Body.Close() }()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- StreamChunk{Done: true}
				return
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				select {
				case ch <- StreamChunk{Content: chunk.Choices[0].Delta.Content}:
				case <-ctx.Done():
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- StreamChunk{Err: err, Done: true}
		}
	}()

	return ch, nil
}

func (c *Client) newRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	return req, nil
}
