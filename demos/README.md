# Demos Overview

These demos build upon each other and walk you through how to use different aspects of Ekaya.

## Start Here

Clone this project and come to this directory.

```bash
git clone git@github.com:ekaya-inc/ekaya-engine.git
cd ekaya-engine/demos
```

Launch the Ekaya Quickstart docker image (built from `ekaya-engine/deploy/quickstart`):

```bash
docker run -p 3443:3443 \
  -v ekaya-data:/var/lib/postgresql/data \
  ghcr.io/ekaya-inc/ekaya-engine-quickstart:latest
```

Open this URL in a browser:

Local Ekaya Quickstart docker instance: [http://localhost:3443/]

## Demos

### Data Engineering

Use Ekaya to load data from CSVs into an empty database.

You will start with a pre-created set of CSVs (or bring your own) and end with a loaded database.

Follow instructions in this [README.md](./01__Data_Engineering/README.md)
