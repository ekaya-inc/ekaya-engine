# Docker Deployment

Build a self-contained Docker image with your config and TLS certs baked in for production deployment.

## TLS and HTTPS

Ekaya Engine uses OAuth 2.1 with PKCE, which requires the browser's Web Crypto API.

Web Crypto only works in secure contexts:

- `http://localhost:3443` -- works (browsers treat localhost as secure)
- `https://your.domain.com` -- works with a valid TLS certificate
- `http://your.domain.com` -- WILL NOT WORK (browser blocks Web Crypto API)

Self-signed certificates work if users' browsers trust the root CA.

If TLS terminates at a reverse proxy or load balancer, skip `tls_cert_path` / `tls_key_path` and just set `base_url` to your public HTTPS URL.

## Setup

### 1. Create config.yaml

```bash
cp ../../config.yaml.example config.yaml
```

Edit `config.yaml`. The key fields for deployment:

**Secrets** -- generate production values with `openssl rand -base64 32`:

```yaml
project_credentials_key: "..."   # encrypts datasource credentials; CANNOT be changed after storing credentials
oauth_session_secret: "..."      # signs OAuth session cookies; all servers in a cluster must share this value
```

**TLS** -- **REQUIRED**

This is the URL that everyone will use to access this server:

```yaml
base_url: "https://data.yourcompany.com:3443"
```

The TLS Certificates for the base_url:

```yaml
tls_cert_path: "/app/certs/cert.pem"    # path inside the container
tls_key_path: "/app/certs/key.pem"      # path inside the container
```

### 2. Place TLS certificates

```bash
cp ./certs/cert.pem certs/cert.pem
cp ./certs/key.pem certs/key.pem
```

### 3. Build the image

```bash
docker build -t ekaya-engine-deploy .
```

The Dockerfile layers your `config.yaml` and `certs/` onto `ghcr.io/ekaya-inc/ekaya-engine:latest`. The resulting image is portable -- push it to a private registry and deploy anywhere.

### 4. Run it

Choose one of three PostgreSQL variants below. All use the same image built in step 3. Migrations run automatically on startup.

## PostgreSQL Variants

### A. Docker Compose (recommended for most deployments)

Runs Ekaya Engine + PostgreSQL 17 in separate containers. Data persists in a named Docker volume.

Set `pg_user: "ekaya"` and `pg_database: "ekaya_engine"` in config.yaml to match the compose defaults, then:

```bash
PGPASSWORD=your-password docker-compose up -d
```

The `PGPASSWORD` env var sets the PostgreSQL container's password and overrides `PGPASSWORD` in the Ekaya Engine container. `PGHOST` is overridden to `postgres` (the compose service name).

### B. Standalone PostgreSQL

Point config.yaml at your own PostgreSQL server (managed, on-prem, etc.) by setting `pg_host`, `pg_port`, `pg_user`, `pg_password`, `pg_database`, and `pg_sslmode` under `engine_database`.

You only need to run the Ekaya Engine.

```bash
docker run -p 3443:3443 ekaya-engine-deploy
```

Requirements:

- PostgreSQL >= 14
- A database and role for Ekaya Engine see [../../README.md](../../README.md) for details.

Note: migrations run automatically when the server runs.

### C. Simple Single-Instance

The `ekaya-engine-deploy` defaults to the Postgres instance running inside its container.

This configuration does not require a separate PostgreSQL instance running but is not recommended for production use and does not support multiple Ekaya Engine instances for capacity or failover.
