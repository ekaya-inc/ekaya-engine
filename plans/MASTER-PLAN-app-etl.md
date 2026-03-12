# MASTER PLAN: ETL Applets

**Status:** Active
**Created:** 2026-03-09
**Branch:** main (worktree: wt-ekaya-engine-etl)

---

## Vision

A collection of single-purpose ETL applets that users install individually from the Ekaya Applications marketplace. Each applet handles one file type or narrow use case: "point at a directory, files get loaded into your SQL database."

**Core design principle:** Works without an ontology. Becomes really powerful with one.

---

## Architecture: Two-Track Extraction

```
File dropped in watched directory
        │
        ├── Structured format? ──→ Go library parses directly (fast, no model needed)
        │   (XLSX, CSV, Parquet,     │
        │    JSON/JSONL, XML)        ▼
        │                     Structured rows
        │
        └── Unstructured format? ──→ Convert to images ──→ Ekaya Model extracts ──→ Structured JSON
            (PDF, scans, PPT,            │
             receipts, contracts)        ▼
                                   Structured rows
                                         │
                                         ▼
                              ┌─── Ontology available? ───┐
                              │                           │
                              No                         Yes
                              │                           │
                        AI infers schema          Ontology matches to
                        Creates new table         existing entities,
                        Best-guess types          validates, links FKs
                              │                           │
                              └───────────┬───────────────┘
                                          ▼
                                    Load into SQL
```

### Ontology Enhancement Ladder

Every applet benefits from the same progressive enhancement:

| Level | Ontology State | Behavior |
|-------|---------------|----------|
| 1 | No ontology, no existing tables | AI infers schema from data, creates new table, best-guess types |
| 2 | No ontology, existing tables | AI tries to match columns by name to existing tables — best-effort |
| 3 | Ontology, no match | Column classifications improve type inference (e.g., "currency" → `NUMERIC(10,2)`) |
| 4 | Ontology, match found | Maps to existing entities, validates constraints, preserves FKs, uses glossary names |

---

## Implementation Strategy

### Go-Native First, WASM Later

The WASM application platform (`DESIGN-wasm-application-platform.md`) does not exist yet. ETL applets will be built as **Go-native services within the engine**, registered as installable apps via the existing `InstalledAppService` framework. When the WASM platform ships, the applets can be refactored into WASM packages — but the core logic (parsing, inference, loading) stays the same.

### Shared Infrastructure ("ETL SDK")

All applets share common infrastructure built once in Phase 1:

- **Directory watcher** — `fsnotify`-based file monitoring with configurable watch paths per project
- **Schema inferrer** — Samples data rows, infers SQL column types (int, float, bool, date, timestamp, text), handles nullability
- **Ontology matcher** — Given inferred columns, queries the ontology for matching entities/columns by name similarity and semantic type
- **Table creator** — Generates and executes `CREATE TABLE` DDL from inferred or matched schema
- **Row loader** — Bulk `INSERT` with configurable batch size, handles type coercion and errors
- **Conflict resolver** — Handles schema drift when a new file has different columns than a previously-loaded file
- **Status tracker** — Records load history (file, rows loaded, errors, timestamp) for the app's UI

### App Registration Pattern

Each ETL applet registers as an installable app following the existing pattern:

1. App constant added to `pkg/models/installed_app.go` (e.g., `AppIDETLExcel = "etl-excel"`)
2. App-specific handler registered in router
3. App gating via `InstalledAppService.IsInstalled()` — same pattern as `ai-data-liaison`
4. Settings stored in `engine_installed_apps.settings` JSONB (watch directory path, batch size, auto-create tables, etc.)

---

## Phases

### Phase 1: Structured Data Loaders (Go libraries, no model needed)

**Plan:** `PLAN-app-etl-structured-data.md`

| Applet | Library | License | Notes |
|--------|---------|---------|-------|
| CSV/TSV → SQL | stdlib `encoding/csv` | BSD | Delimiter auto-detection, encoding handling |
| XLSX → SQL | excelize (~19.7K stars) | BSD-3 | Streaming API, multi-sheet support |

Builds the shared ETL SDK infrastructure. These two applets cover the highest-demand "I have a file, get it into my database" scenarios.

### Phase 2: Model-Powered Loaders (Qwen3.5-35B-A3B vision extraction)

**Depends on:** Phase 1 shared infrastructure + Qwen3.5 model deployed on Spark unit

| Applet | Extraction Method | Notes |
|--------|------------------|-------|
| PDF Tables → SQL | Pages → images → model → JSON | Handles complex layouts, scanned docs |
| Receipts → SQL | Photo/scan → model → JSON | Vendor, date, line items, amounts, tax |
| Invoices → SQL | Scan → model → JSON | Invoice #, PO #, line items, payment terms |
| Contracts → SQL | Pages → images → model → JSON | Parties, dates, terms, obligations |
| PowerPoint → SQL | Slides → images → model → JSON | Tables, charts-as-data, key-value pairs |

**Architecture addition for Phase 2:**
- Image conversion layer (file → page images via Go library or system tool)
- Model extraction prompt templates per document type
- Structured JSON schema validation before loading
- Uses existing `LLMClient.GenerateResponse()` with vision-capable model endpoint

### Phase 3: Additional Structured Loaders (if demand warrants)

| Applet | Library | License | Notes |
|--------|---------|---------|-------|
| Parquet → SQL | parquet-go (~571 stars) | Apache 2.0 | Schema embedded in file, near-zero inference |
| JSON/JSONL → SQL | goccy/go-json (~3.6K stars) | MIT | Nested object flattening, array handling |
| XML → SQL | stdlib + etree (~1.6K stars) | BSD-2 | XPath extraction, repeated elements as rows |
| HTML Tables → SQL | goquery (~14.9K stars) | BSD-3 | Extract `<table>` elements from saved pages |
| Markdown Tables → SQL | goldmark (~4.6K stars) | MIT | GFM table extraction |

---

## Competitive Advantage: Local Model Extraction

For Phase 2, running Qwen3.5-35B-A3B locally on Spark units provides:

- **No data leaves premises** — critical for contracts, invoices, financial documents
- **No per-page API costs** — flat infrastructure cost vs. Google Document AI / AWS Textract pricing
- **Low latency** — model on same network as engine
- **Works offline** — no cloud dependency
- **Apache 2.0 license** — commercially permissive

Model specs: 35B total params, 3B active (MoE), 262K context, 91% OCR benchmark, 89.3% document understanding.

---

## Related Documents

- `DESIGN-app-etl-genius.md` — ETL Genius app (MCP tools + Advisors for Data Engineers, complementary)
- `DESIGN-wasm-application-platform.md` — Future WASM runtime these applets could migrate to
- `BRAINSTORM-ekaya-engine-applications.md` — Full application brainstorm
