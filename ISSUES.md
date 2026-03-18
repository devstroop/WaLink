# ISSUES.md — WaLink Open Issues & Gaps

> Updated — March 18, 2026

---

## MCP ↔ REST API Coverage

The REST API exposes **~64 endpoints**; the MCP server exposes **26 tools** covering all core WhatsApp operations.

### Not Exposed via MCP (Admin/Config — Low Priority)

| API Endpoint | Description |
|-------------|-------------|
| `POST /accounts` | Create account |
| `PATCH /accounts/{id}` | Update account |
| `DELETE /accounts/{id}` | Delete account |
| `GET/PUT/DELETE /proxy` | Proxy configuration |
| `GET/PUT/DELETE /webhook` | Webhook configuration |
| Newsletter tools | Follow/unfollow/mute channels |

---

## Web UI Gaps

### Bugs

| # | Severity | Issue |
|---|----------|-------|
| W1 | HIGH | **404 is raw plaintext** — unknown routes return Go's default `404 page not found` instead of styled error page |
| W2 | MED | **No favicon** — every page triggers `GET /favicon.ico → 404` |

### Missing Web UI Features

| Category | Status | Notes |
|----------|--------|-------|
| Webhook config | Not built | Account detail tab exists but has no form |
| Proxy config | Not built | Account detail tab exists but has no form |
| Account edit | Not built | Detail page is read-only (API supports PATCH) |

### UX Gaps

| # | Issue |
|---|-------|
| U1 | No pagination on accounts list |
| U2 | Non-admin sidebar filtering not verified |

---

## Resolved Issues

All code quality issues from the initial audit have been fixed:

- ✅ Repeated connection-check pattern — extracted `requireConnectedClient()` helper
- ✅ Webhook config dead code — config fields now drive dispatch behavior
- ✅ Webhook dispatch retries and timeout — exponential back-off with configurable timeout
- ✅ Media type detection — images, videos, audio, stickers use proper message types
- ✅ Per-account rate limiting — token bucket (30 msg/min)
- ✅ RBAC authentication — users, roles, permissions, JWT, API keys
- ✅ MCP full tool coverage — 26 tools covering all core operations
- ✅ Web messaging UI — full chat interface with send, bulk, reactions, channels
- ✅ Admin pages — users, roles, API keys management
- ✅ Routing inconsistencies — all fixed
