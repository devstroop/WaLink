# Architecture

## Overview

WaLink is an HTTP API server, implementing WhatsApp's multi-device protocol. No browser or headless Chrome needed.

```
┌──────────────┐     HTTP/JSON      ┌──────────────────────────────────┐
│   Client     │  ◄──────────────►  │           WaLink Server          │
│  (any lang)  │                    │                                  │
└──────────────┘                    │  ┌──────────┐  ┌──────────────┐  │
                                    │  │ Handlers │──│  Middleware  │  │
                                    │  └────┬─────┘  │ (Auth, CORS) │  │
                                    │       │        └──────────────┘  │
                                    │  ┌────▼──────────────┐           │
                                    │  │  AccountManager   │           │
                                    │  │  (multi-account)  │           │
                                    │  └────┬──────────────┘           │
                                    │       │                          │
                                    │  ┌────▼────────────────┐         │
                                    │  │  Account            │         │
                                    │  │  (WA Client)        │         │
                                    │  └────┬────────────────┘         │
                                    │       │                          │
                                    │  ┌────▼────┐  ┌──────────┐       │
                                    │  │ SQLite  │  │ WhatsApp │       │
                                    │  │ (store) │  │ Servers  │       │
                                    │  └─────────┘  └──────────┘       │
                                    └──────────────────────────────────┘
```

## Project Layout

```
walink/
├── cmd/walink/main.go              Entry point, server bootstrap
├── config/                         TOML configuration files
└── internal/
    ├── config/config.go            Config loading & defaults
    ├── database/database.go        Account registry (SQLite CRUD)
    ├── handler/
    │   ├── health.go               Health check endpoints
    │   ├── routes.go               Route registration
    │   ├── accounts.go             Account CRUD handlers
    │   └── whatsapp.go             WhatsApp operations (auth, messaging)
    ├── middleware/middleware.go     Bearer auth & CORS
    ├── model/model.go              Request/response types
    └── service/
        ├── account.go              WhatsApp-backed account lifecycle
        └── manager.go              Multi-account orchestration
```

## Key Components

### AccountManager (`service/manager.go`)

Owns all accounts. Responsible for:
- Creating/deleting accounts (DB + in-memory map)
- Discovering existing accounts at startup from SQLite
- Graceful shutdown of all connections

### Account (`service/account.go`)

Wraps a single WhatsApp client connection. Each account has:
- **Isolated data directory** with its own `session.db` (Signal protocol keys, device session)
- **Lifecycle states**: `sleeping` → `connecting` → `active` → (idle timeout) → `sleeping`
- **Idle timer**: Background goroutine polls every 30s, disconnects after `idle_timeout`
- **Auto-connect**: Any API request triggers `EnsureConnected()` — no manual warmup needed
- **Crash recovery**: If the connection drops, the next request detects it and reconnects

### Database (`database/database.go`)

Account registry in SQLite. Stores account metadata (ID, phone, name, data dir, idle timeout, status). A separate SQLite database per account stores Signal protocol state.

### Middleware (`middleware/middleware.go`)

- **Auth**: Validates `Authorization: Bearer <key>` against configured secret. Returns 401 on mismatch.
- **CORS**: Sets Access-Control headers from config. Handles OPTIONS preflight.

## Request Flow

```
HTTP Request
  → CORS middleware
    → Auth middleware (Bearer token check)
      → Handler (route matched)
        → AccountManager.GetAccount(id)
          → Account.EnsureConnected()   ← auto-warms if sleeping
            → WA Client operation
              → WhatsApp servers (encrypted WebSocket)
```

## Two SQLite Databases

1. **`~/.walink/db/walink.db`** — Account registry. One row per account.
2. **`~/.walink/accounts/{uuid}/session.db`** — Per-account. Contains Signal protocol keys, identity store, session keys, pre-keys, sender keys, contacts, chat settings.

## Protocol

WaLink implements WhatsApp's multi-device protocol:
- **Noise Protocol** for encrypted WebSocket transport
- **Signal Protocol** (libsignal) for end-to-end encrypted messages
- **Protobuf** for message serialization
- No browser, no DOM, no Chrome — direct protocol communication
