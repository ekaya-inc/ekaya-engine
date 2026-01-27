# PLAN: Project Overview for Knowledge Seeding

## Overview

Add a required project overview textarea to the "Ready to Extract Ontology" screen. The user-provided overview will:
1. Be stored as project knowledge (key: `project_overview`, source: `manual`)
2. Persist across ontology deletions (repopulates the textarea on re-extraction)
3. Be processed by the KnowledgeSeeding DAG step to extract domain facts
4. Ground all downstream LLM prompts with business context

## Requirements

- **Textarea:** 20 character minimum, 500 character maximum
- **Validation:** "Start Extraction" button disabled until 20+ characters
- **Persistence:** Overview survives ontology deletion, repopulates on re-extraction
- **Processing:** KnowledgeSeeding node extracts facts from overview + schema context

---

## Implementation Tasks

### 1. Frontend: Add Overview Textarea to Empty State [x]

**File:** `ui/src/components/ontology/OntologyDAG.tsx`

**Changes:**

1. Add state for project overview:
```typescript
const [projectOverview, setProjectOverview] = useState('');
const [isLoadingOverview, setIsLoadingOverview] = useState(true);
```

2. Fetch existing overview on mount (for re-extraction scenario):
```typescript
// In init() effect, after checking DAG status:
const overviewResponse = await engineApi.getProjectOverview(projectId);
if (overviewResponse.data?.overview) {
  setProjectOverview(overviewResponse.data.overview);
}
setIsLoadingOverview(false);
```

3. Modify empty state (lines 506-522) to include textarea:
```typescript
if (!dagStatus) {
  return (
    <div className="rounded-lg border border-border-light bg-surface-primary p-12 shadow-sm">
      <div className="text-center">
        <Network className="h-16 w-16 text-purple-300 mx-auto mb-4" />
        <h2 className="text-xl font-semibold text-text-primary mb-2">
          Ready to Extract Ontology
        </h2>
        <p className="text-text-secondary max-w-md mx-auto mb-6">
          Before we analyze your schema, tell us about your application. Who uses it
          and what do they do with this data? This context helps build a more accurate
          business ontology.
        </p>

        {/* Overview textarea */}
        <div className="max-w-xl mx-auto mb-6 text-left">
          <label className="block text-sm font-medium text-text-primary mb-2">
            Describe your application
          </label>
          <textarea
            value={projectOverview}
            onChange={(e) => setProjectOverview(e.target.value.slice(0, 500))}
            placeholder="Example: This is our e-commerce platform for B2B wholesale. Customers are businesses that purchase products in bulk, while Users are employee accounts that manage orders..."
            className="w-full h-32 p-3 border border-border-light rounded-lg bg-surface-secondary text-text-primary placeholder:text-text-tertiary resize-none focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent"
            maxLength={500}
            disabled={isLoadingOverview}
          />
          <div className="flex justify-between mt-1 text-sm text-text-tertiary">
            <span>
              {projectOverview.length < 20
                ? `${20 - projectOverview.length} more characters required`
                : 'Ready to start'}
            </span>
            <span>{projectOverview.length}/500</span>
          </div>
        </div>

        {/* Button - disabled until 20 chars */}
        <Button
          onClick={() => void handleStart()}
          disabled={isStarting || projectOverview.length < 20}
          className="bg-purple-600 hover:bg-purple-700 text-white disabled:opacity-50"
        >
          ...
        </Button>
      </div>
    </div>
  );
}
```

4. Pass overview to start extraction:
```typescript
const handleStart = useCallback(async () => {
  // ... existing code ...
  await engineApi.startOntologyExtraction(projectId, datasourceId, projectOverview);
  // ...
}, [projectId, datasourceId, projectOverview, ...]);
```

---

### 2. Frontend: API Service Methods [x]

**File:** `ui/src/services/engineApi.ts`

**Add methods:**

```typescript
// Get project overview (for repopulating textarea)
async getProjectOverview(projectId: string): Promise<ApiResponse<{ overview: string | null }>> {
  return this.makeRequest<{ overview: string | null }>(
    `/${projectId}/project-knowledge/overview`
  );
}

// Modify startOntologyExtraction to accept overview
async startOntologyExtraction(
  projectId: string,
  datasourceId: string,
  projectOverview?: string
): Promise<ApiResponse<DAGStatusResponse>> {
  return this.makeRequest<DAGStatusResponse>(
    `/${projectId}/datasources/${datasourceId}/ontology/extract`,
    {
      method: 'POST',
      body: JSON.stringify({ project_overview: projectOverview }),
    }
  );
}
```

---

### 3. Backend: Handler - Accept Overview in Start Request [x]

**File:** `pkg/handlers/ontology_dag_handler.go`

**Add request type:**
```go
type StartExtractionRequest struct {
    ProjectOverview string `json:"project_overview"`
}
```

**Modify StartExtraction handler:**
```go
func (h *OntologyDAGHandler) StartExtraction(w http.ResponseWriter, r *http.Request) {
    projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
    if !ok {
        return
    }

    // Parse request body for project overview
    var req StartExtractionRequest
    if r.Body != nil {
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
            h.logger.Warn("Failed to parse request body, continuing without overview",
                zap.Error(err))
        }
    }

    // Pass overview to DAG service
    dag, err := h.dagService.Start(r.Context(), projectID, datasourceID, req.ProjectOverview)
    // ...
}
```

