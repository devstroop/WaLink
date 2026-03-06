package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/devstroop/walink/internal/service"
)

// New creates an MCP server backed by the given AccountManager.
// It exposes walink's core capabilities as MCP tools.
func New(mgr *service.AccountManager, version string) *server.MCPServer {
	s := server.NewMCPServer(
		"WaLink",
		version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	registerTools(s, mgr)
	return s
}

// helper to marshal any value to JSON text for MCP tool results.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

// requireAccount is a helper that looks up an account and returns an error result if not found.
func requireAccount(mgr *service.AccountManager, req mcp.CallToolRequest) (*service.Account, *mcp.CallToolResult) {
	id, err := req.RequireString("account_id")
	if err != nil {
		return nil, mcp.NewToolResultError("account_id is required")
	}
	acct := mgr.GetAccount(id)
	if acct == nil {
		return nil, mcp.NewToolResultError(fmt.Sprintf("account %q not found", id))
	}
	return acct, nil
}

// requireConnectedAccount looks up an account and ensures it's connected.
func requireConnectedAccount(ctx context.Context, mgr *service.AccountManager, req mcp.CallToolRequest) (*service.Account, *mcp.CallToolResult) {
	acct, errResult := requireAccount(mgr, req)
	if errResult != nil {
		return nil, errResult
	}
	if err := acct.EnsureConnected(ctx); err != nil {
		return nil, mcp.NewToolResultError(fmt.Sprintf("connect: %v", err))
	}
	return acct, nil
}

func registerTools(s *server.MCPServer, mgr *service.AccountManager) {
	// ── Account management ──────────────────────────

	s.AddTool(
		mcp.NewTool("list_accounts",
			mcp.WithDescription("List all WhatsApp accounts managed by WaLink"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return jsonResult(mgr.ListAccounts())
		},
	)

	s.AddTool(
		mcp.NewTool("get_session",
			mcp.WithDescription("Get session/authentication status for a WhatsApp account"),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			acct, errResult := requireAccount(mgr, req)
			if errResult != nil {
				return errResult, nil
			}
			return jsonResult(acct.StatusResponse())
		},
	)

	// ── Messaging ───────────────────────────────────

	s.AddTool(
		mcp.NewTool("send_message",
			mcp.WithDescription("Send a text message to a WhatsApp chat or phone number"),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account ID")),
			mcp.WithString("text", mcp.Required(), mcp.Description("Message text")),
			mcp.WithString("phone", mcp.Description("Phone number (international, digits only). Provide phone or jid, not both.")),
			mcp.WithString("jid", mcp.Description("WhatsApp JID (e.g. 919999999999@s.whatsapp.net). Provide phone or jid, not both.")),
			mcp.WithString("reply_to", mcp.Description("Message ID to reply to (optional)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			acct, errResult := requireConnectedAccount(ctx, mgr, req)
			if errResult != nil {
				return errResult, nil
			}

			text, err := req.RequireString("text")
			if err != nil {
				return mcp.NewToolResultError("text is required"), nil
			}

			phone := req.GetString("phone", "")
			jid := req.GetString("jid", "")
			replyTo := req.GetString("reply_to", "")

			if phone == "" && jid == "" {
				return mcp.NewToolResultError("phone or jid is required"), nil
			}
			if phone != "" && jid != "" {
				return mcp.NewToolResultError("provide phone or jid, not both"), nil
			}

			chatJID := jid
			if phone != "" {
				resolved, err := acct.ResolvePhone(ctx, phone)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				chatJID = resolved
			}

			var msgID string
			if replyTo != "" {
				msgID, err = acct.SendReply(ctx, chatJID, replyTo, text)
			} else {
				msgID, err = acct.SendMessage(ctx, chatJID, text)
			}
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(map[string]string{"status": "sent", "message_id": msgID})
		},
	)

	// ── Contacts ────────────────────────────────────

	s.AddTool(
		mcp.NewTool("check_contacts",
			mcp.WithDescription("Check if phone numbers are registered on WhatsApp"),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account ID")),
			mcp.WithString("phones", mcp.Required(), mcp.Description("Comma-separated phone numbers (international, digits only)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			acct, errResult := requireConnectedAccount(ctx, mgr, req)
			if errResult != nil {
				return errResult, nil
			}

			phonesStr, err := req.RequireString("phones")
			if err != nil {
				return mcp.NewToolResultError("phones is required"), nil
			}

			results, err := acct.CheckContacts(ctx, splitCSV(phonesStr))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(results)
		},
	)

	s.AddTool(
		mcp.NewTool("get_contact",
			mcp.WithDescription("Get contact details (name, picture) by JID"),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account ID")),
			mcp.WithString("jid", mcp.Required(), mcp.Description("Contact JID (e.g. 919999999999@s.whatsapp.net)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			acct, errResult := requireConnectedAccount(ctx, mgr, req)
			if errResult != nil {
				return errResult, nil
			}

			jid, err := req.RequireString("jid")
			if err != nil {
				return mcp.NewToolResultError("jid is required"), nil
			}

			info, err := acct.GetContactInfo(ctx, jid)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(info)
		},
	)

	// ── Groups ──────────────────────────────────────

	s.AddTool(
		mcp.NewTool("list_groups",
			mcp.WithDescription("List all WhatsApp groups the account has joined"),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			acct, errResult := requireConnectedAccount(ctx, mgr, req)
			if errResult != nil {
				return errResult, nil
			}

			groups, err := acct.ListGroups(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(map[string]any{"groups": groups, "total": len(groups)})
		},
	)

	s.AddTool(
		mcp.NewTool("get_group",
			mcp.WithDescription("Get detailed info about a WhatsApp group"),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account ID")),
			mcp.WithString("jid", mcp.Required(), mcp.Description("Group JID (e.g. 120363012345@g.us)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			acct, errResult := requireConnectedAccount(ctx, mgr, req)
			if errResult != nil {
				return errResult, nil
			}

			jid, err := req.RequireString("jid")
			if err != nil {
				return mcp.NewToolResultError("jid is required"), nil
			}

			info, err := acct.GetGroupInfo(ctx, jid)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(info)
		},
	)

	// ── Profile ─────────────────────────────────────

	s.AddTool(
		mcp.NewTool("get_profile",
			mcp.WithDescription("Get the WhatsApp profile of an account (name, about, picture)"),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			acct, errResult := requireConnectedAccount(ctx, mgr, req)
			if errResult != nil {
				return errResult, nil
			}

			profile, err := acct.GetProfile(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(profile)
		},
	)
}

// splitCSV splits a comma-separated string into trimmed, non-empty parts.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
