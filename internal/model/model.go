package model

import "time"

// AccountStatus represents the lifecycle state of an account.
type AccountStatus string

const (
	StatusSleeping   AccountStatus = "sleeping"
	StatusConnecting AccountStatus = "connecting"
	StatusActive     AccountStatus = "active"
	StatusError      AccountStatus = "error"
)

// ──────────────────────────────────────────────────────
// Account CRUD
// ──────────────────────────────────────────────────────

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
	PhoneNumber string `json:"phone_number"`
	AccountName string `json:"account_name"`
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

// UpdateAccountRequest is the JSON body for PATCH /accounts/{id}.
type UpdateAccountRequest struct {
	AccountName *string `json:"account_name,omitempty"`
	PhoneNumber *string `json:"phone_number,omitempty"`
}

// ──────────────────────────────────────────────────────
// Session
// ──────────────────────────────────────────────────────

// WhatsAppStatusResponse is the response for GET /accounts/{id}/session.
type WhatsAppStatusResponse struct {
	AccountID   string  `json:"account_id"`
	PhoneNumber *string `json:"phone_number"`
	Status      string  `json:"status"`
	Authorized  bool    `json:"authorized"`
}

// PhoneLinkResponse is the response for phone-number pairing.
type PhoneLinkResponse struct {
	LinkingCode string `json:"linking_code"`
}

// ──────────────────────────────────────────────────────
// Messaging
// ──────────────────────────────────────────────────────

// SendMessageRequest is the JSON body for POST /accounts/{id}/messages.
type SendMessageRequest struct {
	Chat string  `json:"chat"`
	Text *string `json:"text,omitempty"`
	// File handled separately via multipart
}

// SendMessageResponse is the response after sending a message.
type SendMessageResponse struct {
	Status    string `json:"status"`
	MessageID string `json:"message_id"`
}

// ReactionRequest is the JSON body for POST /accounts/{id}/messages/react.
type ReactionRequest struct {
	Chat      string `json:"chat"`
	MessageID string `json:"message_id"`
	Emoji     string `json:"emoji"`
}

// ReplyMessageRequest is the JSON body for POST /accounts/{id}/messages/reply.
type ReplyMessageRequest struct {
	Chat      string `json:"chat"`
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
}

// MarkReadRequest is the JSON body for POST /accounts/{id}/messages/read.
type MarkReadRequest struct {
	Chat       string   `json:"chat"`
	MessageIDs []string `json:"message_ids"`
}

// ──────────────────────────────────────────────────────
// Chats
// ──────────────────────────────────────────────────────

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

// MessageListResponse is the response for GET /accounts/{id}/chats/{jid}/messages.
type MessageListResponse struct {
	ChatID   string        `json:"chat_id"`
	ChatName *string       `json:"chat_name"`
	Messages []MessageInfo `json:"messages"`
	Total    int           `json:"total"`
	HasMore  bool          `json:"has_more"`
}

// ──────────────────────────────────────────────────────
// Contacts
// ──────────────────────────────────────────────────────

// ContactInfo represents a contact's details.
type ContactInfo struct {
	ID         string  `json:"id"`
	Name       *string `json:"name"`
	PushName   *string `json:"push_name"`
	Phone      *string `json:"phone"`
	Status     *string `json:"status"`
	IsBusiness bool    `json:"is_business"`
}

// ──────────────────────────────────────────────────────
// Groups
// ──────────────────────────────────────────────────────

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
	InviteLink       *string            `json:"invite_link,omitempty"`
}

// GroupListResponse is the response for GET /accounts/{id}/groups.
type GroupListResponse struct {
	Groups []GroupInfo `json:"groups"`
	Total  int         `json:"total"`
}

// CreateGroupRequest is the JSON body for POST /accounts/{id}/groups.
type CreateGroupRequest struct {
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
}

// UpdateGroupRequest is the JSON body for PATCH /accounts/{id}/groups/{jid}.
type UpdateGroupRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Locked      *bool   `json:"locked,omitempty"`
	Announce    *bool   `json:"announce,omitempty"`
}

// GroupParticipantsRequest is the JSON body for POST /accounts/{id}/groups/{jid}/participants.
type GroupParticipantsRequest struct {
	Participants []string `json:"participants"`
	Action       string   `json:"action"` // add, remove, promote, demote
}

// GroupInviteLinkResponse is the response for GET /accounts/{id}/groups/{jid}/invite.
type GroupInviteLinkResponse struct {
	InviteLink string `json:"invite_link"`
}

// ──────────────────────────────────────────────────────
// Presence
// ──────────────────────────────────────────────────────

// PresenceRequest is the JSON body for POST /accounts/{id}/presence.
type PresenceRequest struct {
	// For chat typing: "composing" or "paused". For global: "available" or "unavailable".
	State string `json:"state"`
	// Optional chat JID for typing indicators. Omit for global presence.
	Chat *string `json:"chat,omitempty"`
}

// ──────────────────────────────────────────────────────
// Profile
// ──────────────────────────────────────────────────────

// ProfileResponse is returned for GET /accounts/{id}/profile.
type ProfileResponse struct {
	ID          string  `json:"id"`
	PhoneNumber *string `json:"phone_number"`
	About       *string `json:"about"`
	PictureURL  *string `json:"picture_url"`
}

// UpdateProfileRequest is the JSON body for PATCH /accounts/{id}/profile.
type UpdateProfileRequest struct {
	About *string `json:"about,omitempty"`
}

// ──────────────────────────────────────────────────────
// Privacy
// ──────────────────────────────────────────────────────

// PrivacySettings maps setting names to their values.
type PrivacySettings struct {
	GroupAdd     string `json:"group_add"`
	LastSeen     string `json:"last_seen"`
	Status       string `json:"status"`
	Profile      string `json:"profile"`
	ReadReceipts string `json:"read_receipts"`
	Online       string `json:"online"`
	CallAdd      string `json:"call_add"`
}

// UpdatePrivacyRequest is the JSON body for PATCH /accounts/{id}/privacy.
type UpdatePrivacyRequest struct {
	GroupAdd     *string `json:"group_add,omitempty"`
	LastSeen     *string `json:"last_seen,omitempty"`
	Status       *string `json:"status,omitempty"`
	Profile      *string `json:"profile,omitempty"`
	ReadReceipts *string `json:"read_receipts,omitempty"`
	Online       *string `json:"online,omitempty"`
	CallAdd      *string `json:"call_add,omitempty"`
}

// ──────────────────────────────────────────────────────
// Newsletters
// ──────────────────────────────────────────────────────

// NewsletterInfo represents a newsletter/channel.
type NewsletterInfo struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     *string `json:"description"`
	SubscriberCount int     `json:"subscriber_count"`
	Role            *string `json:"role"`
	Muted           bool    `json:"muted"`
	PictureURL      *string `json:"picture_url"`
}

// NewsletterListResponse is the response for GET /accounts/{id}/newsletters.
type NewsletterListResponse struct {
	Newsletters []NewsletterInfo `json:"newsletters"`
	Total       int              `json:"total"`
}

// CreateNewsletterRequest is the JSON body for POST /accounts/{id}/newsletters.
type CreateNewsletterRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ──────────────────────────────────────────────────────
// Common
// ──────────────────────────────────────────────────────

// TypingRequest is kept for backward compat, but PresenceRequest is preferred.
type TypingRequest = PresenceRequest

// ErrorResponse is a JSON error payload.
type ErrorResponse struct {
	Error   string  `json:"error"`
	Message *string `json:"message,omitempty"`
}
