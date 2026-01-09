# ekaya-engine

Regional controller for Ekaya platform with clean architecture Go backend and React frontend.

## Development

### Prerequisites

- Go 1.21+
- Node.js 18+
- PostgreSQL database
- Redis (optional, for caching)

### Setup

```bash
# Install dependencies
cd ui && npm install && cd ..

# Configure authentication (choose one)
make setup-auth-dev    # Recommended: uses auth.dev.ekaya.ai
make setup-auth-local  # Requires ekaya-central emulator running locally

# Set required environment variables
export PGPASSWORD=your_db_password
export PROJECT_CREDENTIALS_KEY=$(openssl rand -base64 32)

# Start development (two terminals)
make dev-server  # Terminal 1: Go API with auto-reload
make dev-ui      # Terminal 2: UI with hot reload
```

### Configuration

Server configuration is auto-derived from defaults for local development:

| Variable | Default | When to set |
|----------|---------|-------------|
| `PORT` | 3443 | Different local port |
| `BIND_ADDR` | 127.0.0.1 | Docker/external access: `0.0.0.0` |
| `BASE_URL` | http://localhost:$PORT | Behind load balancer or internal server |

Database and Redis use standard PG* and REDIS_* environment variables.

### Quality Checks

```bash
make check  # Format, lint, typecheck, unit tests, integration tests (backend + frontend)
```

**Note:** Integration tests require Docker to be running (spins up a PostgreSQL container).

## Deployment

Push to `main` deploys to dev. Push to `prod` deploys to production.
