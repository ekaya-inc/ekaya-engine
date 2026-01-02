# PLAN: Text2SQL - Implementation Guide

> **Navigation:** [Overview](PLAN-text2sql-overview.md) | [Vector Infrastructure](PLAN-text2sql-vector.md) | [Service](PLAN-text2sql-service.md) | [Enhancements](PLAN-text2sql-enhancements.md) | [Security](PLAN-text2sql-security.md) | [Ontology Linking](PLAN-text2sql-ontology-linking.md) | [Memory System](PLAN-text2sql-memory.md) | [Implementation](PLAN-text2sql-implementation.md)

## Implementation Steps

### Step 1: Create Database Migration
- [ ] Create `migrations/010_vector_embeddings.up.sql`
  - `CREATE EXTENSION IF NOT EXISTS vector`
  - Create `vector_embeddings` table with columns: `id`, `project_id`, `collection_name`, `content`, `embedding_1024`, `metadata`, timestamps
  - **Add pattern learning columns (from claudontology):** `confidence`, `success_count`, `failure_count`, `last_succeeded_at`, `last_failed_at`, `parameter_slots`
  - Create `ivfflat` index on `embedding_1024` using `vector_cosine_ops`
  - Create indexes on `(project_id, collection_name)` and `(project_id, collection_name, embedding_1024)`
  - **Add confidence index (from claudontology):** `idx_vector_embeddings_confidence` on `(project_id, collection_name, confidence DESC)` for query_history collection
  - Enable RLS with policy: `project_id = current_setting('app.current_project_id')::uuid`
- [ ] Create `migrations/010_vector_embeddings.down.sql`
  - `DROP TABLE vector_embeddings CASCADE`

**Why ivfflat index?** Approximate nearest neighbor search - fast for large vector collections. Lists=100 balances speed and recall for datasets up to ~100k vectors.

### Step 2: Vector Infrastructure (`pkg/vector/`)
- [ ] Create `pkg/vector/interface.go`
  - Define `VectorStore` interface with methods: `Initialize`, `StoreDocument`, `StoreDocumentAsync`, `DeleteDocuments`, `SimilarSearch`, `SimilarSearchWithScores`, `GetStats`
  - Define `Chunk` struct: `ID`, `Content`, `Metadata`
  - Define `SearchResult` struct: `Chunk`, `Score`
- [ ] Create `pkg/vector/qwen_client.go`
  - Implement `QwenClient` struct with `baseURL`, `model`, `apiKey`, `httpClient`
  - Implement `GenerateBatchEmbeddings(ctx, texts)` - POST to `/embeddings` endpoint
  - Implement `GenerateEmbedding(ctx, text)` - wrapper for single text
  - HTTP timeout: 30 seconds
  - Error handling: include HTTP status and response body
- [ ] Create `pkg/vector/embedding_cache.go`
  - Implement `EmbeddingCache` wrapper around `QwenClient`
  - Use Redis for caching with MD5(text) as cache key
  - Cache TTL: 24 hours
  - Batch processing: check cache for all texts first, only generate uncached ones
- [ ] Create `pkg/vector/pgvector_store.go`
  - Implement `PgVectorStore` struct with `pool`, `embeddingClient`, `collectionName`, `projectID`
  - Implement `Initialize(ctx)` - verify `vector_embeddings` table exists and pgvector extension enabled
  - Implement `StoreDocument(ctx, chunks)` - batch embed, then UPSERT to database
  - Implement `StoreDocumentAsync(chunks)` - goroutine wrapper
  - Implement `SimilarSearchWithScores(ctx, query, topK, scoreThreshold)` - embed query, execute `<=>` cosine distance search with score filter
  - Implement `SimilarSearch(ctx, query, topK)` - wrapper with scoreThreshold=0.0
  - Implement `DeleteDocuments(ctx, chunkIDs)` - delete by ID array
  - Implement `GetStats(ctx)` - return count and metadata
  - Implement `formatVectorForPgVector(embedding)` - convert `[]float32` to `"[1.0,2.0,3.0]"` string format
