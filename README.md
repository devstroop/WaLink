# WaLink

WhatsApp HTTP API. Multi-account, browserless, native multi-device protocol.

## Quick Start

```bash
cd walink
cp config/app.example.toml config/app.toml   # edit secret_key
go mod tidy
go run ./cmd/walink
```

Server starts on `http://localhost:3000`. Interactive API docs at `/api-docs`.

## API

All endpoints require `Authorization: Bearer <secret_key>` except `/api/health` and `/api-docs`.

Base: `/api/v1/accounts`  |  `{id}` = account UUID

### Health

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check |

### Accounts

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/accounts` | List all accounts |
| POST | `/api/v1/accounts` | Create account |
| GET | `/api/v1/accounts/{id}` | Get account |
| PATCH | `/api/v1/accounts/{id}` | Update account |
| DELETE | `/api/v1/accounts/{id}?delete_data=true` | Delete account |

### Session

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/session` | WhatsApp session status |
| GET | `/{id}/session/qr` | QR code PNG for linking |
| POST | `/{id}/session/pair` | Phone-number pairing code |
| DELETE | `/{id}/session` | Logout / unlink |

### Messaging

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/messages?chat=JID&limit=50&before=RFC3339` | Chat message history (paginated) |
| POST | `/{id}/messages` | Send message (JSON or multipart) |
| POST | `/{id}/messages/react` | React to a message |
| POST | `/{id}/messages/read` | Mark messages as read |
| DELETE | `/{id}/messages/{msg_id}?chat=JID` | Revoke (delete for everyone) |

### Chats

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/chats` | List chats (sorted by pinned → newest) |

### Contacts

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/contacts` | List contacts |
| POST | `/{id}/contacts/check` | Check phone numbers on WhatsApp |
| GET | `/{id}/contacts/{jid}` | Get contact info |

### Groups

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/groups` | List joined groups |
| POST | `/{id}/groups` | Create group |
| GET | `/{id}/groups/{jid}` | Get group info |
| PATCH | `/{id}/groups/{jid}` | Update group (name, topic, locked, announce) |
| DELETE | `/{id}/groups/{jid}` | Leave group |
| GET | `/{id}/groups/{jid}/invite` | Get invite link |
| POST | `/{id}/groups/{jid}/participants` | Add/remove/promote/demote participants |

### Presence

| Method | Path | Description |
|--------|------|-------------|
| POST | `/{id}/presence` | Typing indicator or global presence |

### Profile

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/profile` | Get own profile (incl. business fields) |
| PATCH | `/{id}/profile` | Update about text |

### Proxy

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/proxy` | Get proxy configuration |
| PUT | `/{id}/proxy` | Set proxy (http/https/socks5) |
| DELETE | `/{id}/proxy` | Remove proxy |

### Webhook

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/webhook` | Get webhook config |
| PUT | `/{id}/webhook` | Set/update webhook URL, events, secret |
| DELETE | `/{id}/webhook` | Remove webhook |

Webhooks deliver `message` and `receipt` events via `POST` to your URL. Each payload includes `event_type`, `account_id`, `timestamp`, and `payload`. An optional HMAC-SHA256 signature is sent in `X-Webhook-Signature` when a secret is configured. Failed deliveries retry up to 3 times with exponential back-off.

## Error Responses

All errors return JSON with an `error` field:

```json
{"error": "description of the problem"}
```

| Status | Meaning |
|--------|---------|
| 400 | Bad request — missing/invalid parameters |
| 401 | Unauthorized — missing or invalid Bearer token |
| 404 | Resource not found (account, webhook, contact) |
| 409 | Conflict — account already exists, or session still linked |
| 429 | Too many requests — concurrency limit reached. Retry after `Retry-After` header |
| 500 | Internal server error |
| 504 | Gateway timeout — QR code generation timed out |

## Examples

```bash
AUTH="Authorization: Bearer change-this-secret-key-in-production"

# Create account
curl -X POST http://localhost:3000/api/v1/accounts \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"phone_number": "+919876543210", "account_name": "main"}'

# Link via QR (returns PNG)
curl http://localhost:3000/api/v1/accounts/{id}/session/qr \
  -H "$AUTH" -o qr.png

# Link via phone pairing code
curl -X POST http://localhost:3000/api/v1/accounts/{id}/session/pair \
  -H "$AUTH"

# Send text message
curl -X POST http://localhost:3000/api/v1/accounts/{id}/messages \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"chat": "919876543210@s.whatsapp.net", "text": "Hello from WaLink!"}'

# Send file with caption
curl -X POST http://localhost:3000/api/v1/accounts/{id}/messages \
  -H "$AUTH" -F "chat=919876543210@s.whatsapp.net" \
  -F "text=Check this out" -F "file=@document.pdf"

# Get message history
curl "http://localhost:3000/api/v1/accounts/{id}/messages?chat=919876543210@s.whatsapp.net&limit=20" \
  -H "$AUTH"

# React to a message
curl -X POST http://localhost:3000/api/v1/accounts/{id}/messages/react \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"chat": "919876543210@s.whatsapp.net", "message_id": "ABCD1234", "emoji": "👍"}'

# Set typing indicator
curl -X POST http://localhost:3000/api/v1/accounts/{id}/presence \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"state": "composing", "chat": "919876543210@s.whatsapp.net"}'

# Configure webhook
curl -X PUT http://localhost:3000/api/v1/accounts/{id}/webhook \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/webhook", "secret": "mysecret", "events": ["message", "receipt"]}'

# Create a group
curl -X POST http://localhost:3000/api/v1/accounts/{id}/groups \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"name": "Project Team", "participants": ["919876543210@s.whatsapp.net"]}'

# Logout
curl -X DELETE http://localhost:3000/api/v1/accounts/{id}/session -H "$AUTH"
```

## Configuration

See `config/app.example.toml` for all options. Key sections:

| Section | Key | Default | Description |
|---------|-----|---------|-------------|
| `server` | `port` | 3000 | HTTP listen port |
| `auth` | `secret_key` | — | Bearer token (required) |
| `limits` | `max_concurrent_requests` | 50 | Rate limiter concurrency cap |
| `webhooks` | `timeout_ms` | 5000 | Webhook delivery timeout |
| `webhooks` | `retry_count` | 3 | Max retry attempts for 5xx |
| `swagger` | `enabled` | true | Serve Swagger UI at `/api-docs` |

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for design details.

## License

MIT
