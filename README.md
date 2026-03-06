# WaLink

**WhatsApp HTTP API for developers.** Connect multiple WhatsApp accounts, send and receive messages, manage groups, and receive webhooks — all through a clean REST interface. No browser, no Selenium, no Chrome. Pure protocol-level integration.

## Highlights

- **Multi-account** — run dozens of WhatsApp numbers from one server
- **Phone-first API** — pass a phone number; WaLink resolves the JID for you
- **Browserless** — native multi-device protocol over encrypted WebSocket
- **Pure Go** — single binary, no CGO, no external dependencies at runtime
- **Auto-connect** — accounts sleep when idle and reconnect on the next request
- **Webhooks** — real-time message and receipt delivery with HMAC signing and retry
- **Swagger UI** — interactive docs at `/api-docs` out of the box

## Quick Start

```bash
cp config/app.example.toml config/app.toml   # set your secret_key
go run ./cmd/walink
```

Server starts on `http://localhost:3000`. Open `/api-docs` to explore the API interactively.

```bash
# Create an account, link via QR, send a message — three calls:
AUTH="Authorization: Bearer <your-secret-key>"

curl -X POST http://localhost:3000/api/v1/accounts \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"phone_number": "+919876543210", "account_name": "main"}'

curl http://localhost:3000/api/v1/accounts/{id}/session/qr \
  -H "$AUTH" -o qr.png          # scan with WhatsApp

curl -X POST "http://localhost:3000/api/v1/accounts/{id}/messages/send?phone=919876543210&text=Hello!" \
  -H "$AUTH"
```

---

## API Reference

All endpoints require `Authorization: Bearer <secret_key>` except `/api/health` and `/api-docs`.

Base path: `/api/v1/accounts` — `{id}` is the account UUID.

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
| POST | `/{id}/messages/send?phone=NUM&text=...` | Send message by phone number |
| POST | `/{id}/messages/send?jid=JID&text=...` | Send message by JID |
| POST | `/{id}/messages` | Send message (legacy JSON body) |
| GET | `/{id}/messages?chat=JID` | Message history (paginated) |
| POST | `/{id}/messages/react` | React to a message |
| POST | `/{id}/messages/read` | Mark messages as read |
| DELETE | `/{id}/messages/{msg_id}?chat=JID` | Revoke / delete for everyone |

> **Phone number support** — `messages`, `react`, `read`, and `revoke` all accept `phone` as an alternative to `chat`/`jid`. WaLink resolves the phone to a JID via WhatsApp automatically.

#### `POST /{id}/messages/send`

Single-call send — no need to resolve the JID yourself.

| Param | In | Required | Description |
|-------|------|----------|-------------|
| `phone` | query | one of | Phone number (e.g. `919876543210`). Auto-resolved to JID. |
| `jid` | query | one of | WhatsApp JID (e.g. `919876543210@s.whatsapp.net`). |
| `text` | query or body | yes* | Message text. Query param takes precedence. *Not required when sending a file. |
| `reply_to` | query or body | no | Message ID to quote-reply. |
| `file` | multipart | no | File attachment. `text` becomes the caption. |

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

### Health

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check (no auth required) |

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
# Create
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
# By phone number (recommended — resolves JID automatically)
curl -X POST "http://localhost:3000/api/v1/accounts/{id}/messages/send?phone=919876543210&text=Hello!" \
  -H "$AUTH"

# By JID
curl -X POST "http://localhost:3000/api/v1/accounts/{id}/messages/send?jid=919876543210@s.whatsapp.net&text=Hello!" \
  -H "$AUTH"

# With file attachment
curl -X POST "http://localhost:3000/api/v1/accounts/{id}/messages/send?phone=919876543210" \
  -H "$AUTH" -F "text=Check this out" -F "file=@photo.jpg"

# ── Read & React ─────────────────────────────────────
curl -X POST http://localhost:3000/api/v1/accounts/{id}/messages/react \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"phone": "919876543210", "message_id": "ABCD1234", "emoji": "👍"}'

curl -X POST http://localhost:3000/api/v1/accounts/{id}/messages/read \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"phone": "919876543210", "message_ids": ["ABCD1234"]}'

# ── Presence ─────────────────────────────────────────
curl -X POST http://localhost:3000/api/v1/accounts/{id}/presence \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"state": "composing", "phone": "919876543210"}'

# ── Groups ───────────────────────────────────────────
curl -X POST http://localhost:3000/api/v1/accounts/{id}/groups \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"name": "Project Team", "participants": ["919876543210", "919876543211"]}'

# ── Webhooks ─────────────────────────────────────────
curl -X PUT http://localhost:3000/api/v1/accounts/{id}/webhook \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/webhook", "secret": "mysecret", "events": ["message", "receipt"]}'

# ── Logout ───────────────────────────────────────────
curl -X DELETE http://localhost:3000/api/v1/accounts/{id}/session -H "$AUTH"
```

---

## Configuration

Copy `config/app.example.toml` to `config/app.toml` and edit. Key settings:

| Section | Key | Default | Description |
|---------|-----|---------|-------------|
| `server` | `port` | `3000` | HTTP listen port |
| `auth` | `secret_key` | — | Bearer token for API auth (required) |
| `limits` | `max_concurrent_requests` | `50` | Concurrency rate limiter |
| `accounts.defaults` | `idle_timeout` | `300` | Seconds before idle auto-disconnect (0 = never) |
| `webhooks` | `timeout_ms` | `5000` | Webhook delivery timeout |
| `webhooks` | `retry_count` | `3` | Retry attempts on 5xx |
| `swagger` | `enabled` | `true` | Serve Swagger UI at `/api-docs` |

See [CONFIGURATION.md](CONFIGURATION.md) for the full reference.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for internals — component design, request flow, data model, and protocol details.

## License

MIT