- [ ] Create `pkg/vector/schema_indexer.go`
  - Implement `SchemaIndexer` struct with `vectorStore`, `schemaRepo`, `projectID`
  - Implement `IndexSchema(ctx, datasourceID, forceRebuild)` - generates embeddings for all tables
  - Implement `buildTableDescription(table, columns)` - creates rich text with table name, column names, column types
  - Each table becomes one chunk with metadata: `table_id`, `table_name`, `datasource_id`, `column_count`
  - Return `IndexResult` with success status, tables indexed count, embeddings generated count

### Step 3: Copy Knowledge Files
- [ ] Copy `knowledge/postgres.md` from ekaya-region to ekaya-engine
- [ ] Copy `knowledge/mssql_syntax.md` from ekaya-region to ekaya-engine
- [ ] Copy `knowledge/clickhouse_syntax.md` from ekaya-region to ekaya-engine
- [ ] Create `pkg/services/text2sql_knowledge.go` to embed knowledge files for runtime access

**Why embed knowledge?** Avoid file I/O at runtime. Embed files at compile time using `//go:embed` directive for fast access and portability.

### Step 3.5: Pattern Learning Enhancement (from claudontology)
- [ ] Create `pkg/services/pattern_matcher.go` - Multi-dimensional similarity scoring
  - Implement `CalculateMultiDimensionalSimilarity(query, pattern)` - combines keyword, entity, operation, structure, parameter scores
  - Implement `ExtractKeywords(query)` - extract searchable keywords
  - Implement `ExtractEntities(query)` - extract table/column mentions
  - Implement `ExtractOperations(query)` - identify aggregate, filter, join, rank operations
  - Implement `CalculateKeywordSimilarity()` - Jaccard similarity on keywords
  - Implement `CalculateEntitySimilarity()` - exact/partial entity match scoring
  - Implement `CalculateOperationSimilarity()` - operation type matching
  - Implement `CheckParameterCompatibility()` - can parameters be substituted?
- [ ] Add pattern confidence tracking methods to `pkg/vector/pgvector_store.go`:
  - `RecordPatternSuccess(ctx, patternID, executionTimeMs)` - increment success_count, update confidence (+0.05), update last_succeeded_at
  - `RecordPatternFailure(ctx, patternID)` - increment failure_count, decrease confidence (-0.05), update last_failed_at
  - `GetHighConfidencePatterns(ctx, minConfidence)` - retrieve patterns with confidence >= threshold
- [ ] Create background job `pkg/services/workqueue/pattern_decay_job.go`:
  - Daily job to decay confidence for patterns unused >30 days (multiply by 0.95)
  - Remove patterns with confidence <0.5 unused >90 days
- [ ] Modify `Text2SQLService.GenerateSQL()` to call pattern matcher before LLM:
  - If pattern confidence ≥0.85 AND similarity ≥0.9 → return cached SQL immediately
  - If pattern confidence ≥0.7 AND similarity ≥0.8 → include as few-shot example
  - After successful execution → call `RecordPatternSuccess()`
  - After failed execution → call `RecordPatternFailure()`

### Step 4: Text2SQL Service
- [ ] Create `pkg/services/text2sql.go`
  - Implement `Text2SQLService` struct with `vectorStore`, `llmClient`, `schemaRepo`, `databaseType`, `logger`
  - Implement `GenerateSQL(ctx, req)` orchestration method:
    1. Schema linking: `vectorStore.SimilarSearchWithScores(question, 5, 0.5)` in "schema" collection
    2. Few-shot retrieval: `vectorStore.SimilarSearchWithScores(question, 3, 0.7)` in "query_history" collection
    3. Build prompt: `buildPrompt(question, schemaResults, fewShotResults)`
    4. LLM generation: `llmClient.GenerateResponse(ctx, prompt, systemMessage, 0.0, false)`
    5. SQL extraction: `extractSQL(response)`
    6. Async storage: `storeQueryHistoryAsync(question, sql)`
  - Implement `buildPrompt(question, schemaResults, fewShotResults)` - 7 sections:
    1. Database type (postgres/mssql/clickhouse)
    2. Syntax reference (from knowledge files)
    3. Relevant tables (from schema vector search)
    4. Few-shot examples (from query history vector search)
    5. Rules and constraints
    6. User question
    7. Output format instructions
  - Implement `getSyntaxKnowledge()` - load from embedded knowledge files
  - Implement `extractSQL(response)` - parse SQL from markdown code blocks
  - Implement `storeQueryHistoryAsync(question, sql, projectID, datasourceID)` - create chunk and call `StoreDocumentAsync`
  - Implement `calculateConfidence(schemaResults, fewShotResults)` - average top 3 schema scores + example bonus

