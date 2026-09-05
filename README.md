# WaLink

**WhatsApp HTTP API + MCP server for developers.** Connect multiple WhatsApp accounts, send and receive messages, manage groups, and receive webhooks — through a clean REST API, an MCP server for AI agents, or a built-in web dashboard. No browser, no Selenium, no Chrome. Pure protocol-level integration.

## Highlights

- **Multi-account** — run dozens of WhatsApp numbers from one server
- **Phone-first API** — pass a phone number; WaLink resolves the JID for you
- **MCP server** — 26 tools for AI agents (Claude, Copilot, etc.) via Streamable HTTP
- **Web dashboard** — messaging UI, account management, admin panel, all built-in
- **RBAC** — users, roles, permissions, API keys, and JWT auth
- **Browserless** — native multi-device protocol over encrypted WebSocket
- **Pure Go** — single binary, no CGO, no external dependencies at runtime
- **Auto-connect** — accounts sleep when idle and reconnect on the next request
- **Webhooks** — real-time message and receipt delivery with HMAC signing and retry
- **Billing** — optional plan-based metering with Stripe integration
- **Docker-ready** — multi-stage Dockerfile and docker-compose included
- **Swagger UI** — interactive API docs at `/api-docs`

## Quick Start

### Binary

```bash
cp config/app.example.toml config/app.toml   # set your secret_key
go run ./cmd/walink
```

### Docker

```bash
docker compose up -d
```

Server starts on `http://localhost:3000`.

| URL | Description |
|-----|-------------|
| `/` | Web dashboard |
| `/api-docs` | Swagger UI |
| `/mcp` | MCP endpoint for AI agents |

```bash
# Create an account, link via QR, send a message — three calls:
AUTH="Authorization: Bearer <your-secret-key>"

curl -X POST http://localhost:3000/api/v1/accounts \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"phone_number": "+919876543210", "account_name": "main"}'

curl http://localhost:3000/api/v1/accounts/{id}/session/qr \
  -H "$AUTH" -o qr.png          # scan with WhatsApp

curl -X POST "http://localhost:3000/api/v1/accounts/{id}/messages?phone=919876543210&text=Hello!" \
  -H "$AUTH"
```

---

## Authentication

WaLink supports three authentication methods. All API endpoints under `/api/v1/` require `Authorization: Bearer <token>`.

| Method | Token format | Use case |
|--------|-------------|----------|
| Static secret key | `Bearer <secret_key>` | System admin — full access, no user context |
| JWT | `Bearer eyJ...` | User login — scoped by role permissions |
| API key | `Bearer walink_...` | Programmatic access — supports expiry and account binding |

### Auth Endpoints (public)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/login` | Login with username + password, returns JWT |
| POST | `/api/v1/auth/register` | Register new user (when enabled) |
| POST | `/api/v1/auth/forgot-password` | Request password reset |
| POST | `/api/v1/auth/reset-password` | Reset password with token |

### RBAC

Two built-in roles: `admin` (full access) and `user` (restricted). Permissions use `resource:action` format — e.g. `messages:write`, `groups:read`, or `*` for full admin access.

---

## REST API Reference

Base path: `/api/v1/accounts/{id}` — `{id}` is the account UUID.

### Accounts

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/accounts` | List all accounts |
| POST | `/api/v1/accounts` | Create account |
| GET | `/api/v1/accounts/{id}` | Get account details |
| PATCH | `/api/v1/accounts/{id}` | Update account |
| DELETE | `/api/v1/accounts/{id}?delete_data=true` | Delete account and optionally wipe data |

### Session

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/session` | Connection and auth status |
| GET | `/{id}/session/qr` | QR code PNG for linking |
| POST | `/{id}/session/pair` | Phone-number pairing code |
| DELETE | `/{id}/session` | Logout and unlink device |

### Messaging

| Method | Path | Description |
|--------|------|-------------|
| POST | `/{id}/messages?phone=NUM&text=...` | Send message by phone number |
| POST | `/{id}/messages?jid=JID&text=...` | Send message by JID |
| GET | `/{id}/messages?chat=JID` | Message history (paginated) |
| POST | `/{id}/messages/reactions` | React to a message |
| POST | `/{id}/messages/mark-read` | Mark messages as read |
| DELETE | `/{id}/messages/{msg_id}?chat=JID` | Revoke / delete for everyone |

> **Phone number support** — `messages`, `react`, `read`, and `revoke` all accept `phone` as an alternative to `chat`/`jid`. WaLink resolves the phone to a JID via WhatsApp automatically.

#### `POST /{id}/messages`

Single-call send — no need to resolve the JID yourself.

