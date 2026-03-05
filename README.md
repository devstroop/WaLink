# WaLink

WhatsApp HTTP API powered by [whatsmeow](https://github.com/tulir/whatsmeow). Multi-account, browserless.

## Quick Start

```bash
cd walink
cp config/app.example.toml config/app.toml   # edit secret_key
go mod tidy
go run ./cmd/walink
```

Server starts on `http://localhost:3000`. Interactive docs at `/docs`.

## API

All endpoints require `Authorization: Bearer <secret_key>` except `/api/health`.

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
| POST | `/{id}/messages` | Send message (JSON or multipart) |
| POST | `/{id}/messages/react` | React to a message |
| POST | `/{id}/messages/reply` | Reply to a message |
| POST | `/{id}/messages/read` | Mark messages as read |

### Chats

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/chats` | List chats |
| GET | `/{id}/chats/{jid}/messages` | Message history |

### Contacts

| Method | Path | Description |
|--------|------|-------------|
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
| GET | `/{id}/profile` | Get own profile |
| PATCH | `/{id}/profile` | Update about text |

### Privacy

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/privacy` | Get privacy settings |
| PATCH | `/{id}/privacy` | Update privacy settings |

### Newsletters

| Method | Path | Description |
|--------|------|-------------|
| GET | `/{id}/newsletters` | List subscribed newsletters |
| POST | `/{id}/newsletters` | Create newsletter |
| GET | `/{id}/newsletters/{jid}` | Get newsletter info |
| POST | `/{id}/newsletters/{jid}/follow` | Follow newsletter |
| DELETE | `/{id}/newsletters/{jid}/follow` | Unfollow newsletter |

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

# React to a message
curl -X POST http://localhost:3000/api/v1/accounts/{id}/messages/react \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"chat": "919876543210@s.whatsapp.net", "message_id": "ABCD1234", "emoji": "👍"}'

# Set typing indicator
curl -X POST http://localhost:3000/api/v1/accounts/{id}/presence \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"state": "composing", "chat": "919876543210@s.whatsapp.net"}'

# Create a group
curl -X POST http://localhost:3000/api/v1/accounts/{id}/groups \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"name": "Project Team", "participants": ["919876543210@s.whatsapp.net"]}'

# Get privacy settings
curl http://localhost:3000/api/v1/accounts/{id}/privacy -H "$AUTH"

# Logout
curl -X DELETE http://localhost:3000/api/v1/accounts/{id}/session -H "$AUTH"
```

## Configuration

See `config/app.example.toml`:

```toml
[server]
port = 3000

[auth]
secret_key = "change-this-secret-key-in-production"

[storage]
data_dir = "./data"
```

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for design details.

## License

MIT
