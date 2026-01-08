# ============================================
# Quickstart Image - All-in-one for demo/trial
# ============================================
# Usage:
#   docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data ghcr.io/ekaya-inc/engine-quickstart:latest
#
# For LLM features (ontology extraction):
#   docker run -p 3443:3443 -e ANTHROPIC_API_KEY=sk-... -v ekaya-data:/var/lib/postgresql/data ghcr.io/ekaya-inc/engine-quickstart:latest

# UI Build stage
FROM node:22-alpine AS ui-builder
WORKDIR /app/ui
COPY ui/package*.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

# Go Build stage
FROM golang:1.25-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=ui-builder /app/ui/dist ./ui/dist

ARG VERSION=dev
ARG BUILD_TAGS=all_adapters

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -tags="${BUILD_TAGS}" \
    -ldflags="-w -s -X main.Version=${VERSION}" \
    -o ekaya-engine \
    main.go

# Runtime stage with Postgres + Redis
FROM debian:bookworm-slim

# Install Postgres 17, Redis, and iproute2 (for host.docker.internal setup on Linux)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gnupg2 \
    lsb-release \
    curl \
    ca-certificates \
    redis-server \
    iproute2 \
    && curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /usr/share/keyrings/postgresql-keyring.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/postgresql-keyring.gpg] http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends postgresql-17 \
    && rm -rf /var/lib/apt/lists/*

# Create app directory
WORKDIR /app

# Copy binary and UI from builders
COPY --from=go-builder /app/ekaya-engine /usr/local/bin/ekaya-engine
COPY --from=ui-builder /app/ui/dist /app/ui/dist

# Copy migrations (engine runs these on startup)
COPY migrations/ /app/migrations/

# Copy quickstart config and scripts
COPY deploy/quickstart/config.quickstart.yaml /app/config.yaml
COPY deploy/quickstart/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Environment variables
ENV PORT=3443 \
    BIND_ADDR=0.0.0.0 \
    PGHOST=localhost \
    PGPORT=5432 \
    PGUSER=ekaya \
    PGPASSWORD=quickstart \
    PGDATABASE=ekaya_engine \
    PGSSLMODE=disable \
    REDIS_HOST=localhost \
    REDIS_PORT=6379 \
    PROJECT_CREDENTIALS_KEY=iJbJ35cBhYXTxtzKE2tMI2QmxfcKh/QYpqIgl0NNoiI=

# Create postgres data directory mount point
VOLUME ["/var/lib/postgresql/data"]

EXPOSE 3443

HEALTHCHECK --interval=10s --timeout=3s --start-period=30s --retries=5 \
    CMD curl -f http://localhost:3443/health || exit 1

ENTRYPOINT ["/entrypoint.sh"]