### Step 5: API Handler
- [ ] Create `pkg/handlers/text2sql.go`
  - Implement `POST /api/projects/{projectId}/datasources/{datasourceId}/text2sql` endpoint
  - Request body: `{"question": "natural language question"}`
  - **Updated response body (with claudontology enhancements):** `{"sql": "...", "confidence": 0.85, "tables_used": [...], "examples_found": 2, "needs_clarification": false, "pattern_used": "..."}`
  - Extract `projectID`, `datasourceID` from path parameters
  - Extract user ID from JWT claims
  - **Parse session context from X-Session-Context header (from claudontology)**
  - Call `text2sqlService.GenerateSQL(ctx, req)`
  - Return generated SQL, clarification request, or error

### Step 5.5: Ambiguity Detection (from claudontology)
- [ ] Create `pkg/services/ambiguity_detector.go`
  - Implement `AmbiguityDetectorService` struct with `ontologyRepo`, `knowledgeRepo`
  - Implement `DetectAmbiguity(ctx, query)` - returns `AmbiguityResult`
  - Implement `CalculateConfidence(term, ctx)` - checks session context → project knowledge → ontology → defaults to 0.5
  - Implement temporal ambiguity detection: "recent", "current", "active", "last period"
  - Implement quantitative ambiguity detection: "high", "top", "significant", "average"
  - Implement entity ambiguity detection: pronoun references, partial names, unclear grouping
  - Implement `GenerateClarificationRequest(ambiguities)` - creates question with suggestions
- [ ] Create `pkg/models/ambiguity.go`
  - Define `AmbiguityResult` struct: `Confidence`, `AmbiguousTerms`, `Assumptions`, `NeedsClarification`
  - Define `AmbiguousTerm` struct: `Term`, `Category`, `Position`, `Suggestions`
  - Define `Assumption` struct for tracking auto-resolved ambiguities
- [ ] Integrate into Text2SQL pipeline:
  - Add ambiguity detection BEFORE pattern cache check in `Text2SQLService.GenerateSQL()`
  - If confidence <0.7 → return clarification request immediately
  - If 0.7 ≤ confidence <0.9 → proceed but note assumptions in response
  - If confidence ≥0.9 → proceed automatically
- [ ] Update handler to return clarification requests:
  - New response format when `needs_clarification: true`
  - Include clarification questions and suggestions

### Step 5.6: Session Context (from claudontology)
- [ ] Create `pkg/middleware/session_context.go`
  - Implement `SessionContextMiddleware(next http.Handler)` - reads X-Session-Context header
  - Parse JSON into SessionContext model
  - Store in request context via `context.WithValue()`
- [ ] Create `pkg/models/session_context.go`
  - Define `SessionContext` struct: `TimeFocus`, `UserSegment`, `ExcludeTest`, `CustomFilters`
- [ ] Modify `Text2SQLService.buildPrompt()`:
  - Add Section 4.5: Session Context
  - Inject time focus, user segment, and custom filters as guidance to LLM
  - Format: "Apply these filters to all queries unless explicitly overridden"
- [ ] Wire up middleware in `main.go`:
  - Add `SessionContextMiddleware` to text2sql route
- [ ] Document session context header format in API documentation

### Step 5.7: Ontology Integration (from claudontology)
- [ ] Modify `pkg/vector/schema_indexer.go`:
  - Update `buildTableDescription()` to accept `ontology *models.Ontology` parameter
  - Use `ontology.GetEntitySummary(tableName)` to get business name and description
  - Use `ontology.GetColumnDetail(tableName, columnName)` to get semantic type and description
  - Include domain classification in table descriptions
  - Format: "Table: users (Business name: Customer Accounts) Description: Core customer data Domain: transactional"
