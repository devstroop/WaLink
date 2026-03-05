package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/devstroop/walink/internal/model"
	"github.com/devstroop/walink/internal/service"
)

// ── Session ─────────────────────────────────────────

// GetSession — GET /api/v1/accounts/{account_id}/session
// If the account has stored credentials but no active connection, this endpoint
// connects to WhatsApp to verify the session is still valid. A session revoked
// from the phone will be detected and cleaned up automatically.
func (a *API) GetSession(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	// If there's stored session data, connect to verify it's still valid.
	// A revoked session will trigger the LoggedOut event, clearing stale data.
	if acct.HasStoredCredentials() && !acct.IsLoggedIn() {
		_ = acct.EnsureConnected(r.Context())   // best-effort
		time.Sleep(2 * time.Second)              // give whatsmeow time to auth or fire LoggedOut
	}

	writeJSON(w, http.StatusOK, acct.StatusResponse())
}

// ConnectSession — POST /api/v1/accounts/{account_id}/session/connect
func (a *API) ConnectSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	acct := a.mgr.GetAccount(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	// Wait briefly for the async auth handshake to complete
	for i := 0; i < 10; i++ {
		if acct.IsLoggedIn() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	writeJSON(w, http.StatusOK, acct.StatusResponse())
}

// GetQR — GET /api/v1/accounts/{account_id}/session/qr
func (a *API) GetQR(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	acct := a.mgr.GetAccount(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	ch, err := acct.GetQR(ctx)
	if err != nil {
		if err.Error() == "already logged in" {
			writeError(w, http.StatusConflict, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	select {
	case item := <-ch:
		if item.Error != nil {
			service.DrainQR(ch)
			writeError(w, http.StatusInternalServerError, item.Error.Error())
			return
		}
		if item.Event == "code" {
			// Start draining remaining QR events AFTER we got our code.
			// This prevents whatsmeow from disconnecting when the
			// channel buffer fills up with subsequent codes.
			service.DrainQR(ch)

			png, err := qrcode.Encode(item.Code, qrcode.Medium, 512)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to render QR: "+err.Error())
				return
			}
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			w.Write(png)
			return
		}
		service.DrainQR(ch)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("unexpected qr event: %s", item.Event))
	case <-ctx.Done():
		service.DrainQR(ch)
		writeError(w, http.StatusGatewayTimeout, "timeout waiting for QR code")
	}
}

// PairPhone — POST /api/v1/accounts/{account_id}/session/pair
func (a *API) PairPhone(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	acct := a.mgr.GetAccount(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	code, err := acct.PairPhone(r.Context(), acct.PhoneNumber)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.PhoneLinkResponse{LinkingCode: code})
}

// DeleteSession — DELETE /api/v1/accounts/{account_id}/session
func (a *API) DeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	acct := a.mgr.GetAccount(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.Logout(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.AccountActionResponse{
		Message:   "unlinked",
		AccountID: id,
	})
}

// ── Messaging ───────────────────────────────────────

// SendMessage — POST /api/v1/accounts/{account_id}/messages
func (a *API) SendMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	acct := a.mgr.GetAccount(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	ct := r.Header.Get("Content-Type")

	// Handle multipart/form-data for file uploads
	if len(ct) > 19 && ct[:19] == "multipart/form-data" {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "parse multipart: "+err.Error())
			return
		}

		chatID := r.FormValue("chat")
		if chatID == "" {
			writeError(w, http.StatusBadRequest, "chat required")
			return
		}
		text := r.FormValue("text")

		file, header, err := r.FormFile("file")
		if err == nil {
			defer file.Close()
			data := make([]byte, header.Size)
			if _, err := file.Read(data); err != nil {
				writeError(w, http.StatusInternalServerError, "read file: "+err.Error())
				return
			}
			var caption *string
			if text != "" {
				caption = &text
			}
			msgID, err := acct.SendMedia(r.Context(), chatID, data, header.Filename, header.Header.Get("Content-Type"), caption)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, model.SendMessageResponse{Status: "sent", MessageID: msgID})
			return
		}

		// No file — send text only
		if text == "" {
			writeError(w, http.StatusBadRequest, "text or file required")
			return
		}
		msgID, err2 := acct.SendMessage(r.Context(), chatID, text)
		if err2 != nil {
			writeError(w, http.StatusInternalServerError, err2.Error())
			return
		}
		writeJSON(w, http.StatusOK, model.SendMessageResponse{Status: "sent", MessageID: msgID})
		return
	}

	// JSON body
	var req model.SendMessageRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.Chat == "" {
		writeError(w, http.StatusBadRequest, "chat required")
		return
	}
	if req.Text == nil || *req.Text == "" {
		writeError(w, http.StatusBadRequest, "text required")
		return
	}

	msgID, err := acct.SendMessage(r.Context(), req.Chat, *req.Text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.SendMessageResponse{Status: "sent", MessageID: msgID})
}

