# TASK: PII Detection Logic in WASM Module

**Status:** PENDING
**Created:** 2026-03-04
**Parent:** PLAN-app-pii-radar.md (Task 5)
**Branch:** wt-ekaya-engine-wasm

## Context

Ekaya Engine is building a WASM application platform. PII Radar is the first app — it scans customer datasource columns for sensitive data (PII, secrets, financial data). The host infrastructure is now in place (Tasks 1-4). This task implements the actual PII detection logic inside a WASM guest module.

This is the first task that builds **application logic** rather than platform infrastructure. The guest module will be the PII Radar application itself.

**What exists today:**

WASM host (`pkg/wasm/`):
- `runtime.go` — `Runtime.LoadAndRun()` loads WASM, registers host functions, calls exports.
- `state.go` — `StateStore`/`MemoryStateStore`, `StateHostFuncs()` for `state_get`/`state_set` with CAS versioning.
- `tools.go` — `ToolInvoker`/`MapToolInvoker`, `ToolInvokeHostFunc()` for `tool_invoke`.
- Guest modules: `echo_guest.wasm`, `state_guest.wasm`, `tool_guest.wasm`, `tool_state_guest.wasm` in `pkg/wasm/testdata/`.
- Guest Rust source in `pkg/wasm/guests/echo_guest/`.

Data access (via MCP tools):
- `get_schema` tool returns structured JSON: tables with columns (name, type, `is_primary_key`, nullability).
- `query` tool executes read-only SELECT SQL (max 1000 rows). Returns `{columns, rows, row_count, truncated}`.
- High-watermark pattern: `SELECT ... FROM table WHERE pk > $hwm ORDER BY pk LIMIT 1000`.

**MCP tool design principle** (from CLAUDE.local.md):
> All MCP tools must return structured JSON that works for both LLM MCP clients and programmatic callers (WASM apps, APIs). One tool, one structured JSON response, usable by all clients.

**Classification categories** (from BRAINSTORM-ekaya-engine-applications.md lines 308-329):
- `secrets` (API keys, tokens, passwords) → Block by default
- `pii_identity` (SSN, passport) → Block by default
- `pii_contact` (email, phone, address) → Flag for review
- `pii_financial` (credit card, bank account) → Block by default

**Origin:** This was identified from a real security finding — `get_context` with `include: ["sample_values"]` exposed LiveKit API keys from a `users.agent_data` JSONB column.

**Reference material:**
- `plans/PLAN-app-pii-radar.md` — Feature-level scope and design decisions.
- `plans/DESIGN-wasm-application-platform.md` — Runtime, auth, data access strategy.
- `plans/BRAINSTORM-ekaya-engine-applications.md` lines 308-329 — Detection patterns and classification categories.

## Objective

Build a PII Radar WASM guest module that:
1. Discovers tables/columns via `tool_invoke("get_schema", ...)`.
2. Scans column data incrementally via `tool_invoke("query", ...)` using a high-watermark.
3. Detects PII using regex patterns on column names and content.
4. Stores scan progress (high-watermarks) and findings in state via `state_get`/`state_set`.
5. Returns a structured report of findings.

## Scope

By the end of this task:
1. A reusable, WASM-compatible Rust PII detection library exists (regex-based, no external dependencies beyond `regex`/`regex-lite` and `serde`).
2. A PII Radar WASM guest module exists (Rust + extism-pdk, compiled to `.wasm`) that uses the library.
3. The module implements column name pattern matching (static analysis — no data access needed).
4. The module implements content pattern scanning (reads rows via `query` tool, applies regex).
5. The module tracks high-watermarks per table in state so it can resume incremental scanning.
6. The module returns a structured JSON report of findings (table, column, category, confidence, sample match).
7. Tests prove the detection logic works using mock tools (no real database needed).
8. `make check` passes.

## Steps

### Step 1: Research existing PII detection pattern sources

Before writing patterns from scratch, review the following projects for regex patterns, entity type coverage, and classification approaches. The goal is to extract the best patterns and logic into our own WASM-compatible Rust code — not to use these as runtime dependencies.

