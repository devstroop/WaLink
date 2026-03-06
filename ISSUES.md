# Known Issues & TODO

Tracking of incomplete features, stubs, and known problems.

---

## Stub Handlers

These API endpoints are registered and routed but return hardcoded empty responses. Each needs a real implementation.

| # | Endpoint | Handler | What's needed |
|---|----------|---------|---------------|
| 1 | ~~`GET /accounts/{id}/chats`~~ | ~~`ListChats`~~ | **Done** — returns contacts from local store |
| 2 | ~~`GET /accounts/{id}/messages?chat=...`~~ | ~~`GetMessages`~~ | **Done** — cursor-paginated from local message store |
| 3 | ~~`POST /accounts/{id}/chats/{chat_id}/typing`~~ | ~~`SendTyping`~~ | **Done** — `SendChatPresence` |
| 4 | ~~`POST /accounts/{id}/chats/{chat_id}/read`~~ | ~~`MarkRead`~~ | **Done** — `MarkRead` with message IDs |
| 5 | ~~`POST /accounts/{id}/messages/{msg_id}/react`~~ | ~~`ReactMessage`~~ | **Done** — `ReactionMessage` |
| 6 | ~~`POST /accounts/{id}/messages/{msg_id}/reply`~~ | ~~`ReplyMessage`~~ | **Done** — `ExtendedTextMessage` with `ContextInfo` |
| 7 | ~~`GET /accounts/{id}/contacts/{contact_id}`~~ | ~~`GetContact`~~ | **Done** — `Store.Contacts.GetContact` |
| 8 | ~~`GET /accounts/{id}/groups/{group_id}`~~ | ~~`GetGroup`~~ | **Done** — `client.GetGroupInfo` |

## Incomplete Internals

| # | File | Issue |
|---|------|-------|
| 9 | ~~`internal/service/account.go` — `handleEvent`~~ | **Done** — incoming messages stored, outgoing messages stored, receipt events forwarded via webhook. `eventCh` removed (dead code). |

## Build & Tooling

| # | Issue |
|---|-------|
| 10 | ~~`go.sum` missing~~ — **resolved** |
| 11 | No CI/CD pipeline (GitHub Actions, etc.) |
| 12 | ~~No integration / end-to-end tests~~ | **Done** — 29 integration tests in `tests/integration_test.go` covering full HTTP stack (health, auth, CORS, Swagger, account CRUD, messages, webhooks, proxy, rate limiting, error format, unconnected endpoints) |
| 13 | ~~CGO required for `mattn/go-sqlite3`~~ — switched to pure-Go `modernc.org/sqlite` |

## Documentation

| # | Issue |
|---|-------|
| 14 | ~~README documents all endpoints as functional; GetMessages still returns empty~~ — **Done** — README fully rewritten with accurate endpoint list |
| 15 | ~~No API error reference / status code documentation~~ — **Done** — Error Responses section added to README |

## Feature Status

| # | Feature | Status |
|---|---------|--------|
| 16 | ~~Webhook delivery for incoming messages / status updates~~ | **Done** — `message` and `receipt` events dispatched with retry (3 attempts, exponential back-off) |
| 17 | ~~Rate limiting middleware~~ | **Done** — concurrency-based limiter (429 when `max_concurrent_requests` exceeded) |
| 18 | Prometheus metrics endpoint | Not started |
| 19 | ~~Auto-reconnect on disconnect~~ | **Done** — exponential retry (2s → 60s, 5 attempts) on `Disconnected` event |
| 20 | ~~Swagger / OpenAPI spec~~ | **Done** — embedded `openapi.json` served via Swagger UI at `/api-docs` |
