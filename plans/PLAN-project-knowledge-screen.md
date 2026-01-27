# PLAN: Project Knowledge Tile and Screen

## Overview

Add a "Project Knowledge" tile and screen to manage domain facts learned during ontology refinement. The tile should appear in the Intelligence section after the Ontology tile. The screen follows the Glossary page pattern but without SQL statements.

## Data Model (Already Exists)

**Table:** `engine_project_knowledge`

| Field | Type | Purpose |
|-------|------|---------|
| `id` | uuid | Primary key |
| `project_id` | uuid | Project FK |
| `ontology_id` | uuid (nullable) | Optional link for CASCADE delete |
| `fact_type` | varchar(100) | Category (e.g., "business_rule", "convention") |
| `key` | varchar(255) | Fact identifier |
| `value` | text | Fact content |
| `context` | text (nullable) | Additional context |
| `source` | text | 'inferred', 'mcp', 'manual' |
| `last_edit_source` | text (nullable) | Source of last edit |
| `created_by` | uuid (nullable) | User who created |
| `updated_by` | uuid (nullable) | User who last updated |
| `created_at` | timestamptz | Creation time |
| `updated_at` | timestamptz | Last update time |

**Go Model:** `pkg/models/project_knowledge.go` - Already exists

---

## Implementation Tasks

### 1. Backend: Knowledge Handler ✓

**File:** `pkg/handlers/knowledge_handler.go` (new)

Create handler following the Glossary handler pattern:

```go
type KnowledgeListResponse struct {
    Facts []*models.ProjectKnowledge `json:"facts"`
    Total int                        `json:"total"`
}

type KnowledgeHandler struct {
    knowledgeService services.KnowledgeService
    logger           *zap.Logger
}
```

**Endpoints:**
| Method | Path | Handler Method |
|--------|------|----------------|
| GET | `/api/projects/{pid}/project-knowledge` | `List` |
| POST | `/api/projects/{pid}/project-knowledge` | `Create` |
| PUT | `/api/projects/{pid}/project-knowledge/{id}` | `Update` |
| DELETE | `/api/projects/{pid}/project-knowledge/{id}` | `Delete` |

**Wire in `main.go`** (after glossaryHandler registration ~line 420):
```go
knowledgeHandler := handlers.NewKnowledgeHandler(knowledgeService, logger)
knowledgeHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)
```

---

### 2. Frontend: TypeScript Types ✓

**File:** `ui/src/types/index.ts`

Add types:
```typescript
export interface ProjectKnowledge {
  id: string;
  project_id: string;
  ontology_id?: string;
  fact_type: string;
  key: string;
  value: string;
  context?: string;
  source: 'inferred' | 'mcp' | 'manual';
  last_edit_source?: string;
  created_by?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
}

export interface ProjectKnowledgeListResponse {
  facts: ProjectKnowledge[];
  total: number;
}

export interface CreateProjectKnowledgeRequest {
  fact_type: string;
  key: string;
  value: string;
  context?: string;
}

export interface UpdateProjectKnowledgeRequest {
  fact_type?: string;
  key?: string;
  value?: string;
  context?: string;
}
```

---

### 3. Frontend: API Service Methods ✓

**File:** `ui/src/services/engineApi.ts`

Add methods:
```typescript
async listProjectKnowledge(projectId: string): Promise<ApiResponse<ProjectKnowledgeListResponse>> {
  return this.makeRequest<ProjectKnowledgeListResponse>(`/${projectId}/project-knowledge`);
}

async createProjectKnowledge(
  projectId: string,
  data: CreateProjectKnowledgeRequest
): Promise<ApiResponse<ProjectKnowledge>> {
  return this.makeRequest<ProjectKnowledge>(`/${projectId}/project-knowledge`, {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

async updateProjectKnowledge(
  projectId: string,
  id: string,
  data: UpdateProjectKnowledgeRequest
): Promise<ApiResponse<ProjectKnowledge>> {
  return this.makeRequest<ProjectKnowledge>(`/${projectId}/project-knowledge/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

