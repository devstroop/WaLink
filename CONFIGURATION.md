# Configuration

WaLink reads configuration from `config/app.toml` (relative to working directory), falling back to `~/.walink/config.toml`. Missing keys use defaults. All settings can be overridden via environment variables using the `WALINK_SECTION_KEY` format (e.g. `WALINK_SERVER_PORT=8080`).

## Reference

```toml
[server]
host = "0.0.0.0"       # Bind address
port = 3000             # Listen port

[auth]
secret_key = "..."      # Bearer token for API authentication (CHANGE THIS)
registration_enabled = false  # Set to true to allow public user registration

[smtp]
# host = "smtp.example.com"  # Required for forgot-password emails
# port = 587
# username = ""
# password = ""
# from = "WaLink <noreply@example.com>"
# tls = false       # Use implicit TLS (port 465)
# starttls = true   # Use STARTTLS (port 587)

[logging]
level = "info"          # trace | debug | info | warn | error

[database]
path = ""               # SQLite path. Default: ~/.walink/db/walink.db

[cors]
allow_origins = ["*"]
allow_methods = ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
allow_headers = ["authorization", "content-type"]

[limits]
max_concurrent_requests = 50
request_timeout_ms = 30000
max_upload_size = 10485760      # 10MB

[accounts]
base_directory = ""             # Default: ~/.walink/accounts
                                # Each account gets a subdirectory with its UUID

[accounts.defaults]
idle_timeout = 300              # Auto-disconnect after N seconds idle (0 = never)

[webhooks]
enabled = false
timeout_ms = 5000
retry_count = 3
retry_delay_ms = 1000

[swagger]
enabled = true
path = "/api-docs"

[mcp]
enabled = true          # Enable/disable MCP server (can also toggle at runtime via admin UI)
path = "/mcp"           # MCP endpoint path

[billing]
enabled = false                        # Set to true to enable plan-based billing
# stripe_secret_key = "sk_..."         # Stripe secret key (for paid plans)
# stripe_webhook_secret = "whsec_..."  # Stripe webhook signing secret
default_plan = "free"                  # Plan assigned to new users
```

## Environment Variables

All config keys map to environment variables with the `WALINK_` prefix. Nested keys use underscores.

| Config Key | Environment Variable | Example |
|-----------|---------------------|---------|
| `server.host` | `WALINK_SERVER_HOST` | `0.0.0.0` |
| `server.port` | `WALINK_SERVER_PORT` | `3000` |
| `auth.secret_key` | `WALINK_AUTH_SECRET_KEY` | `my-secret` |
| `auth.registration_enabled` | `WALINK_AUTH_REGISTRATION_ENABLED` | `true` |
| `logging.level` | `WALINK_LOG_LEVEL` | `debug` |
| `database.path` | `WALINK_DATABASE_PATH` | `/data/db/walink.db` |
| `accounts.base_directory` | `WALINK_ACCOUNTS_DIR` | `/data/accounts` |
| `cors.allow_origins` | `WALINK_CORS_ORIGINS` | `*` |
| `swagger.enabled` | `WALINK_SWAGGER_ENABLED` | `true` |
| `billing.enabled` | `WALINK_BILLING_ENABLED` | `false` |

## Data Layout

```
~/.walink/
├── db/
│   └── walink.db               # Account registry, users, roles, API keys, settings, billing
└── accounts/
    └── {uuid}/
        └── session.db          # WhatsApp session (Signal keys, device state)
```

## Docker

The included `docker-compose.yml` maps all key settings to environment variables:

```bash
docker compose up -d
```

Data is persisted in a named volume (`walink-data`) mounted at `/data`.

## Notes

- **SQLite**: Uses pure-Go `modernc.org/sqlite` — no CGO or C compiler required.
- **Idle timeout**: A background goroutine polls every 30s. When an account has been idle longer than `idle_timeout`, it disconnects automatically. Any API request to that account reconnects it on demand.
- **secret_key**: Used for Bearer token auth and JWT signing. All API endpoints under `/api/v1/` require authentication. Health and public auth endpoints are unauthenticated.
- **MCP runtime toggle**: The MCP endpoint can be enabled/disabled at runtime via the admin UI or `PATCH /api/v1/mcp` without restarting the server.
- **Billing enforcement**: When enabled, the billing middleware checks message quotas and feature gates (MCP access, webhooks) against the user's plan. System admin (secret key) bypasses all billing.
