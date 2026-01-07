# TLS Certificates

## Why TLS?

Web Crypto API (used for OAuth PKCE) only works in "secure contexts" - either HTTPS or localhost. When testing with custom hostnames like `engine.local`, you need TLS certificates.

## Generate Self-Signed Certificate

```bash
openssl req -x509 -newkey rsa:4096 -keyout certs/key.pem -out certs/cert.pem -days 365 -nodes \
  -subj "/CN=engine.local" \
  -addext "subjectAltName=DNS:engine.local,DNS:localhost"
```

## Configure Local Hostname

Add to `/etc/hosts`:
```
127.0.0.1 engine.local
```

## Enable TLS in config.yaml

```yaml
tls_cert_path: "./certs/cert.pem"
tls_key_path: "./certs/key.pem"
```

## Browser Warning

Self-signed certificates trigger browser warnings. Click through to proceed, or import the certificate into your system trust store.

## Production

For production deployments, use real certificates (e.g., Let's Encrypt). Cloud Run handles TLS termination automatically.