// ReactMessage — POST /api/v1/accounts/{account_id}/messages/react
func (a *API) ReactMessage(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	var req model.ReactionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.Chat == "" || req.MessageID == "" || req.Emoji == "" {
		writeError(w, http.StatusBadRequest, "chat, message_id, and emoji required")
		return
	}

	if err := acct.SendReaction(r.Context(), req.Chat, req.MessageID, req.Emoji); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": req.MessageID,
		"emoji":      req.Emoji,
	})
}

// ReplyMessage — POST /api/v1/accounts/{account_id}/messages/reply
func (a *API) ReplyMessage(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	var req model.ReplyMessageRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.Chat == "" || req.MessageID == "" || req.Text == "" {
		writeError(w, http.StatusBadRequest, "chat, message_id, and text required")
		return
	}

	replyID, err := acct.SendReply(r.Context(), req.Chat, req.MessageID, req.Text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.SendMessageResponse{
		Status:    "sent",
		MessageID: replyID,
	})
}

// MarkRead — POST /api/v1/accounts/{account_id}/messages/read
func (a *API) MarkRead(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	var req model.MarkReadRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.Chat == "" || len(req.MessageIDs) == 0 {
		writeError(w, http.StatusBadRequest, "chat and message_ids required")
		return
	}

	if err := acct.MarkRead(r.Context(), req.Chat, req.MessageIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"chat":          req.Chat,
		"messages_read": len(req.MessageIDs),
	})
}

// ── Chats ───────────────────────────────────────────

// ListChats — GET /api/v1/accounts/{account_id}/chats
func (a *API) ListChats(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	chats, err := acct.ListChats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.ChatListResponse{
		Chats: chats,
		Total: len(chats),
	})
}

// GetMessages — GET /api/v1/accounts/{account_id}/chats/{jid}/messages
// Stub: requires local message store (not yet implemented).
func (a *API) GetMessages(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	chatJID := r.PathValue("jid")

	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	writeJSON(w, http.StatusOK, model.MessageListResponse{
		ChatID:   chatJID,
		Messages: []model.MessageInfo{},
		Total:    0,
		HasMore:  false,
	})
}

// ── Contacts ────────────────────────────────────────

