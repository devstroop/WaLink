# Known Issues & TODO

Tracking of incomplete features, stubs, and known problems.

---

## Stub Handlers

These API endpoints are registered and routed but return hardcoded empty responses. Each needs a real whatsmeow implementation.

| # | Endpoint | Handler | What's needed |
|---|----------|---------|---------------|
| 1 | ~~`GET /accounts/{id}/chats`~~ | ~~`ListChats`~~ | **Done** — returns contacts from whatsmeow store |
| 2 | `GET /accounts/{id}/chats/{chat_id}/messages` | `GetMessages` | Requires a local message store (not yet built) |
| 3 | ~~`POST /accounts/{id}/chats/{chat_id}/typing`~~ | ~~`SendTyping`~~ | **Done** — `SendChatPresence` |
| 4 | ~~`POST /accounts/{id}/chats/{chat_id}/read`~~ | ~~`MarkRead`~~ | **Done** — `MarkRead` with message IDs |
| 5 | ~~`POST /accounts/{id}/messages/{msg_id}/react`~~ | ~~`ReactMessage`~~ | **Done** — `ReactionMessage` via whatsmeow |
| 6 | ~~`POST /accounts/{id}/messages/{msg_id}/reply`~~ | ~~`ReplyMessage`~~ | **Done** — `ExtendedTextMessage` with `ContextInfo` |
| 7 | ~~`GET /accounts/{id}/contacts/{contact_id}`~~ | ~~`GetContact`~~ | **Done** — `Store.Contacts.GetContact` |
| 8 | ~~`GET /accounts/{id}/groups/{group_id}`~~ | ~~`GetGroup`~~ | **Done** — `client.GetGroupInfo` |

## Incomplete Internals

| # | File | Issue |
|---|------|-------|
| 9 | `internal/service/account.go` — `handleEvent` | Event handler now logs `Message`, `Receipt`, `PushName`, `HistorySync`, `Connected`, `LoggedOut`, `Disconnected`. Still no incoming message persistence or delivery receipt forwarding. The `eventCh` channel is created but never consumed. |

## Build & Tooling

| # | Issue |
|---|-------|
| 10 | ~~`go.sum` missing~~ — **resolved** |
| 11 | No CI/CD pipeline (GitHub Actions, etc.) |
| 12 | No integration / end-to-end tests (only unit tests exist) |
| 13 | ~~CGO required for `mattn/go-sqlite3`~~ — switched to pure-Go `modernc.org/sqlite` |

## Documentation

| # | Issue |
|---|-------|
| 14 | README documents all 20 endpoints as functional; GetMessages still returns empty |
| 15 | No API error reference / status code documentation |

## Future Enhancements

| # | Feature |
|---|---------|
| 16 | Webhook delivery for incoming messages / status updates |
| 17 | Rate limiting middleware |
| 18 | Prometheus metrics endpoint |
| 19 | Multi-device session persistence across restarts (auto-reconnect) |
| 20 | Swagger / OpenAPI spec generation |
