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

Use a simple **shell script** (`entrypoint.sh`) to manage three processes:
1. PostgreSQL 17
2. Redis 7
3. ekaya-engine

The entrypoint script handles:
- Process startup ordering (Postgres first, then Redis, then engine)
- Graceful shutdown signal propagation
- No auto-restart on crash (if a process dies, container exits - appropriate for demo)

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

### Step 3: Create entrypoint.sh [x]

**Status:** Complete

**What was done:**
- Created `deploy/quickstart/entrypoint.sh` - a simple shell script to manage all processes
- Decided against supervisord in favor of simpler shell script approach (appropriate for demo image)
- Script handles first-run initialization: initdb, pg_hba.conf, postgresql.conf, user/database creation
- Starts PostgreSQL and Redis in background, then runs ekaya-engine
- Includes graceful shutdown handler via trap for SIGTERM/SIGINT
- If any process exits, the container shuts down (no auto-restart - keeps demo simple)

**Key features:**
- First-run detection: checks if `/var/lib/postgresql/data` is empty
- Creates `ekaya` user with password `quickstart`
- Creates `ekaya_engine` database owned by `ekaya`
- Waits for both Postgres and Redis to be ready before starting engine
- Graceful shutdown propagates to all processes

**File location:** `deploy/quickstart/entrypoint.sh`

### Step 4: Create Dockerfile [x]

**Status:** Complete

**What was done:**
- Created `deploy/quickstart/Dockerfile` with multi-stage build
- Three build stages: UI (node:22-alpine), Go (golang:1.25-alpine), Runtime (debian:bookworm-slim)
- Runtime stage installs PostgreSQL 17 and Redis 7 from official repositories
- Copies built binary, UI dist, migrations, config.yaml, and entrypoint.sh into image
- Sets all required environment variables for quickstart mode
- Exposes port 3443 with health check endpoint
- Uses `/entrypoint.sh` to manage all processes

**Key implementation decisions:**
- Base image: `debian:bookworm-slim` (required for official Postgres 17 packages)
- Postgres installation: Official PostgreSQL APT repository with GPG key verification
- Redis: Standard `redis-server` package from Debian repos
- Volume mount: `/var/lib/postgresql/data` for data persistence
- Healthcheck: Polls `/health` endpoint every 10s after 30s startup period
- Build args: `VERSION` and `BUILD_TAGS` (default: `all_adapters`)

**Dependencies:**
- Requires `deploy/quickstart/config.quickstart.yaml` (created in step 2)
- Requires `deploy/quickstart/entrypoint.sh` (created in step 3)
- Requires `migrations/` directory in repo root (already exists)

**File location:** `deploy/quickstart/Dockerfile`

**Notes for next session:**
- The Dockerfile is ready to build but has NOT been tested yet
- Next step should add a Makefile target for convenience
- The image will be ~600-800MB due to Postgres + Redis
- First container startup will be ~10-15 seconds (Postgres init + migrations)

### Step 5: Create README.md

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

### Step 6: Add Makefile target

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
