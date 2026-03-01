# PLAN: Multi-Statement Support for execute Tool

Status: DONE

## Problem

The `execute` MCP tool rejects multi-statement SQL input with: `"cannot insert multiple commands into a prepared statement"`. This is a PostgreSQL pgx limitation — `pool.Query()` uses the extended query protocol which only supports single statements. MCP clients are forced to make one tool call per statement, which:

- Wastes MCP round-trips on DDL-heavy workflows (e.g., 11 enum types = 11 calls)
- Fills the client's context window with repetitive tool call/result pairs
- Provides zero transactional guarantees across statements (each call auto-commits independently)

## Approach

Push multi-statement handling down to the adapter layer. Each adapter uses its database library's native capabilities to execute multi-statement blocks within a transaction. No SQL parsing or statement splitting is needed at the tool layer.

- The tool layer (`developer.go`) passes the SQL string through as-is — no changes to how it calls `executor.Execute()`
- Each adapter's `Execute()` method detects multi-statement input and wraps it in a transaction
- Adapters that cannot support multi-statement return a clear error: `"this datasource does not support multi-statement execution"`
- RETURNING clauses are not supported in multi-statement batches (the simple query protocol / Exec doesn't return rows) — return a clear error if detected

## Changes

### 1. PostgreSQL adapter — `pkg/adapters/datasource/postgres/query_executor.go`

Modify `Execute()` (line 201) to handle multi-statement input:

- Detect multi-statement input (contains `;` followed by non-whitespace content — a lightweight heuristic, not a full parser)
- **Single statement (current behavior)**: continue using `pool.Query()` with extended query protocol. No change.
- **Multi-statement**: acquire a connection from the pool, `BEGIN` a transaction, execute the full block via `conn.Exec()` (simple query protocol, handles multi-statement natively), then `COMMIT`. On error, `ROLLBACK` and return the error.
- `conn.Exec()` does not return rows. If the batch contains RETURNING clauses, the results are silently discarded — this is an acceptable limitation since RETURNING in a DDL batch is not a realistic use case. Document this in the tool description if needed.
- Return an `ExecuteResult` with aggregate `RowsAffected` from the command tag (pgx returns the last statement's tag for multi-statement exec).

### 2. SQL Server adapter — `pkg/adapters/datasource/mssql/query_executor.go`

Modify `Execute()` (line 233) to handle multi-statement input:

- Same multi-statement detection heuristic
- **Single statement**: no change to current behavior
- **Multi-statement**: wrap in a transaction using `db.BeginTx()`, execute via `tx.ExecContext()` (SQL Server natively handles batches), then `tx.Commit()`. On error, `tx.Rollback()`.
- Same RETURNING/OUTPUT limitation applies

### 3. Tool layer — `pkg/mcp/tools/developer.go`

Minimal changes to the execute handler (line 762):

- `DetectSQLType()` will classify multi-statement blocks as the first statement's type or UNKNOWN — this only affects the log label, not execution behavior. This is acceptable.
- Audit logging logs the full SQL block as one execution entry — correct since it's one atomic operation
- No changes to error handling — adapter errors propagate as they do today

## Out of Scope

- No new `execute_batch` tool — the existing `execute` tool handles both cases
- No SQL parser or statement splitter — adapters use native library support
- No changes to `ExecuteWithParams` — parameterized queries remain single-statement only
- No changes to the `ExecuteResult` struct — multi-statement batches return one result

## Checklist

- [x] Modify PostgreSQL `Execute()` to detect multi-statement input and wrap in a transaction using `conn.Exec()`
- [x] Modify SQL Server `Execute()` to detect multi-statement input and wrap in a transaction using `tx.ExecContext()`
- [x] Add unit tests for multi-statement execution in both adapters (success, failure/rollback, single-statement unchanged)