| Param | In | Required | Description |
|-------|------|----------|-------------|
| `phone` | query | one of | Phone number (e.g. `919876543210`). Auto-resolved to JID. |
| `jid` | query | one of | WhatsApp JID (e.g. `919876543210@s.whatsapp.net`). |
| `text` | query or body | yes* | Message text. Query param takes precedence. *Not required when sending a file. |
| `reply_to` | query or body | no | Message ID to quote-reply. |
| `file` | multipart | no | File attachment (auto-detects image/video/audio/document). `text` becomes the caption. |

### Chats & Contacts

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/chats` | List chats (pinned first, then by recency) |
| GET | `/{id}/contacts` | List all contacts |
| POST | `/{id}/contacts/check` | Check if phone numbers are on WhatsApp |
| GET | `/{id}/contacts/{jid}` | Get contact info (also accepts `?phone=`) |

### Groups

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/groups` | List joined groups |
| POST | `/{id}/groups` | Create group (participants can be phone numbers) |
| GET | `/{id}/groups/{jid}` | Group info |
| PATCH | `/{id}/groups/{jid}` | Update name, topic, locked, announce |
| DELETE | `/{id}/groups/{jid}` | Leave group |
| GET | `/{id}/groups/{jid}/invite` | Get invite link |
| POST | `/{id}/groups/{jid}/participants` | Add / remove / promote / demote (accepts phone numbers) |

### Newsletters (Channels)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/newsletters` | List followed channels |
| POST | `/{id}/newsletters/follow` | Follow a channel |
| POST | `/{id}/newsletters/unfollow` | Unfollow a channel |
| GET | `/{id}/newsletters/{jid}` | Channel info |
| GET | `/{id}/newsletters/{jid}/messages` | Channel messages |
| POST | `/{id}/newsletters/{jid}/mute` | Mute/unmute channel |

### Presence & Profile

| Method | Path | Description |
|--------|------|-------------|
| POST | `/{id}/presence` | Send typing indicator or set global presence (accepts `phone`) |
| GET | `/{id}/profile` | Get own profile (includes business fields) |
| PATCH | `/{id}/profile` | Update about text |

### Proxy

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/proxy` | Get proxy config |
| PUT | `/{id}/proxy` | Set proxy (http / https / socks5) |
| DELETE | `/{id}/proxy` | Remove proxy |

### Webhook

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/webhook` | Get webhook config |
| PUT | `/{id}/webhook` | Set webhook URL, events, and HMAC secret |
| DELETE | `/{id}/webhook` | Remove webhook |

Webhooks deliver `message` and `receipt` events via POST. Each payload includes `event_type`, `account_id`, `timestamp`, and `payload`. When a secret is configured, an HMAC-SHA256 signature is sent in `X-Webhook-Signature`. Failed deliveries retry up to 3 times with exponential back-off.

### Users & Roles (Admin)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users` | List users |
| POST | `/api/v1/users` | Create user |
| GET | `/api/v1/users/{id}` | Get user |
| PATCH | `/api/v1/users/{id}` | Update user |
| DELETE | `/api/v1/users/{id}` | Delete user |
| GET | `/api/v1/roles` | List roles |
| POST | `/api/v1/roles` | Create role |
| GET | `/api/v1/roles/{id}` | Get role |
| PATCH | `/api/v1/roles/{id}` | Update role |
| DELETE | `/api/v1/roles/{id}` | Delete role |

### API Keys

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/api-keys` | List API keys |
| POST | `/api/v1/api-keys` | Create API key (supports expiry and account binding) |
| DELETE | `/api/v1/api-keys/{id}` | Delete API key |

### Billing

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/billing/plans` | List plans (public) |
| GET | `/api/v1/billing` | Current subscription |
| GET | `/api/v1/billing/usage` | Daily usage stats |

### MCP Settings (Admin)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/mcp` | Get MCP enabled setting |
| PATCH | `/api/v1/mcp` | Toggle MCP on/off (no restart needed) |

### Health

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check (no auth required) |

---

## MCP Server

WaLink exposes a **Model Context Protocol** server at `/mcp` for AI agents. Uses Streamable HTTP transport with stateful SSE sessions.

### Setup (VS Code / Copilot)

Add to `.vscode/mcp.json`:

```json
{
  "servers": {
    "walink": {
      "type": "http",
      "url": "http://localhost:3000/mcp",
      "headers": {
        "Authorization": "Bearer <your-secret-key>"
      }
    }
  }
}
```

### Tools (26)