// GetContact — GET /api/v1/accounts/{account_id}/contacts/{jid}
func (a *API) GetContact(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	info, err := acct.GetContactInfo(r.Context(), r.PathValue("jid"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// ── Groups ──────────────────────────────────────────

// ListGroups — GET /api/v1/accounts/{account_id}/groups
func (a *API) ListGroups(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	groups, err := acct.ListGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.GroupListResponse{
		Groups: groups,
		Total:  len(groups),
	})
}

// CreateGroup — POST /api/v1/accounts/{account_id}/groups
func (a *API) CreateGroup(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	var req model.CreateGroupRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	info, err := acct.CreateGroup(r.Context(), req.Name, req.Participants)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, info)
}

// GetGroup — GET /api/v1/accounts/{account_id}/groups/{jid}
func (a *API) GetGroup(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	info, err := acct.GetGroupInfo(r.Context(), r.PathValue("jid"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// UpdateGroup — PATCH /api/v1/accounts/{account_id}/groups/{jid}
func (a *API) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	var req model.UpdateGroupRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if err := acct.UpdateGroup(r.Context(), r.PathValue("jid"), req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch updated group info
	info, err := acct.GetGroupInfo(r.Context(), r.PathValue("jid"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// LeaveGroup — DELETE /api/v1/accounts/{account_id}/groups/{jid}
func (a *API) LeaveGroup(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	if err := acct.LeaveGroup(r.Context(), r.PathValue("jid")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// GetGroupInvite — GET /api/v1/accounts/{account_id}/groups/{jid}/invite
func (a *API) GetGroupInvite(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	reset := r.URL.Query().Get("reset") == "true"
	link, err := acct.GetGroupInviteLink(r.Context(), r.PathValue("jid"), reset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.GroupInviteLinkResponse{InviteLink: link})
}

// UpdateGroupParticipants — POST /api/v1/accounts/{account_id}/groups/{jid}/participants
func (a *API) UpdateGroupParticipants(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	var req model.GroupParticipantsRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if len(req.Participants) == 0 || req.Action == "" {
		writeError(w, http.StatusBadRequest, "participants and action required")
		return
	}

	if err := acct.UpdateGroupParticipants(r.Context(), r.PathValue("jid"), req.Participants, req.Action); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "action": req.Action})
}

// ── Presence ────────────────────────────────────────

// SendPresence — POST /api/v1/accounts/{account_id}/presence
func (a *API) SendPresence(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	var req model.PresenceRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	// Chat-level typing indicator
	if req.Chat != nil && *req.Chat != "" {
		if err := acct.SendChatPresence(r.Context(), *req.Chat, req.State); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "chat": *req.Chat, "state": req.State})
		return
	}

	// Global presence
	if err := acct.SendPresence(r.Context(), req.State); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "state": req.State})
}

// ── Profile ─────────────────────────────────────────

// GetProfile — GET /api/v1/accounts/{account_id}/profile
func (a *API) GetProfile(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	profile, err := acct.GetProfile(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

// UpdateProfile — PATCH /api/v1/accounts/{account_id}/profile
func (a *API) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	var req model.UpdateProfileRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.About != nil {
		if err := acct.SetStatusMessage(r.Context(), *req.About); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	profile, err := acct.GetProfile(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

// ── Privacy ─────────────────────────────────────────

// GetPrivacy — GET /api/v1/accounts/{account_id}/privacy
func (a *API) GetPrivacy(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	settings, err := acct.GetPrivacySettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

// UpdatePrivacy — PATCH /api/v1/accounts/{account_id}/privacy
func (a *API) UpdatePrivacy(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	var req model.UpdatePrivacyRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	updates := map[string]*string{
		"group_add":     req.GroupAdd,
		"last_seen":     req.LastSeen,
		"status":        req.Status,
		"profile":       req.Profile,
		"read_receipts": req.ReadReceipts,
		"online":        req.Online,
		"call_add":      req.CallAdd,
	}

	for name, val := range updates {
		if val != nil {
			if err := acct.SetPrivacySetting(r.Context(), name, *val); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	settings, err := acct.GetPrivacySettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

// ── Newsletters ─────────────────────────────────────

// ListNewsletters — GET /api/v1/accounts/{account_id}/newsletters
func (a *API) ListNewsletters(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	newsletters, err := acct.ListNewsletters(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.NewsletterListResponse{
		Newsletters: newsletters,
		Total:       len(newsletters),
	})
}

// CreateNewsletter — POST /api/v1/accounts/{account_id}/newsletters
func (a *API) CreateNewsletter(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	var req model.CreateNewsletterRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	info, err := acct.CreateNewsletter(r.Context(), req.Name, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, info)
}

// GetNewsletter — GET /api/v1/accounts/{account_id}/newsletters/{jid}
func (a *API) GetNewsletter(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	info, err := acct.GetNewsletterInfo(r.Context(), r.PathValue("jid"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// FollowNewsletter — POST /api/v1/accounts/{account_id}/newsletters/{jid}/follow
func (a *API) FollowNewsletter(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	if err := acct.FollowNewsletter(r.Context(), r.PathValue("jid")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// UnfollowNewsletter — DELETE /api/v1/accounts/{account_id}/newsletters/{jid}/follow
func (a *API) UnfollowNewsletter(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	if err := acct.UnfollowNewsletter(r.Context(), r.PathValue("jid")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
