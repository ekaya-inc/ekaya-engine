# PLAN: ETL Structured Data Loaders (Phase 1)

**Status:** Complete
**Parent:** `MASTER-PLAN-app-etl.md`
**Created:** 2026-03-09

---

## Goal

Build two ETL applets — **CSV → SQL** and **XLSX → SQL** — plus the shared infrastructure they both use. When complete, a user can install either applet, configure a watch directory, and drop files in to have them automatically loaded into their project's datasource as SQL tables.

---

## What Gets Built

### Shared ETL Infrastructure

All ETL applets share this foundation:

1. **Directory watcher service** — Monitors configured directory for new/changed files using `fsnotify`. Dispatches to the appropriate applet based on file extension. Runs as a background goroutine per project that has any ETL applet installed.

2. **Schema inference engine** — Samples N rows (configurable, default 100) from parsed data. For each column, tries parsing as: boolean → integer → float → date → timestamp → text (first match wins). Tracks nullability. Outputs a `[]InferredColumn{Name, SQLType, Nullable}`.

3. **Ontology matcher** — Given inferred columns and a table name candidate:
   - **No ontology:** Uses inferred schema as-is, creates new table
   - **Ontology available:** Queries ontology for tables/columns with similar names. Uses column classifications (semantic types like "currency", "email", "identifier") to improve type mapping. Proposes mapping to existing table or confirms new table creation. If matched, validates inferred types against ontology types and prefers ontology.

4. **Table creator** — Generates `CREATE TABLE IF NOT EXISTS` DDL from final schema (inferred or ontology-matched). Executes via `QueryExecutor.Execute()`. Handles table naming: sanitizes filename → snake_case → deduplicates if table exists.

5. **Row loader** — Bulk inserts parsed rows via `QueryExecutor.ExecuteWithParams()` with parameterized `INSERT` statements. Configurable batch size (default 500). Tracks success/failure counts. Handles type coercion errors per-row (logs and skips bad rows, does not abort entire load).

6. **Load status tracker** — Records each load operation: file path, table name, rows attempted, rows loaded, rows skipped, errors, started_at, completed_at. Stored in engine's internal database (not the project datasource). Queryable by the applet's UI/API.

7. **ETL settings model** — Per-applet settings stored in `engine_installed_apps.settings` JSONB:
   ```json
   {
     "watch_directory": "/data/imports",
     "auto_create_tables": true,
     "batch_size": 500,
     "sample_rows": 100,
     "on_conflict": "append",
     "use_ontology": true
   }
   ```

### CSV/TSV Applet

**App ID:** `etl-csv`
**Library:** stdlib `encoding/csv` (no external dependency)

- Registers as installable app with `InstalledAppService`
- File extensions: `.csv`, `.tsv`, `.txt` (with delimiter detection)
- Delimiter auto-detection: tries comma, tab, semicolon, pipe — picks the one that produces consistent column counts
- Encoding detection: UTF-8 assumed, falls back to Latin-1 for common encoding errors
- Header detection: first row treated as headers by default (configurable)
- Hands parsed rows to shared schema inference → ontology matcher → table creator → row loader pipeline

### XLSX Applet

**App ID:** `etl-excel`
**Library:** `github.com/xuri/excelize/v2` (BSD-3, ~19.7K stars)

- Registers as installable app with `InstalledAppService`
- File extensions: `.xlsx`, `.xlsm`, `.xltx`
- Multi-sheet handling: each sheet → separate table (sheet name becomes table name suffix)
- Streaming reader for large files (`excelize.OpenReader` + `Rows` iterator)
- Handles merged cells, date serial numbers, formula cells (uses computed value)
- Skips empty rows/columns
- Hands parsed rows to same shared pipeline as CSV

### API Endpoints

Each applet exposes endpoints gated by app installation:

- `GET /api/projects/{pid}/etl/status` — Load history across all ETL applets
- `GET /api/projects/{pid}/etl/{appId}/status` — Load history for specific applet
- `POST /api/projects/{pid}/etl/{appId}/load` — Manual file upload (multipart form, alternative to directory watch)
- `GET /api/projects/{pid}/etl/{appId}/preview` — Parse file and return inferred schema + sample rows without loading
- `POST /api/projects/{pid}/etl/{appId}/confirm` — Confirm schema and load after preview

### UI

Each applet gets a tile on the Applications page (existing pattern). When installed, a dedicated page shows:

- Watch directory configuration
- Load history (table of recent loads with status, row counts, timestamps)
- Manual upload dropzone
- Preview/confirm flow for schema review before loading

---

## Implementation Tasks

### Shared Infrastructure

- [x] Add `fsnotify` dependency and create `pkg/services/etl/watcher.go` — directory watcher service that monitors a path, filters by file extension, and dispatches file events to registered handlers. Starts/stops per project based on which ETL applets are installed.

- [x] Create `pkg/services/etl/inference.go` — schema inference engine. Input: `[][]string` (rows of string values) + column names. Output: `[]InferredColumn{Name, SQLType, Nullable, SampleValues}`. Type detection priority: bool → int → bigint → float → date → timestamp → text. Must handle empty strings as nullable.

