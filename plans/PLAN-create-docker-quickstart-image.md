# PLAN: Create Docker Quickstart Image

## Overview

Create an all-in-one Docker image (`ghcr.io/ekaya-inc/engine-quickstart`) that bundles Postgres, Redis, and ekaya-engine for a zero-config "try Ekaya in 60 seconds" experience.

**Target user experience:**
```bash
docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data ghcr.io/ekaya-inc/engine-quickstart:latest
# Open http://localhost:3443
```

## Scope

**In scope:**
- Create Dockerfile and supporting files
- Local build and test
- Documentation for the quickstart image

**Out of scope (separate plan):**
- CI/CD pipeline to publish to GHCR
- Website integration (us.ekaya.ai/get-started)

## Architecture

### Process Management

Use **supervisord** to manage three processes:
1. PostgreSQL 17
2. Redis 7
3. ekaya-engine

Supervisord handles:
- Process startup ordering (Postgres first, then Redis, then engine)
- Automatic restart on crash
- Graceful shutdown signal propagation

### Data Persistence

Volume mount: `-v ekaya-data:/var/lib/postgresql/data`

The entrypoint script will:
1. Check if `/var/lib/postgresql/data` is empty (first run)
2. If empty, run `initdb` to initialize Postgres
3. Start supervisord which manages all processes

### Embedded Configuration

Static values baked into the image:
```
PROJECT_CREDENTIALS_KEY=quickstart-demo-key-not-for-production
PGHOST=localhost
PGPORT=5432
PGUSER=ekaya
PGPASSWORD=quickstart
PGDATABASE=ekaya_engine
PGSSLMODE=disable
REDIS_HOST=localhost
REDIS_PORT=6379
BIND_ADDR=0.0.0.0
PORT=3443
```

**Auth disabled** - No external auth server required. Users can explore the UI without signing in.

**LLM features** - Users who want ontology extraction must provide their own `ANTHROPIC_API_KEY`:
```bash
docker run -p 3443:3443 -e ANTHROPIC_API_KEY=sk-... -v ekaya-data:/var/lib/postgresql/data ghcr.io/ekaya-inc/engine-quickstart:latest
```

## Directory Structure

```
deploy/
└── quickstart/
    ├── Dockerfile
    ├── supervisord.conf
    ├── entrypoint.sh
    ├── config.quickstart.yaml
    └── README.md
```

## Implementation Steps

### Step 1: Create directory structure [x]

**Status:** Complete

**What was done:**
- Created `deploy/quickstart/` directory structure
- Verified the directory exists and is ready for subsequent files

**Notes for next session:**
- The directory is empty and ready for config files, scripts, and Dockerfile
- No other changes were made to the codebase in this step

### Step 2: Create config.quickstart.yaml [x]

**Status:** Complete

**What was done:**
- Created `deploy/quickstart/config.quickstart.yaml` with auth disabled for demos
- Configuration targets the all-in-one Docker environment (Postgres + Redis on localhost)
- Password provided via `PGPASSWORD` environment variable (set in Dockerfile/supervisord)
- Encryption key provided via `PROJECT_CREDENTIALS_KEY` environment variable

**Key configuration decisions:**
- `auth.enable_verification: false` - No external auth server required for quickstart
- `database.ssl_mode: disable` - Localhost connection, no TLS needed
- `enabled_tools: []` - Empty list means all tools enabled by default
- `ontology_max_iterations: 3` and `ontology_target_confidence: 0.8` - Standard settings

**Notes for next session:**
- The config file is ready to be copied into the Docker image
- Environment variables (`PGPASSWORD`, `PROJECT_CREDENTIALS_KEY`) will be set in Dockerfile/supervisord
- This config is only for the quickstart image, not for production use

**File location:** `deploy/quickstart/config.quickstart.yaml`

### Step 3: Create supervisord.conf

