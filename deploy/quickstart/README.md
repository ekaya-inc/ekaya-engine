# Ekaya Engine Quickstart Image

All-in-one Docker image for running Ekaya Engine and PostgreSQL locally.

To set your own base URL with HTTPS (required for OAuth) with multiple deployment options, see [READMD.md](../docker/README.md).

## Quick Start

From the repo root:

```bash
make run-quickstart
```

Or run directly:

```bash
docker run -p 3443:3443 -v ekaya-data:/var/lib/postgresql/data ghcr.io/ekaya-inc/ekaya-engine-quickstart:latest
```

Then open your browser to [http://localhost:3443](http://localhost:3443).

## Data Persistence

The `-v ekaya-data:/var/lib/postgresql/data` mount persists your data between container restarts.

To start fresh, remove the volume:

```bash
docker volume rm ekaya-data
```