- [ ] Modify `Text2SQLService.buildPrompt()`:
  - Add Section 1.5: Domain Context (before Database Type)
  - Load domain summary from ontology tier 0
  - Include business domain description to help LLM understand context
- [ ] Update column descriptions to include semantic type hints:
  - "revenue (numeric, measure): Total revenue in USD" → suggests aggregation
  - "user_id (uuid, identifier): Unique user identifier" → suggests JOIN
  - "signup_date (date, dimension): User registration date" → suggests GROUP BY

### Step 6: Service Initialization in main.go
- [ ] Modify `main.go` to wire up text2sql components:
  - Create Qwen embedding client: `vector.NewQwenClient(baseURL, model, apiKey)`
  - Create embedding cache: `vector.NewEmbeddingCache(qwenClient, redisClient, model)`
  - Create pgvector store for schema: `vector.NewPgVectorStore(pool, embeddingCache, "schema", projectID)`
  - Create pgvector store for query history: `vector.NewPgVectorStore(pool, embeddingCache, "query_history", projectID)`
  - Create schema indexer: `vector.NewSchemaIndexer(schemaVectorStore, schemaRepo, projectID)`
  - Create text2sql service: `services.NewText2SQLService(queryHistoryVectorStore, llmClient, schemaRepo, databaseType, logger)`
  - Create text2sql handler: `handlers.NewText2SQLHandler(text2sqlService)`
  - Register route: `router.POST("/api/projects/:projectId/datasources/:datasourceId/text2sql", text2sqlHandler.GenerateSQL)`

### Step 7: Schema Indexing Endpoint
- [ ] Create `POST /api/projects/{projectId}/datasources/{datasourceId}/index-schema` endpoint in `pkg/handlers/schema.go`
  - Request body: `{"force_rebuild": false}`
  - Call `schemaIndexer.IndexSchema(ctx, datasourceID, forceRebuild)`
  - Return `IndexResult` JSON

**When to call?** After schema refresh (`RefreshDatasourceSchema`). Index schema automatically so text2sql has up-to-date table embeddings.

### Step 8: Integration with Existing Query Flow
- [ ] Modify `pkg/services/query.go` to optionally use text2sql:
  - Add `GenerateFromNaturalLanguage(ctx, projectID, datasourceID, question)` method
  - Calls `text2sqlService.GenerateSQL(ctx, req)`
  - Returns generated SQL for execution via existing `Execute` method
- [ ] Update `pkg/handlers/queries.go` to support natural language queries:
  - Add `POST /api/projects/{projectId}/datasources/{datasourceID}/queries/generate` endpoint
  - Request body: `{"question": "natural language question"}`
  - Response: `{"sql": "...", "confidence": 0.85}`

### Step 9: Configuration
- [ ] Add text2sql configuration to `pkg/config/config.go`:
  - `EmbeddingAPIURL` (default: `http://sparktwo:30001/v1`)
  - `EmbeddingModel` (default: `Qwen/Qwen3-Embedding-0.6B`)
  - `EmbeddingAPIKey` (default: empty for community models)
  - `Text2SQLEnabled` (default: false - explicit opt-in)
- [ ] Add environment variables:
  - `EMBEDDING_API_URL`
  - `EMBEDDING_MODEL`
  - `EMBEDDING_API_KEY`
  - `TEXT2SQL_ENABLED`

### Step 10: Testing
- [ ] Create `pkg/vector/pgvector_store_test.go`
  - Test pgvector operations: Initialize, StoreDocument, SimilarSearch, DeleteDocuments
  - Use testcontainers for PostgreSQL with pgvector extension
  - Verify cosine similarity scores
  - Test multi-tenancy (different projects don't see each other's vectors)
- [ ] Create `pkg/vector/schema_indexer_test.go`
  - Test schema indexing: IndexSchema, IsSchemaIndexed, GetIndexedTableCount
  - Mock schema repository
  - Verify chunk content format
- [ ] Create `pkg/services/text2sql_test.go`
  - Test text2sql flow: GenerateSQL end-to-end
  - Mock vector store, LLM client, schema repo
  - Test prompt building with different scenarios
  - Test SQL extraction from various LLM response formats
  - Test confidence calculation
