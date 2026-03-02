# PLAN: Multi-word search for search_schema tool

**Status:** TODO
**File:** `pkg/mcp/tools/search.go`
**Test file:** `pkg/mcp/tools/search_integration_test.go`

## Problem

The `search_schema` tool treats the entire query as a single substring match (`LIKE '%' || $1 || '%'`). Searching "user billing" looks for the literal string "user billing" — which almost never matches. LLM agents frequently search with multiple terms (e.g., "user status", "billing transaction", "order amount").

## Solution

Split the query into individual words in Go, then search each word independently against table/column names and descriptions. Rank results by how many words matched and where they matched.

## Approach

**Go-side word splitting:**
- Split `query` on whitespace into individual terms in Go before passing to SQL
- Single-word queries behave exactly as today (no regression)
- For multi-word queries, construct SQL that matches each word independently

**SQL strategy:**
- For each word, generate ILIKE conditions against name/description fields
- Score = sum of per-word relevance (exact name match > prefix > description substring)
- Results that match more words rank higher than results matching fewer words
- Use OR logic for filtering (match ANY word) but rank by match count

**Relevance scoring:**
- Per-word score: exact name match (1.0), prefix match (0.9), description match (0.6)
- Aggregate relevance = sum of per-word scores / number of query words (normalized 0-1)
- This means a result matching all words ranks above a result matching one word

## Implementation

- [ ] 1. **Add word-splitting helper** — Add a `splitSearchTerms(query string) []string` function that splits on whitespace, lowercases, and deduplicates. Single-word queries return a single-element slice.

- [ ] 2. **Refactor `searchTables` for multi-word** — Replace the single `$1` ILIKE with dynamically constructed SQL that checks each word independently. Build the WHERE clause as `(word1 conditions) OR (word2 conditions)` and the relevance score as the sum of per-word CASE expressions. Use parameterized queries (no string interpolation into SQL).

- [ ] 3. **Refactor `searchColumns` for multi-word** — Same pattern as tables: per-word ILIKE conditions on `column_name`, `purpose`, and `description` fields with aggregated relevance scoring.

- [ ] 4. **Add integration tests for multi-word search** — Test cases:
  - Single-word query still works (regression check)
  - Two-word query matching different fields of same item (e.g., table name + description)
  - Two-word query where both words appear in the table/column name (e.g., "billing transactions" matches `billing_transactions`)
  - Partial match ranking: item matching 2/2 words ranks above item matching 1/2 words
  - Query with extra whitespace / duplicate words handled gracefully

- [ ] 5. **Run `make check`** — Verify lint, tests, and build all pass.