```ini
[supervisord]
nodaemon=true
user=root
logfile=/var/log/supervisord.log
pidfile=/var/run/supervisord.pid

[program:postgresql]
command=/usr/lib/postgresql/17/bin/postgres -D /var/lib/postgresql/data -c config_file=/etc/postgresql/postgresql.conf
user=postgres
autostart=true
autorestart=true
priority=100
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0

[program:redis]
command=/usr/bin/redis-server --bind 127.0.0.1 --port 6379
autostart=true
autorestart=true
priority=200
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0

[program:ekaya-engine]
command=/usr/local/bin/ekaya-engine
directory=/app
autostart=true
autorestart=true
priority=300
startsecs=5
startretries=10
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
environment=PGPASSWORD="quickstart",PROJECT_CREDENTIALS_KEY="quickstart-demo-key-not-for-production"
```

Note: Engine has `startsecs=5` and `startretries=10` to allow Postgres to initialize on first run.

### Step 4: Create entrypoint.sh

```bash
#!/bin/bash
set -e

PGDATA=/var/lib/postgresql/data

# Initialize Postgres if data directory is empty
if [ -z "$(ls -A $PGDATA 2>/dev/null)" ]; then
    echo "Initializing PostgreSQL database..."
    chown -R postgres:postgres $PGDATA
    su postgres -c "/usr/lib/postgresql/17/bin/initdb -D $PGDATA"

    # Configure pg_hba.conf for local trust auth
    echo "local all all trust" > $PGDATA/pg_hba.conf
    echo "host all all 127.0.0.1/32 trust" >> $PGDATA/pg_hba.conf
    echo "host all all ::1/128 trust" >> $PGDATA/pg_hba.conf

    # Configure postgresql.conf
    echo "listen_addresses = 'localhost'" >> $PGDATA/postgresql.conf
    echo "port = 5432" >> $PGDATA/postgresql.conf
fi

# Ensure postgres owns the data directory (for volume mounts)
chown -R postgres:postgres $PGDATA

# Start supervisord (manages postgres, redis, engine)
exec /usr/bin/supervisord -c /etc/supervisor/conf.d/supervisord.conf
```

### Step 5: Create Dockerfile

Multi-stage build that:
1. Builds the Go binary (reuse existing builder stages)
2. Builds the UI
3. Creates runtime image with Postgres + Redis + supervisord

```dockerfile
# ============================================
# Quickstart Image - All-in-one for demo/trial
# ============================================
# Usage:
#   docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data ghcr.io/ekaya-inc/engine-quickstart:latest
#
# For LLM features (ontology extraction):
#   docker run -p 3443:3443 -e ANTHROPIC_API_KEY=sk-... -v ekaya-data:/var/lib/postgresql/data ghcr.io/ekaya-inc/engine-quickstart:latest

# UI Build stage
FROM node:22-alpine AS ui-builder
WORKDIR /app/ui
COPY ui/package*.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

# Go Build stage
FROM golang:1.25-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=ui-builder /app/ui/dist ./ui/dist

ARG VERSION=dev
ARG BUILD_TAGS=all_adapters

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -tags="${BUILD_TAGS}" \
    -ldflags="-w -s -X main.Version=${VERSION}" \
    -o ekaya-engine \
    main.go

# Runtime stage with Postgres + Redis + supervisord
FROM debian:bookworm-slim

# Install Postgres 17, Redis, and supervisord
RUN apt-get update && apt-get install -y --no-install-recommends \
    gnupg2 \
    lsb-release \
    curl \
    ca-certificates \
    supervisor \
    redis-server \
    && curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /usr/share/keyrings/postgresql-keyring.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/postgresql-keyring.gpg] http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends postgresql-17 \
    && rm -rf /var/lib/apt/lists/*

# Create app directory
WORKDIR /app

# Copy binary and UI from builders
COPY --from=go-builder /app/ekaya-engine /usr/local/bin/ekaya-engine
COPY --from=ui-builder /app/ui/dist /app/ui/dist

# Copy migrations (engine runs these on startup)
COPY migrations/ /app/migrations/

# Copy quickstart config and scripts
COPY deploy/quickstart/config.quickstart.yaml /app/config.yaml
COPY deploy/quickstart/supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY deploy/quickstart/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Environment variables
ENV PORT=3443 \
    BIND_ADDR=0.0.0.0 \
    PGHOST=localhost \
    PGPORT=5432 \
    PGUSER=ekaya \
    PGDATABASE=ekaya_engine \
    PGSSLMODE=disable \
    REDIS_HOST=localhost \
    REDIS_PORT=6379 \
    PROJECT_CREDENTIALS_KEY=quickstart-demo-key-not-for-production

# Create postgres data directory mount point
VOLUME ["/var/lib/postgresql/data"]

EXPOSE 3443

HEALTHCHECK --interval=10s --timeout=3s --start-period=30s --retries=5 \
    CMD curl -f http://localhost:3443/health || exit 1

ENTRYPOINT ["/entrypoint.sh"]
```

