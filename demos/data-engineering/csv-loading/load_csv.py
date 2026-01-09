#!/usr/bin/env python3
"""
Example ETL script to load CSV files into PostgreSQL.

Usage:
    source .env
    python load_csv.py
"""

import os
import pandas as pd
from sqlalchemy import create_engine


def get_connection_string():
    """Build connection string from environment variables."""
    host = os.environ.get("PGHOST", "localhost")
    port = os.environ.get("PGPORT", "5432")
    database = os.environ.get("PGDATABASE", "demo_csvload")
    user = os.environ.get("PGUSER", "demo_user")
    password = os.environ.get("PGPASSWORD", "demo_password")
    return f"postgresql://{user}:{password}@{host}:{port}/{database}"


def load_csv_to_table(csv_path: str, table_name: str, engine):
    """Load a CSV file into a database table."""
    print(f"Loading {csv_path} into {table_name}...")
    df = pd.read_csv(csv_path)
    df.to_sql(table_name, engine, if_exists="replace", index=False)
    print(f"  Loaded {len(df)} rows")


def main():
    engine = create_engine(get_connection_string())

    # Example: Load all CSV files from data/ directory
    data_dir = os.path.join(os.path.dirname(__file__), "data")

    if not os.path.exists(data_dir):
        print(f"No data directory found at {data_dir}")
        print("Create the directory and add CSV files to load.")
        return

    csv_files = [f for f in os.listdir(data_dir) if f.endswith(".csv")]

    if not csv_files:
        print(f"No CSV files found in {data_dir}")
        return

    for csv_file in csv_files:
        csv_path = os.path.join(data_dir, csv_file)
        table_name = os.path.splitext(csv_file)[0].lower().replace("-", "_")
        load_csv_to_table(csv_path, table_name, engine)

    print("Done!")


if __name__ == "__main__":
    main()
