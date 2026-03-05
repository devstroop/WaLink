package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/itsalfredakku/walink/internal/model"
)

// GetStatus — GET /api/v1/accounts/{account_id}/status
func (a *API) GetStatus(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	writeJSON(w, http.StatusOK, acct.StatusResponse())
}

// GetQR — GET /api/v1/accounts/{account_id}/link/qr
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

	// Wait for first QR code
	select {
	case item := <-ch:
		if item.Error != nil {
			writeError(w, http.StatusInternalServerError, item.Error.Error())
			return
		}
		if item.Event == "code" {
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
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("unexpected qr event: %s", item.Event))
	case <-ctx.Done():
		writeError(w, http.StatusGatewayTimeout, "timeout waiting for QR code")
	}
}

// LinkPhone — POST /api/v1/accounts/{account_id}/link/phone
func (a *API) LinkPhone(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	acct := a.mgr.GetAccount(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	// Ensure connected first
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

// Unlink — DELETE /api/v1/accounts/{account_id}/unlink
func (a *API) Unlink(w http.ResponseWriter, r *http.Request) {
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

// SendMessage — POST /api/v1/accounts/{account_id}/chats/{chat_id}/messages
func (a *API) SendMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	chatID := r.PathValue("chat_id")

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

	if req.Text == nil || *req.Text == "" {
		writeError(w, http.StatusBadRequest, "text required")
		return
	}

	msgID, err := acct.SendMessage(r.Context(), chatID, *req.Text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.SendMessageResponse{Status: "sent", MessageID: msgID})
}

// GetMessages — GET /api/v1/accounts/{account_id}/chats/{chat_id}/messages
// NOTE: Requires a local message store (not yet implemented). Returns empty for now.
func (a *API) GetMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	chatID := r.PathValue("chat_id")

	acct := a.mgr.GetAccount(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	writeJSON(w, http.StatusOK, model.MessageListResponse{
		ChatID:   chatID,
		Messages: []model.MessageInfo{},
		Total:    0,
		HasMore:  false,
	})
}

// ListChats — GET /api/v1/accounts/{account_id}/chats
func (a *API) ListChats(w http.ResponseWriter, r *http.Request) {
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

// SendTyping — POST /api/v1/accounts/{account_id}/chats/{chat_id}/typing
func (a *API) SendTyping(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	chatID := r.PathValue("chat_id")

	acct := a.mgr.GetAccount(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	var req model.TypingRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if err := acct.SendChatPresence(r.Context(), chatID, req.State); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"chat_id": chatID,
		"state":   req.State,
	})
}

// MarkRead — POST /api/v1/accounts/{account_id}/chats/{chat_id}/read
func (a *API) MarkRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	chatID := r.PathValue("chat_id")

	acct := a.mgr.GetAccount(id)
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

	if len(req.MessageIDs) == 0 {
		writeError(w, http.StatusBadRequest, "message_ids required")
		return
	}

	if err := acct.MarkRead(r.Context(), chatID, req.MessageIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"chat_id":       chatID,
		"messages_read": len(req.MessageIDs),
	})
}

// ReactMessage — POST /api/v1/accounts/{account_id}/chats/{chat_id}/messages/{message_id}/react
func (a *API) ReactMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	chatID := r.PathValue("chat_id")
	messageID := r.PathValue("message_id")

	acct := a.mgr.GetAccount(id)
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

	if err := acct.SendReaction(r.Context(), chatID, messageID, req.Emoji); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": messageID,
		"emoji":      req.Emoji,
	})
}

// ReplyMessage — POST /api/v1/accounts/{account_id}/chats/{chat_id}/messages/{message_id}/reply
func (a *API) ReplyMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	chatID := r.PathValue("chat_id")
	messageID := r.PathValue("message_id")

	acct := a.mgr.GetAccount(id)
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

	replyID, err := acct.SendReply(r.Context(), chatID, messageID, req.Text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.SendMessageResponse{
		Status:    "sent",
		MessageID: replyID,
	})
}

// GetContact — GET /api/v1/accounts/{account_id}/contacts/{contact_id}
func (a *API) GetContact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	contactID := r.PathValue("contact_id")

	acct := a.mgr.GetAccount(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	info, err := acct.GetContactInfo(r.Context(), contactID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// GetGroup — GET /api/v1/accounts/{account_id}/groups/{group_id}
func (a *API) GetGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("account_id")
	groupID := r.PathValue("group_id")

	acct := a.mgr.GetAccount(id)
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return
	}

	info, err := acct.GetGroupInfo(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, info)
}
