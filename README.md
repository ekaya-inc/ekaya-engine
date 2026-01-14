# ekaya-engine

Regional controller for Ekaya platform with clean architecture Go backend and React frontend.

## Development

### Setup

```bash
# Install dependencies
cd ui && npm install && cd ..

# Copy configuration template (includes working defaults for quickstart)
cp config.yaml.example config.yaml

# Start development (two terminals)
make dev-server  # Terminal 1: Go API with auto-reload
make dev-ui      # Terminal 2: UI with hot reload
```

For production, generate secure secrets in `config.yaml`:
```bash
openssl rand -base64 32  # For project_credentials_key and oauth_session_secret
```

### Quality Checks

```bash
make check  # Format, lint, typecheck, tests (requires Docker for integration tests)
```

## Deployment

Push to `main` deploys to dev. Push to `prod` deploys to production.
