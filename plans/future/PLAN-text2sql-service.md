# PLAN: Text2SQL - Service Layer

> **Navigation:** [Overview](PLAN-text2sql-overview.md) | [Vector Infrastructure](PLAN-text2sql-vector.md) | [Service](PLAN-text2sql-service.md) | [Enhancements](PLAN-text2sql-enhancements.md) | [Security](PLAN-text2sql-security.md) | [Ontology Linking](PLAN-text2sql-ontology-linking.md) | [Memory System](PLAN-text2sql-memory.md) | [Implementation](PLAN-text2sql-implementation.md)

## Text2SQL Service (`pkg/services/text2sql.go`)

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

## Knowledge Files

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