---

### 4. Backend: Handler - Get Overview Endpoint [x]

**File:** `pkg/handlers/knowledge_handler.go`

**Add endpoint:**
```go
// In RegisterRoutes:
mux.HandleFunc("GET "+base+"/overview",
    authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetOverview)))

// Handler method:
func (h *KnowledgeHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
    projectID, ok := ParseProjectID(w, r, h.logger)
    if !ok {
        return
    }

    facts, err := h.knowledgeService.GetByType(r.Context(), projectID, "overview")
    if err != nil {
        WriteError(w, http.StatusInternalServerError, "Failed to get overview")
        return
    }

    var overview *string
    for _, fact := range facts {
        if fact.Key == "project_overview" {
            overview = &fact.Value
            break
        }
    }

    WriteJSON(w, http.StatusOK, map[string]interface{}{
        "overview": overview,
    })
}
```

---

### 5. Backend: DAG Service - Pass Overview to KnowledgeSeeding [x]

**File:** `pkg/services/ontology_dag_service.go`

**Modify Start signature:**
```go
func (s *ontologyDAGService) Start(ctx context.Context, projectID, datasourceID uuid.UUID, projectOverview string) (*models.OntologyDAG, error) {
    // ... existing code ...

    // Store overview as project knowledge immediately (manual source)
    if projectOverview != "" {
        if err := s.knowledgeService.StoreWithSource(ctx, projectID, "overview", "project_overview", projectOverview, "", "manual"); err != nil {
            s.logger.Warn("Failed to store project overview", zap.Error(err))
            // Non-fatal - continue with extraction
        }
    }

    // ... rest of existing code ...
}
```

**Interface update:**
```go
type OntologyDAGService interface {
    Start(ctx context.Context, projectID, datasourceID uuid.UUID, projectOverview string) (*models.OntologyDAG, error)
    // ... other methods unchanged ...
}
```

---

### 6. Backend: Knowledge Service - StoreWithSource Method [x]

**File:** `pkg/services/knowledge.go`

**Add method to support explicit source:**
```go
// StoreWithSource creates or updates a knowledge fact with explicit source.
func (s *knowledgeService) StoreWithSource(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo, source string) (*models.KnowledgeFact, error) {
    var ontologyID *uuid.UUID
    ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
    if err == nil && ontology != nil {
        ontologyID = &ontology.ID
    }

    fact := &models.KnowledgeFact{
        ProjectID:  projectID,
        OntologyID: ontologyID,  // nil for manual overview - survives ontology deletion
        FactType:   factType,
        Key:        key,
        Value:      value,
        Context:    contextInfo,
        Source:     source,  // "manual" for user-entered, "inferred" for LLM-extracted
    }

    if err := s.repo.Upsert(ctx, fact); err != nil {
        return nil, err
    }
    return fact, nil
}
```

**Important:** For the project_overview fact, set `OntologyID = nil` so it survives ontology deletion (not subject to CASCADE delete).

---

### 7. Backend: KnowledgeSeeding Node - Extract Facts from Overview [x]

**File:** `pkg/services/dag/knowledge_seeding_node.go`

**Transform from no-op to active node:**

```go
type KnowledgeSeedingNode struct {
    *BaseNode
    knowledgeService services.KnowledgeService
    schemaService    services.SchemaService
    llmService       services.LLMService
}

func NewKnowledgeSeedingNode(
    dagRepo repositories.OntologyDAGRepository,
    knowledgeService services.KnowledgeService,
    schemaService services.SchemaService,
    llmService services.LLMService,
    logger *zap.Logger,
) *KnowledgeSeedingNode {
    return &KnowledgeSeedingNode{
        BaseNode:         NewBaseNode(models.DAGNodeKnowledgeSeeding, dagRepo, logger),
        knowledgeService: knowledgeService,
        schemaService:    schemaService,
        llmService:       llmService,
    }
}

func (n *KnowledgeSeedingNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
    n.Logger().Info("Starting knowledge seeding", zap.String("project_id", dag.ProjectID.String()))

    // 1. Get the project overview from knowledge table
    facts, err := n.knowledgeService.GetByType(ctx, dag.ProjectID, "overview")
    if err != nil {
        return fmt.Errorf("failed to get project overview: %w", err)
    }

    var projectOverview string
    for _, fact := range facts {
        if fact.Key == "project_overview" {
            projectOverview = fact.Value
            break
        }
    }

    if projectOverview == "" {
        n.Logger().Info("No project overview provided, skipping knowledge extraction")
        return n.ReportProgress(ctx, 1, 1, "No overview provided")
    }

    // 2. Get schema summary for context
    schema, err := n.schemaService.GetSchemaSummary(ctx, dag.ProjectID, dag.DatasourceID)
    if err != nil {
        n.Logger().Warn("Failed to get schema summary", zap.Error(err))
        // Continue without schema context
    }

    // 3. Use LLM to extract knowledge facts
    if err := n.ReportProgress(ctx, 0, 1, "Extracting domain knowledge from overview..."); err != nil {
        n.Logger().Warn("Failed to report progress", zap.Error(err))
    }

    extractedFacts, err := n.extractKnowledgeFacts(ctx, dag.ProjectID, projectOverview, schema)
    if err != nil {
        return fmt.Errorf("failed to extract knowledge facts: %w", err)
    }

    // 4. Store extracted facts
    for _, fact := range extractedFacts {
        if _, err := n.knowledgeService.StoreWithSource(
            ctx, dag.ProjectID, fact.FactType, fact.Key, fact.Value, fact.Context, "inferred",
        ); err != nil {
            n.Logger().Warn("Failed to store extracted fact",
                zap.String("key", fact.Key), zap.Error(err))
        }
    }

    n.Logger().Info("Knowledge seeding complete",
        zap.Int("facts_extracted", len(extractedFacts)))

    return n.ReportProgress(ctx, 1, 1, fmt.Sprintf("Extracted %d domain facts", len(extractedFacts)))
}

func (n *KnowledgeSeedingNode) extractKnowledgeFacts(
    ctx context.Context,
    projectID uuid.UUID,
    overview string,
    schema *models.SchemaSummary,
) ([]models.KnowledgeFact, error) {
    // Build prompt with overview and schema context
    // Ask LLM to extract:
    //   - business_rule facts (e.g., "All timestamps are UTC")
    //   - convention facts (e.g., "Currency amounts are in cents")
    //   - domain_term facts (e.g., "A 'channel' represents a video creator")
    //   - entity_hint facts (e.g., "Users and Customers are different concepts")

    // Return structured facts from LLM response
    // ...
}
```