### Step 6: Create README.md

```markdown
# Ekaya Engine Quickstart Image

All-in-one Docker image for trying Ekaya Engine locally.

## Quick Start

```bash
docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data ghcr.io/ekaya-inc/engine-quickstart:latest
```

Open http://localhost:3443

## With LLM Features

For ontology extraction and AI features, provide your Anthropic API key:

```bash
docker run -p 3443:3443 \
  -e ANTHROPIC_API_KEY=sk-ant-... \
  -v ekaya-data:/var/lib/postgresql/data \
  ghcr.io/ekaya-inc/engine-quickstart:latest
```

## Data Persistence

The `-v ekaya-data:/var/lib/postgresql/data` mount persists your data between container restarts.

To start fresh, remove the volume:

```bash
docker volume rm ekaya-data
```

## What's Included

- PostgreSQL 17
- Redis 7
- Ekaya Engine with UI

## Not for Production

This image is for evaluation only:
- Uses static encryption key
- Auth verification disabled
- Single-container architecture

For production, see the main deployment documentation.
```

### Step 7: Add Makefile target

Add to Makefile:

```makefile
# Quickstart image for demos/trials
quickstart-build: ## Build the all-in-one quickstart Docker image
	@echo "$(YELLOW)Building quickstart image...$(NC)"
	@docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TAGS=$(BUILD_TAGS) \
		-f deploy/quickstart/Dockerfile \
		-t engine-quickstart:local \
		.
	@echo "$(GREEN)✓ Quickstart image built: engine-quickstart:local$(NC)"
	@echo ""
	@echo "Run with:"
	@echo "  docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data engine-quickstart:local"

quickstart-run: quickstart-build ## Build and run the quickstart image
	@echo "$(YELLOW)Starting quickstart container...$(NC)"
	@docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data engine-quickstart:local
```

## Testing Checklist

### Build Test

```bash
make quickstart-build
```

Expected: Image builds successfully.

### First Run Test

```bash
docker volume rm ekaya-data 2>/dev/null || true
docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data engine-quickstart:local
```

Expected:
- Postgres initializes (first run only)
- Migrations run
- Server starts
- `/health` returns healthy
- UI loads at http://localhost:3443

### Persistence Test

1. Start container, create a project
2. Stop container (Ctrl+C)
3. Start container again
4. Project should still exist

### Health Check Test

```bash
curl http://localhost:3443/ping
curl http://localhost:3443/health
```

Expected: Both return valid JSON responses.

### LLM Feature Test (optional)

```bash
docker run -p 3443:3443 \
  -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
  -v ekaya-data:/var/lib/postgresql/data \
  engine-quickstart:local
```

Expected: Ontology extraction works when triggered from UI.

## Known Limitations

1. **Image size**: ~600-800MB due to Postgres + Redis
2. **Startup time**: 10-15 seconds on first run (Postgres init + migrations)
3. **Single volume**: Postgres data only; Redis is ephemeral (acceptable for demo)
4. **No TLS**: HTTP only (localhost is fine, but not production-ready)

## Future Enhancements (not in this plan)

- CI/CD pipeline to publish to GHCR on release
- Slim variant using Alpine (if size becomes an issue)
- Optional: include sample data for immediate exploration
