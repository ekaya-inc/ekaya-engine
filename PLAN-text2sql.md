# PLAN: Text2SQL with Vector-Based Schema Filtering and Few-Shot Learning

## Justification

**Audience:** This document is written for Claude Code (you, in a future session) to understand why we're implementing text2SQL this way and how to execute it correctly.

**Problem:** ekaya-engine currently has NO text2SQL functionality. Users cannot ask natural language questions about their data. Generating SQL from natural language is challenging because:

1. **Schema size explosion** - Enterprise databases have hundreds of tables. Sending all schema metadata to LLMs hits token limits and degrades accuracy
2. **Syntax variation** - Different databases (PostgreSQL, ClickHouse, MSSQL) have incompatible SQL dialects
3. **Learning curve** - LLMs improve with examples, but we don't know which examples are relevant to a given question
4. **No query history** - We can't learn from successful past queries to improve future generations

**Solution:** Implement a two-level vector search system:

**Level 1: Schema Filtering (Relevance)**
- User asks: "What were our top selling products last month?"
- Vector search finds the 5 most relevant tables (e.g., `orders`, `order_items`, `products`, `timestamps`)
- LLM only sees 5 tables instead of 500 tables → stays within token limits, better accuracy

**Level 2: Few-Shot Examples (Learning)**
- Vector search finds 3 similar past successful queries
- These examples teach the LLM the specific patterns this database uses
- Example: If past queries used `DATE_TRUNC('month', created_at)` for monthly grouping, LLM learns that pattern

**Enhanced Pattern Matching (from claudontology proof-of-concept):**
- Multi-dimensional similarity scoring (not just embedding cosine distance)
- Confidence learning with success/failure tracking
- Pattern confidence evolution based on execution results
- Decision thresholds: High confidence (>0.85) = use cached SQL directly, bypassing LLM generation

**Why this matters:**
- **Token efficiency:** 5 tables @ ~200 tokens each = 1000 tokens vs. 500 tables = 100k+ tokens
- **Accuracy:** LLM sees only relevant schema → less confusion, better joins
- **Learning:** LLM learns from actual working queries in this database → correct syntax, idioms
- **Adaptability:** System improves over time as query history grows
- **Performance:** Cached high-confidence patterns skip expensive LLM calls (from claudontology)

**Reference architecture:** ekaya-region implements this exact pattern in its text2sql system. We're adapting it for ekaya-engine's multi-tenant architecture with PostgreSQL customer databases.

## Current State Analysis

### What EXISTS
- **LLM client** (`pkg/llm/client.go`) - OpenAI-compatible chat completions (used for ontology chat)
- **Database adapters** (`pkg/adapters/datasource/postgres/`) - PostgreSQL connection handling via pgxpool
- **Schema discovery** (`pkg/services/schema.go`) - Already discovers tables, columns, foreign keys
- **Query execution** (`pkg/services/query.go`) - Can execute SQL and return results
- **Syntax knowledge** - We have `knowledge/postgres.md`, `knowledge/mssql_syntax.md`, `knowledge/clickhouse_syntax.md` from ekaya-region

### What DOES NOT EXIST
- ❌ **Vector embeddings infrastructure** - No `vector_embeddings` table, no pgvector extension configuration
- ❌ **Embedding generation** - No Qwen3-Embedding-0.6B client
- ❌ **Schema indexing** - No mechanism to embed table schemas for vector search
- ❌ **Query history storage** - No vector storage of successful queries for few-shot learning
- ❌ **Text2SQL service** - No component that orchestrates the entire flow
- ❌ **Prompt builder** - No logic to construct the 7-section LLM prompt with filtered schema + examples

### Key Dependencies
- **Embedding API:** Qwen3-Embedding-0.6B at `http://sparktwo:30001/v1` (1024-dimensional vectors)
- **Redis:** Already available for embedding caching (`pkg/database/redis.go`)
- **PostgreSQL:** ekaya_region database for metadata, needs pgvector extension
- **Connection Manager:** From PLAN-connection-manager.md - will provide pooled connections to customer datasources

## Architecture Design

### Component Diagram

```
User Question
    ↓
┌─────────────────────────────────────────────────────────────────┐
│ Text2SQLService (pkg/services/text2sql.go)                      │
│                                                                   │
│  1. Validate question (not empty, not malicious)                │
│  2. Schema linking (vector search → top 5 tables)               │
│  3. Few-shot retrieval (vector search → top 3 similar queries)  │
│  4. Build 7-section prompt                                       │
│  5. LLM generation                                               │
│  6. SQL extraction                                               │
│  7. Async storage (question+SQL for future few-shot)            │
│     ↓                                                             │
└─────┼─────────────────────────────────────────────────────────────┘
      ↓
   Generated SQL
      ↓
Execute via QueryService (existing)
      ↓
   Results
      ↓
Store (question, SQL) async → Vector Store
```

### Database Schema (Migration)

**File:** `migrations/010_vector_embeddings.up.sql`

```sql
-- Enable pgvector extension (if not already enabled)
CREATE EXTENSION IF NOT EXISTS vector;

-- Unified vector embeddings table
-- Stores both schema embeddings and query history embeddings
CREATE TABLE IF NOT EXISTS vector_embeddings (
    id TEXT PRIMARY KEY,
    project_id UUID NOT NULL,
    collection_name TEXT NOT NULL,  -- 'schema' or 'query_history'
    content TEXT NOT NULL,           -- Text that was embedded
    embedding_1024 vector(1024) NOT NULL,  -- Qwen3-Embedding-0.6B vector
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for fast vector similarity search (cosine distance)
CREATE INDEX IF NOT EXISTS idx_vector_embeddings_cosine
    ON vector_embeddings
    USING ivfflat (embedding_1024 vector_cosine_ops)
    WITH (lists = 100);

-- Index for filtering by project + collection
CREATE INDEX IF NOT EXISTS idx_vector_embeddings_project_collection
    ON vector_embeddings (project_id, collection_name);

-- Composite index for project-scoped similarity search
CREATE INDEX IF NOT EXISTS idx_vector_embeddings_project_collection_embedding
    ON vector_embeddings (project_id, collection_name, embedding_1024 vector_cosine_ops);

-- Enable Row Level Security (RLS) for multi-tenant isolation
ALTER TABLE vector_embeddings ENABLE ROW LEVEL SECURITY;

-- RLS policy: users can only see vectors for their projects
CREATE POLICY vector_embeddings_tenant_isolation ON vector_embeddings
    USING (project_id = current_setting('app.current_project_id', true)::uuid);
```

**File:** `migrations/010_vector_embeddings.down.sql`

```sql
DROP TABLE IF EXISTS vector_embeddings CASCADE;
-- Note: We don't drop the vector extension as it may be used elsewhere
```

### Vector Infrastructure (`pkg/vector/`)

**Why separate from services layer?** Vector operations (embedding generation, similarity search) are infrastructure concerns that multiple services might use. Keeping them in `pkg/vector/` maintains clean separation of concerns.

#### `pkg/vector/interface.go`

```go
package vector

import "context"

// VectorStore defines operations for vector storage backends
type VectorStore interface {
    // Initialize ensures vector store is ready (idempotent)
    Initialize(ctx context.Context) error

    // StoreDocument stores chunks with embeddings (UPSERT semantics)
    StoreDocument(ctx context.Context, chunks []Chunk) error

    // StoreDocumentAsync stores chunks asynchronously (non-blocking)
    StoreDocumentAsync(chunks []Chunk)

    // DeleteDocuments removes chunks by IDs (idempotent)
    DeleteDocuments(ctx context.Context, chunkIDs []string) error

    // SimilarSearch finds top K similar chunks (no score filtering)
    SimilarSearch(ctx context.Context, query string, topK int) ([]Chunk, error)

    // SimilarSearchWithScores finds similar chunks with score threshold
    SimilarSearchWithScores(ctx context.Context, query string, topK int, scoreThreshold float64) ([]SearchResult, error)

    // GetStats returns collection statistics
    GetStats(ctx context.Context) map[string]interface{}
}

// Chunk represents a document chunk with metadata
type Chunk struct {
    ID       string                 // Unique identifier
    Content  string                 // Text content to embed
    Metadata map[string]interface{} // Arbitrary metadata
}

// SearchResult contains a chunk and its similarity score [0, 1]
type SearchResult struct {
    Chunk Chunk
    Score float64  // 1.0 = perfect match, 0.0 = no similarity
}
```

**Design rationale:**
- Interface allows swapping vector backends (future: Qdrant, Pinecone, etc.)
- Chunk abstraction works for both schema and query history
- Metadata flexibility (table names, SQL metadata, user IDs)
- Score threshold enables quality filtering (e.g., only use examples with score >= 0.7)

#### `pkg/vector/qwen_client.go`

```go
package vector

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

// QwenClient generates 1024-dim embeddings via OpenAI-compatible API
type QwenClient struct {
    baseURL    string
    model      string
    apiKey     string
    httpClient *http.Client
}

// NewQwenClient creates a Qwen embedding client
func NewQwenClient(baseURL, model, apiKey string) *QwenClient {
    return &QwenClient{
        baseURL: baseURL,
        model:   model,
        apiKey:  apiKey,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}

type embeddingRequest struct {
    Model string   `json:"model"`
    Input []string `json:"input"`
}

type embeddingResponse struct {
    Data []struct {
        Embedding []float32 `json:"embedding"`
        Index     int       `json:"index"`
    } `json:"data"`
}

// GenerateBatchEmbeddings generates embeddings for multiple texts
func (c *QwenClient) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
    if len(texts) == 0 {
        return [][]float32{}, nil
    }

    url := c.baseURL + "/embeddings"
    reqBody := embeddingRequest{
        Model: c.model,
        Input: texts,
    }

    bodyBytes, err := json.Marshal(reqBody)
    if err != nil {
        return nil, fmt.Errorf("marshal request: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")
    if c.apiKey != "" {
        req.Header.Set("Authorization", "Bearer "+c.apiKey)
    }

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("http request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
    }

    var embResp embeddingResponse
    if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
        return nil, fmt.Errorf("decode response: %w", err)
    }

    // Sort by index to maintain order
    embeddings := make([][]float32, len(texts))
    for _, item := range embResp.Data {
        embeddings[item.Index] = item.Embedding
    }

    return embeddings, nil
}

// GenerateEmbedding generates a single embedding
func (c *QwenClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
    embeddings, err := c.GenerateBatchEmbeddings(ctx, []string{text})
    if err != nil {
        return nil, err
    }
    return embeddings[0], nil
}
```

**Design notes:**
- OpenAI-compatible API format (standard across many embedding services)
- Batch support reduces API calls (embed 100 texts in one request)
- Respects context cancellation
- Error handling includes HTTP status and response bodies

#### `pkg/vector/embedding_cache.go`

```go
package vector

import (
    "context"
    "crypto/md5"
    "encoding/json"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

// EmbeddingCache wraps QwenClient with Redis caching layer
type EmbeddingCache struct {
    qwenClient *QwenClient
    redisClient *redis.Client
    model       string
    cacheTTL    time.Duration
}

// NewEmbeddingCache creates a cached embedding client
func NewEmbeddingCache(qwenClient *QwenClient, redisClient *redis.Client, model string) *EmbeddingCache {
    return &EmbeddingCache{
        qwenClient:  qwenClient,
        redisClient: redisClient,
        model:       model,
        cacheTTL:    24 * time.Hour,
    }
}

// GenerateBatchEmbeddings with caching
func (ec *EmbeddingCache) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
    embeddings := make([][]float32, len(texts))
    uncachedIndices := []int{}
    uncachedTexts := []string{}

    // Check cache for all texts
    for i, text := range texts {
        cacheKey := ec.getCacheKey(text)
        cached, err := ec.getFromCache(ctx, cacheKey)
        if err == nil && cached != nil {
            embeddings[i] = cached
        } else {
            uncachedIndices = append(uncachedIndices, i)
            uncachedTexts = append(uncachedTexts, text)
        }
    }

    // Generate embeddings for uncached texts
    if len(uncachedTexts) > 0 {
        newEmbeddings, err := ec.qwenClient.GenerateBatchEmbeddings(ctx, uncachedTexts)
        if err != nil {
            return nil, err
        }

        // Fill in results and cache them
        for i, embedding := range newEmbeddings {
            idx := uncachedIndices[i]
            embeddings[idx] = embedding
            cacheKey := ec.getCacheKey(uncachedTexts[i])
            _ = ec.saveToCache(ctx, cacheKey, embedding)
        }
    }

    return embeddings, nil
}

func (ec *EmbeddingCache) getCacheKey(text string) string {
    hash := md5.Sum([]byte(text))
    return fmt.Sprintf("embedding:%s:%x", ec.model, hash)
}

func (ec *EmbeddingCache) getFromCache(ctx context.Context, key string) ([]float32, error) {
    data, err := ec.redisClient.Get(ctx, key).Bytes()
    if err != nil {
        return nil, err
    }

    var embedding []float32
    if err := json.Unmarshal(data, &embedding); err != nil {
        return nil, err
    }
    return embedding, nil
}

func (ec *EmbeddingCache) saveToCache(ctx context.Context, key string, embedding []float32) error {
    data, err := json.Marshal(embedding)
    if err != nil {
        return err
    }
    return ec.redisClient.Set(ctx, key, data, ec.cacheTTL).Err()
}
```

