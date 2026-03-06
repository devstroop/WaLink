# ISSUES.md — WaLink Open Issues & Gaps

> Auto-generated audit — March 2026

---

## MCP ↔ REST API Coverage Gap

The REST API exposes **30 endpoints**; the MCP server now exposes **22 tools**.
8 low-priority administrative capabilities remain MCP-only via REST.

### Currently Exposed via MCP

| MCP Tool | API Equivalent |
|----------|----------------|
| `list_accounts` | `GET /api/v1/accounts` |
| `get_session` | `GET /accounts/{id}/session` |
| `send_message` | `POST /accounts/{id}/messages/send` (text only) |
| `check_contacts` | `POST /accounts/{id}/contacts/check` |
| `get_contact` | `GET /accounts/{id}/contacts/{jid}` |
| `list_groups` | `GET /accounts/{id}/groups` |
| `get_group` | `GET /accounts/{id}/groups/{jid}` |
| `get_profile` | `GET /accounts/{id}/profile` |

### ✅ Implemented (previously missing)

| Priority | API Endpoint | MCP Tool | Status |
|----------|-------------|----------|--------|
| High | `POST /messages/send` (multipart) | `send_media` | ✅ Done (base64 input) |
| High | `GET /messages` | `get_messages` | ✅ Done |
| High | `GET /chats` | `list_chats` | ✅ Done |
| High | `GET /contacts` | `list_contacts` | ✅ Done |
| Medium | `POST /messages/react` | `react_message` | ✅ Done |
| Medium | `POST /messages/read` | `mark_read` | ✅ Done |
| Medium | `DELETE /messages/{id}` | `revoke_message` | ✅ Done |
| Medium | `POST /presence` | `send_presence` | ✅ Done |
| Medium | `PATCH /profile` | `update_profile` | ✅ Done (about text) |
| Medium | `POST /groups` | `create_group` | ✅ Done |
| Medium | `PATCH /groups/{jid}` | `update_group` | ✅ Done |
| Medium | `DELETE /groups/{jid}` | `leave_group` | ✅ Done |
| Medium | `POST /groups/{jid}/participants` | `update_participants` | ✅ Done |
| Medium | `GET /groups/{jid}/invite` | `get_group_invite` | ✅ Done |

*Bonus:* `send_chat_presence` tool added (typing/paused indicator per chat) — not in original proposal.

### Still Missing from MCP (Low priority — admin/config)

| Priority | API Endpoint | Suggested MCP Tool |
|----------|-------------|-------------------|
| Low | `POST /accounts` | `create_account` — add new account |
| Low | `PATCH /accounts/{id}` | `update_account` — rename account |
| Low | `DELETE /accounts/{id}` | `delete_account` — remove account |
| Low | `GET /session/qr` | `get_qr` — get QR code for linking |
| Low | `POST /session/pair` | `pair_phone` — pair via phone number |
| Low | `DELETE /session` | `logout` — disconnect session |
| Low | `GET/PUT/DELETE /proxy` | `manage_proxy` — proxy configuration |
| Low | `GET/PUT/DELETE /webhook` | `manage_webhook` — webhook configuration |

---

## Code Quality Issues

### 1. Repeated Connection-Check Pattern (26+ instances)
**Severity:** High  
**Location:** `internal/service/account.go`

The pattern below is copy-pasted in **26 methods**:
```go
client := a.getClient()
if client == nil || !client.IsConnected() {
    return ..., fmt.Errorf("account %s is not connected", a.id)
}
```

**Affected methods:** `ResolvePhone`, `SendMessage`, `SendMedia`, `SendChatPresence`, `MarkRead`, `SendReaction`, `SendReply`, `GetContactInfo`, `GetGroupInfo`, `ListGroups`, `CreateGroup`, `LeaveGroup`, `UpdateGroup`, `UpdateGroupParticipants`, `GetGroupInviteLink`, `SendPresence`, `GetProfile`, `SetStatusMessage`, `ListContacts`, `CheckContacts`, `RevokeMessage`, `DownloadMedia`, and more.

**Fix:** Extract a `requireConnectedClient()` helper that returns `(*whatsmeow.Client, error)`.

---

### 2. Webhook Config Fields Are Dead Code
**Severity:** High  
**Location:** `internal/config/config.go` → `WebhookConfig`

```go
type WebhookConfig struct {
    Enabled    bool   // ← never read (per-account DB config used instead)
    TimeoutMs  int64  // ← never read anywhere
    RetryCount int    // ← never read anywhere
    RetryDelay int64  // ← never read anywhere
}
```

The global `WebhookConfig` is defined but **never referenced** outside config loading.  
Per-account webhook settings are stored in the database (`WebhookConfigRecord`).  
These fields are misleading — either implement them or remove them.

---

### 3. Webhook Dispatch: No Retries, No Timeout
**Severity:** Medium  
**Location:** `internal/service/account.go` → `doDispatchWebhook()`

- HTTP client has **no timeout** — a slow/hung endpoint blocks the goroutine forever.
- **No retry logic** — if the POST fails, the event is silently lost.
- The `RetryCount` / `RetryDelay` / `TimeoutMs` config fields exist but are unused.

**Fix:** Use `http.Client{Timeout: ...}`, add exponential backoff retries up to `RetryCount`.

---

### 4. Media Uploads Limited to DocumentMessage
**Severity:** Medium  
**Location:** `internal/service/account.go` → `SendMedia()`

All uploaded files are sent as `DocumentMessage` regardless of MIME type.  
WhatsApp supports specialized message types that render with previews:

| MIME prefix | Should use | Currently uses |
|------------|-----------|---------------|
| `image/*` | `ImageMessage` | `DocumentMessage` |
| `video/*` | `VideoMessage` | `DocumentMessage` |
| `audio/*` | `AudioMessage` | `DocumentMessage` |
| `image/webp` | `StickerMessage` | `DocumentMessage` |

Images sent as documents don't get inline previews; videos don't auto-play; audio doesn't show waveform UI.

**Fix:** Detect MIME type and use the appropriate `whatsmeow.Media*` upload type and protobuf message.

---

### 5. No Rate Limiting on Message Send
**Severity:** Low  
**Location:** API-wide

The global `RateLimit` middleware limits concurrent requests, but there's no per-account or per-endpoint throttle for message sending. A client could send thousands of messages in quick succession, risking a WhatsApp ban.

**Fix:** Add per-account send rate limiting (e.g., token bucket, ~30 msg/min).

---

## Feature Gaps

### ~~6. No Message History via MCP~~ ✅ Resolved
`get_messages` tool now exposes paginated chat history with cursor support.

### ~~7. No Chat List via MCP~~ ✅ Resolved
`list_chats` tool now exposes conversations with last message and unread counts.

### ~~8. No Media Sending via MCP~~ ✅ Resolved
`send_media` tool accepts base64-encoded file data with filename and MIME type.

### ~~9. MCP Has No Reaction/Read Receipt Tools~~ ✅ Resolved
`react_message` and `mark_read` tools now available.