async deleteProjectKnowledge(projectId: string, id: string): Promise<ApiResponse<void>> {
  return this.makeRequest<void>(`/${projectId}/project-knowledge/${id}`, {
    method: 'DELETE',
  });
}
```

---

### 4. Frontend: Project Knowledge Page ✓

**File:** `ui/src/pages/ProjectKnowledgePage.tsx` (new)

Use `GlossaryPage.tsx` as template with these differences:

1. **No SQL fields** - Remove SQL-related columns and editor fields
2. **Show fact_type** - Display as a badge/tag
3. **Show source** - Display provenance (inferred/mcp/manual) as a badge
4. **Group by fact_type** - Optional: group facts by type for better organization

**Page Layout:**
```
┌─────────────────────────────────────────────────────────┐
│ ← Back to Dashboard                      [+ Add Fact]   │
├─────────────────────────────────────────────────────────┤
│ Project Knowledge                                       │
│ Domain facts and business rules learned during          │
│ ontology refinement                                     │
├─────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────┐ │
│ │ [business_rule]  [inferred]                    Edit │ │
│ │ Key: timezone_convention                     Delete │ │
│ │ Value: All timestamps are stored in UTC            │ │
│ │ Context: Inferred from channel_created_at column   │ │
│ └─────────────────────────────────────────────────────┘ │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ [convention]  [manual]                         Edit │ │
│ │ Key: currency_code                           Delete │ │
│ │ Value: Amounts are in cents (USD)                  │ │
│ └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

**State Management:**
- `facts: ProjectKnowledge[]`
- `loading: boolean`
- `error: string | null`
- `editorOpen: boolean`
- `editingFact: ProjectKnowledge | null`

---

### 5. Frontend: Project Knowledge Editor Component ✓

**File:** `ui/src/components/ProjectKnowledgeEditor.tsx` (new)

Modal form for creating/editing facts:

**Fields:**
| Field | Type | Required | Notes |
|-------|------|----------|-------|
| Fact Type | Select/Input | Yes | Dropdown with common types + custom option |
| Key | Text input | Yes | Identifier for the fact |
| Value | Textarea | Yes | The actual fact content |
| Context | Textarea | No | Optional additional context |

**Common Fact Types (for dropdown):**
- `business_rule` - Business rules and constraints
- `convention` - Naming or data conventions
- `domain_term` - Domain-specific terminology
- `relationship` - Entity relationship rules
- `custom` - User-defined type

---

### 6. Frontend: Add Route

**File:** `ui/src/App.tsx`

Add route after glossary (around line 48):
```typescript
<Route path="project-knowledge" element={<ProjectKnowledgePage />} />
```

---

### 7. Frontend: Add Tile to Dashboard

**File:** `ui/src/pages/ProjectDashboard.tsx`

Add tile to Intelligence section (after Ontology tile, before Entities):

```typescript
// In intelligenceTiles array (around line 408)
{
  title: 'Project Knowledge',
  icon: Brain,  // or Lightbulb from lucide-react
  path: 'project-knowledge',
  disabled: !hasOntology,
  color: 'cyan' as TileColor,  // or 'indigo' to differentiate
},
```

**Tile Order in Intelligence Section:**
1. Ontology
2. **Project Knowledge** (new)
3. Entities
4. Relationships
5. Glossary

---

## File Changes Summary

| File | Action |
|------|--------|
| `pkg/handlers/knowledge_handler.go` | Create |
| `main.go` | Wire handler |
| `ui/src/types/index.ts` | Add types |
| `ui/src/services/engineApi.ts` | Add API methods |
| `ui/src/pages/ProjectKnowledgePage.tsx` | Create |
| `ui/src/components/ProjectKnowledgeEditor.tsx` | Create |
| `ui/src/App.tsx` | Add route |
| `ui/src/pages/ProjectDashboard.tsx` | Add tile |

---

## Testing

1. **Backend API:**
   - `curl http://localhost:3443/api/projects/{pid}/project-knowledge` (list)
   - Create/update/delete via API

2. **UI Flow:**
   - Navigate to dashboard, verify tile appears after Ontology
   - Click tile, verify page loads with facts list
   - Create new fact, verify it appears
   - Edit fact, verify changes persist
   - Delete fact, verify removal

3. **MCP Sync (when fixed):**
   - Create fact via MCP, verify appears in UI
   - Edit fact in UI, verify via MCP