**Why cache?**
- Embedding API calls are expensive (latency + cost)
- Schema text rarely changes → high cache hit rate
- Similar user questions → reuse embeddings
- 24-hour TTL balances freshness vs. performance

#### `pkg/vector/pgvector_store.go`

```go
package vector

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/lib/pq"
)

// PgVectorStore implements VectorStore using PostgreSQL with pgvector extension
type PgVectorStore struct {
    pool            *pgxpool.Pool
    embeddingClient *EmbeddingCache
    collectionName  string
    projectID       uuid.UUID
}

// NewPgVectorStore creates a pgvector-based vector store
func NewPgVectorStore(
    pool *pgxpool.Pool,
    embeddingClient *EmbeddingCache,
    collectionName string,
    projectID uuid.UUID,
) (*PgVectorStore, error) {
    if pool == nil {
        return nil, fmt.Errorf("pool cannot be nil")
    }
    if collectionName == "" {
        return nil, fmt.Errorf("collection name cannot be empty")
    }

    return &PgVectorStore{
        pool:            pool,
        embeddingClient: embeddingClient,
        collectionName:  collectionName,
        projectID:       projectID,
    }, nil
}

// Initialize verifies vector_embeddings table exists
func (pvs *PgVectorStore) Initialize(ctx context.Context) error {
    query := `SELECT EXISTS (
        SELECT FROM pg_tables
        WHERE tablename = 'vector_embeddings'
          AND schemaname = current_schema()
    )`

    var exists bool
    err := pvs.pool.QueryRow(ctx, query).Scan(&exists)
    if err != nil {
        return fmt.Errorf("check table existence: %w", err)
    }

    if !exists {
        return fmt.Errorf("vector_embeddings table not found - run migrations/010_vector_embeddings.up.sql")
    }

    return nil
}

// StoreDocument stores chunks with embeddings (UPSERT)
func (pvs *PgVectorStore) StoreDocument(ctx context.Context, chunks []Chunk) error {
    if len(chunks) == 0 {
        return nil
    }

    // Generate embeddings for all chunks
    contents := make([]string, len(chunks))
    for i, chunk := range chunks {
        contents[i] = chunk.Content
    }

    embeddings, err := pvs.embeddingClient.GenerateBatchEmbeddings(ctx, contents)
    if err != nil {
        return fmt.Errorf("generate embeddings: %w", err)
    }

    // UPSERT query
    query := `
        INSERT INTO vector_embeddings (id, project_id, collection_name, content, embedding_1024, metadata)
        VALUES ($1, $2, $3, $4, $5, $6)
        ON CONFLICT (id) DO UPDATE SET
            content = EXCLUDED.content,
            embedding_1024 = EXCLUDED.embedding_1024,
            metadata = EXCLUDED.metadata,
            updated_at = NOW()
    `

    for i, chunk := range chunks {
        metadataJSON, err := json.Marshal(chunk.Metadata)
        if err != nil {
            return fmt.Errorf("marshal metadata for %s: %w", chunk.ID, err)
        }

        _, err = pvs.pool.Exec(ctx, query,
            chunk.ID,
            pvs.projectID,
            pvs.collectionName,
            chunk.Content,
            formatVectorForPgVector(embeddings[i]),
            metadataJSON,
        )
        if err != nil {
            return fmt.Errorf("insert chunk %s: %w", chunk.ID, err)
        }
    }

    return nil
}

// StoreDocumentAsync stores chunks asynchronously
func (pvs *PgVectorStore) StoreDocumentAsync(chunks []Chunk) {
    go func() {
        ctx := context.Background()
        _ = pvs.StoreDocument(ctx, chunks)
        // TODO: Log errors in production
    }()
}

// SimilarSearchWithScores finds similar chunks using cosine similarity
func (pvs *PgVectorStore) SimilarSearchWithScores(
    ctx context.Context,
    query string,
    topK int,
    scoreThreshold float64,
) ([]SearchResult, error) {
    // Generate embedding for query
    embeddings, err := pvs.embeddingClient.GenerateBatchEmbeddings(ctx, []string{query})
    if err != nil {
        return nil, fmt.Errorf("generate query embedding: %w", err)
    }
    queryEmbedding := embeddings[0]

    // Vector similarity search using <=> (cosine distance)
    // Score = 1 - distance (higher is more similar)
    sqlQuery := `
        SELECT
            id,
            content,
            metadata,
            1 - (embedding_1024 <=> $1::vector) AS score
        FROM vector_embeddings
        WHERE
            project_id = $2
            AND collection_name = $3
            AND 1 - (embedding_1024 <=> $1::vector) >= $4
        ORDER BY embedding_1024 <=> $1::vector
        LIMIT $5
    `

    rows, err := pvs.pool.Query(ctx, sqlQuery,
        formatVectorForPgVector(queryEmbedding),
        pvs.projectID,
        pvs.collectionName,
        scoreThreshold,
        topK,
    )
    if err != nil {
        return nil, fmt.Errorf("similarity search: %w", err)
    }
    defer rows.Close()

    var results []SearchResult
    for rows.Next() {
        var id, content string
        var metadataJSON []byte
        var score float64

        if err := rows.Scan(&id, &content, &metadataJSON, &score); err != nil {
            return nil, fmt.Errorf("scan row: %w", err)
        }

        var metadata map[string]interface{}
        if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
            return nil, fmt.Errorf("unmarshal metadata: %w", err)
        }

        results = append(results, SearchResult{
            Chunk: Chunk{
                ID:       id,
                Content:  content,
                Metadata: metadata,
            },
            Score: score,
        })
    }

    return results, rows.Err()
}

// SimilarSearch finds similar chunks without score filtering
func (pvs *PgVectorStore) SimilarSearch(ctx context.Context, query string, topK int) ([]Chunk, error) {
    results, err := pvs.SimilarSearchWithScores(ctx, query, topK, 0.0)
    if err != nil {
        return nil, err
    }

    chunks := make([]Chunk, len(results))
    for i, result := range results {
        chunks[i] = result.Chunk
    }
    return chunks, nil
}

// DeleteDocuments removes chunks by IDs (idempotent)
func (pvs *PgVectorStore) DeleteDocuments(ctx context.Context, chunkIDs []string) error {
    if len(chunkIDs) == 0 {
        return nil
    }

    query := `
        DELETE FROM vector_embeddings
        WHERE id = ANY($1) AND project_id = $2 AND collection_name = $3
    `

    _, err := pvs.pool.Exec(ctx, query, pq.Array(chunkIDs), pvs.projectID, pvs.collectionName)
    return err
}

// GetStats returns collection statistics
func (pvs *PgVectorStore) GetStats(ctx context.Context) map[string]interface{} {
    var count int
    query := `SELECT COUNT(*) FROM vector_embeddings WHERE project_id = $1 AND collection_name = $2`
    _ = pvs.pool.QueryRow(ctx, query, pvs.projectID, pvs.collectionName).Scan(&count)

    return map[string]interface{}{
        "collection_name": pvs.collectionName,
        "document_count":  count,
        "project_id":      pvs.projectID.String(),
        "backend":         "pgvector",
        "dimension":       1024,
    }
}

// formatVectorForPgVector converts []float32 to pgvector format string "[1.0,2.0,3.0]"
func formatVectorForPgVector(embedding []float32) string {
    if len(embedding) == 0 {
        return "[]"
    }

    var builder strings.Builder
    builder.WriteString("[")
    for i, val := range embedding {
        if i > 0 {
            builder.WriteString(",")
        }
        builder.WriteString(fmt.Sprintf("%v", val))
    }
    builder.WriteString("]")
    return builder.String()
}
```

