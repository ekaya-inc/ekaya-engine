# PLAN: Add TLS Support to ekaya-engine

## Context

ekaya-engine currently only works properly on `localhost` because:

1. **Web Crypto API restriction**: `crypto.subtle` (used for PKCE) only works in "secure contexts" (HTTPS or localhost). Custom hostnames over HTTP fail with "Cannot read properties of undefined (reading 'digest')".

2. **No TLS support**: ekaya-engine only supports HTTP. Customers cannot deploy with HTTPS without a reverse proxy.

**Related:** See `PLAN-central-use-url.md` for the ekaya-central changes needed to validate OAuth redirect_uri for custom domains.

## What Already Works

| Component | Status | Notes |
|-----------|--------|-------|
| Frontend redirect_uri | ✅ | Uses `window.location.origin` - already dynamic |
| Cookie secure flag | ✅ | Auto-detected from BaseURL scheme |
| Well-known TLS detection | ✅ | Checks `r.TLS` and `X-Forwarded-Proto` |
| MCP OAuth flow | ✅ | Accepts redirect_uri per-request |

## Implementation

### 1. Add TLS Config Options ✅ COMPLETE

**Files Modified:**
- `pkg/config/config.go` - Added TLS config fields and validation
- `pkg/config/config_test.go` - Added comprehensive TLS validation tests

**Implementation Details:**

Added two optional TLS config fields to `Config` struct:
```go
// TLS configuration (optional - if both provided, server uses HTTPS)
TLSCertPath string `yaml:"tls_cert_path" env:"TLS_CERT_PATH" env-default:""`
TLSKeyPath  string `yaml:"tls_key_path" env:"TLS_KEY_PATH" env-default:""`
```

Added `validateTLS()` method called during `Load()`:
- Validates both cert and key are provided together (or both empty)
- Verifies files exist and are readable using `os.Stat()`
- Returns clear error messages for configuration issues
- Note: Actual certificate validity is checked by Go's `tls.LoadX509KeyPair()` at server startup

**Test Coverage:**
- No TLS config (both empty) - passes ✅
- Both cert and key provided - passes ✅
- Only cert provided - fails with "both must be provided" error ✅
- Only key provided - fails with "both must be provided" error ✅
- Cert file not found - fails with "cert file not readable" error ✅
- Key file not found - fails with "key file not readable" error ✅
- TLS config from env vars - passes ✅

**Patterns for Future Tasks:**
- Config validation happens in `Load()` by calling specialized validation methods
- Tests use `t.TempDir()` for isolated test environments
- Tests use `t.Setenv()` for environment variable isolation
- File permission errors (beyond existence) are deferred to server startup

### 2. Update Server Startup ✅ COMPLETE

**File Modified:** `main.go:468-482`

**Implementation Details:**

Modified server startup to conditionally enable TLS:
- Added conditional check: if both `cfg.TLSCertPath` and `cfg.TLSKeyPath` are non-empty, use HTTPS
- HTTPS path: calls `server.ListenAndServeTLS(cfg.TLSCertPath, cfg.TLSKeyPath)` and logs "Starting HTTPS server" with cert path
- HTTP path: calls `server.ListenAndServe()` and logs "Starting HTTP server"
- Both paths log the bind address and version
- Error handling unchanged: checks for `http.ErrServerClosed` before calling `logger.Fatal()`

**Key Decision:**
- Simple boolean logic based on config presence (validated in task 1)
- No separate TLS enable flag - presence of cert/key paths implies TLS intent
- Falls back to HTTP if paths are empty (for local development and CI)

**Testing:**
- Build verification: ✅ Compiles successfully (`make check` passes)
- Unit tests: ✅ All existing tests pass
- Manual HTTPS testing: Pending (requires actual cert/key files - see Testing section below)

### 3. Update Config Templates ✅ COMPLETE

**Files Modified:**
- `config/config.dev.yaml`
- `config/config.local.yaml`
- `config/config.prod.yaml`

**Implementation Details:**

