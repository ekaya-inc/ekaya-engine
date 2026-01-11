# UI Build stage
FROM node:22-alpine AS ui-builder

# Set working directory for UI
WORKDIR /app/ui

# Copy UI package files
COPY ui/package*.json ./

# Install UI dependencies
RUN npm ci

# Copy UI source code
COPY ui/ ./

# Build the UI
RUN npm run build

# Go Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./

# Download dependencies (if any)
RUN go mod download

# Copy source code
COPY . .

# Copy built UI from previous stage
COPY --from=ui-builder /app/ui/dist ./ui/dist

# Build arguments for version and adapter selection
ARG VERSION=dev
ARG BUILD_TAGS=all_adapters

# Build the binary with optimizations for Cloud Run
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -tags="${BUILD_TAGS}" \
    -ldflags="-w -s -X main.Version=${VERSION}" \
    -o ekaya-engine \
    main.go

# Final stage - minimal image optimized for Cloud Run
FROM alpine:3.22

# Install ca-certificates for HTTPS calls and wget for health checks
RUN apk --no-cache add ca-certificates wget

# Create non-root user (Cloud Run best practice)
RUN addgroup -g 1000 -S ekaya && \
    adduser -u 1000 -S ekaya -G ekaya

# Copy binary from builder
COPY --from=builder /app/ekaya-engine /usr/local/bin/ekaya-engine

# Switch to non-root user
USER ekaya

# Expose port (Cloud Run uses PORT env var, default 3443)
EXPOSE 3443

# Health check for Cloud Run (uses /health endpoint)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${PORT:-3443}/health || exit 1

# Cloud Run optimizations via environment variables
ENV PORT=3443 \
    BIND_ADDR=0.0.0.0 \
    GOMAXPROCS=2

# Run the binary
ENTRYPOINT ["ekaya-engine"]
