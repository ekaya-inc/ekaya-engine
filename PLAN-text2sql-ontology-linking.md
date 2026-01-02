# PLAN: Text2SQL - Ontology Linking Architecture

> **Navigation:** [Overview](PLAN-text2sql-overview.md) | [Vector Infrastructure](PLAN-text2sql-vector.md) | [Service](PLAN-text2sql-service.md) | [Enhancements](PLAN-text2sql-enhancements.md) | [Security](PLAN-text2sql-security.md) | [Ontology Linking](PLAN-text2sql-ontology-linking.md) | [Memory System](PLAN-text2sql-memory.md) | [Implementation](PLAN-text2sql-implementation.md)

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

