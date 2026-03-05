package model

import "time"

// AccountStatus represents the lifecycle state of an account.
type AccountStatus string

const (
	StatusSleeping  AccountStatus = "sleeping"
	StatusConnecting AccountStatus = "connecting"
	StatusActive    AccountStatus = "active"
	StatusError     AccountStatus = "error"
)

// AccountInfo is the API-facing account representation.
type AccountInfo struct {
	ID          string        `json:"id"`
	PhoneNumber *string       `json:"phone_number"`
	AccountName string        `json:"account_name"`
	Status      AccountStatus `json:"status"`
	Authorized  bool          `json:"authorized"`
	CreatedAt   time.Time     `json:"created_at"`
}

// CreateAccountRequest is the JSON body for POST /accounts.
type CreateAccountRequest struct {
	PhoneNumber string  `json:"phone_number"`
	AccountName string  `json:"account_name"`
	IdleTimeout *int64  `json:"idle_timeout,omitempty"`
}

// CreateAccountResponse is returned after creating an account.
type CreateAccountResponse struct {
	ID          string `json:"id"`
	PhoneNumber string `json:"phone_number"`
	AccountName string `json:"account_name"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

// AccountListResponse is the response for GET /accounts.
type AccountListResponse struct {
	Accounts []AccountInfo `json:"accounts"`
	Total    int           `json:"total"`
}

// AccountActionResponse is a generic action acknowledgement.
type AccountActionResponse struct {
	Message   string `json:"message"`
	AccountID string `json:"account_id"`
}

// DeleteAccountResponse is the response for DELETE /accounts/{id}.
type DeleteAccountResponse struct {
	Message     string `json:"message"`
	AccountID   string `json:"account_id"`
	DataDeleted bool   `json:"data_deleted"`
}

// UpdateAccountConfigRequest is the JSON body for PUT /accounts/{id}/config.
type UpdateAccountConfigRequest struct {
	AccountName *string `json:"account_name,omitempty"`
	IdleTimeout *int64  `json:"idle_timeout,omitempty"`
}

// AccountConfig is the API-facing account configuration.
type AccountConfig struct {
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name"`
	IdleTimeout int64  `json:"idle_timeout"`
}

// WhatsAppStatusResponse is the response for GET /accounts/{id}/status.
type WhatsAppStatusResponse struct {
	AccountID   string  `json:"account_id"`
	PhoneNumber *string `json:"phone_number"`
	Status      string  `json:"status"`
	Authorized  bool    `json:"authorized"`
}

// PhoneLinkResponse is the response for phone-number linking.
type PhoneLinkResponse struct {
	LinkingCode string `json:"linking_code"`
}

// ChatInfo represents a single chat in the chat list.
type ChatInfo struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	LastMessage       *string `json:"last_message"`
	LastMessageSender *string `json:"last_message_sender"`
	Timestamp         *string `json:"timestamp"`
	UnreadCount       int     `json:"unread_count"`
	IsGroup           bool    `json:"is_group"`
}

// ChatListResponse is the response for GET /accounts/{id}/chats.
type ChatListResponse struct {
	Chats []ChatInfo `json:"chats"`
	Total int        `json:"total"`
}

// MessageInfo represents a single message.
type MessageInfo struct {
	ID            string  `json:"id"`
	FromMe        bool    `json:"from_me"`
	Sender        *string `json:"sender"`
	Text          *string `json:"text"`
	MessageType   string  `json:"message_type"`
	Timestamp     *string `json:"timestamp"`
	TimestampUnix *int64  `json:"timestamp_unix"`
	Status        *string `json:"status"`
	MediaInfo     *string `json:"media_info"`
}

// MessageListResponse is the response for GET /accounts/{id}/chats/{chat_id}/messages.
type MessageListResponse struct {
	ChatID   string        `json:"chat_id"`
	ChatName *string       `json:"chat_name"`
	Messages []MessageInfo `json:"messages"`
	Total    int           `json:"total"`
	HasMore  bool          `json:"has_more"`
}

// SendMessageRequest is the JSON body for POST /accounts/{id}/chats/{chat_id}/messages.
type SendMessageRequest struct {
	Text *string `json:"text,omitempty"`
	// File handled separately via multipart
}

// SendMessageResponse is the response after sending a message.
type SendMessageResponse struct {
	Status    string `json:"status"`
	MessageID string `json:"message_id"`
}

// ContactInfo represents a contact's details.
type ContactInfo struct {
	ID         string  `json:"id"`
	Name       *string `json:"name"`
	PushName   *string `json:"push_name"`
	Phone      *string `json:"phone"`
	Status     *string `json:"status"`
	IsBusiness bool    `json:"is_business"`
}

// GroupParticipant is a member of a group.
type GroupParticipant struct {
	ID      string  `json:"id"`
	Name    *string `json:"name"`
	IsAdmin bool    `json:"is_admin"`
}

// GroupInfo represents a group's details.
type GroupInfo struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Description      *string            `json:"description"`
	CreatedAt        *string            `json:"created_at"`
	CreatedBy        *string            `json:"created_by"`
	ParticipantCount int                `json:"participant_count"`
	Participants     []GroupParticipant `json:"participants"`
	IsAnnounce       bool               `json:"is_announce"`
	IsLocked         bool               `json:"is_locked"`
}

// TypingRequest is the JSON body for POST /accounts/{id}/chats/{chat_id}/typing.
type TypingRequest struct {
	State string `json:"state"` // "composing" or "paused"
}

// ReactionRequest is the JSON body for reacting to a message.
type ReactionRequest struct {
	Emoji string `json:"emoji"`
}

// ReplyMessageRequest is the JSON body for replying to a message.
type ReplyMessageRequest struct {
	Text string `json:"text"`
}

// MarkReadRequest is the JSON body for marking messages as read.
type MarkReadRequest struct {
	MessageIDs []string `json:"message_ids"`
}

// ErrorResponse is a JSON error payload.
type ErrorResponse struct {
	Error   string  `json:"error"`
	Message *string `json:"message,omitempty"`
}