**Key implementation details:**
- Uses pgxpool (aligns with ekaya-engine's PostgreSQL patterns)
- RLS isolation via project_id (multi-tenant safety)
- Cosine distance (`<=>` operator) for similarity
- Batch embedding generation (efficiency)
- Async storage option (non-blocking for background indexing)

#### `pkg/vector/schema_indexer.go`

```go
package vector

import (
    "context"
    "fmt"
    "strings"

    "github.com/ekaya-inc/ekaya-engine/pkg/models"
    "github.com/ekaya-inc/ekaya-engine/pkg/repositories"
    "github.com/google/uuid"
)

// SchemaIndexer indexes database schemas as vector embeddings
type SchemaIndexer struct {
    vectorStore VectorStore
    schemaRepo  repositories.SchemaRepository
    projectID   uuid.UUID
}

// NewSchemaIndexer creates a schema indexer
func NewSchemaIndexer(
    vectorStore VectorStore,
    schemaRepo repositories.SchemaRepository,
    projectID uuid.UUID,
) *SchemaIndexer {
    return &SchemaIndexer{
        vectorStore: vectorStore,
        schemaRepo:  schemaRepo,
        projectID:   projectID,
    }
}

// IndexResult represents schema indexing result
type IndexResult struct {
    Success             bool   `json:"success"`
    Message             string `json:"message"`
    TablesIndexed       int    `json:"tables_indexed"`
    EmbeddingsGenerated int    `json:"embeddings_generated"`
}

// IndexSchema generates embeddings for all tables in a datasource
func (si *SchemaIndexer) IndexSchema(ctx context.Context, datasourceID uuid.UUID, forceRebuild bool) (*IndexResult, error) {
    // Check if already indexed (unless forcing rebuild)
    if !forceRebuild {
        existing, err := si.vectorStore.SimilarSearch(ctx, "test", 1)
        if err == nil && len(existing) > 0 {
            return &IndexResult{
                Success: true,
                Message: "Schema already indexed. Use force_rebuild=true to recreate.",
            }, nil
        }
    }

    // Get all tables for this datasource
    tables, err := si.schemaRepo.GetAllTables(ctx, si.projectID, datasourceID)
    if err != nil {
        return nil, fmt.Errorf("get tables: %w", err)
    }

    chunks := []Chunk{}

    for _, table := range tables {
        // Get columns for this table
        columns, err := si.schemaRepo.GetTableColumns(ctx, si.projectID, datasourceID, table.ID)
        if err != nil {
            continue // Skip tables with errors
        }

        // Create comprehensive table description
        content := si.buildTableDescription(table, columns)

        chunk := Chunk{
            ID:      fmt.Sprintf("schema_%s_%s", datasourceID, table.Name),
            Content: content,
            Metadata: map[string]interface{}{
                "table_id":      table.ID.String(),
                "table_name":    table.Name,
                "datasource_id": datasourceID.String(),
                "type":          "table_schema",
                "column_count":  len(columns),
            },
        }
        chunks = append(chunks, chunk)
    }

    if len(chunks) == 0 {
        return &IndexResult{
            Success: false,
            Message: "No tables found to index",
        }, nil
    }

    // Store embeddings
    if err := si.vectorStore.StoreDocument(ctx, chunks); err != nil {
        return nil, fmt.Errorf("store embeddings: %w", err)
    }

    return &IndexResult{
        Success:             true,
        Message:             fmt.Sprintf("Successfully indexed %d tables", len(tables)),
        TablesIndexed:       len(tables),
        EmbeddingsGenerated: len(chunks),
    }, nil
}

// buildTableDescription creates rich text for embedding
func (si *SchemaIndexer) buildTableDescription(table models.Table, columns []models.Column) string {
    var desc strings.Builder

    desc.WriteString(fmt.Sprintf("Table: %s\n", table.Name))

    // Column names
    columnNames := make([]string, len(columns))
    for i, col := range columns {
        columnNames[i] = col.Name
    }
    desc.WriteString(fmt.Sprintf("Columns: %s\n", strings.Join(columnNames, ", ")))

    // Column types
    columnTypes := make([]string, len(columns))
    for i, col := range columns {
        columnTypes[i] = fmt.Sprintf("%s(%s)", col.Name, col.DataType)
    }
    desc.WriteString(fmt.Sprintf("Column types: %s", strings.Join(columnTypes, ", ")))

    return desc.String()
}
```

**Schema indexing strategy:**
- Each table becomes one chunk
- Content includes: table name, column names, column types
- Metadata stores table_id, datasource_id for filtering
- Idempotent (checks if already indexed)
- Force rebuild option (for schema changes)

### Text2SQL Service (`pkg/services/text2sql.go`)

```go
package services

import (
    "context"
    "fmt"
    "strings"

    "github.com/ekaya-inc/ekaya-engine/pkg/llm"
    "github.com/ekaya-inc/ekaya-engine/pkg/repositories"
    "github.com/ekaya-inc/ekaya-engine/pkg/vector"
    "github.com/google/uuid"
    "go.uber.org/zap"
)

// Text2SQLService generates SQL from natural language questions
type Text2SQLService struct {
    vectorStore   vector.VectorStore
    llmClient     *llm.Client
    schemaRepo    repositories.SchemaRepository
    logger        *zap.Logger
    databaseType  string // "postgres", "mssql", "clickhouse"
}

// NewText2SQLService creates a text2sql service
func NewText2SQLService(
    vectorStore vector.VectorStore,
    llmClient *llm.Client,
    schemaRepo repositories.SchemaRepository,
    databaseType string,
    logger *zap.Logger,
) *Text2SQLService {
    return &Text2SQLService{
        vectorStore:  vectorStore,
        llmClient:    llmClient,
        schemaRepo:   schemaRepo,
        databaseType: strings.ToLower(databaseType),
        logger:       logger.Named("text2sql"),
    }
}

// GenerateSQLRequest contains user question and context
type GenerateSQLRequest struct {
    Question     string
    ProjectID    uuid.UUID
    DatasourceID uuid.UUID
}

// GenerateSQLResponse contains generated SQL and metadata
type GenerateSQLResponse struct {
    SQL           string                 `json:"sql"`
    Confidence    float64                `json:"confidence"`
    TablesUsed    []string               `json:"tables_used"`
    ExamplesFound int                    `json:"examples_found"`
    Metadata      map[string]interface{} `json:"metadata"`
}

// GenerateSQL orchestrates the text2sql flow
func (s *Text2SQLService) GenerateSQL(ctx context.Context, req *GenerateSQLRequest) (*GenerateSQLResponse, error) {
    s.logger.Info("Generating SQL", zap.String("question", req.Question))

    // Step 1: Schema linking via vector search (top 5 tables)
    schemaResults, err := s.vectorStore.SimilarSearchWithScores(ctx, req.Question, 5, 0.5)
    if err != nil {
        return nil, fmt.Errorf("schema search: %w", err)
    }

    if len(schemaResults) == 0 {
        return nil, fmt.Errorf("no relevant tables found - schema may not be indexed")
    }

    // Step 2: Few-shot retrieval via vector search (top 3 similar queries)
    // Search in query_history collection for similar past queries
    fewShotResults, err := s.vectorStore.SimilarSearchWithScores(ctx, req.Question, 3, 0.7)
    if err != nil {
        s.logger.Warn("Few-shot search failed", zap.Error(err))
        fewShotResults = []vector.SearchResult{} // Continue without examples
    }

    // Step 3: Build LLM prompt with filtered schema + examples
    prompt := s.buildPrompt(req.Question, schemaResults, fewShotResults)

    // Step 4: LLM generation
    systemMessage := s.getSystemMessage()
    llmResp, err := s.llmClient.GenerateResponse(ctx, prompt, systemMessage, 0.0, false)
    if err != nil {
        return nil, fmt.Errorf("LLM generation: %w", err)
    }

    // Step 5: Extract SQL from LLM response
    sql := s.extractSQL(llmResp.Content)
    if sql == "" {
        return nil, fmt.Errorf("no SQL found in LLM response")
    }

    // Step 6: Store (question, SQL) async for future few-shot learning
    s.storeQueryHistoryAsync(req.Question, sql, req.ProjectID, req.DatasourceID)

    tablesUsed := s.extractTablesFromResults(schemaResults)

    return &GenerateSQLResponse{
        SQL:           sql,
        Confidence:    s.calculateConfidence(schemaResults, fewShotResults),
        TablesUsed:    tablesUsed,
        ExamplesFound: len(fewShotResults),
        Metadata: map[string]interface{}{
            "top_table_scores": s.extractScores(schemaResults),
        },
    }, nil
}

// buildPrompt constructs the 7-section LLM prompt
func (s *Text2SQLService) buildPrompt(question string, schemaResults, fewShotResults []vector.SearchResult) string {
    var prompt strings.Builder

    // Section 1: Database context
    prompt.WriteString(fmt.Sprintf("# Database Type\n%s\n\n", s.databaseType))

    // Section 2: Syntax reference (from knowledge files)
    syntaxKnowledge := s.getSyntaxKnowledge()
    prompt.WriteString(fmt.Sprintf("# SQL Syntax Reference\n%s\n\n", syntaxKnowledge))

    // Section 3: Relevant schema (from vector search)
    prompt.WriteString("# Relevant Tables\n")
    for _, result := range schemaResults {
        prompt.WriteString(fmt.Sprintf("## %s (relevance: %.2f)\n", result.Chunk.Metadata["table_name"], result.Score))
        prompt.WriteString(result.Chunk.Content)
        prompt.WriteString("\n\n")
    }

    // Section 4: Few-shot examples (from query history)
    if len(fewShotResults) > 0 {
        prompt.WriteString("# Similar Past Queries\n")
        for i, result := range fewShotResults {
            prompt.WriteString(fmt.Sprintf("## Example %d (similarity: %.2f)\n", i+1, result.Score))
            prompt.WriteString(fmt.Sprintf("Question: %s\n", result.Chunk.Metadata["question"]))
            prompt.WriteString(fmt.Sprintf("SQL: %s\n\n", result.Chunk.Metadata["sql"]))
        }
    }

    // Section 5: Rules and constraints
    prompt.WriteString("# Rules\n")
    prompt.WriteString("- Generate ONLY executable SQL, no explanations\n")
    prompt.WriteString("- Use only the tables and columns shown above\n")
    prompt.WriteString("- Wrap SQL in ```sql ``` code blocks\n")
    prompt.WriteString("- Follow the syntax patterns from similar past queries\n\n")

    // Section 6: User question
    prompt.WriteString(fmt.Sprintf("# Question\n%s\n\n", question))

    // Section 7: Output format
    prompt.WriteString("# Output\n")
    prompt.WriteString("```sql\n-- SQL here\n```\n")

    return prompt.String()
}

// getSyntaxKnowledge loads database-specific syntax from knowledge files
func (s *Text2SQLService) getSyntaxKnowledge() string {
    // In practice, this would read from embedded knowledge/ files
    // For now, return key patterns
    switch s.databaseType {
    case "postgres":
        return "PostgreSQL syntax: Use double quotes for identifiers, single quotes for strings, LIMIT for row limiting."
    case "mssql":
        return "T-SQL syntax: Use brackets for identifiers, TOP for row limiting, GETDATE() for current time."
    case "clickhouse":
        return "ClickHouse syntax: Use backticks for identifiers, LIMIT for row limiting, now() for current time."
    default:
        return "Standard SQL syntax."
    }
}

// getSystemMessage returns the LLM system prompt
func (s *Text2SQLService) getSystemMessage() string {
    return "You are an expert SQL query generator. Given a natural language question and database schema, generate accurate, efficient SQL queries."
}

// extractSQL extracts SQL from LLM response (handles markdown code blocks)
func (s *Text2SQLService) extractSQL(response string) string {
    // Look for ```sql ... ``` blocks
    start := strings.Index(response, "```sql")
    if start == -1 {
        start = strings.Index(response, "```")
    }
    if start == -1 {
        return strings.TrimSpace(response) // No code block, assume entire response is SQL
    }

    end := strings.Index(response[start+6:], "```")
    if end == -1 {
        return ""
    }

    sql := response[start+6 : start+6+end]
    sql = strings.TrimPrefix(sql, "sql")
    return strings.TrimSpace(sql)
}

// storeQueryHistoryAsync stores (question, SQL) for future few-shot learning
func (s *Text2SQLService) storeQueryHistoryAsync(question, sql string, projectID, datasourceID uuid.UUID) {
    chunk := vector.Chunk{
        ID:      fmt.Sprintf("query_%s_%s", projectID, uuid.New()),
        Content: question, // Embed the question for similarity search
        Metadata: map[string]interface{}{
            "question":      question,
            "sql":           sql,
            "project_id":    projectID.String(),
            "datasource_id": datasourceID.String(),
            "timestamp":     time.Now().Unix(),
        },
    }

    s.vectorStore.StoreDocumentAsync([]vector.Chunk{chunk})
}

// calculateConfidence estimates query confidence based on schema and example scores
func (s *Text2SQLService) calculateConfidence(schemaResults, fewShotResults []vector.SearchResult) float64 {
    if len(schemaResults) == 0 {
        return 0.0
    }

    // Average top 3 schema scores
    schemaScore := 0.0
    for i := 0; i < min(3, len(schemaResults)); i++ {
        schemaScore += schemaResults[i].Score
    }
    schemaScore /= float64(min(3, len(schemaResults)))

    // Bonus for having high-quality examples
    exampleBonus := 0.0
    if len(fewShotResults) > 0 {
        for _, result := range fewShotResults {
            exampleBonus += result.Score
        }
        exampleBonus /= float64(len(fewShotResults))
        exampleBonus *= 0.3 // Examples contribute up to 30% of confidence
    }

    return min(1.0, schemaScore+exampleBonus)
}

func (s *Text2SQLService) extractTablesFromResults(results []vector.SearchResult) []string {
    tables := []string{}
    for _, result := range results {
        if tableName, ok := result.Chunk.Metadata["table_name"].(string); ok {
            tables = append(tables, tableName)
        }
    }
    return tables
}

