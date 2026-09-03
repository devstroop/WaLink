# Architecture

## Overview

WaLink is an HTTP API server and MCP server implementing WhatsApp's multi-device protocol. It provides a REST API, an MCP endpoint for AI agents, and an embedded web dashboard — all from a single Go binary. No browser or headless Chrome needed.

```
┌──────────────┐     HTTP/JSON      ┌──────────────────────────────────────┐
│   Client     │  ◄──────────────►  │           WaLink Server              │
│  (any lang)  │                    │                                      │
└──────────────┘                    │  ┌──────────┐  ┌──────────────────┐  │
                                    │  │ Handlers │──│   Middleware      │  │
┌──────────────┐     MCP/SSE       │  │ (REST)   │  │ (Auth, RBAC,     │  │
│  AI Agent    │  ◄──────────────►  │  └────┬─────┘  │  CORS, Billing)  │  │
│(Claude, etc) │                    │       │        └──────────────────┘  │
└──────────────┘                    │  ┌────▼──────────────┐               │
                                    │  │  AccountManager   │               │
┌──────────────┐     HTMX/HTML     │  │  (multi-account)  │               │
│  Browser     │  ◄──────────────►  │  └────┬──────────────┘               │
│  (Web UI)    │                    │       │                              │
└──────────────┘                    │  ┌────▼────────────────┐             │
                                    │  │  Account            │             │
                                    │  │  (whatsmeow.Client) │             │
                                    │  └────┬────────────────┘             │
                                     │       │                              │
                                       │  ┌────▼────┐  ┌──────────┐           │
                                       │  │PostgreSQL│  │ WhatsApp │           │
                                       │  │ (store) │  │ Servers  │           │
                                       │  └─────────┘  └──────────┘           │
                                       └──────────────────────────────────────┘
```

## Project Layout

```
walink/
├── cmd/walink/main.go              Entry point, server bootstrap, MCP transport
├── config/                         TOML configuration files
└── internal/
    ├── config/config.go            Config loading & defaults
    ├── database/
       │   ├── database.go             Account registry, users, roles, API keys, settings (PostgreSQL)
       │   └── billing.go              Plans, subscriptions, usage tracking
    ├── handler/
    │   ├── routes.go               REST API route registration (64 routes)
    │   ├── accounts.go             Account CRUD
    │   ├── whatsapp.go             WhatsApp operations (messaging, groups, etc.)
    │   ├── auth.go                 Login, register, password reset
    │   ├── users.go                User CRUD (admin)
    │   ├── roles.go                Role CRUD (admin)
    │   ├── apikeys.go              API key management
    │   ├── billing.go              Plans, subscriptions, usage
    │   ├── mcp.go                  MCP settings (toggle on/off, change path)
    │   ├── health.go               Health check
    │   ├── proxy.go                Per-account proxy config
    │   ├── webhook.go              Per-account webhook config
    │   ├── messages.go             Message send/history/react/revoke
    │   └── swagger.go              Swagger UI handler
    ├── mcpserver/server.go         MCP tool definitions (26 tools)
    ├── middleware/middleware.go     Auth (JWT + API key + secret), RBAC, CORS, rate limit, billing
    ├── model/model.go              Request/response types, billing models
    ├── service/
    │   ├── account.go              WhatsApp-backed account lifecycle
    │   ├── manager.go              Multi-account orchestration
    │   └── proxy.go                SOCKS5/HTTP proxy support
    ├── smtp/smtp.go                Email delivery for password resets
    └── web/
        ├── routes.go               Web dashboard route registration
        ├── accounts.go             Account UI handlers
        ├── admin.go                User/role admin pages
        ├── auth.go                 Login/register/forgot-password pages
        ├── middleware.go           Web session auth
        ├── render.go               Template rendering
        ├── static/                 CSS, JS assets
        └── templates/              HTML templates (HTMX)
            └── pages/
                ├── messaging.html  Full messaging UI (chats, send, media, bulk)
                ├── pricing.html    Billing plans page
                └── ...
```

## Key Components

### AccountManager (`service/manager.go`)

Owns all accounts. Responsible for:
- Creating/deleting accounts (DB + in-memory map)
- Discovering existing accounts at startup from PostgreSQL
- Graceful shutdown of all connections

### Account (`service/account.go`)

Wraps a single WhatsApp client connection. Each account has:
- **Isolated data directory** with its own `session.db` (Signal protocol keys, device session)
- **Lifecycle states**: `sleeping` → `connecting` → `active` → (idle timeout) → `sleeping`
- **Idle timer**: Background goroutine polls every 30s, disconnects after `idle_timeout`
- **Auto-connect**: Any API request triggers `EnsureConnected()` — no manual warmup needed
- **Crash recovery**: If the connection drops, the next request detects it and reconnects
- **Phone resolution**: Accepts phone numbers, resolves to JIDs via WhatsApp
- **Per-account rate limiting**: Token bucket (30 msg/min) for send operations
- **Webhook dispatch**: Configurable timeout, retries with exponential back-off, HMAC signing

### Database (`database/database.go`)

PostgreSQL database storing:
- Account registry (ID, phone, name, data dir, idle timeout, status, user_id)
- Users (bcrypt passwords, role FK)
- Roles and permissions (`resource:action` format)
- API keys (SHA-256 hashed, expiry, account binding)
- Settings (key-value store for runtime config like MCP path)
- Billing: plans, subscriptions, daily usage

### MCP Server (`mcpserver/server.go`)

26 tools covering accounts, sessions, messaging, contacts, groups, presence, and profile. Uses Streamable HTTP transport with stateful SSE sessions. Supports account scoping via API key binding or explicit `account_id` parameter.

### Middleware (`middleware/middleware.go`)

- **Auth**: Three-path — static secret key → JWT → API key. Returns 401 on failure.
- **RBAC**: `RequirePermission("resource:action")` checks identity permissions.
- **MCPScope**: Auto-scopes MCP requests to the API key's bound account.
- **CORS**: Configurable origins, methods, headers with preflight support.
- **RateLimit**: Semaphore-based concurrent request limiter (429 when full).
- **BillingEnforcer**: Optional plan-based enforcement (message quotas, feature gates).

## Request Flow

```
HTTP Request
  → CORS middleware
    → Auth middleware (secret key / JWT / API key)
      → RBAC permission check
        → Billing enforcer (if enabled)
          → Handler (route matched)
            → AccountManager.GetAccount(id)
              → Account.EnsureConnected()   ← auto-warms if sleeping
                → WA Client operation
                  → WhatsApp servers (encrypted WebSocket)
```

MCP requests follow a similar path but through the MCP transport:

```
MCP Request (Streamable HTTP + SSE)
  → Auth middleware
    → MCPScope middleware (account binding)
      → MCP tool handler
        → AccountManager.GetAccount(id)
          → Account.EnsureConnected()
            → WA Client operation
```

## Data Layout

```
~/.walink/
├── db/
│   └── walink.db               # Account registry, users, roles, API keys, settings, billing
└── accounts/
    └── {uuid}/
        └── session.db          # WhatsApp session (Signal keys, device state)
```

## Protocol

WaLink implements WhatsApp's multi-device protocol:
- **Noise Protocol** for encrypted WebSocket transport
- **Signal Protocol** (libsignal) for end-to-end encrypted messages
- **Protobuf** for message serialization
- No browser, no DOM, no Chrome — direct protocol communication