- [ ] Create integration test for full text2sql flow:
  - Index sample schema
  - Generate SQL from question
  - Execute SQL (verify it runs without errors)
  - Store query history
  - Generate SQL for similar question (verify few-shot learning works)
## Files to Create

**Migrations:**
- `migrations/010_vector_embeddings.up.sql` (enhanced with pattern learning columns from claudontology)
- `migrations/010_vector_embeddings.down.sql`

**Vector Infrastructure:**
- `pkg/vector/interface.go`
- `pkg/vector/qwen_client.go`
- `pkg/vector/embedding_cache.go`
- `pkg/vector/pgvector_store.go` (enhanced with pattern confidence methods from claudontology)
- `pkg/vector/schema_indexer.go` (enhanced with ontology integration from claudontology)

**Services:**
- `pkg/services/text2sql.go` (enhanced with ambiguity detection, session context, ontology from claudontology)
- `pkg/services/text2sql_knowledge.go`
- `pkg/services/pattern_matcher.go` - **NEW from claudontology:** Multi-dimensional similarity scoring
- `pkg/services/ambiguity_detector.go` - **NEW from claudontology:** Ambiguity detection service
- `pkg/services/workqueue/pattern_decay_job.go` - **NEW from claudontology:** Background confidence decay job

**Security Infrastructure:**
- `pkg/security/rules.go` - Deterministic rule engine (Layer 1)
- `pkg/security/complexity.go` - Complexity and anomaly analyzer (Layer 2)
- `pkg/security/classifier.go` - XGBoost ML classifier wrapper (Layer 3)
- `pkg/security/llm_verifier.go` - LLM sandbox verification (Layer 4)
- `pkg/security/pipeline.go` - Orchestration layer
- `pkg/security/features.go` - Feature extraction utilities
- `pkg/security/training/train_classifier.py` - Model training script
- `pkg/security/training/malicious_examples.jsonl` - SQLi training data
- `pkg/security/training/benign_examples.jsonl` - Legitimate query training data

**Handlers:**
- `pkg/handlers/text2sql.go` (enhanced with clarification response handling from claudontology)

**Middleware:**
- `pkg/middleware/session_context.go` - **NEW from claudontology:** Session context middleware

**Models:**
- `pkg/models/ambiguity.go` - **NEW from claudontology:** Ambiguity detection models
- `pkg/models/session_context.go` - **NEW from claudontology:** Session context model

**Tests:**
- `pkg/vector/pgvector_store_test.go`
- `pkg/vector/schema_indexer_test.go`
- `pkg/services/text2sql_test.go`
- `pkg/services/pattern_matcher_test.go` - **NEW from claudontology**
- `pkg/services/ambiguity_detector_test.go` - **NEW from claudontology**
- `pkg/security/rules_test.go`
- `pkg/security/complexity_test.go`
- `pkg/security/classifier_test.go`
- `pkg/security/pipeline_test.go`

**Knowledge Files:**
- `knowledge/postgres.md` (copy from ekaya-region)
- `knowledge/mssql_syntax.md` (copy from ekaya-region)
- `knowledge/clickhouse_syntax.md` (copy from ekaya-region)

## Files to Modify

**Configuration:**
- `pkg/config/config.go` - Add text2sql configuration fields

**Main:**
- `main.go` - Wire up text2sql components, session context middleware, pattern decay job

**Existing Services:**
- `pkg/services/query.go` - Add natural language query generation method
- `pkg/handlers/queries.go` - Add text2sql generation endpoint
- `pkg/handlers/schema.go` - Add schema indexing endpoint
- `pkg/vector/schema_indexer.go` - **Modified from original plan:** Integrate ontology data into table descriptions (from claudontology)
- `pkg/services/text2sql.go` - **Modified from original plan:** Add ambiguity detection, session context injection, pattern matching before LLM (from claudontology)

## Dependencies

**New Go modules:**
- `github.com/pgvector/pgvector-go` - pgvector support for PostgreSQL

**Already available:**
- `github.com/jackc/pgx/v5/pgxpool` - PostgreSQL connection pooling
- `github.com/redis/go-redis/v9` - Redis client for embedding cache
- `github.com/google/uuid` - UUID handling
- `github.com/sashabaranov/go-openai` - LLM client (already used for ontology chat)
- `go.uber.org/zap` - Logging