func (s *Text2SQLService) extractScores(results []vector.SearchResult) []float64 {
    scores := make([]float64, len(results))
    for i, result := range results {
        scores[i] = result.Score
    }
    return scores
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
```

**Text2SQL flow summary:**
1. **Schema linking** - Find top 5 relevant tables via vector search
2. **Few-shot retrieval** - Find top 3 similar past queries (score >= 0.7)
3. **Prompt building** - Construct 7-section prompt with filtered schema + examples + syntax rules
4. **LLM generation** - Generate SQL with temperature=0 (deterministic)
5. **SQL extraction** - Parse SQL from markdown code blocks
6. **Async storage** - Store (question, SQL) for future learning

### Knowledge Files (Copy from ekaya-region)

**Files to copy directly:**
- `knowledge/postgres.md` → Comprehensive PostgreSQL syntax reference
- `knowledge/mssql_syntax.md` → T-SQL syntax reference
- `knowledge/clickhouse_syntax.md` → ClickHouse syntax reference

These files contain database-specific SQL patterns, functions, and best practices that guide LLM SQL generation.

**Why these matter:**
- **Syntax correctness:** Different databases have incompatible SQL dialects (PostgreSQL `LIMIT` vs. MSSQL `TOP`)
- **Function names:** `NOW()` (PostgreSQL) vs. `GETDATE()` (MSSQL) vs. `now()` (ClickHouse)
- **Identifier quoting:** Double quotes (PostgreSQL) vs. brackets (MSSQL) vs. backticks (ClickHouse)
- **Anti-patterns:** Each database has specific things to avoid (PostgreSQL: `SELECT *`, MSSQL: `NOLOCK everywhere`)

## Enhancements from claudontology Proof-of-Concept

The ekaya-claudontology project demonstrated several advanced concepts that improve text2SQL accuracy, performance, and user experience. This section outlines how to integrate these proven patterns.

### 1. Enhanced Pattern Cache with Multi-Dimensional Similarity

**Current gap:** Basic vector search for (question, SQL) pairs uses only embedding cosine distance.

**Enhancement from claudontology:**

Multi-dimensional similarity scoring combines multiple signals for better pattern matching:

| Dimension | Weight | Description |
|-----------|--------|-------------|
| Keyword overlap | 30% | Jaccard similarity on extracted keywords |
| Entity match | 25% | Tables/columns mentioned (exact match crucial) |
| Operation match | 20% | Query operation types (aggregate, filter, join, rank) |
| Structure match | 15% | Query structure patterns (single table, multi-join, subquery) |
| Parameter compatibility | 10% | Whether time/value parameters can be substituted |

**Confidence evolution system:**
- New patterns start at 0.7 confidence
- Confidence increases by 0.05 per successful execution (capped at 1.0)
- Patterns unused for 30+ days have confidence decayed by 5%
- Patterns with <0.5 confidence unused for 90+ days are auto-removed

**Decision thresholds:**
- Pattern confidence ≥0.85 AND similarity ≥0.9 → Use cached SQL directly (skip LLM)
- Pattern confidence ≥0.7 AND similarity ≥0.8 → Use as few-shot example
- Otherwise → Generate fresh SQL

**Database schema additions:**
```sql
ALTER TABLE vector_embeddings ADD COLUMN
    confidence FLOAT DEFAULT 0.7,
    success_count INT DEFAULT 0,
    failure_count INT DEFAULT 0,
    last_succeeded_at TIMESTAMPTZ,
    last_failed_at TIMESTAMPTZ,
    parameter_slots JSONB DEFAULT '{}'::jsonb;

CREATE INDEX idx_vector_embeddings_confidence
    ON vector_embeddings (project_id, collection_name, confidence DESC)
    WHERE collection_name = 'query_history';
```

### 2. Session Context for Implicit Filters

**Problem:** Users often have implicit context ("analyzing Q4 data", "focus on premium users") that applies to multiple queries but isn't repeated.

**Solution from claudontology:** Two-tier memory system with session-scoped context injection.

**Implementation approach:**

1. **Session context header:**
   ```
   X-Session-Context: {"time_focus": "Q4 2024", "user_segment": "premium", "exclude_test": true}
   ```

2. **Context model:**
   ```go
   type SessionContext struct {
       TimeFocus     string            `json:"time_focus,omitempty"`
       UserSegment   string            `json:"user_segment,omitempty"`
       ExcludeTest   bool              `json:"exclude_test,omitempty"`
       CustomFilters map[string]string `json:"custom_filters,omitempty"`
   }
   ```

3. **Automatic filter application in prompt:**
   ```go
   // In buildPrompt(), add Section 4.5: Session Context
   if sessionCtx := getSessionContext(ctx); sessionCtx != nil {
       prompt.WriteString("# Current Analysis Context\n")
       prompt.WriteString(fmt.Sprintf("- Time focus: %s\n", sessionCtx.TimeFocus))
       prompt.WriteString(fmt.Sprintf("- User segment: %s\n", sessionCtx.UserSegment))
       prompt.WriteString("Apply these filters to all queries unless explicitly overridden.\n\n")
   }
   ```

### 3. Ambiguity Detection with Confidence Thresholds

**Problem:** Vague terms like "recent", "active", "top", "high-performing" mean different things to different users. LLMs guess, often incorrectly.

**Solution from claudontology:** Detect ambiguous terms before SQL generation and either ask for clarification or auto-resolve using knowledge hierarchy.

**Ambiguity categories:**

| Category | Examples | Resolution Strategy |
|----------|----------|---------------------|
| Temporal | "recent", "latest", "current" | Check session context → project knowledge → ask |
| Quantitative | "top", "best", "high" | Check if metric specified → ask for threshold |
| State | "active", "valid", "good" | Check ontology column definitions → ask |
| Aggregation | "total", "average", "typical" | Usually safe to infer from context |

**Resolution hierarchy (from claudontology):**
1. Session context (highest priority)
2. Project knowledge (`engine_project_knowledge`)
3. Ontology column definitions (semantic types, descriptions)
4. Statistical inference (data patterns)
5. Ask user (lowest priority, most accurate)

**Confidence thresholds:**
- Confidence <0.7 → Stop and ask for clarification
- Confidence 0.7-0.9 → Proceed with stated assumption
- Confidence ≥0.9 → Auto-resolve using knowledge

**Response format when clarification needed:**
```json
{
    "needs_clarification": true,
    "clarifications": [
        {
            "term": "active",
            "question": "How do you define 'active' users?",
            "suggestions": [
                "Logged in within last 30 days",
                "Made a purchase within last 90 days",
                "Has non-zero session count"
            ]
        }
    ]
}
```

### 4. Ontology-Enhanced Schema Context

**Current gap:** The plan uses raw schema (table names, column types) for context. But ekaya-engine already has a rich 3-tier ontology with business semantics.

**Enhancement:** Leverage existing ontology tiers for better SQL generation.

**Ontology tiers to use:**

1. **Tier 0 - Domain Summary:**
   - Include domain context in system prompt
   - Helps LLM understand business domain (e.g., "e-commerce platform" vs. "healthcare system")

2. **Tier 1 - Entity Summaries:**
   - Use business names instead of technical table names
   - Include table descriptions in schema linking results
   - Use domain classifications for better table selection

3. **Tier 2 - Column Details:**
   - Use semantic types (dimension, measure, identifier) to guide aggregation
   - Use column descriptions for ambiguous terms
   - Use enum values for validation

**Schema indexing enhancement:**
```go
func (si *SchemaIndexer) buildTableDescription(table models.Table, columns []models.Column, ontology *models.Ontology) string {
    var desc strings.Builder

    // Use ontology business name if available
    entitySummary := ontology.GetEntitySummary(table.Name)
    if entitySummary != nil {
        desc.WriteString(fmt.Sprintf("Table: %s (Business name: %s)\n", table.Name, entitySummary.BusinessName))
        desc.WriteString(fmt.Sprintf("Description: %s\n", entitySummary.Description))
        desc.WriteString(fmt.Sprintf("Domain: %s\n", entitySummary.DomainClassification))
    } else {
        desc.WriteString(fmt.Sprintf("Table: %s\n", table.Name))
    }

    // Include column semantic types
    for _, col := range columns {
        colDetail := ontology.GetColumnDetail(table.Name, col.Name)
        if colDetail != nil {
            desc.WriteString(fmt.Sprintf("  - %s (%s, %s): %s\n",
                col.Name, col.DataType, colDetail.SemanticType, colDetail.Description))
        } else {
            desc.WriteString(fmt.Sprintf("  - %s (%s)\n", col.Name, col.DataType))
        }
    }

    return desc.String()
}
```

**Semantic type hints for aggregation:**
- Columns with `semantic_type: "measure"` → suggest SUM, AVG, COUNT
- Columns with `semantic_type: "dimension"` → suggest GROUP BY
- Columns with `semantic_type: "identifier"` → suggest JOINs

### 5. Updated Text2SQL Flow with Enhancements

```
User Question
    ↓
Ambiguity Detection (NEW) ←── If confidence < 0.7, return clarification request
    ↓
Pattern Cache Check (ENHANCED)
    ↓ If high confidence match (≥0.85), return cached SQL
    ↓ Otherwise, continue...
Schema Linking (with ontology enhancement)
    ↓
Few-Shot Retrieval (with multi-dimensional similarity)
    ↓
Build Prompt (with session context + domain summary)
    ↓
LLM Generation
    ↓
SQL Extraction
    ↓
Execute & Record Success/Failure → Update pattern confidence
```

### Summary of claudontology Enhancements

The claudontology proof-of-concept validated four key improvements to text2SQL systems:

1. **Smarter Pattern Matching** - Multi-dimensional similarity (not just embeddings) + confidence learning reduces unnecessary LLM calls and improves accuracy over time.

2. **Ambiguity Detection** - Identifying vague terms ("recent", "active", "high") before SQL generation prevents incorrect guesses and improves user trust.

3. **Session Context** - Implicit filters that apply to multiple queries reduce repetition and improve user experience.

4. **Ontology Leverage** - Using existing business semantics (table descriptions, semantic types) produces better SQL than raw schema alone.

**Implementation priority:** Pattern matching (#1) has highest ROI (performance + accuracy). Ambiguity detection (#2) has highest UX impact. Session context (#3) and ontology (#4) are incremental improvements.

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

## SQL Security & Validation Architecture

**Problem:** LLM-generated SQL can contain errors, hallucinations, or be manipulated via prompt injection. Without guardrails, malicious or broken SQL reaches the database.

**Solution:** Tiered defense system that combines deterministic rules, structural analysis, ML classification, and LLM verification. Each layer is progressively more expensive but catches different attack types.

### Security Architecture Overview

```
User Question + Generated SQL
            ↓
┌────────────────────────────────────────┐
│ Layer 1: Deterministic Rules (instant) │
│ - Blocklist: DROP, TRUNCATE, xp_cmd... │
│ - Injection patterns: UNION SELECT,    │
│   stacked queries, comment abuse       │
│ - Structure: multiple statements, hex  │
│ → BLOCK / WARN / PASS                  │
└────────────────────────────────────────┘
            ↓ (if not blocked)
┌────────────────────────────────────────┐
│ Layer 2: Complexity & Anomaly Analysis │
│ - Parse SQL → complexity score         │
│ - Compare to expected for question     │
│ - Flag: semantic mismatch, anomalies   │
│ → complexity_score, anomaly_flags      │
└────────────────────────────────────────┘
            ↓
┌────────────────────────────────────────┐
│ Layer 3: ML Classifier (fast, ~1ms)    │
│ Features:                              │
│  - L1 warning count                    │
│  - L2 complexity score + anomaly flags │
│  - Token n-grams, keyword presence     │
│ → risk_probability (0.0 - 1.0)         │
└────────────────────────────────────────┘
            ↓ (only if 0.3 < risk < 0.8)
┌────────────────────────────────────────┐
│ Layer 4: LLM Sandbox (expensive)       │
│ - Only for ambiguous cases             │
│ - "Classify this SQL's intent"         │
│ → ALLOW / BLOCK + explanation          │
└────────────────────────────────────────┘
```

### Layer 1: Deterministic Rules (`pkg/security/rules.go`)

Fast regex and pattern matching. Catches 70-80% of attacks instantly.

**Blocklist patterns:**
- DDL operations: `DROP`, `TRUNCATE`, `ALTER`, `CREATE`
- Dangerous functions: `xp_cmdshell`, `LOAD_FILE`, `INTO OUTFILE`, `pg_read_file`
- Stacked queries: multiple `;` separated statements
- Comment injection: `--`, `/**/`, `#` in suspicious positions
- Hex/char encoding: `0x`, `CHAR()`, `CHR()` obfuscation
- UNION-based injection: `UNION SELECT`, `UNION ALL SELECT`
- Information schema probing: `information_schema`, `pg_catalog`, `sys.tables`

**Structure validation:**
- Single statement only (no `;` splitting)
- Balanced parentheses and quotes
- No dynamic SQL construction (`EXEC`, `EXECUTE IMMEDIATE`)

```go
type RuleResult struct {
    Blocked     bool     `json:"blocked"`
    Warnings    []string `json:"warnings"`
    Violations  []string `json:"violations"`
}

func (r *RulesEngine) Evaluate(sql string) *RuleResult
```

### Layer 2: Complexity & Anomaly Analysis (`pkg/security/complexity.go`)

**Key insight:** Query complexity is a security signal. Legitimate queries have predictable complexity based on the question. Attacks often show complexity anomalies.

**Complexity scoring (adapted from ekaya-query):**

| Pattern | Points | Rationale |
|---------|--------|-----------|
| JOIN | 1 each | More tables = more complexity |
| Subquery | 2 each | Nesting increases attack surface |
| Window function | 2 each | Advanced SQL, unusual in simple questions |
| GROUP BY + HAVING | 1 | Aggregation logic |
| UNION/INTERSECT/EXCEPT | 2 each | Set operations, common in injection |
| CASE/WHEN | 1 each | Conditional logic |
| Nested depth > 3 | 3 | Deep nesting is suspicious |

**Anomaly detection:**

1. **Semantic mismatch** - Simple question → complex SQL
   - "count of users" should not produce 5 JOINs or window functions
   - Mismatch score = actual_complexity - expected_complexity

2. **Structural anomalies**
   - UNION with different column counts
   - SELECT * with UNION (injection pattern)
   - WHERE clause more complex than SELECT

3. **Keyword density** - High concentration of suspicious keywords

```go
type ComplexityResult struct {
    Score           int      `json:"score"`
    JoinCount       int      `json:"join_count"`
    SubqueryCount   int      `json:"subquery_count"`
    NestingDepth    int      `json:"nesting_depth"`
    AnomalyFlags    []string `json:"anomaly_flags"`
    ExpectedScore   int      `json:"expected_score"`   // Based on question
    MismatchScore   int      `json:"mismatch_score"`   // actual - expected
}

func (c *ComplexityAnalyzer) Analyze(question, sql string) *ComplexityResult
```

**Expected complexity estimation:**
- Embed question, compare to training examples of (question, complexity) pairs
- Or: simple keyword heuristics ("count" → low, "trend over time" → medium, "compare YoY" → high)

### Layer 3: ML Classifier (`pkg/security/classifier.go`)

**Why XGBoost over Naive Bayes:**
- Naive Bayes assumes feature independence (bad for SQL where patterns combine)
- XGBoost handles feature interactions naturally (UNION + information_schema = high risk)
- Fast inference (~1ms), works with small training data
- Feature importance is interpretable

**Feature vector:**

```go
type SecurityFeatures struct {
    // From Layer 1
    WarningCount      int
    ViolationCount    int
    HasUnion          bool
    HasSubquery       bool
    HasCommentPattern bool

    // From Layer 2
    ComplexityScore   int
    MismatchScore     int
    JoinCount         int
    NestingDepth      int
    AnomalyFlagCount  int

    // Token features
    SuspiciousKeywordCount int
    TokenCount            int
    AvgTokenLength        float64

    // Structural
    StatementCount    int
    ParenthesisDepth  int
}
```

**Training data:**
- Collect labeled examples: (SQL, malicious/benign)
- Sources: OWASP SQLi examples, internal query logs, generated adversarial examples
- Start with ~1000 examples, expand over time

**Output:**
```go
type ClassifierResult struct {
    RiskProbability float64 `json:"risk_probability"` // 0.0 - 1.0
    Confidence      float64 `json:"confidence"`
    TopFeatures     []string `json:"top_features"` // Explainability
}
```

**Decision thresholds:**
- risk < 0.3 → ALLOW (fast path)
- risk > 0.8 → BLOCK (high confidence malicious)
- 0.3 ≤ risk ≤ 0.8 → Send to Layer 4 (LLM verification)

### Layer 4: LLM Sandbox Verification (`pkg/security/llm_verifier.go`)

Only invoked for ambiguous cases (10-20% of queries). Uses sandboxed LLM call.

**Prompt structure:**
```
You are a SQL security analyst. Analyze this SQL query for potential security issues.

Question asked: {user_question}
Generated SQL: {sql}
Complexity score: {complexity_score}
Warnings from static analysis: {warnings}

Classify as one of:
- SAFE: Normal query matching the question
- SUSPICIOUS: Unusual patterns but possibly legitimate
- MALICIOUS: Clear attack attempt

Provide brief reasoning.
```

**Output:**
```go
type LLMVerificationResult struct {
    Classification string `json:"classification"` // SAFE, SUSPICIOUS, MALICIOUS
    Reasoning      string `json:"reasoning"`
    Confidence     float64 `json:"confidence"`
}
```

### Security Pipeline Integration

The security check runs **after** SQL generation, **before** execution:

```go
func (s *Text2SQLService) GenerateSQL(ctx context.Context, req *GenerateSQLRequest) (*GenerateSQLResponse, error) {
    // ... existing generation logic ...

    sql := s.extractSQL(llmResp.Content)

    // Security validation pipeline
    securityResult, err := s.securityPipeline.Validate(ctx, req.Question, sql)
    if err != nil {
        return nil, fmt.Errorf("security validation: %w", err)
    }

    if securityResult.Blocked {
        return nil, &SecurityBlockedError{
            Reason:   securityResult.BlockReason,
            Details:  securityResult.Details,
        }
    }

    // Include security metadata in response
    return &GenerateSQLResponse{
        SQL:           sql,
        Confidence:    s.calculateConfidence(schemaResults, fewShotResults),
        SecurityScore: securityResult.RiskScore,
        // ...
    }, nil
}
```

### Implementation Steps for Security

#### Step 4.5: Security Infrastructure (`pkg/security/`)

- [ ] Create `pkg/security/rules.go`
  - Implement `RulesEngine` struct with compiled regex patterns
  - Implement `Evaluate(sql)` - runs all deterministic checks
  - Return `RuleResult` with blocked status, warnings, violations
  - Patterns: DDL blocklist, injection signatures, structure validation

- [ ] Create `pkg/security/complexity.go`
  - Implement `ComplexityAnalyzer` struct
  - Implement `Analyze(question, sql)` - parses SQL, calculates scores
  - Scoring: JOINs, subqueries, window functions, nesting depth
  - Anomaly detection: semantic mismatch, structural anomalies
  - Return `ComplexityResult` with scores and flags

- [ ] Create `pkg/security/classifier.go`
  - Implement `SecurityClassifier` struct wrapping XGBoost model
  - Implement `ExtractFeatures(ruleResult, complexityResult, sql)` - build feature vector
  - Implement `Predict(features)` - run model inference
  - Return `ClassifierResult` with risk probability
  - Include model loading from embedded binary or file

- [ ] Create `pkg/security/llm_verifier.go`
  - Implement `LLMVerifier` struct with LLM client
  - Implement `Verify(ctx, question, sql, warnings)` - sandboxed LLM call
  - Structured output parsing for classification
  - Return `LLMVerificationResult`

- [ ] Create `pkg/security/pipeline.go`
  - Implement `SecurityPipeline` struct orchestrating all layers
  - Implement `Validate(ctx, question, sql)` - runs full pipeline
  - Short-circuit on L1 block, skip L4 if L3 confident
  - Return `SecurityResult` with final decision and audit trail

- [ ] Create `pkg/security/training/` directory
  - Store training data for classifier
  - Include scripts for model training and evaluation
  - Export trained model for Go inference

#### Security Files to Create

**New files:**
- `pkg/security/rules.go` - Deterministic rule engine
- `pkg/security/complexity.go` - Complexity and anomaly analyzer
- `pkg/security/classifier.go` - ML classifier wrapper
- `pkg/security/llm_verifier.go` - LLM sandbox verification
- `pkg/security/pipeline.go` - Orchestration layer
- `pkg/security/features.go` - Feature extraction utilities
- `pkg/security/training/train_classifier.py` - Model training script
- `pkg/security/training/malicious_examples.jsonl` - Training data
- `pkg/security/training/benign_examples.jsonl` - Training data

**Tests:**
- `pkg/security/rules_test.go` - Test all blocklist patterns
- `pkg/security/complexity_test.go` - Test scoring accuracy
- `pkg/security/classifier_test.go` - Test model predictions
- `pkg/security/pipeline_test.go` - Integration tests

### Security Success Criteria

- [ ] Layer 1 blocks known SQLi patterns (OWASP test suite)
- [ ] Layer 2 detects semantic mismatch (simple question → complex SQL)
- [ ] Layer 3 classifier achieves >95% accuracy on test set
- [ ] Layer 4 correctly classifies ambiguous cases
- [ ] Full pipeline latency < 50ms for 90% of queries (L1+L2+L3)
- [ ] False positive rate < 1% (don't block legitimate queries)
- [ ] All blocked queries logged with full audit trail

### Security Monitoring

- Track block rate by layer (which layer caught it)
- Track false positive reports (user complaints)
- Track L4 invocation rate (should be <20%)
- Track classifier confidence distribution
- Track new attack patterns (anomalies not caught by L1)

## Ontology Linking Architecture

**Problem:** Pure vector embeddings have blind spots for structured ontology retrieval:
- **Semantic drift**: "revenue" embeds similarly to "income", "earnings", "profit" but ontology might only define "revenue"
- **No structural awareness**: embeddings don't know that "by region" requires a dimension column
- **Miss exact matches**: if user says "order_total", exact match should beat fuzzy similarity
- **No relationship traversal**: embeddings don't know which tables are joinable

**Solution:** Multi-layer ontology linking that combines semantic search with deterministic constraints.

### Ontology Linking Pipeline

```
Natural Language Query
        ↓
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: Entity Extraction                                   │
│ - LLM or regex-based extraction of entities from query      │
│ - Identifies: table mentions, column mentions, values,      │
│   temporal references, aggregation intents                   │
│ → extracted_entities: ["customers", "revenue", "last month"]│
└─────────────────────────────────────────────────────────────┘
        ↓
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: Synonym Expansion                                   │
│ - Map extracted entities to ontology canonical names        │
│ - Use ontology synonyms from Tier 2 column details          │
│ - "revenue" → ["revenue_total", "total_revenue", "amount"]  │
│ → expanded_entities with ontology mappings                   │
└─────────────────────────────────────────────────────────────┘
        ↓
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: Hybrid Retrieval (Embedding + BM25)                │
│ - Coarse retrieval of top 20 candidate tables/columns       │
│ - score = α * embedding_similarity + (1-α) * bm25_score     │
│ - α ≈ 0.7 (favor embeddings, but BM25 catches exact matches)│
│ → candidate_set: 20 tables/columns with scores              │
└─────────────────────────────────────────────────────────────┘
        ↓
┌─────────────────────────────────────────────────────────────┐
│ Layer 4: Semantic Type Filtering                             │
│ - Parse query intent: aggregation, lookup, trend, ranking   │
│ - Apply constraints based on intent:                         │
│   • "average X" → X must be semantic_type=measure           │
│   • "by Y" → Y must be semantic_type=dimension              │
│   • "top N" → needs measure for ranking                     │
│ - Filter candidates that don't satisfy type constraints      │
│ → filtered_candidates: candidates matching semantic types    │
└─────────────────────────────────────────────────────────────┘
        ↓
┌─────────────────────────────────────────────────────────────┐
│ Layer 5: Relationship Graph Traversal                        │
│ - Find minimum spanning subgraph connecting all candidates  │
│ - Use ontology relationships (Tier 1) for join paths        │
│ - Ensure all selected tables are reachable via JOINs        │
│ - Add bridge tables if needed for connectivity              │
│ → connected_subgraph: tables + join paths                    │
└─────────────────────────────────────────────────────────────┘
        ↓
┌─────────────────────────────────────────────────────────────┐
│ Layer 6: Deterministic Re-ranking                            │
│ - Re-score candidates with deterministic features:          │
│   • Exact name match: +0.30                                  │
│   • Synonym match: +0.20                                     │
│   • Semantic type fit: +0.20                                 │
│   • Joinable to other candidates: +0.15                      │
│   • Has required columns: +0.15                              │
│ - Select top 5 tables for final context                      │
│ → final_tables: 5 most relevant tables with full context     │
└─────────────────────────────────────────────────────────────┘
```

### Layer Implementation Details

#### Layer 1: Entity Extraction (`pkg/ontology/entity_extractor.go`)

```go
type ExtractedEntities struct {
    TableMentions    []string          `json:"table_mentions"`    // "customers", "orders"
    ColumnMentions   []string          `json:"column_mentions"`   // "revenue", "created_at"
    ValueMentions    []string          `json:"value_mentions"`    // "premium", "active"
    TemporalRefs     []TemporalRef     `json:"temporal_refs"`     // "last month", "Q4 2024"
    AggregationIntent string           `json:"aggregation_intent"` // "sum", "average", "count"
    QueryType        string            `json:"query_type"`        // "aggregation", "lookup", "trend", "ranking"
}

type TemporalRef struct {
    Original   string `json:"original"`    // "last month"
    Normalized string `json:"normalized"`  // "2024-11-01 to 2024-11-30"
    Type       string `json:"type"`        // "relative", "absolute", "range"
}
```

**Extraction approaches:**
1. **Regex patterns** for common patterns (fast, deterministic):
   - Temporal: `last (week|month|quarter|year)`, `in (January|Q[1-4]|2024)`
   - Aggregation: `(total|sum|average|count|max|min) of`
   - Ranking: `top \d+`, `bottom \d+`, `best`, `worst`

2. **LLM extraction** for complex queries (slower, more accurate):
   - Structured output: "Extract entities from this query"
   - Use when regex confidence is low

#### Layer 2: Synonym Expansion (`pkg/ontology/synonym_resolver.go`)

```go
type SynonymResolver struct {
    ontologyRepo OntologyRepository
    cache        map[string][]string // term → canonical names
}

func (r *SynonymResolver) Expand(term string, projectID uuid.UUID) []OntologyMatch {
    // 1. Exact match on table/column names
    // 2. Exact match on business names (Tier 1)
    // 3. Synonym match from column details (Tier 2)
    // 4. Fuzzy match (Levenshtein distance < 2)
}

type OntologyMatch struct {
    CanonicalName string  `json:"canonical_name"` // Actual table/column name
    BusinessName  string  `json:"business_name"`  // Human-friendly name
    MatchType     string  `json:"match_type"`     // "exact", "synonym", "fuzzy"
    Confidence    float64 `json:"confidence"`     // 1.0 for exact, 0.8 for synonym, 0.6 for fuzzy
    EntityType    string  `json:"entity_type"`    // "table", "column"
    TableName     string  `json:"table_name"`     // Parent table (for columns)
}
```

#### Layer 3: Hybrid Retrieval (`pkg/ontology/hybrid_retriever.go`)

```go
type HybridRetriever struct {
    vectorStore   VectorStore
    bm25Index     *BM25Index
    alpha         float64 // Weight for embedding score (default 0.7)
}

func (r *HybridRetriever) Retrieve(ctx context.Context, query string, topK int) []ScoredCandidate {
    // Get embedding-based results
    embeddingResults := r.vectorStore.SimilarSearchWithScores(ctx, query, topK*2, 0.3)

    // Get BM25-based results
    bm25Results := r.bm25Index.Search(query, topK*2)

    // Merge with weighted scoring
    merged := r.mergeResults(embeddingResults, bm25Results, r.alpha)

    // Return top K
    return merged[:topK]
}

type ScoredCandidate struct {
    TableName       string  `json:"table_name"`
    ColumnName      string  `json:"column_name,omitempty"` // Empty for table-level match
    EmbeddingScore  float64 `json:"embedding_score"`
    BM25Score       float64 `json:"bm25_score"`
    CombinedScore   float64 `json:"combined_score"`
}
```

**BM25 Index:** Build from ontology text (table names, business names, descriptions, column names, synonyms). Rebuild when ontology changes.

#### Layer 4: Semantic Type Filtering (`pkg/ontology/type_filter.go`)

```go
type QueryIntent struct {
    Type        string   // "aggregation", "lookup", "trend", "ranking", "comparison"
    Measures    []string // Columns that need to be measures
    Dimensions  []string // Columns that need to be dimensions
    Identifiers []string // Columns that need to be identifiers (for JOINs)
}

func ParseQueryIntent(query string, entities ExtractedEntities) QueryIntent {
    intent := QueryIntent{}

    switch {
    case containsAggregation(query):
        intent.Type = "aggregation"
        // "average revenue by region" → revenue=measure, region=dimension
    case containsRanking(query):
        intent.Type = "ranking"
        // "top 10 customers by sales" → sales=measure, customers=identifier
    case containsTrend(query):
        intent.Type = "trend"
        // "revenue over time" → revenue=measure, time=dimension
    default:
        intent.Type = "lookup"
    }

    return intent
}

func (f *TypeFilter) Filter(candidates []ScoredCandidate, intent QueryIntent, ontology *Ontology) []ScoredCandidate {
    var filtered []ScoredCandidate

    for _, c := range candidates {
        colDetail := ontology.GetColumnDetail(c.TableName, c.ColumnName)
        if colDetail == nil {
            continue
        }

        // Check if semantic type matches intent requirements
        if intent.RequiresMeasure(c.ColumnName) && colDetail.SemanticType != "measure" {
            continue // Skip non-measures when measure is required
        }
        if intent.RequiresDimension(c.ColumnName) && colDetail.SemanticType != "dimension" {
            continue // Skip non-dimensions when dimension is required
        }

        filtered = append(filtered, c)
    }

    return filtered
}
```

#### Layer 5: Relationship Graph Traversal (`pkg/ontology/graph_traverser.go`)

```go
type OntologyGraph struct {
    tables        map[string]*TableNode
    relationships map[string][]Relationship // table → outgoing relationships
}

type TableNode struct {
    Name        string
    Columns     []string
    IsSelected  bool
}

type Relationship struct {
    FromTable  string
    ToTable    string
    FromColumn string
    ToColumn   string
    Type       string // "1:N", "N:1", "1:1", "N:M"
}

func (g *OntologyGraph) FindConnectingSubgraph(requiredTables []string) (*Subgraph, error) {
    if len(requiredTables) == 0 {
        return nil, errors.New("no tables to connect")
    }

    if len(requiredTables) == 1 {
        return &Subgraph{Tables: requiredTables, Joins: nil}, nil
    }

    // Use Dijkstra or BFS to find shortest paths between all required tables
    // Build minimum spanning tree connecting all required tables
    // Add bridge tables if direct connections don't exist

    return g.buildMinimumSpanningSubgraph(requiredTables)
}

type Subgraph struct {
    Tables []string        // All tables in subgraph (including bridges)
    Joins  []JoinPath      // How to connect them
}

type JoinPath struct {
    LeftTable  string
    RightTable string
    JoinType   string // "INNER", "LEFT"
    Condition  string // "left.id = right.left_id"
}
```

#### Layer 6: Deterministic Re-ranking (`pkg/ontology/reranker.go`)

```go
type RerankerWeights struct {
    ExactNameMatch    float64 // 0.30 - Exact table/column name match
    SynonymMatch      float64 // 0.20 - Match via synonym
    SemanticTypeFit   float64 // 0.20 - Correct semantic type for intent
    Joinability       float64 // 0.15 - Can join to other selected tables
    HasRequiredCols   float64 // 0.15 - Table has columns mentioned in query
}

func (r *Reranker) Rerank(candidates []ScoredCandidate, context RerankerContext) []ScoredCandidate {
    for i := range candidates {
        bonus := 0.0

        // Exact name match
        if context.HasExactMatch(candidates[i].TableName) {
            bonus += r.weights.ExactNameMatch
        }

        // Synonym match
        if context.HasSynonymMatch(candidates[i].TableName) {
            bonus += r.weights.SynonymMatch
        }

        // Semantic type fit
        if context.FitsSemanticType(candidates[i]) {
            bonus += r.weights.SemanticTypeFit
        }

        // Joinability to other candidates
        if context.IsJoinableToOthers(candidates[i].TableName) {
            bonus += r.weights.Joinability
        }

        // Has required columns
        if context.HasRequiredColumns(candidates[i].TableName) {
            bonus += r.weights.HasRequiredCols
        }

        candidates[i].CombinedScore += bonus
    }

    // Sort by final score
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].CombinedScore > candidates[j].CombinedScore
    })

    return candidates
}
```

### Ontology Linking Service (`pkg/services/ontology_linker.go`)

Orchestrates all layers:

```go
type OntologyLinkerService struct {
    entityExtractor  *EntityExtractor
    synonymResolver  *SynonymResolver
    hybridRetriever  *HybridRetriever
    typeFilter       *TypeFilter
    graphTraverser   *GraphTraverser
    reranker         *Reranker
    ontologyRepo     OntologyRepository
    logger           *zap.Logger
}

type LinkingResult struct {
    SelectedTables  []TableContext     `json:"selected_tables"`
    JoinPaths       []JoinPath         `json:"join_paths"`
    ExtractedIntent QueryIntent        `json:"extracted_intent"`
    Confidence      float64            `json:"confidence"`
    DebugInfo       *LinkingDebugInfo  `json:"debug_info,omitempty"`
}

type TableContext struct {
    TableName       string            `json:"table_name"`
    BusinessName    string            `json:"business_name"`
    Description     string            `json:"description"`
    RelevantColumns []ColumnContext   `json:"relevant_columns"`
    MatchScore      float64           `json:"match_score"`
    MatchReasons    []string          `json:"match_reasons"` // "exact_match", "synonym", "embedding"
}

type ColumnContext struct {
    ColumnName    string `json:"column_name"`
    BusinessName  string `json:"business_name"`
    Description   string `json:"description"`
    SemanticType  string `json:"semantic_type"`
    DataType      string `json:"data_type"`
}

func (s *OntologyLinkerService) Link(ctx context.Context, query string, projectID uuid.UUID) (*LinkingResult, error) {
    // Get active ontology
    ontology, err := s.ontologyRepo.GetActiveOntology(ctx, projectID)
    if err != nil {
        return nil, fmt.Errorf("get ontology: %w", err)
    }

    // Layer 1: Entity extraction
    entities, err := s.entityExtractor.Extract(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("extract entities: %w", err)
    }

    // Layer 2: Synonym expansion
    expanded := s.synonymResolver.ExpandAll(entities, projectID)

    // Layer 3: Hybrid retrieval
    candidates, err := s.hybridRetriever.Retrieve(ctx, query, 20)
    if err != nil {
        return nil, fmt.Errorf("hybrid retrieval: %w", err)
    }

    // Layer 4: Semantic type filtering
    intent := ParseQueryIntent(query, entities)
    filtered := s.typeFilter.Filter(candidates, intent, ontology)

    // Layer 5: Graph traversal
    tableNames := extractTableNames(filtered)
    subgraph, err := s.graphTraverser.FindConnectingSubgraph(tableNames)
    if err != nil {
        s.logger.Warn("Could not connect all tables", zap.Error(err))
        // Continue with disconnected tables, LLM might handle it
    }

    // Layer 6: Re-ranking
    rerankerContext := buildRerankerContext(entities, expanded, intent, subgraph)
    reranked := s.reranker.Rerank(filtered, rerankerContext)

    // Select top 5 and build full context
    selected := reranked[:min(5, len(reranked))]

    return s.buildLinkingResult(selected, subgraph, intent, ontology)
}
```

### Integration with Text2SQL Pipeline

Update the Text2SQL service to use ontology linking instead of simple vector search:

```go
func (s *Text2SQLService) GenerateSQL(ctx context.Context, req *GenerateSQLRequest) (*GenerateSQLResponse, error) {
    // BEFORE: Simple vector search
    // schemaResults, err := s.vectorStore.SimilarSearchWithScores(ctx, req.Question, 5, 0.5)

    // AFTER: Full ontology linking
    linkingResult, err := s.ontologyLinker.Link(ctx, req.Question, req.ProjectID)
    if err != nil {
        return nil, fmt.Errorf("ontology linking: %w", err)
    }

    // Build prompt with rich ontology context
    prompt := s.buildPromptWithOntology(req.Question, linkingResult, fewShotResults)

    // ... rest of pipeline
}
```

### Files to Create for Ontology Linking

**Core linking components:**
- `pkg/ontology/entity_extractor.go` - Layer 1: Extract entities from query
- `pkg/ontology/synonym_resolver.go` - Layer 2: Expand to canonical names
- `pkg/ontology/hybrid_retriever.go` - Layer 3: Embedding + BM25 search
- `pkg/ontology/type_filter.go` - Layer 4: Semantic type constraints
- `pkg/ontology/graph_traverser.go` - Layer 5: Relationship graph traversal
- `pkg/ontology/reranker.go` - Layer 6: Deterministic re-ranking
- `pkg/ontology/bm25_index.go` - BM25 index for keyword search

**Service layer:**
- `pkg/services/ontology_linker.go` - Orchestrates all layers

**Models:**
- `pkg/models/linking.go` - LinkingResult, TableContext, ColumnContext, QueryIntent

**Tests:**
- `pkg/ontology/entity_extractor_test.go`
- `pkg/ontology/synonym_resolver_test.go`
- `pkg/ontology/hybrid_retriever_test.go`
- `pkg/ontology/type_filter_test.go`
- `pkg/ontology/graph_traverser_test.go`
- `pkg/ontology/reranker_test.go`
- `pkg/services/ontology_linker_test.go`

### Implementation Steps for Ontology Linking

Add these steps to the implementation plan:

#### Step 2.5: BM25 Index Infrastructure
- [ ] Create `pkg/ontology/bm25_index.go` with BM25 scoring implementation
- [ ] Build index from ontology text (table names, business names, descriptions, synonyms)
- [ ] Add index rebuild trigger when ontology changes
- [ ] Store index in Redis for fast access

#### Step 4.5: Ontology Linking Pipeline
- [ ] Create `pkg/ontology/entity_extractor.go` with regex + LLM fallback
- [ ] Create `pkg/ontology/synonym_resolver.go` using ontology Tier 2 data
- [ ] Create `pkg/ontology/hybrid_retriever.go` combining vector + BM25
- [ ] Create `pkg/ontology/type_filter.go` for semantic type constraints
- [ ] Create `pkg/ontology/graph_traverser.go` for relationship traversal
- [ ] Create `pkg/ontology/reranker.go` with deterministic scoring
- [ ] Create `pkg/services/ontology_linker.go` orchestrating all layers
- [ ] Integrate ontology linker into Text2SQL service

### Success Criteria for Ontology Linking

- [ ] Entity extraction identifies table/column mentions with >90% recall
- [ ] Synonym resolver maps user terms to ontology with >85% accuracy
- [ ] Hybrid retrieval outperforms pure embedding on exact match queries
- [ ] Semantic type filtering removes 20-30% of irrelevant candidates
- [ ] Graph traversal finds valid join paths for multi-table queries
- [ ] Re-ranking improves top-1 accuracy by >10% over raw retrieval
- [ ] End-to-end linking selects correct tables for >80% of test queries
- [ ] Linking adds <100ms latency to query processing

### Key Design Decisions for Ontology Linking

**Why 6 layers instead of just embeddings?**
Each layer catches different failure modes:
- Layer 1-2: Catches exact matches embeddings miss
- Layer 3: Balances semantic similarity with keyword relevance
- Layer 4: Enforces structural constraints (measure/dimension)
- Layer 5: Ensures selected tables can actually be JOINed
- Layer 6: Combines signals for final ranking

**Why α=0.7 for hybrid retrieval?**
Embeddings capture semantic meaning better than keywords for most queries. But 30% BM25 weight ensures exact matches aren't lost. Tunable per deployment based on query patterns.

**Why build BM25 index instead of just using database LIKE?**
- BM25 handles term frequency and document length normalization
- Pre-computed scores are faster than real-time LIKE queries
- Supports phrase matching and boosting

**Why graph traversal after filtering?**
Filter first to reduce graph size. Traversing a 5-node graph is fast; traversing 100+ nodes is slow. Order matters for performance.

## Memory System Architecture

**Problem:** Valuable knowledge is lost after each query session:
- User clarifies "active means logged in within 30 days" → forgotten next session
- User prefers "clients" instead of "users" → must re-learn every time
- Domain expert defines "high-value customer = >$10k annual spend" → not captured
- One user's terminology differs from another's → no personalization

**Solution:** Two-tier memory system that captures and applies learned knowledge.

### Memory Tiers

```
┌─────────────────────────────────────────────────────────────────┐
│                     Memory Resolution Order                      │
├─────────────────────────────────────────────────────────────────┤
│  1. Session Context (highest priority, ephemeral)               │
│     → X-Session-Context header, current conversation only       │
│                                                                  │
│  2. User Memory (persistent, user-specific)                     │
│     → Terminology preferences, personal definitions             │
│     → "This user calls customers 'clients'"                     │
│                                                                  │
│  3. Project Knowledge (persistent, shared)                      │
│     → Domain facts, business rules, canonical definitions       │
│     → "Active user = logged in within 30 days"                  │
│                                                                  │
│  4. Ontology Definitions (lowest priority, authoritative)       │
│     → Semantic types, column descriptions, relationships        │
└─────────────────────────────────────────────────────────────────┘
```

### Memory Types and Use Cases

| Memory Type | Scope | Persistence | Example |
|-------------|-------|-------------|---------|
| **Session Context** | Single session | Ephemeral | "Focus on Q4 2024 data" |
| **User Memory** | One user, all sessions | Persistent | "User prefers 'clients' over 'users'" |
| **Project Knowledge** | All users in project | Persistent | "Fiscal year ends June 30" |
| **Ontology** | All users in project | Persistent | "revenue_total is a measure column" |

### Database Schema

**File:** `migrations/011_memory_system.up.sql`

```sql
-- User-specific memory (terminology, preferences, personal definitions)
CREATE TABLE IF NOT EXISTS engine_user_memory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,  -- From JWT 'sub' claim

    -- Memory content
    memory_type TEXT NOT NULL,  -- 'terminology', 'preference', 'definition', 'correction'
    key TEXT NOT NULL,          -- The term or concept (e.g., "clients", "active")
    value TEXT NOT NULL,        -- The learned meaning (e.g., "refers to customers table", "logged in within 30 days")

    -- Learning metadata
    source TEXT NOT NULL,       -- 'clarification', 'correction', 'explicit', 'inferred'
    confidence FLOAT DEFAULT 1.0,
    usage_count INT DEFAULT 0,
    last_used_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    -- Constraints
    UNIQUE(project_id, user_id, memory_type, key)
);

-- Indexes
CREATE INDEX idx_user_memory_lookup
    ON engine_user_memory(project_id, user_id, key);
CREATE INDEX idx_user_memory_type
    ON engine_user_memory(project_id, user_id, memory_type);

-- RLS
ALTER TABLE engine_user_memory ENABLE ROW LEVEL SECURITY;
CREATE POLICY user_memory_isolation ON engine_user_memory
    USING (project_id = current_setting('app.current_project_id', true)::uuid);

-- Extend existing project_knowledge table with source tracking
ALTER TABLE engine_project_knowledge
    ADD COLUMN IF NOT EXISTS source TEXT DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS learned_from_user_id TEXT,
    ADD COLUMN IF NOT EXISTS learned_from_query TEXT,
    ADD COLUMN IF NOT EXISTS usage_count INT DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ;
```

**File:** `migrations/011_memory_system.down.sql`

```sql
DROP TABLE IF EXISTS engine_user_memory CASCADE;
ALTER TABLE engine_project_knowledge
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS learned_from_user_id,
    DROP COLUMN IF EXISTS learned_from_query,
    DROP COLUMN IF EXISTS usage_count,
    DROP COLUMN IF EXISTS last_used_at;
```

### Memory Types

#### 1. Terminology Memory
User-specific vocabulary mappings.

```go
type TerminologyMemory struct {
    UserTerm      string `json:"user_term"`      // What user says: "clients"
    CanonicalTerm string `json:"canonical_term"` // What ontology uses: "customers"
    EntityType    string `json:"entity_type"`    // "table", "column", "value"
    TableName     string `json:"table_name"`     // If column/value, which table
}

// Examples:
// - "clients" → "customers" (table)
// - "revenue" → "total_amount" (column in orders)
// - "premium" → "subscription_tier = 'premium'" (value)
```

#### 2. Definition Memory
User or project-level concept definitions.

```go
type DefinitionMemory struct {
    Term       string `json:"term"`       // "active user"
    Definition string `json:"definition"` // "logged in within last 30 days"
    SQLHint    string `json:"sql_hint"`   // "last_login_at > NOW() - INTERVAL '30 days'"
    Scope      string `json:"scope"`      // "user" or "project"
}

// Examples:
// - "active user" = "last_login_at > NOW() - INTERVAL '30 days'"
// - "high-value customer" = "lifetime_value > 10000"
// - "recent order" = "created_at > NOW() - INTERVAL '7 days'"
```

#### 3. Preference Memory
User-specific query preferences.

```go
type PreferenceMemory struct {
    PreferenceType string `json:"preference_type"` // "aggregation", "time_grain", "default_filter"
    Value          string `json:"value"`
}

// Examples:
// - aggregation: "weekly" (user prefers weekly over monthly)
// - time_grain: "day" (default to daily when unspecified)
// - default_filter: "exclude_test_accounts = true"
```

#### 4. Correction Memory
When user corrects a generated query.

```go
type CorrectionMemory struct {
    OriginalQuery   string `json:"original_query"`   // "show me client orders"
    GeneratedSQL    string `json:"generated_sql"`    // SELECT * FROM clients...
    CorrectedSQL    string `json:"corrected_sql"`    // SELECT * FROM customers...
    CorrectionType  string `json:"correction_type"`  // "wrong_table", "wrong_column", "wrong_filter"
    LearnedMapping  string `json:"learned_mapping"`  // "clients → customers"
}
```

### Memory Service (`pkg/services/memory_service.go`)

```go
type MemoryService struct {
    userMemoryRepo    UserMemoryRepository
    projectKnowledgeRepo ProjectKnowledgeRepository
    logger            *zap.Logger
}

// MemoryContext combines all memory tiers for query resolution
type MemoryContext struct {
    SessionContext   *SessionContext           `json:"session_context,omitempty"`
    UserMemories     []UserMemory              `json:"user_memories"`
    ProjectKnowledge []ProjectKnowledge        `json:"project_knowledge"`
}

// Resolveterm checks all memory tiers for a term's meaning
func (s *MemoryService) ResolveTerm(ctx context.Context, term string, userID string, projectID uuid.UUID) (*TermResolution, error) {
    resolution := &TermResolution{
        OriginalTerm: term,
        Resolved:     false,
    }

    // 1. Check session context (passed via context)
    if sessionCtx := getSessionContext(ctx); sessionCtx != nil {
        if def := sessionCtx.GetDefinition(term); def != "" {
            resolution.ResolvedMeaning = def
            resolution.Source = "session"
            resolution.Confidence = 1.0
            resolution.Resolved = true
            return resolution, nil
        }
    }

    // 2. Check user memory
    userMem, err := s.userMemoryRepo.FindByKey(ctx, projectID, userID, term)
    if err == nil && userMem != nil {
        resolution.ResolvedMeaning = userMem.Value
        resolution.Source = "user_memory"
        resolution.Confidence = userMem.Confidence
        resolution.Resolved = true

        // Update usage stats
        go s.userMemoryRepo.IncrementUsage(context.Background(), userMem.ID)
        return resolution, nil
    }

    // 3. Check project knowledge
    knowledge, err := s.projectKnowledgeRepo.FindByTerm(ctx, projectID, term)
    if err == nil && knowledge != nil {
        resolution.ResolvedMeaning = knowledge.Fact
        resolution.Source = "project_knowledge"
        resolution.Confidence = 0.9
        resolution.Resolved = true

        // Update usage stats
        go s.projectKnowledgeRepo.IncrementUsage(context.Background(), knowledge.ID)
        return resolution, nil
    }

    // 4. Not found in memory - will need ontology lookup or clarification
    resolution.Resolved = false
    return resolution, nil
}

type TermResolution struct {
    OriginalTerm    string  `json:"original_term"`
    Resolved        bool    `json:"resolved"`
    ResolvedMeaning string  `json:"resolved_meaning,omitempty"`
    Source          string  `json:"source,omitempty"` // "session", "user_memory", "project_knowledge", "ontology"
    Confidence      float64 `json:"confidence,omitempty"`
}
```

### Learning from Interactions

#### Learning from Clarifications

When user answers a clarification question, capture the knowledge:

```go
func (s *MemoryService) LearnFromClarification(
    ctx context.Context,
    projectID uuid.UUID,
    userID string,
    term string,
    userAnswer string,
    originalQuery string,
) error {
    // Determine if this is user-specific or project-wide knowledge
    scope := s.classifyScope(term, userAnswer)

    if scope == "user" {
        // User-specific terminology or preference
        return s.userMemoryRepo.Upsert(ctx, &UserMemory{
            ProjectID:  projectID,
            UserID:     userID,
            MemoryType: "definition",
            Key:        term,
            Value:      userAnswer,
            Source:     "clarification",
            Confidence: 0.9,
        })
    } else {
        // Project-wide knowledge (promote to project_knowledge)
        return s.projectKnowledgeRepo.Upsert(ctx, &ProjectKnowledge{
            ProjectID:         projectID,
            Fact:              fmt.Sprintf("%s means %s", term, userAnswer),
            Category:          "learned_definition",
            Source:            "clarification",
            LearnedFromUserID: userID,
            LearnedFromQuery:  originalQuery,
        })
    }
}

// classifyScope determines if a learned fact is user-specific or project-wide
func (s *MemoryService) classifyScope(term, answer string) string {
    // User-specific indicators:
    // - Terminology preferences ("I call them clients")
    // - Personal shortcuts ("when I say recent, I mean this week")

    // Project-wide indicators:
    // - Business definitions ("active user means...")
    // - Domain rules ("fiscal year ends...")
    // - Canonical meanings ("revenue includes...")

    userSpecificPatterns := []string{
        "I call", "I refer to", "I mean", "I prefer", "for me",
    }

    for _, pattern := range userSpecificPatterns {
        if strings.Contains(strings.ToLower(answer), pattern) {
            return "user"
        }
    }

    return "project" // Default to project-wide
}
```

#### Learning from Corrections

When user corrects a generated query:

```go
func (s *MemoryService) LearnFromCorrection(
    ctx context.Context,
    projectID uuid.UUID,
    userID string,
    originalQuery string,
    generatedSQL string,
    correctedSQL string,
) error {
    // Analyze what changed
    diff := s.analyzeCorrection(generatedSQL, correctedSQL)

    for _, change := range diff.Changes {
        switch change.Type {
        case "table_substitution":
            // User corrected table name: learn terminology
            s.userMemoryRepo.Upsert(ctx, &UserMemory{
                ProjectID:  projectID,
                UserID:     userID,
                MemoryType: "terminology",
                Key:        change.Original,
                Value:      fmt.Sprintf("refers to %s table", change.Corrected),
                Source:     "correction",
                Confidence: 0.95,
            })

        case "column_substitution":
            // User corrected column name
            s.userMemoryRepo.Upsert(ctx, &UserMemory{
                ProjectID:  projectID,
                UserID:     userID,
                MemoryType: "terminology",
                Key:        change.Original,
                Value:      fmt.Sprintf("refers to %s.%s", change.Table, change.Corrected),
                Source:     "correction",
                Confidence: 0.95,
            })

        case "filter_addition":
            // User added a filter: might be a preference
            s.userMemoryRepo.Upsert(ctx, &UserMemory{
                ProjectID:  projectID,
                UserID:     userID,
                MemoryType: "preference",
                Key:        "default_filter",
                Value:      change.Corrected,
                Source:     "correction",
                Confidence: 0.7,
            })
        }
    }

    return nil
}
```

#### Explicit Memory Commands (Optional API)

Allow users to explicitly add/manage memory:

```
POST /api/projects/{pid}/memory/user
{
    "type": "terminology",
    "key": "clients",
    "value": "refers to customers table"
}

GET /api/projects/{pid}/memory/user
→ Returns all user memories for current user

DELETE /api/projects/{pid}/memory/user/{id}
→ Delete a user memory

POST /api/projects/{pid}/knowledge
{
    "fact": "Fiscal year ends June 30",
    "category": "business_rule"
}
```

### Integration with Query Pipeline

Update ambiguity detection to check memory first:

```go
func (s *AmbiguityDetectorService) Detect(ctx context.Context, query string, userID string, projectID uuid.UUID) (*AmbiguityResult, error) {
    // Extract potentially ambiguous terms
    terms := s.extractAmbiguousTerms(query)

    result := &AmbiguityResult{
        Confidence:     1.0,
        AmbiguousTerms: []AmbiguousTerm{},
        Assumptions:    []Assumption{},
    }

    for _, term := range terms {
        // Check memory first
        resolution, err := s.memoryService.ResolveTerm(ctx, term.Text, userID, projectID)
        if err != nil {
            continue
        }

        if resolution.Resolved {
            // Term found in memory - add as assumption, not ambiguity
            result.Assumptions = append(result.Assumptions, Assumption{
                Term:       term.Text,
                Meaning:    resolution.ResolvedMeaning,
                Source:     resolution.Source,
                Confidence: resolution.Confidence,
            })
        } else {
            // Term not in memory - check ontology, then mark as ambiguous
            ontologyResolution := s.checkOntology(ctx, term.Text, projectID)
            if ontologyResolution != nil {
                result.Assumptions = append(result.Assumptions, Assumption{
                    Term:       term.Text,
                    Meaning:    ontologyResolution.Definition,
                    Source:     "ontology",
                    Confidence: 0.8,
                })
            } else {
                // Truly ambiguous - needs clarification
                result.AmbiguousTerms = append(result.AmbiguousTerms, term)
                result.Confidence -= 0.15
            }
        }
    }

    result.NeedsClarification = result.Confidence < 0.7
    return result, nil
}
```

Update prompt building to include memory context:

```go
func (s *Text2SQLService) buildPromptWithMemory(
    query string,
    linkingResult *LinkingResult,
    memoryContext *MemoryContext,
    fewShotResults []SearchResult,
) string {
    var prompt strings.Builder

    // ... existing sections ...

    // Section: User Context (from memory)
    if len(memoryContext.UserMemories) > 0 || len(memoryContext.ProjectKnowledge) > 0 {
        prompt.WriteString("# Context from Previous Interactions\n\n")

        if len(memoryContext.UserMemories) > 0 {
            prompt.WriteString("## This User's Terminology\n")
            for _, mem := range memoryContext.UserMemories {
                prompt.WriteString(fmt.Sprintf("- \"%s\" %s\n", mem.Key, mem.Value))
            }
            prompt.WriteString("\n")
        }

        if len(memoryContext.ProjectKnowledge) > 0 {
            prompt.WriteString("## Domain Definitions\n")
            for _, k := range memoryContext.ProjectKnowledge {
                prompt.WriteString(fmt.Sprintf("- %s\n", k.Fact))
            }
            prompt.WriteString("\n")
        }
    }

    // ... rest of prompt ...
}
```

### Memory Decay and Cleanup

Memories that aren't used should decay:

```go
// Background job: run daily
func (s *MemoryService) DecayUnusedMemories(ctx context.Context) error {
    // User memories unused for 90 days: reduce confidence by 10%
    // User memories with confidence < 0.3 and unused for 180 days: delete
    // Project knowledge is more stable: decay after 180 days unused

    return s.userMemoryRepo.DecayUnused(ctx, DecayConfig{
        DaysUntilDecay:     90,
        DecayAmount:        0.1,
        DaysUntilDelete:    180,
        MinConfidenceKeep:  0.3,
    })
}
```

### Files to Create for Memory System

**Database:**
- `migrations/011_memory_system.up.sql`
- `migrations/011_memory_system.down.sql`

**Repository:**
- `pkg/repositories/user_memory_repository.go`

**Service:**
- `pkg/services/memory_service.go`

**Models:**
- `pkg/models/memory.go` - UserMemory, TermResolution, MemoryContext

**Handlers:**
- `pkg/handlers/memory.go` - CRUD endpoints for user memory

**Background jobs:**
- `pkg/services/workqueue/memory_decay_job.go`

**Tests:**
- `pkg/services/memory_service_test.go`
- `pkg/repositories/user_memory_repository_test.go`

### Implementation Steps for Memory System

Add these steps:

#### Step 6: Memory System
- [ ] Create migration `011_memory_system.up.sql` with `engine_user_memory` table
- [ ] Create `pkg/repositories/user_memory_repository.go`
- [ ] Create `pkg/models/memory.go` with memory types
- [ ] Create `pkg/services/memory_service.go` with ResolveTerm, LearnFromClarification, LearnFromCorrection
- [ ] Integrate memory resolution into AmbiguityDetectorService
- [ ] Add memory context to prompt building
- [ ] Create memory CRUD API endpoints
- [ ] Create background job for memory decay
- [ ] Add tests for memory service

### Success Criteria for Memory System

- [ ] User terminology is captured from clarifications
- [ ] User corrections update user memory
- [ ] Memory resolution follows priority order (session → user → project → ontology)
- [ ] Learned definitions persist across sessions
- [ ] User-specific memories don't leak to other users
- [ ] Unused memories decay over time
- [ ] Memory improves query accuracy over time (measurable reduction in clarification requests)

### Key Design Decisions for Memory System

**Why separate user memory from project knowledge?**
- "Clients" meaning "customers" might be one user's preference, not company-wide
- User A's shortcuts shouldn't confuse User B
- Project knowledge is authoritative; user memory is personalization

**Why confidence scoring?**
- Not all learned facts are equally reliable
- Corrections (0.95) are more reliable than inferences (0.7)
- Decay unused memories rather than keeping everything forever

**Why check memory before ontology?**
- User's clarified definition should override generic ontology description
- "Active" means "last 30 days" per user clarification, even if ontology says "is_active = true"

**How to decide user vs. project scope?**
- Terminology preferences → user scope ("I call them clients")
- Business definitions → project scope ("active means logged in within 30 days")
- When unclear, start with user scope; admin can promote to project if widely applicable

## Key Design Decisions

### Why Two Vector Collections?
**"schema" collection:** Stores table metadata for schema linking
**"query_history" collection:** Stores (question, SQL) pairs for few-shot learning

**Rationale:** Different search purposes require different content. Schema search needs "find tables about X" while few-shot search needs "find similar questions".

### Why Similarity Threshold = 0.7 for Few-Shot?
High threshold ensures only high-quality examples are used. Bad examples hurt more than no examples.

**Empirical finding:** Score >= 0.7 means semantically similar questions. Score < 0.7 is often irrelevant.

### Why Top 5 Tables Instead of Top 10?
Token budget and accuracy tradeoff. 5 tables fit comfortably in LLM context with examples and syntax knowledge. 10 tables risk exceeding token limits and confusing the LLM.

**Adaptive strategy (future):** Could dynamically adjust based on question complexity.

### Why Async Storage of Query History?
Non-blocking - don't make user wait for embedding generation and storage. They just want their SQL result.

**Trade-off:** Eventual consistency. A query won't immediately become available as few-shot example. Acceptable for this use case.

### Why Embed Question Instead of SQL for Few-Shot?
We want to find similar *questions*, not similar SQL. User asks "top selling products" → we want examples of similar questions, regardless of SQL structure.

**Alternative considered:** Embed both question and SQL, search both. Rejected as overly complex for marginal benefit.

### Why Cosine Distance Instead of Euclidean or Dot Product?
Cosine distance is magnitude-invariant - measures direction, not length. Perfect for semantic similarity where "top products last month" and "best-selling items in April" should be close despite different word counts.

**Technical:** pgvector's `<=>` operator computes cosine distance. Score = 1 - distance for intuitive "higher is better" semantics.

## Integration with Connection Manager

**From PLAN-connection-manager.md:** Connection manager pools customer datasource connections by `(projectID, userID, datasourceID)` tuple.

**Text2SQL integration points:**

1. **Schema indexing** - When indexing schema, use connection manager to get pooled connection to customer database
2. **Metadata queries** - schemaRepo queries use connection manager (already implemented in connection manager plan)
3. **SQL execution** - Generated SQL executes via existing QueryService, which will use connection manager

**No direct text2sql→connection_manager dependency.** Text2SQL generates SQL, QueryService executes it using connection manager.

## Success Criteria

**Core Text2SQL (Original Plan):**
- [ ] Vector embeddings table created with pgvector extension
- [ ] Qwen embedding client generates 1024-dim vectors
- [ ] Redis caching reduces duplicate embedding API calls
- [ ] Schema indexing embeds all tables for a datasource
- [ ] Vector search finds relevant tables for natural language questions
- [ ] Query history storage enables few-shot learning
- [ ] Text2SQL generates syntactically correct SQL for PostgreSQL
- [ ] Generated SQL executes without errors on customer databases
- [ ] Confidence scores accurately reflect query quality
- [ ] System learns from successful queries (few-shot improves over time)
- [ ] Multi-tenant isolation (projects can't see each other's vectors)
- [ ] Integration tests pass end-to-end
- [ ] Knowledge files provide correct syntax guidance for PostgreSQL

**Enhanced Pattern Matching (from claudontology):**
- [ ] Multi-dimensional similarity scoring (keyword, entity, operation, structure, parameter) is implemented
- [ ] Pattern confidence increases after successful query execution
- [ ] Pattern confidence decays for unused patterns (daily job runs)
- [ ] Low-confidence old patterns are automatically removed
- [ ] High-confidence patterns (≥0.85 with ≥0.9 similarity) skip LLM generation and return cached SQL
- [ ] Pattern success/failure tracking influences future pattern selection

**Ambiguity Detection (from claudontology):**
- [ ] Ambiguous terms (temporal, quantitative, state) are detected before SQL generation
- [ ] Clarification requests are returned when confidence <0.7
- [ ] System proceeds with stated assumptions when 0.7 ≤ confidence <0.9
- [ ] Resolution hierarchy (session context → project knowledge → ontology → ask user) is implemented
- [ ] Users receive clear clarification questions with suggested options

**Session Context (from claudontology):**
- [ ] Session context is read from X-Session-Context header
- [ ] Session context filters (time focus, user segment, etc.) are injected into prompts
- [ ] Context applies to multiple queries without repetition
- [ ] Generated SQL respects session-level filters

**Ontology Integration (from claudontology):**
- [ ] Schema indexing uses ontology business names instead of raw table names
- [ ] Domain summary is included in LLM prompts
- [ ] Column semantic types (measure, dimension, identifier) guide SQL generation
- [ ] Table descriptions include domain classification and business descriptions
- [ ] Ontology column descriptions are used for ambiguous term resolution

## Future Enhancements (Out of Scope for Initial Implementation)

- **Multi-database support** - Extend to MSSQL, ClickHouse (knowledge files already exist)
- **Query optimization** - Suggest indexes, rewrite inefficient queries
- **Iterative refinement** - If query fails, use error message to regenerate
- **Semantic caching** - Cache SQL for semantically identical questions
- **User feedback loop** - Let users mark good/bad SQL, use as training signal
- **Adaptive schema selection** - Dynamically adjust top-K based on question complexity
- **Cross-datasource queries** - Generate SQL joining multiple datasources (requires federation layer)
- **SQL alternatives generation** - Generate 2-3 different SQL approaches for complex questions
- **Semantic syntax retrieval** - Index syntax patterns as vectors, retrieve relevant patterns per question (from ekaya-query)

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

## Glossary

**Vector embedding:** Numerical representation of text as 1024-dimensional array. Semantically similar texts have similar embeddings.

**Cosine distance:** Measure of similarity between vectors. Range [0, 2], where 0 = identical, 2 = opposite.

**Cosine similarity:** 1 - cosine_distance. Range [0, 1], where 1 = identical, 0 = orthogonal.

**Schema linking:** Finding relevant database tables for a natural language question using vector search.

**Few-shot learning:** Providing examples to LLM to guide output format and patterns.

**pgvector:** PostgreSQL extension enabling vector similarity search with specialized indexes (ivfflat, hnsw).

**ivfflat:** Inverted file with flat compression - approximate nearest neighbor index. Fast for medium-scale datasets.

**UPSERT:** INSERT or UPDATE - creates new row if not exists, updates if exists. `ON CONFLICT DO UPDATE` in PostgreSQL.

**RLS (Row Level Security):** PostgreSQL feature that filters rows based on user/tenant context. Ensures multi-tenant isolation.

**Chunk:** Unit of text stored in vector database. For text2sql: one chunk per table (schema) or one chunk per query (history).

**Collection:** Logical grouping of chunks. For text2sql: "schema" collection and "query_history" collection.

**Token budget:** Maximum number of tokens (words/subwords) an LLM can process in one request. Exceeding budget causes errors or truncation.

**Temperature:** LLM sampling parameter. 0 = deterministic (always same output), 1 = creative/random. Text2SQL uses 0 for consistency.

**Semantic mismatch:** Security signal where the complexity of generated SQL doesn't match the expected complexity for the user's question. Example: "count of users" producing SQL with 5 JOINs and window functions.

**Complexity score:** Numerical measure of SQL query complexity based on structural patterns (JOINs, subqueries, window functions, nesting depth). Used as a security feature.

**Tiered defense:** Security architecture with multiple layers of increasing cost and sophistication. Fast deterministic rules catch most attacks; expensive LLM verification handles edge cases.

**XGBoost:** Gradient boosted decision tree algorithm. Fast inference, handles feature interactions well, works with small training data. Used for ML classifier layer.

**SQL injection (SQLi):** Attack technique where malicious SQL is injected through user input. Patterns include UNION-based, stacked queries, comment injection, and encoding obfuscation.

**Feature vector:** Numerical representation of a SQL query's characteristics for ML classification. Includes structural features (JOIN count, nesting depth), token features, and anomaly flags.
