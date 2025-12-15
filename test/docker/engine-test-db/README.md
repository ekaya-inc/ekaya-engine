# Ekaya Engine Test Database Container

Custom PostgreSQL 18 Docker image with pre-loaded test database for fast integration testing.

## Quick Start

### Build and Push (One-time Setup)

```bash
# Build locally
make build-test-image

# Push to dev registry (requires gcloud auth)
make push-test-image
```

### Pull from Registry

```bash
# Pull the image (CI/CD and local development)
make pull-test-image
```

## Database Details

- **User:** `ekaya`
- **Password:** `test_password`
- **PostgreSQL Version:** 18-alpine

### Databases

| Database | Purpose |
|----------|---------|
| `test_data` | Pre-loaded with 38 tables (test datasource) |
| `empty_db` | Empty database for empty datasource tests |
| `ekaya_engine_test` | Empty database for engine integration tests (migrations applied at runtime) |

## Key Features

- **Fast startup:** Database is pre-initialized during build
- **Small size:** 259KB of data (27x smaller than original)
- **Realistic schema:** Real-world database structure with 38 tables
- **Test-ready data:** 7 tables with 100 rows each, others with original row counts (≤95 rows)

## Tables with 100 Rows (Trimmed)

1. events
2. marketing_touches
3. billing_activity_messages
4. billing_engagements
5. offers
6. billing_transactions
7. engagement_payment_intents

## Connection String

```
postgresql://ekaya:test_password@localhost:5432/test_data
```

## Registry Image

The image is stored in Google Artifact Registry:
```
us-central1-docker.pkg.dev/ekaya-dev-shared/ekaya-dev-containers/ekaya-engine-test-image:latest
```

## Files

- `Dockerfile` - Image definition
- `test_data.dump` - Pre-trimmed database dump (259KB)
- `README.md` - This file
