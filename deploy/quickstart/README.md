# Ekaya Engine Quickstart Image

All-in-one Docker image for running Ekaya Engine locally. Authenticates through production ekaya-central (us.ekaya.ai).

## Quick Start

```bash
docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data ghcr.io/ekaya-inc/engine-quickstart:latest
```

Open http://localhost:3443

## With LLM Features

For ontology extraction and AI features, provide your Anthropic API key:

```bash
docker run -p 3443:3443 \
  -e ANTHROPIC_API_KEY=sk-ant-... \
  -v ekaya-data:/var/lib/postgresql/data \
  ghcr.io/ekaya-inc/engine-quickstart:latest
```

## Data Persistence

The `-v ekaya-data:/var/lib/postgresql/data` mount persists your data between container restarts.

To start fresh, remove the volume:

```bash
docker volume rm ekaya-data
```

## What's Included

- PostgreSQL 17
- Redis 7
- Ekaya Engine with UI

## Not for Production

This image is for evaluation only:
- Uses static encryption key
- Single-container architecture

For production, see the main deployment documentation.
