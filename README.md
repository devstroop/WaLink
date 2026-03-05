# WaLink

WhatsApp HTTP API. Browserless connection to WhatsApp's protocol, with multi-account support.

## Quick Start

```bash
# Prerequisites: Go 1.25+, CGO enabled (for SQLite)

cd walink
cp config/app.example.toml config/app.toml   # edit secret_key
go mod tidy
go run ./cmd/walink
```

Server starts on `http://localhost:3000`.

## API

All endpoints require `Authorization: Bearer <secret_key>` except health checks.

### Health

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check |
| GET | `/api/ready` | Readiness check |

### Accounts

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/accounts` | List all accounts |
| POST | `/api/v1/accounts` | Create account |
| GET | `/api/v1/accounts/{id}` | Get account info |
| DELETE | `/api/v1/accounts/{id}?delete_data=true` | Delete account |
| GET | `/api/v1/accounts/{id}/config` | Get config |
| PUT | `/api/v1/accounts/{id}/config` | Update config |

### Lifecycle

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/accounts/{id}/warmup` | Connect to WhatsApp |
| DELETE | `/api/v1/accounts/{id}/reset` | Clear session data |

### Authentication & Linking

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/accounts/{id}/status` | Auth status |
| GET | `/api/v1/accounts/{id}/link/qr` | Get QR code for pairing |
| POST | `/api/v1/accounts/{id}/link/phone` | Get phone linking code |
| DELETE | `/api/v1/accounts/{id}/unlink` | Logout |

### Messaging

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/accounts/{id}/chats` | List chats |
| GET | `/api/v1/accounts/{id}/chats/{chat_id}/messages` | Get messages |
| POST | `/api/v1/accounts/{id}/chats/{chat_id}/messages` | Send message |
| POST | `/api/v1/accounts/{id}/chats/{chat_id}/typing` | Send typing indicator |
| POST | `/api/v1/accounts/{id}/chats/{chat_id}/read` | Mark as read |
| POST | `/api/v1/accounts/{id}/chats/{chat_id}/messages/{msg_id}/react` | React to message |
| POST | `/api/v1/accounts/{id}/chats/{chat_id}/messages/{msg_id}/reply` | Reply to message |

### Contacts & Groups

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/accounts/{id}/contacts/{contact_id}` | Get contact |
| GET | `/api/v1/accounts/{id}/groups/{group_id}` | Get group info |

## Usage Examples

```bash
# Create account
curl -X POST http://localhost:3000/api/v1/accounts \
  -H "Authorization: Bearer change-this-secret-key-in-production" \
  -H "Content-Type: application/json" \
  -d '{"phone_number": "919876543210", "account_name": "main"}'

# Connect & get QR code
curl -X POST http://localhost:3000/api/v1/accounts/{id}/warmup \
  -H "Authorization: Bearer change-this-secret-key-in-production"

curl http://localhost:3000/api/v1/accounts/{id}/link/qr \
  -H "Authorization: Bearer change-this-secret-key-in-production"

# Or link via phone number
curl -X POST http://localhost:3000/api/v1/accounts/{id}/link/phone \
  -H "Authorization: Bearer change-this-secret-key-in-production" \
  -H "Content-Type: application/json" \
  -d '{"phone_number": "+919876543210"}'

# Send message
curl -X POST http://localhost:3000/api/v1/accounts/{id}/chats/919876543210@s.whatsapp.net/messages \
  -H "Authorization: Bearer change-this-secret-key-in-production" \
  -H "Content-Type: application/json" \
  -d '{"text": "Hello from WaLink!"}'

# Send file with caption
curl -X POST http://localhost:3000/api/v1/accounts/{id}/chats/919876543210@s.whatsapp.net/messages \
  -H "Authorization: Bearer change-this-secret-key-in-production" \
  -F "text=Check this out" \
  -F "file=@document.pdf"
```

## Configuration

See [CONFIGURATION.md](CONFIGURATION.md) for all options, or copy `config/app.example.toml`.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for design details.

## License

MIT
