# Changelog

## [0.1.0] — 2026-02-27

Initial scaffolding.

### Added
- Multi-account management with UUID-based isolation
- whatsmeow integration for native WhatsApp multi-device protocol
- QR code and phone number pairing for authentication
- Send text messages and file attachments (documents with captions)
- Account lifecycle: auto-connect on demand, idle timeout auto-disconnect
- SQLite-backed account registry
- Bearer token authentication middleware
- CORS middleware with configurable origins
- TOML configuration with sensible defaults
- RESTful API with 20 endpoints covering accounts, auth, messaging, contacts, groups
- Graceful shutdown with connection cleanup

### Endpoints
- Account CRUD: create, get, list, delete, config
- Lifecycle: warmup, reset
- Auth: status, QR link, phone link, unlink
- Messaging: list chats, get messages, send message (text + file)
- Actions: typing indicator, mark read, react, reply
- Info: contacts, groups
- Health: health check, readiness check
