# CLAUDE.md

This is an Ekaya demo project for loading CSV files into a PostgreSQL database.

## Project Overview

This demo walks users through using Claude Code with Ekaya's MCP Server to:
1. Analyze CSV file schemas
2. Design and create database tables
3. Generate Python ETL scripts to load the data

## Environment Setup

The project uses a `.env` file for database credentials. The user should:
1. Copy `.env.example` to `.env`
2. Update values if using non-default settings

Default credentials connect to the PostgreSQL database running inside the Ekaya Quickstart container.

## Database Connection

```
Host: localhost
Port: 5432
Database: demo_csvload
User: demo_user
Password: demo_password
```

These are exposed by the Ekaya Quickstart Docker container.

## MCP Server

The `.mcp.json` file must be updated with the user's Ekaya MCP Server URL. The URL format is:
```
http://localhost:3443/mcp/{project-id}
```

The project-id is obtained from the Ekaya UI after creating a project.

## Python Environment

Activate the virtual environment before running scripts:
```bash
source venv/bin/activate
source .env
```

## File Structure

- `data/` - CSV files to load (user provides these)
- `load_csv.py` - Example ETL script using environment variables
- `requirements.txt` - Python dependencies

## Workflow

When the user asks to load CSV data:
1. First examine the CSV files in `data/` to understand their schema
2. Use the Ekaya MCP tools to query existing schema if needed
3. Create appropriate tables in the `demo_csvload` database
4. Generate or modify the Python script to load the data
5. Run the script to load the CSV files

## Database Commands

Connect to the database:
```bash
source .env
psql -h $PGHOST -p $PGPORT -U $PGUSER -d $PGDATABASE
```
