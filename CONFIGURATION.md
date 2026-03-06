# Configuration

WaLink reads configuration from `config/app.toml` (relative to working directory), falling back to `~/.walink/config.toml`. Missing keys use defaults.

## Reference

```toml
[server]
host = "0.0.0.0"       # Bind address
port = 3000             # Listen port

[auth]
secret_key = "..."      # Bearer token for API authentication (CHANGE THIS)

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
```

## Data Layout

```
~/.walink/
├── db/
│   └── walink.db               # Account registry (SQLite)
└── accounts/
    └── {uuid}/
        └── session.db          # WhatsApp session (Signal keys, device state)
```

## Environment Notes

- **SQLite**: Uses pure-Go `modernc.org/sqlite` — no CGO or C compiler required.
- **Idle timeout**: A background goroutine polls every 30s. When an account has been idle longer than `idle_timeout`, it disconnects automatically. Any API request to that account reconnects it on demand.
- **secret_key**: Used for Bearer token auth. All API endpoints under `/api/v1/` require `Authorization: Bearer <secret_key>`. Health endpoints are unauthenticated.