---

### 8. Backend: Wire Dependencies for KnowledgeSeeding Node

**File:** `pkg/services/ontology_dag_service.go`

**Update node creation in getNodeExecutor:**
```go
case models.DAGNodeKnowledgeSeeding:
    node := dag.NewKnowledgeSeedingNode(
        s.dagRepo,
        s.knowledgeService,  // Add this dependency
        s.schemaService,     // Add this dependency
        s.llmService,        // Add this dependency
        s.logger,
    )
    return node, nil
```

**Update service struct and constructor to include these dependencies.**

---

## Data Flow Summary

```
┌─────────────────────────────────────────────────────────────────┐
│                        User enters overview                      │
│  "This is our B2B wholesale platform. Customers are businesses  │
│   that buy in bulk. Users are employee accounts..."              │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Click "Start Extraction"                      │
│  POST /api/projects/{pid}/datasources/{dsid}/ontology/extract   │
│  Body: { "project_overview": "..." }                            │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    DAG Service: Start()                          │
│  1. Store overview → engine_project_knowledge                   │
│     (fact_type="overview", key="project_overview", source="manual")│
│     (ontology_id=NULL so it survives deletion)                  │
│  2. Create DAG record                                           │
│  3. Start execution                                             │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                   KnowledgeSeeding Node                          │
│  1. Read overview from project_knowledge                        │
│  2. Get schema summary for context                              │
│  3. LLM extracts domain facts:                                  │
│     - business_rule: "Currency amounts are in cents"            │
│     - convention: "Timestamps are UTC"                          │
│     - domain_term: "Channel = video creator"                    │
│  4. Store facts → engine_project_knowledge (source="inferred")  │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              Continue to EntityDiscovery, etc.                   │
│  (All subsequent LLM calls can include project overview         │
│   and extracted facts as grounding context)                     │
└─────────────────────────────────────────────────────────────────┘
```

---

## File Changes Summary

| File | Action |
|------|--------|
| `ui/src/components/ontology/OntologyDAG.tsx` | Add textarea, validation, fetch overview |
| `ui/src/services/engineApi.ts` | Add `getProjectOverview`, modify `startOntologyExtraction` |
| `pkg/handlers/ontology_dag_handler.go` | Parse `project_overview` from request body |
| `pkg/handlers/knowledge_handler.go` | Add `GET .../project-knowledge/overview` endpoint |
| `pkg/services/ontology_dag_service.go` | Accept overview param, store as knowledge, wire dependencies |
| `pkg/services/knowledge.go` | Add `StoreWithSource` method |
| `pkg/services/dag/knowledge_seeding_node.go` | Implement LLM-based fact extraction |

---

## Testing

1. **Empty state with textarea:**
   - Navigate to Ontology page with no existing ontology
   - Verify textarea appears with character counter
   - Verify button disabled until 20+ characters

2. **Overview persistence:**
   - Start extraction with overview
   - Delete ontology
   - Return to Ontology page
   - Verify textarea pre-populated with previous overview

3. **Knowledge extraction:**
   - Start extraction with descriptive overview
   - Check `engine_project_knowledge` table for extracted facts
   - Verify overview stored with `source='manual'`
   - Verify extracted facts stored with `source='inferred'`

```sql
-- Verify knowledge was seeded
SELECT fact_type, key, LEFT(value, 50), source
FROM engine_project_knowledge
WHERE project_id = '<pid>'
ORDER BY created_at;
```
