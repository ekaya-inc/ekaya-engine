# CSV Loading Demo

Load CSV files into PostgreSQL using Claude Code and Ekaya's MCP Server.

**Full walkthrough:** https://ekaya.ai/demos/csv-loading

## Quick Start

1. Start the Ekaya Quickstart container:
   ```bash
   docker run --pull always -d --name ekaya-quickstart \
     -p 3443:3443 \
     -p 5432:5432 \
     -v ekaya-data:/var/lib/postgresql/data \
     ghcr.io/ekaya-inc/ekaya-engine-quickstart:latest
   ```

2. Set up the Python environment:
   ```bash
   python3 -m venv venv
   source venv/bin/activate
   pip install -r requirements.txt
   ```

3. Configure credentials:
   ```bash
   cp .env.example .env
   source .env
   ```

4. Update `.mcp.json` with your Ekaya MCP Server URL (get the project-id from the Ekaya UI)

5. Add your CSV files to the `data/` directory

6. Launch Claude Code:
   ```bash
   claude
   ```

7. Follow prompts to analyze CSVs, create schema, and load data
