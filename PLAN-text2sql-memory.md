# PLAN: Text2SQL - Memory System Architecture

> **Navigation:** [Overview](PLAN-text2sql-overview.md) | [Vector Infrastructure](PLAN-text2sql-vector.md) | [Service](PLAN-text2sql-service.md) | [Enhancements](PLAN-text2sql-enhancements.md) | [Security](PLAN-text2sql-security.md) | [Ontology Linking](PLAN-text2sql-ontology-linking.md) | [Memory System](PLAN-text2sql-memory.md) | [Implementation](PLAN-text2sql-implementation.md)

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