Added TLS configuration section to all three config files with:
- Clear comments explaining when to use TLS (leave empty for local dev, provide paths for production)
- Reference to Web Crypto API requirement (secure contexts needed for OAuth PKCE)
- Commented-out example paths: `tls_cert_path` and `tls_key_path`
- Consistent formatting across all three files
- Section placed after `cookie_domain` and before `auth:` section for logical grouping

**Note:** The actual files are `config.*.yaml` (not `.yaml.template`). These files serve as templates that users copy to `config.yaml` via the `make setup-auth-*` commands.

**What Future Sessions Need to Know:**
- The TLS config is optional by design - empty values default to HTTP mode
- Comments emphasize that localhost works with HTTP (Web Crypto API allows it)
- Production deployments with custom domains will need to uncomment and set these values
- The validation from Task 1 ensures both cert and key are provided together or both empty

### 4. Validate TLS Config ✅ COMPLETE

**File Modified:** `pkg/config/config.go:183-203`

**Implementation Details:**

Added `validateTLS()` method called during `Load()`:
- Validates both cert and key are provided together (or both empty)
- Verifies files exist and are readable using `os.Stat()`
- Returns clear error messages for configuration issues
- Note: Actual certificate validity is checked by Go's `tls.LoadX509KeyPair()` at server startup

**Test Coverage:**
- No TLS config (both empty) - passes ✅
- Both cert and key provided - passes ✅
- Only cert provided - fails with "both must be provided" error ✅
- Only key provided - fails with "both must be provided" error ✅
- Cert file not found - fails with "cert file not readable" error ✅
- Key file not found - fails with "key file not readable" error ✅
- TLS config from env vars - passes ✅

**Patterns for Future Tasks:**
- Config validation happens in `Load()` by calling specialized validation methods
- Tests use `t.TempDir()` for isolated test environments
- Tests use `t.Setenv()` for environment variable isolation
- File permission errors (beyond existence) are deferred to server startup

## Testing

### 5. Local Development Testing ✅ COMPLETE

**Testing Completed: 2026-01-07**

Successfully tested HTTPS functionality with self-signed certificates:

1. **Generated self-signed certificate** ✅
   ```bash
   openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes \
     -subj "/CN=engine.local"
   ```
   - Created `cert.pem` (1.8KB) and `key.pem` (3.2KB)
   - Certificate valid for `CN=engine.local`, expires 2027-01-07

2. **Created TLS test configuration** ✅
   - Created `config.tls-test.yaml` with TLS enabled
   - Set `base_url: "https://engine.local:3444"` (or any free port)
   - Set `tls_cert_path: "./cert.pem"` and `tls_key_path: "./key.pem"`

3. **Server startup verified** ✅
   - Server logs: `INFO Starting HTTPS server {"addr": "127.0.0.1:3450", "version": "dev", "cert": "./cert.pem"}`
   - Successfully binds to port and loads certificate files
   - No errors during TLS initialization

4. **HTTPS endpoints verified** ✅
   - `/health` endpoint: Returns valid JSON over HTTPS
   - `/ping` endpoint: Returns valid JSON over HTTPS
   - Tested with: `curl -k https://127.0.0.1:3450/health`

**Manual Steps Required:**

To test with `engine.local` hostname (optional):
```bash
# Add to /etc/hosts (requires sudo)
echo "127.0.0.1 engine.local" | sudo tee -a /etc/hosts

# Test with hostname
curl -k https://engine.local:3450/ping
```

**Note:** For testing OAuth flow end-to-end with Web Crypto API (PKCE), the certificate must be trusted in the browser or use browser developer mode flags

### Integration Tests

- Test HTTP mode still works (no TLS config)
- Test HTTPS mode with valid certs
- Test config validation rejects partial TLS config (only cert or only key)
- Test server fails gracefully if cert files don't exist

## Deployment Options for Customers

After this work, customers have three deployment options:

| Option | Complexity | Use Case |
|--------|------------|----------|
| Direct TLS | Low | Single server, own domain, Let's Encrypt |
| Reverse Proxy | Medium | Existing infrastructure, load balancing |
| localhost only | Minimal | Local development only |

## Files to Modify

- `pkg/config/config.go` - Add TLS config fields and validation
- `main.go` - Add TLS server startup logic
- `config/config.*.yaml.template` - Add TLS config examples