**External services:**
- Qwen3-Embedding-0.6B API at `http://sparktwo:30001/v1`
- Redis for embedding caching
- PostgreSQL with pgvector extension

## Testing Strategy

### Unit Tests
- Vector store operations (Initialize, Store, Search, Delete)
- Embedding cache (cache hits, cache misses, batch processing)
- Schema indexer (table description building, chunk creation)
- Text2SQL prompt building (verify 7 sections, syntax inclusion)
- SQL extraction (handle various markdown formats)
- Confidence calculation (verify scoring logic)

### Integration Tests
- **Full text2sql flow:**
  1. Create test datasource with sample schema
  2. Index schema
  3. Ask natural language question
  4. Verify generated SQL is valid
  5. Execute SQL (should run without errors)
  6. Ask similar question
  7. Verify few-shot example was used
- **Multi-tenancy:**
  1. Create vectors for project A
  2. Create vectors for project B
  3. Verify project A cannot search project B's vectors
- **Schema updates:**
  1. Index initial schema
  2. Modify schema (add table)
  3. Reindex with force_rebuild=true
  4. Verify new table is searchable

### Load Tests
- Index schema with 100+ tables
- Generate 1000 SQL queries concurrently
- Verify embedding cache hit rate > 80%
- Verify vector search latency < 500ms p95
- Verify no connection pool exhaustion

## Migration Notes

### Backward Compatibility
- Text2SQL is opt-in via `TEXT2SQL_ENABLED` environment variable
- Existing query execution flow unchanged
- No breaking changes to existing APIs

### Rollout Strategy
1. Deploy with `TEXT2SQL_ENABLED=false` (disabled by default)
2. Run migration to create `vector_embeddings` table
3. Enable for internal testing projects
4. Index schemas for test projects
5. Monitor embedding API latency and costs
6. Gradually enable for production projects
7. Monitor SQL generation quality and execution success rates

### Monitoring
- Track embedding API call rate and latency
- Track vector search query latency
- Track SQL generation success rate (% of queries that execute without errors)
- Track confidence scores distribution
- Track few-shot example usage (% of queries with examples found)
- Track Redis cache hit rate

## Testing Strategy

### Unit Tests
- Vector store operations (Initialize, Store, Search, Delete)
- Embedding cache (cache hits, cache misses, batch processing)
- Schema indexer (table description building, chunk creation)
- Text2SQL prompt building (verify 7 sections, syntax inclusion)
- SQL extraction (handle various markdown formats)
- Confidence calculation (verify scoring logic)

### Integration Tests
- **Full text2sql flow:**
  1. Create test datasource with sample schema
  2. Index schema
  3. Ask natural language question
  4. Verify generated SQL is valid
  5. Execute SQL (should run without errors)
  6. Ask similar question
  7. Verify few-shot example was used
- **Multi-tenancy:**
  1. Create vectors for project A
  2. Create vectors for project B
  3. Verify project A cannot search project B's vectors
- **Schema updates:**
  1. Index initial schema
  2. Modify schema (add table)
  3. Reindex with force_rebuild=true
  4. Verify new table is searchable

### Load Tests
- Index schema with 100+ tables
- Generate 1000 SQL queries concurrently
- Verify embedding cache hit rate > 80%
- Verify vector search latency < 500ms p95
- Verify no connection pool exhaustion

## Migration Notes

### Backward Compatibility
- Text2SQL is opt-in via `TEXT2SQL_ENABLED` environment variable
- Existing query execution flow unchanged
- No breaking changes to existing APIs

### Rollout Strategy
1. Deploy with `TEXT2SQL_ENABLED=false` (disabled by default)
2. Run migration to create `vector_embeddings` table
3. Enable for internal testing projects
4. Index schemas for test projects
5. Monitor embedding API latency and costs
6. Gradually enable for production projects
7. Monitor SQL generation quality and execution success rates

### Monitoring
- Track embedding API call rate and latency
- Track vector search query latency
- Track SQL generation success rate (% of queries that execute without errors)
- Track confidence scores distribution
- Track few-shot example usage (% of queries with examples found)
- Track Redis cache hit rate