| Tool | Description |
|------|-------------|
| `list_accounts` | List WhatsApp accounts |
| `get_session` | Get session status |
| `get_qr` | Get QR code for linking |
| `pair_phone` | Pair via phone number |
| `logout` | Disconnect session |
| `send_message` | Send text message |
| `send_media` | Send file (base64) |
| `get_messages` | Get message history |
| `list_chats` | List conversations |
| `react_message` | React to a message |
| `mark_read` | Mark messages as read |
| `revoke_message` | Delete message for everyone |
| `list_contacts` | List contacts |
| `check_contacts` | Check phone numbers on WhatsApp |
| `get_contact` | Get contact info |
| `list_groups` | List groups |
| `get_group` | Get group info |
| `create_group` | Create a group |
| `update_group` | Update group settings |
| `leave_group` | Leave a group |
| `update_participants` | Add/remove/promote/demote members |
| `get_group_invite` | Get group invite link |
| `get_profile` | Get own profile |
| `update_profile` | Update about text |
| `send_presence` | Set online/offline status |
| `send_chat_presence` | Send typing indicator |

API key account binding works with MCP — create an API key scoped to a specific account and the MCP tools automatically operate on that account without needing `account_id`.

---

## Error Responses

All errors return JSON:

```json
{ "error": "description of the problem" }
```

| Status | Meaning |
|--------|---------|
| 400 | Bad request — missing or invalid parameters |
| 401 | Unauthorized — missing or wrong Bearer token |
| 403 | Forbidden — insufficient permissions |
| 404 | Resource not found |
| 409 | Conflict — duplicate account or session still linked |
| 429 | Rate limited — retry after `Retry-After` header value |
| 500 | Internal server error |
| 504 | Gateway timeout (e.g. QR generation timed out) |

---

## Examples

```bash
AUTH="Authorization: Bearer change-this-secret-key-in-production"

# ── Accounts ─────────────────────────────────────────
curl -X POST http://localhost:3000/api/v1/accounts \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"phone_number": "+919876543210", "account_name": "main"}'

# ── Session ──────────────────────────────────────────
# Link via QR
curl http://localhost:3000/api/v1/accounts/{id}/session/qr \
  -H "$AUTH" -o qr.png

# Link via pairing code
curl -X POST http://localhost:3000/api/v1/accounts/{id}/session/pair \
  -H "$AUTH"

# ── Send messages ────────────────────────────────────
# By phone number (recommended)
curl -X POST "http://localhost:3000/api/v1/accounts/{id}/messages?phone=919876543210&text=Hello!" \
  -H "$AUTH"

# With file attachment
curl -X POST "http://localhost:3000/api/v1/accounts/{id}/messages?phone=919876543210" \
  -H "$AUTH" -F "text=Check this out" -F "file=@photo.jpg"

# ── Read & React ─────────────────────────────────────
curl -X POST http://localhost:3000/api/v1/accounts/{id}/messages/reactions \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"phone": "919876543210", "message_id": "ABCD1234", "emoji": "👍"}'

curl -X POST http://localhost:3000/api/v1/accounts/{id}/messages/mark-read \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"phone": "919876543210", "message_ids": ["ABCD1234"]}'

# ── Groups ───────────────────────────────────────────
curl -X POST http://localhost:3000/api/v1/accounts/{id}/groups \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"name": "Project Team", "participants": ["919876543210", "919876543211"]}'

# ── Webhooks ─────────────────────────────────────────
curl -X PUT http://localhost:3000/api/v1/accounts/{id}/webhook \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/webhook", "secret": "mysecret", "events": ["message", "receipt"]}'

# ── Users (admin) ────────────────────────────────────
curl -X POST http://localhost:3000/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "secret"}'

# ── API Keys ─────────────────────────────────────────
curl -X POST http://localhost:3000/api/v1/api-keys \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"name": "my-bot", "account_id": "{id}"}'
```

---

## Configuration

Copy `config/app.example.toml` to `config/app.toml` and edit. All settings can be overridden via environment variables (`WALINK_SECTION_KEY` format).

| Section | Key | Default | Description |
|---------|-----|---------|-------------|
| `server` | `port` | `3000` | HTTP listen port |
| `auth` | `secret_key` | — | Bearer token for API auth (required) |
| `auth` | `registration_enabled` | `false` | Allow public user registration |
| `smtp` | `host` | — | SMTP server for password reset emails |
| `limits` | `max_concurrent_requests` | `50` | Concurrency rate limiter |
| `accounts.defaults` | `idle_timeout` | `300` | Seconds before idle auto-disconnect (0 = never) |
| `webhooks` | `timeout_ms` | `5000` | Webhook delivery timeout |
| `webhooks` | `retry_count` | `3` | Retry attempts on failure |
| `swagger` | `enabled` | `true` | Serve Swagger UI at `/api-docs` |
| `mcp` | `enabled` | `true` | Enable MCP server |
| `billing` | `enabled` | `false` | Enable plan-based billing |

See [CONFIGURATION.md](CONFIGURATION.md) for the full reference.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for internals — component design, request flow, data model, and protocol details.

## License

MIT