- [ ] **Review [Microsoft Presidio](https://github.com/microsoft/presidio)** (Python, MIT License).
  The gold standard for PII detection. Focus on `presidio-analyzer/presidio_analyzer/predefined_recognizers/` — each recognizer file contains regex patterns, validation logic, and context words for a specific entity type (credit card, SSN, email, phone, IBAN, etc.). Extract:
  - Regex patterns per entity type
  - Context word lists (words that appear near PII and boost confidence)
  - Validation logic (e.g., Luhn check for credit cards, checksum for SSNs)
  - Entity type taxonomy and confidence scoring approach

- [ ] **Review [PIISA pii-extract-plg-regex](https://github.com/piisa/pii-extract-plg-regex)** (Python, Apache 2.0 License).
  Country/language-specific regex patterns. Focus on the pattern definitions for:
  - US: SSN, phone, ZIP, driver's license
  - International: IBAN, UK NHS/NINO, passport formats
  - Note which patterns include validation beyond regex (e.g., checksum)

- [ ] **Review [`pii` crate](https://docs.rs/pii/latest/pii/)** (Rust, check license).
  Rust-native PII detection. Evaluate:
  - What recognizers/patterns does it define?
  - Can any pattern definitions be extracted without pulling in the NLP/ML dependencies?
  - Is the license compatible (for pattern extraction, not as a dependency)?

- [ ] **Review [censgate/redact](https://github.com/censgate/redact)** (Rust, check license).
  Claims 36+ pattern-based entity types with a `redact-wasm` crate. Evaluate:
  - What entity types are covered?
  - Quality and completeness of regex patterns
  - Does `redact-core` compile to WASM without the ONNX/NER dependencies?
  - If `redact-core` is WASM-compatible and well-licensed, consider using it directly instead of extracting patterns
  - If not WASM-compatible, extract pattern definitions

- [ ] **Document findings.** For each project, record:
  - License (and compatibility with our use)
  - Entity types covered
  - Pattern quality (are there validation checks beyond regex?)
  - Whether patterns can be extracted or the library used directly
  - Create a comparison table in this TASK file or in DESIGN-wasm-application-platform.md

### Step 2: Build the PII detection library

- [ ] **Create a WASM-compatible Rust PII detection library.** Location: `pkg/wasm/guests/pii_detector/` (or a shared crate that PII Radar depends on). This library should:
  - Be a pure Rust crate with no dependencies beyond `regex` (or `regex-lite`), `serde`, `serde_json`
  - Not depend on extism-pdk — it's a reusable library, not a WASM plugin
  - Compile to `wasm32-unknown-unknown` as part of the PII Radar guest
  - Incorporate the best patterns found in Step 1 (respecting licenses — attribute sources)
  - Implement two detection modes:
    - **Column name matching:** regex against column names → category + confidence
    - **Content scanning:** regex against cell values → category + confidence + redacted sample
  - Include validation logic where available (Luhn for credit cards, SSN checksum, etc.)
  - Support classification categories: `secrets`, `pii_identity`, `pii_contact`, `pii_financial`
  - Return structured results (not just boolean match — include category, pattern name, confidence, redacted sample)

### Step 3: Build the PII Radar guest module

- [ ] **Design the PII Radar guest module structure.** The module's `run` export should:
  1. Call `state_get` to load previous scan state (high-watermarks, known findings).
  2. Call `tool_invoke("get_schema", {})` to get the list of tables and columns.
  3. **Phase 1 — Column name scan:** Use the PII detection library to match column names. Flag matches with category and confidence. This is instant (no data access).
  4. **Phase 2 — Content scan:** For each table, construct a `SELECT` query using the high-watermark. Call `tool_invoke("query", {sql, limit})`. Use the PII detection library to scan returned values. Track new high-watermark.
  5. Call `state_set` to persist updated high-watermarks and findings.
  6. Return a JSON report.

- [ ] **Design the state schema.** The JSON blob stored via `state_get`/`state_set`:
  ```json
  {
    "high_watermarks": {
      "schema.table": {"column": "id", "value": "12345"}
    },
    "findings": [
      {
        "table": "public.users",
        "column": "agent_data",
        "category": "secrets",
        "pattern": "api_key_in_json",
        "confidence": "high",
        "sample": "sk_live_****",
        "detected_at": "2026-03-04T..."
      }
    ],
    "last_scan": "2026-03-04T...",
    "scan_count": 5
  }
  ```

- [ ] **Build the Rust guest module.** Create a new guest module in `pkg/wasm/guests/pii_radar/` using extism-pdk. It depends on the PII detection library crate (workspace dependency or path dependency). Compile to `wasm32-unknown-unknown`. Check in the compiled `.wasm` binary at `pkg/wasm/testdata/pii_radar.wasm`.

### Step 4: Test and validate

- [ ] **Write Go tests with mock tools.** Test using `MapToolInvoker` with mock `get_schema` and `query` responses:
  - Column name detection: mock schema with columns named `user_password`, `api_key`, `email`, `first_name` → detects password/api_key/email, ignores first_name.
  - Content detection: mock query results containing SSN, email, JWT token values → detects them with correct categories.
  - High-watermark: run the module twice with different mock query results → second run uses the stored high-watermark.
  - Empty schema: no tables → clean report, no errors.
  - No PII found: clean data → empty findings.
  - JSONB content: mock query results with stringified JSON containing `api_key` values → detected as secrets.

- [ ] **Run `make check`.** Ensure linting, existing tests, and build all pass.

## Design Notes

- **Redaction in reports:** Sample matches in findings should be redacted (e.g., `sk_live_****`, `***-**-1234`). The module should never store full sensitive values in state — this would defeat the purpose.
- **Confidence levels:** Column name matches are "high" confidence (the column is named `password`). Content pattern matches vary: SSN/credit card patterns are "high," email is "medium" (could be a legitimate business field), JSON key matches are "medium." Context words (from Presidio's approach) can boost confidence.
- **Incremental scanning:** The module scans a configurable number of rows per invocation (e.g., 1000). If there are more rows, the high-watermark is saved and the next invocation continues. This keeps each run bounded.
- **JSONB columns:** The original security finding involved a JSONB column (`users.agent_data`) containing API keys. Content scanning should handle stringified JSON values — scan the text representation, not just structured fields.
- **Regex in Rust WASM:** The `regex` crate compiles to WASM but the binary size will increase. Consider using `regex-lite` if size is a concern. The compiled `.wasm` may be larger than the test fixtures (echo_guest is 74KB; PII radar with regex could be 500KB-1MB).
- **License attribution:** If patterns are extracted from MIT/Apache-licensed projects (Presidio, PIISA, etc.), include attribution in the crate's license file and/or source comments.

## Out of Scope

- Policy enforcement (Task 8 — "ignore PII in table.column")
- Admin notification on findings (Task 7)
- Scheduling / periodic execution (Task 6)
- UI (Tasks 9-10)
- Production ToolInvoker wired to real MCP server
- Database-backed state persistence

## Success Criteria

Tests prove: PII Radar WASM module discovers schema via mock `get_schema`, scans data via mock `query`, detects PII in column names and content using regex patterns sourced from best-in-class open-source projects, tracks high-watermarks across invocations, and returns a structured findings report. `make check` passes.
