# Docker Deployment

Build a Docker image with your config and TLS certs baked in for production deployment.

The image is based on the quickstart image (`ghcr.io/ekaya-inc/ekaya-engine-quickstart`), which bundles PostgreSQL 17 and Ekaya Engine. The bundled PostgreSQL is used by default, but you can point `config.yaml` at an external PostgreSQL instance instead.

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

**TLS** -- **REQUIRED** for non-localhost deployments.

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
cp /path/to/cert.pem certs/cert.pem
cp /path/to/key.pem certs/key.pem
```

### 3. Build the image

```bash
docker build -t ekaya-engine-deploy .
```

The Dockerfile layers your `config.yaml` and `certs/` onto the quickstart image. The resulting image is portable -- push it to a private registry and deploy anywhere.

### 4. Run it

```bash
docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data ekaya-engine-deploy
```

Migrations run automatically on startup. Data persists in the `ekaya-data` volume.

## Using an External PostgreSQL

To use your own PostgreSQL instance instead of the bundled one, set the `engine_database` fields in `config.yaml`:

```yaml
engine_database:
  pg_host: "your-postgres-host"
  pg_port: 5432
  pg_user: "ekaya"
  pg_password: "your-password"
  pg_database: "ekaya_engine"
  pg_sslmode: "require"
```

Requirements:

- PostgreSQL >= 14
- A database and role for Ekaya Engine (see [../../README.md](../../README.md) for details)

## Docker Compose

For deployments with a separate PostgreSQL container (instead of the bundled one), use docker-compose:

Set `pg_user: "ekaya"` and `pg_database: "ekaya_engine"` in config.yaml to match the compose defaults, then:

```bash
PGPASSWORD=your-password docker-compose up -d
```

The `PGPASSWORD` env var sets the PostgreSQL container's password and overrides `PGPASSWORD` in the Ekaya Engine container. `PGHOST` is overridden to `postgres` (the compose service name).