- [x] Create `pkg/services/etl/ontology_matcher.go` — given inferred columns + candidate table name, queries ontology (via existing `OntologyService` or repository) for matching tables/columns. Returns `MatchResult{MatchedTable, ColumnMappings[], Confidence, IsNewTable}`. Uses column name similarity (case-insensitive, underscore/space normalization) and semantic type alignment when ontology column classifications are available.

- [x] Create `pkg/services/etl/loader.go` — table creator + row loader. Generates DDL from schema, executes via `QueryExecutor`. Bulk inserts with parameterized queries in configurable batches. Returns `LoadResult{TableName, RowsAttempted, RowsLoaded, RowsSkipped, Errors[]}`.

- [x] Create `pkg/models/etl.go` — data models: `InferredColumn`, `MatchResult`, `LoadResult`, `LoadStatus`, `ETLSettings`. Add app ID constants `AppIDETLCSV = "etl-csv"` and `AppIDETLExcel = "etl-excel"` to `installed_app.go`.

- [x] Create migration for `engine_etl_load_history` table — tracks load operations: `id`, `project_id`, `app_id`, `file_name`, `table_name`, `rows_attempted`, `rows_loaded`, `rows_skipped`, `errors` (JSONB), `started_at`, `completed_at`, `status` (pending/running/completed/failed). RLS on `project_id`.

### CSV Applet

- [x] Create `pkg/services/etl/csv_parser.go` — parses CSV/TSV files. Auto-detects delimiter by trying comma/tab/semicolon/pipe on first 5 rows and picking most consistent column count. Returns `ParseResult{Headers []string, Rows [][]string, Delimiter rune}`. Uses stdlib `encoding/csv` with `LazyQuotes` for resilience.

- [x] Register `etl-csv` app in the installation framework — add to known app constants, wire up handler registration and app gating.

### XLSX Applet

- [x] Add `github.com/xuri/excelize/v2` dependency. Create `pkg/services/etl/xlsx_parser.go` — parses XLSX files using streaming reader. Returns `[]SheetData{SheetName, Headers, Rows}`. Handles date serial number conversion, merged cells (fills down/right), and empty row/column skipping.

- [x] Register `etl-excel` app in the installation framework — add to known app constants, wire up handler registration and app gating.

### API & Handler

- [x] Create `pkg/handlers/etl.go` — HTTP handler for ETL endpoints. `GET .../etl/status`, `GET .../etl/{appId}/status`, `POST .../etl/{appId}/load` (multipart upload), `GET .../etl/{appId}/preview`, `POST .../etl/{appId}/confirm`. All gated by app installation check. Preview returns inferred schema + ontology match suggestions + sample rows. Confirm triggers actual load.

- [x] Wire ETL routes in router — register ETL handler endpoints, gated by respective app installation.

### UI

- [x] Add ETL app tiles to ApplicationsPage — `etl-csv` and `etl-excel` tiles with descriptions, install/uninstall buttons following existing pattern.

- [x] Create ETL app page component — shared page for both CSV and XLSX applets showing: settings panel (watch directory, batch size, auto-create toggle), load history table, manual upload dropzone, preview/confirm modal with schema review and ontology match suggestions.

---

## Key Decisions

**Directory watching vs. upload-only:** Support both. Directory watch is the "turnkey" experience. Manual upload via API is the fallback and useful for one-off loads. Directory watch requires the engine process to have filesystem access to the configured path.

**Schema conflict on re-load:** When a file is loaded into an existing table but has different columns, the `on_conflict` setting controls behavior:
- `append` (default) — Insert rows using columns that match, ignore extra columns, NULL for missing columns
- `replace` — Drop and recreate the table with new schema
- `error` — Fail the load and report the mismatch

**Table naming:** Filename → lowercase → replace spaces/hyphens with underscores → strip non-alphanumeric → truncate to 63 chars (Postgres identifier limit). For XLSX, append `_sheetname` suffix. If table exists and schema matches, append to it.

**Ontology matching is advisory:** The preview/confirm flow shows ontology match suggestions, but the user confirms before loading. In auto-create mode (directory watch), ontology matching runs automatically but logs its decisions for auditability.

---

## Dependencies

- `fsnotify/fsnotify` — Directory watching (MIT, ~10K stars, pure Go)
- `github.com/xuri/excelize/v2` — XLSX parsing (BSD-3, ~19.7K stars, pure Go)
- No other new dependencies — CSV uses stdlib

## Existing Infrastructure Used

- `InstalledAppService` — App lifecycle (install, activate, settings)
- `QueryExecutor.Execute()` / `ExecuteWithParams()` — DDL and DML against project datasource
- `OntologyService` or ontology repositories — Column/table metadata for matching
- `database.GetTenantScope(ctx)` — Multi-tenant isolation
- Router and handler patterns — Existing REST API conventions
- UI `useInstalledApps` hook — App state management in frontend
