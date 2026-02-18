# Build from Source

## Using make

Build release binaries for all platforms:

```bash
make build-release
```

This builds the frontend UI, cross-compiles for macOS (Intel + Apple Silicon), Linux (x86_64 + ARM64), and Windows (x86_64), and outputs archives with checksums to `dist/`.

## Manual build

Build the frontend first, then the Go binary:

```bash
cd ui && npm install && npm run build && cd ..
go build -tags=all_adapters -ldflags="-X main.Version=$(git describe --tags --always --dirty)" -o ekaya-engine .
```

Cross-compile for other platforms by setting `GOOS` and `GOARCH`:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -tags=all_adapters -trimpath \
  -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty)" \
  -o ekaya-engine .
```

## Configure

Ekaya Engine looks for `config.yaml` in the current working directory first, then falls back to `~/.ekaya/config.yaml`. Copy the example and edit it:

```bash
cp config.yaml.example config.yaml
```

At minimum, set `project_credentials_key` and `oauth_session_secret` to secure values:

```bash
openssl rand -base64 32  # generate a secret
```

See `config.yaml.example` for all options with detailed comments. Key settings for production:

| Setting | Env Var | Purpose |
|---------|---------|---------|
| `base_url` | `BASE_URL` | Public URL of your deployment (auto-derived if not set) |
| `tls_cert_path` | `TLS_CERT_PATH` | Path to TLS certificate (PEM) |
| `tls_key_path` | `TLS_KEY_PATH` | Path to TLS private key (PEM) |
| `bind_addr` | `BIND_ADDR` | Listen address (default `127.0.0.1`; set to `0.0.0.0` for all interfaces) |
| `port` | `PORT` | Listen port (default `3443`) |
| `project_credentials_key` | `PROJECT_CREDENTIALS_KEY` | Encrypts stored datasource credentials. **Cannot be changed after storing credentials.** |
| `oauth_session_secret` | `OAUTH_SESSION_SECRET` | Signs OAuth session cookies. All servers in a cluster must share the same value. |

All settings can be overridden with environment variables.

## TLS certificates

Ekaya Engine uses OAuth 2.1 with PKCE for authentication. PKCE requires the browser's Web Crypto API, which **only works in secure contexts** (HTTPS or `localhost`). This means:

- `http://localhost:3443` -- works (browsers treat localhost as secure)
- `https://your.domain.com` -- works with a valid TLS certificate
- `http://your.domain.com` -- **will not work** (browser blocks Web Crypto API)

For non-localhost deployments, provide TLS certificates in `config.yaml`:

```yaml
tls_cert_path: "/path/to/cert.pem"
tls_key_path: "/path/to/key.pem"
base_url: "https://data.yourcompany.com:3443"
```

Self-signed certificates work if users' browsers trust your root CA. If TLS is terminated by a reverse proxy or load balancer, leave `tls_cert_path`/`tls_key_path` unset and set `base_url` to your public HTTPS URL.

## Run

```bash
./ekaya-engine
```

Migrations run automatically on startup. The server logs its listen address and base URL on start.
