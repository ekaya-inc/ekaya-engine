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

### Step 5: Create README.md [x]

**Status:** Complete

**What was done:**
- Created `deploy/quickstart/README.md` with quick start instructions
- Included example commands for running with and without Anthropic API key
- Documented data persistence using Docker volumes
- Listed what's included (PostgreSQL 17, Redis 7, Ekaya Engine with UI)
- Added clear production warning about static encryption key and disabled auth

**Key sections:**
- Quick Start: Basic `docker run` command with volume mount
- With LLM Features: Same command with `ANTHROPIC_API_KEY` environment variable
- Data Persistence: Volume mount explanation and reset instructions
- What's Included: Component list
- Not for Production: Clear warnings about evaluation-only limitations

**File location:** `deploy/quickstart/README.md`

### Step 6: Add Makefile target [x]

**Status:** Complete

**What was done:**
- Added two new Makefile targets: `quickstart-build` and `quickstart-run`
- Updated `.PHONY` declaration to include both new targets
- Both targets appear in `make help` output with descriptions

**Implementation details:**
- `quickstart-build` target:
  - Builds Docker image from `deploy/quickstart/Dockerfile`
  - Tags as `engine-quickstart:local`
  - Passes `VERSION` (from git describe) and `BUILD_TAGS` (default: all_adapters) as build arguments
  - Shows helpful post-build message with example run command
- `quickstart-run` target:
  - Depends on `quickstart-build` to ensure fresh build
  - Runs container with `-p 3443:3443` port mapping
  - Mounts `ekaya-data` volume to `/var/lib/postgresql/data` for persistence
  - Runs in foreground (not detached) for easy testing

**File modified:** `Makefile` (lines 1, 330-345)

**Notes for next session:**
- The Makefile targets are ready but have NOT been tested yet
- Next step should test the build and run process
- The targets use the same VERSION injection mechanism as other Docker builds in this project
- The image will be tagged as `engine-quickstart:local` to distinguish from CI/CD builds

## Testing Checklist

### Build Test [x]

**Status:** Complete

**What was done:**
- Executed `make quickstart-build` successfully
- Image built without errors and tagged as `engine-quickstart:local`
- Final image size: 493MB (PostgreSQL 17 + Redis 7 + ekaya-engine with UI)
- Build process completed in approximately 2 minutes

**Verification:**
- Image ID: e77b0e7d74b9
- Tag: engine-quickstart:local
- All build stages completed successfully:
  - UI build stage (node:22-alpine)
  - Go build stage (golang:1.25-alpine)
  - Runtime stage (debian:bookworm-slim with PostgreSQL 17 and Redis 7)

**Notes:**
- Build process works as expected
- Multi-stage build optimizes final image size
- Ready for first run testing

### First Run Test [x]

**Status:** Complete

**What was done:**
- Fixed entrypoint.sh to properly handle PostgreSQL environment variables during initialization
- Fixed encryption key in Dockerfile to use a valid 32-byte base64-encoded value
- Successfully tested first container run with clean volume

**Verification results:**
- ✅ Postgres initializes successfully on first run
- ✅ Database migrations applied (version 32)
- ✅ Server starts and listens on 0.0.0.0:3443
- ✅ `/health` endpoint returns healthy status
- ✅ UI loads successfully at http://localhost:3443
- ✅ Container reports healthy status
- ✅ Docker volume `ekaya-data` created for persistence

**Issues found and fixed:**
1. **PostgreSQL environment variable interference**: The `PGUSER` and `PGDATABASE` environment variables set in the Dockerfile were being inherited by the `su postgres -c "psql ..."` commands, causing authentication failures during database initialization.
   - **Root cause**: When the entrypoint script runs `su postgres -c "psql ..."` to create the ekaya user and database, it inherits `PGUSER=ekaya` and `PGDATABASE=ekaya_engine` from the Dockerfile environment. Since these don't exist yet, authentication fails.
   - **Solution**: Explicitly override environment variables in initialization commands by prefixing with `PGUSER=postgres PGDATABASE=postgres` to connect as the postgres superuser to the postgres database during setup.
   - **Files modified**: `deploy/quickstart/entrypoint.sh` (lines 35-36, 40-42, 69)

