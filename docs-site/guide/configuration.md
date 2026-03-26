# Configuration
<!-- audited 2026-03-20 -->

AYB resolves configuration in this exact order:

`defaults -> ayb.toml -> AYB_* environment variables -> CLI flags`

## ayb.toml baseline

Create `ayb.toml` in your working directory:

```toml
[server]
host = "127.0.0.1"
port = 8090
site_url = "https://api.example.com" # optional public base URL for links

# Enables automatic HTTPS when set.
tls_domain = "api.example.com"
tls_email = "ops@example.com"
tls_staging = false

[database]
# Leave empty for managed PostgreSQL.
url = ""
max_conns = 25
min_conns = 2
health_check_interval = 30
migrations_dir = "./migrations"

# Optional read replicas.
# [[database.replicas]]
# url = "postgresql://replica-1/db"
# weight = 1
# max_lag_bytes = 10485760

[admin]
enabled = true
path = "/admin"
# password = "set-a-strong-password"

[logging]
level = "info"
format = "json"
```

## Key environment overrides

These environment variables are applied by `config.Load` via `applyEnv`:

| Variable | Config key |
|---|---|
| `AYB_SERVER_HOST` | `server.host` |
| `AYB_SERVER_PORT` | `server.port` |
| `AYB_SERVER_SITE_URL` | `server.site_url` |
| `AYB_TLS_DOMAIN` | `server.tls_domain` |
| `AYB_TLS_EMAIL` | `server.tls_email` |
| `AYB_TLS_STAGING` | `server.tls_staging` |
| `AYB_DATABASE_URL` | `database.url` |
| `AYB_DATABASE_REPLICA_URLS` | `database.replicas` (CSV URLs) |
| `AYB_ADMIN_PASSWORD` | `admin.password` |
| `AYB_CORS_ORIGINS` | `server.cors_allowed_origins` (CSV) |
| `AYB_LOG_LEVEL` | `logging.level` |

Example:

```bash
export AYB_DATABASE_URL="postgresql://user:pass@db:5432/app"
export AYB_SERVER_SITE_URL="https://api.example.com"
export AYB_TLS_DOMAIN="api.example.com"
export AYB_TLS_EMAIL="ops@example.com"
ayb start --port 8091
```

Notes:

- `database.health_check_interval` is a real file key (`[database] health_check_interval`) and is measured in seconds.
- `AYB_DATABASE_REPLICA_URLS` accepts a comma-separated list (for example `url1,url2`).
- Setting `AYB_TLS_DOMAIN` or `--domain` enables TLS during validation.

## CLI overrides

`ayb start` accepts these runtime override flags:

- `--database-url`
- `--port`
- `--host`
- `--domain` (maps to `tls_domain`)

`ayb start --port 9000` overrides both file and environment values for the server port.

## Generating and editing config safely

### Print resolved config

```bash
ayb config
```

### Read one key

```bash
ayb config get server.port
```

### Set one key

```bash
ayb config set server.port 8091
```

`GenerateDefault` and `config set` write config files with `0600` permissions.

If you create files manually (for example `ayb config > ayb.toml`), set permissions yourself:

```bash
chmod 600 ayb.toml
```

## Per-app API key scoping

Per-app API key scoping is configured in the admin UI and API-key payload, not in server config files.
No server configuration is required.
