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

### 2. Update Server Startup

**File:** `main.go`

Modify server startup logic (around line 471):
```go
if cfg.TLSCertPath != "" && cfg.TLSKeyPath != "" {
    logger.Info("Starting HTTPS server",
        zap.String("addr", cfg.BindAddr+":"+cfg.Port),
        zap.String("cert", cfg.TLSCertPath))
    err = server.ListenAndServeTLS(cfg.TLSCertPath, cfg.TLSKeyPath)
} else {
    logger.Info("Starting HTTP server",
        zap.String("addr", cfg.BindAddr+":"+cfg.Port))
    err = server.ListenAndServe()
}
```

### 3. Update Config Templates

**Files:** `config/config.*.yaml.template`

Add TLS section to templates:
```yaml
# TLS Configuration (optional)
# For local development, leave empty to use HTTP
# For production, provide paths to cert and key files
# tls_cert_path: "/path/to/cert.pem"
# tls_key_path: "/path/to/key.pem"
```

### 4. Validate TLS Config

**File:** `pkg/config/config.go` (in `Load()` or validation function)

Add validation:
- If one of cert/key is provided, both must be provided
- If provided, verify files exist and are readable
- Log warning if BaseURL is HTTPS but no TLS config (assumes reverse proxy)

```go
func (c *Config) validateTLS() error {
    certSet := c.TLSCertPath != ""
    keySet := c.TLSKeyPath != ""

    if certSet != keySet {
        return fmt.Errorf("both tls_cert_path and tls_key_path must be provided together")
    }

    if certSet {
        if _, err := os.Stat(c.TLSCertPath); err != nil {
            return fmt.Errorf("TLS cert file not readable: %w", err)
        }
        if _, err := os.Stat(c.TLSKeyPath); err != nil {
            return fmt.Errorf("TLS key file not readable: %w", err)
        }
    }

    return nil
}
```

## Testing

### Local Development Testing

1. Generate self-signed certificate:
   ```bash
   openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes \
     -subj "/CN=engine.local"
   ```

2. Add to `/etc/hosts`:
   ```
   127.0.0.1 engine.local
   ```

3. Configure ekaya-engine:
   ```yaml
   base_url: "https://engine.local:3443"
   tls_cert_path: "./cert.pem"
   tls_key_path: "./key.pem"
   ```

4. Trust the certificate in your browser/system, or use Chrome flags for local dev

5. Test the OAuth flow works end-to-end

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