2. **Invalid encryption key**: The placeholder key `quickstart-demo-key-not-for-production` was not a valid base64-encoded 32-byte key, causing server startup failure.
   - **Root cause**: The PROJECT_CREDENTIALS_KEY must be exactly 32 bytes when base64-decoded for AES-256 encryption.
   - **Solution**: Generated a valid key using `openssl rand -base64 32` and replaced the placeholder value.
   - **File modified**: `deploy/quickstart/Dockerfile` (line 78)
   - **New key**: `iJbJ35cBhYXTxtzKE2tMI2QmxfcKh/QYpqIgl0NNoiI=`

**Notes for next session:**
- The quickstart image is now fully functional for evaluation and demo purposes
- First run takes approximately 10-15 seconds for Postgres initialization and migrations
- Container health check confirms server is responding correctly
- **Critical pattern for future work**: When setting `PGUSER`/`PGDATABASE` environment variables that are used by the application, initialization scripts must explicitly override them to connect as the postgres superuser during setup
- The encryption key is hardcoded for evaluation purposes only - DO NOT use this image pattern for production deployments

### Persistence Test [x]

**Status:** Complete

**What was done:**
- Started the quickstart container using the existing image
- Created a test project directly in the PostgreSQL database
- Stopped the container using `docker stop`
- Restarted the container using `docker start`
- Verified the project persisted with the same ID, name, and timestamp

**Verification results:**
- ✅ Project created: ID `eb8a5fbe-2528-4e2d-98cd-b7bf210d02fe`, name "Persistence Test Project"
- ✅ Container stopped gracefully
- ✅ Container restarted successfully and became healthy
- ✅ Project data persisted: Same ID, name, and creation timestamp after restart
- ✅ Docker volume `ekaya-data` correctly persisted PostgreSQL data across container lifecycle

**Test process:**
1. Started container with: `docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data engine-quickstart:local`
2. Created project: `docker exec <container> psql -U ekaya -d ekaya_engine -c "INSERT INTO engine_projects (id, name) VALUES (gen_random_uuid(), 'Persistence Test Project') RETURNING id, name;"`
3. Verified creation: Queried database and confirmed project exists
4. Stopped container: `docker stop <container>`
5. Restarted container: `docker start <container>`
6. Verified persistence: Queried database and confirmed same project with identical data

**Notes:**
- The Docker volume mount `-v ekaya-data:/var/lib/postgresql/data` successfully persists PostgreSQL data
- Data survives container stop/start cycles as expected
- The quickstart image is ready for users who need data persistence across sessions

### Health Check Test [x]

**Status:** Complete

**What was done:**
- Started quickstart container in background using `docker run -d`
- Tested `/ping` endpoint using curl - returned valid JSON with server status information
- Tested `/health` endpoint using curl - returned valid JSON with connection pool information
- Validated JSON structure using `jq` to ensure proper formatting

**Verification results:**
- ✅ `/ping` endpoint returns: `{"status":"ok","version":"73cf871-dirty","service":"ekaya-engine","go_version":"go1.25.5","hostname":"f0117f9dd6be","environment":"quickstart"}`
- ✅ `/health` endpoint returns: `{"status":"ok","connections":{...}}` with full connection pool details
- ✅ Both endpoints return well-formed JSON (validated with jq)
- ✅ Server responds correctly after container startup
- ✅ Container health check confirms healthy status

**Notes:**
- The `/ping` endpoint includes useful debugging information: version, go_version, hostname, and environment
- The `/health` endpoint provides connection pool metrics useful for monitoring
- Both endpoints are working as expected in the quickstart image

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
