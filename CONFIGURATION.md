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
# dsn = "postgres://walink:walink@localhost:5432/walink?sslmode=disable"  # PostgreSQL DSN

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

