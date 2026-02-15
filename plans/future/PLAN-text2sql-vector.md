# PLAN: Text2SQL - Vector Infrastructure

> **Navigation:** [Overview](PLAN-text2sql-overview.md) | [Vector Infrastructure](PLAN-text2sql-vector.md) | [Service](PLAN-text2sql-service.md) | [Enhancements](PLAN-text2sql-enhancements.md) | [Security](PLAN-text2sql-security.md) | [Ontology Linking](PLAN-text2sql-ontology-linking.md) | [Memory System](PLAN-text2sql-memory.md) | [Implementation](PLAN-text2sql-implementation.md)

## Database Schema (Migration)

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

## Vector Infrastructure (`pkg/vector/`)

**Why separate from services layer?** Vector operations (embedding generation, similarity search) are infrastructure concerns that multiple services might use. Keeping them in `pkg/vector/` maintains clean separation of concerns.

### `pkg/vector/interface.go`

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

### `pkg/vector/qwen_client.go`

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

### `pkg/vector/embedding_cache.go`

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

### `pkg/vector/pgvector_store.go`

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

### `pkg/vector/schema_indexer.go`

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
