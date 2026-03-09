# Deployment

AYB is a single binary with no runtime dependencies. Deploy it however you deploy Go binaries.

## Docker

### Quick start

```bash
docker run -p 8090:8090 ghcr.io/gridlhq-staging/allyourbase
```

This starts AYB with managed PostgreSQL — no external database needed.

### With external PostgreSQL

```bash
docker run -p 8090:8090 \
  -e AYB_DATABASE_URL="postgresql://user:pass@host:5432/mydb" \
  ghcr.io/gridlhq-staging/allyourbase
```

### Docker Compose

```yaml
services:
  ayb:
    image: ghcr.io/gridlhq-staging/allyourbase
    ports:
      - "8090:8090"
    environment:
      AYB_DATABASE_URL: "postgresql://ayb:ayb@postgres:5432/ayb?sslmode=disable"
      AYB_AUTH_ENABLED: "true"
      AYB_AUTH_JWT_SECRET: "change-me-to-a-secure-random-string-at-least-32-chars"
      AYB_STORAGE_ENABLED: "true"
    depends_on:
      postgres:
        condition: service_healthy
    volumes:
      - ayb_storage:/app/ayb_storage

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: ayb
      POSTGRES_PASSWORD: ayb
      POSTGRES_DB: ayb
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ayb"]
      interval: 5s
      timeout: 3s
      retries: 5

volumes:
  pgdata:
  ayb_storage:
```

```bash
docker compose up -d
```

## Bare metal / VPS

### Download and install

```bash
curl -fsSL https://allyourbase.io/install.sh | sh
```

### systemd service

Create `/etc/systemd/system/ayb.service`:

```ini
[Unit]
Description=Allyourbase
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=ayb
Group=ayb
WorkingDirectory=/opt/ayb
ExecStart=/usr/local/bin/ayb start
Restart=always
RestartSec=5
Environment=AYB_DATABASE_URL=postgresql://ayb:password@localhost:5432/ayb

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable ayb
sudo systemctl start ayb
```

## Fly.io

```bash
fly launch --image ghcr.io/gridlhq-staging/allyourbase
fly secrets set AYB_DATABASE_URL="postgresql://..."
fly secrets set AYB_AUTH_JWT_SECRET="..."
```

Create `fly.toml`:

```toml
[env]
  AYB_AUTH_ENABLED = "true"
  AYB_STORAGE_ENABLED = "true"

[[services]]
  internal_port = 8090
  protocol = "tcp"

  [[services.ports]]
    port = 443
    handlers = ["tls", "http"]
```

## Railway

1. Create a new project on Railway
2. Add a PostgreSQL database
3. Deploy from the Docker image `ghcr.io/gridlhq-staging/allyourbase`
4. Set environment variables:
   - `AYB_DATABASE_URL` = Railway's `DATABASE_URL`
   - `AYB_AUTH_JWT_SECRET` = a random 32+ character string
   - `AYB_SERVER_PORT` = `$PORT` (Railway's dynamic port)

## Embedded vs external PostgreSQL

| | Embedded | External |
|---|---|---|
| Setup | Zero config | Provide `database.url` |
| Best for | Development, prototyping, single-server | Production, scaling |
| Data location | `~/.ayb/data` (configurable) | Your PostgreSQL server |
| Backups | Manual | Your existing PG backup strategy |
| Performance | Good for moderate workloads | Full PostgreSQL performance |

For production, use an external PostgreSQL instance with proper backups, replication, and monitoring.

## Configuration in production

Key settings for production:

```bash
# Required
AYB_DATABASE_URL="postgresql://..."
AYB_AUTH_JWT_SECRET="..."          # min 32 chars, keep secret

# Recommended
AYB_ADMIN_PASSWORD="..."          # protect the admin dashboard
AYB_CORS_ORIGINS="https://yourapp.com"
AYB_LOG_LEVEL="info"
AYB_EMAIL_BACKEND="smtp"          # or "webhook"
```

## Health check

```bash
curl http://localhost:8090/health
```

Returns `200 OK` when the server is running and the database is connected. Use this for load balancer health checks and container orchestration.
