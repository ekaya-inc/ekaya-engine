# PLAN: CSV Loading Demo

Demo project at `demos/data-engineering/csv-loading/` that showcases using Claude Code with Ekaya's MCP Server to load CSV files into PostgreSQL.

## Context

This demo is part of the Ekaya public launch. Users clone the repo, start the Quickstart container, and use Claude Code to analyze CSVs, create schema, and load data.

## Completed

- [x] Project structure created
- [x] `CLAUDE.md` - Instructions for Claude Code
- [x] `README.md` - Quick start guide with link to demo walkthrough
- [x] `.env.example` - Database credentials template
- [x] `.mcp.json` - MCP server config template
- [x] `requirements.txt` - Python dependencies
- [x] `load_csv.py` - Example ETL script
- [x] `data/` directory with `.gitkeep`
- [x] `.gitignore` - Ignores venv, .env, CSV files

## Remaining Tasks

### HUMAN: Provide CSV Files

Add sample CSV files to `demos/data-engineering/csv-loading/data/`. These should be:
- Representative of a real-world dataset (e.g., e-commerce orders, customer data)
- Small enough to load quickly but interesting enough to demonstrate schema analysis
- Multiple files with relationships between them (to show FK detection)

Suggested dataset ideas:
- E-commerce: customers, orders, order_items, products
- SaaS: users, subscriptions, invoices
- Events: events, attendees, venues

### HUMAN: Create Demo Walkthrough Page

Create the walkthrough page at `https://ekaya.ai/demos/csv-loading` with:
- Screenshots of each step
- Expected Claude Code prompts and responses
- Troubleshooting section

### HUMAN: Update Docker Image Details

Update `README.md` with the actual:
- Docker image name (currently `ekaya/quickstart:latest`)
- Port mappings if different from defaults
- Any additional docker run flags needed

### HUMAN: Verify Demo User Credentials

Ensure the Quickstart container creates:
- Database: `demo_csvload`
- User: `demo_user` with password `demo_password`
- Appropriate grants for the demo user

Or update `.env.example` with the actual credentials.

### Claude Code: Test the Demo Flow

Once CSVs are provided, test the full flow:
1. Start Quickstart container
2. Set up Python venv
3. Connect Claude Code via MCP
4. Analyze CSV schemas
5. Create tables
6. Load data
7. Verify loaded data

### Claude Code: Refine load_csv.py

After testing, the `load_csv.py` script may need:
- Data type mappings specific to the CSV files
- Error handling for common issues
- Progress reporting for larger files

### HUMAN: Finalize Demo URL

Update `README.md` link if the final URL differs from `https://ekaya.ai/demos/csv-loading`.

## Dependencies

- Ekaya Quickstart Docker image must be published
- ekaya.ai website must have the demo walkthrough page
