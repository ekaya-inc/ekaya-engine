# FIX: BUG-5 - Missing Critical Tikr Domain Knowledge

**Bug Reference:** plans/BUGS-ontology-extraction.md - BUG-5
**Severity:** High
**Category:** Domain Understanding

## Problem Summary

The ontology doesn't capture critical Tikr-specific business concepts:

| Concept | Details | Source |
|---------|---------|--------|
| **Tik** | 6 seconds of engagement time (billing unit) | `billing_helpers.go:413` |
| **Host vs Visitor** | Content creator vs viewer roles | Column naming (`host_id`, `visitor_id`) |
| **Fee Structure** | 4.5% platform fees, 30% Tikr share | `billing_helpers.go:373-379` |

Instead, the glossary contains generic SaaS terms like "Active Subscribers", "Churn Rate", "Average Order Value" that don't apply to Tikr's pay-per-use model.

## Root Cause

### 1. No Code Analysis Integration

**Problem:** Domain knowledge exists in code, but the system only sees database schema.

The system analyzes:
- ✓ Table/column names and types
- ✓ FK relationships (if database-level)
- ✓ Sample column values

The system does NOT analyze:
- ✗ Source code (constants, calculations)
- ✗ Documentation (README, ARCHITECTURE)
- ✗ Configuration files
- ✗ Comments

**Impact:** Business rules like `DurationPerTik = 6` and fee calculations are invisible to extraction.

### 2. Glossary Discovery Prompts Generic SaaS Metrics

**File:** `pkg/services/glossary_service.go:445-462`

The prompt explicitly suggests generic terms:
```
Common business term categories:
- Key Performance Indicators (KPIs)
- Financial metrics (revenue, costs, margins, GMV, AOV)
- User/customer metrics (active users, retention, churn, lifetime value)
- Transaction metrics (volume, value, conversion rates)
```

**Impact:** LLM generates "Active Subscribers" instead of "Engagement Revenue" or "Tik Duration".

### 3. Entity Roles Depend on Naming Conventions

**File:** `pkg/services/entity_discovery_task.go:247-296`

Role detection relies on column names:
- `visitor_id` → role = "visitor"
- `host_id` → role = "host"

**Impact:** Works for well-named columns but misses:
- Semantic meaning of roles (Host = content creator who earns money)
- Role implications for billing/reporting

### 4. Project Knowledge Requires Manual Entry

**File:** `pkg/mcp/tools/knowledge.go`

The `update_project_knowledge` MCP tool exists but:
- Requires explicit calls
- Never called automatically during extraction
- Knowledge captured in one session doesn't inform extraction in another

## Fix Implementation

### Short-Term: Knowledge Seeding Before Extraction

Allow project-level knowledge to be seeded before extraction runs.

#### 1. Add Knowledge Seeding Step to DAG

**File:** `pkg/services/dag/knowledge_seeding_node.go` (NEW)

```go
// New DAG node that runs BEFORE entity discovery
type KnowledgeSeedingNode struct {
    knowledgeRepo repositories.KnowledgeRepository
    projectRepo   repositories.ProjectRepository
}

func (n *KnowledgeSeedingNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
    // Check if project has seeded knowledge
    project, _ := n.projectRepo.GetByID(ctx, dag.ProjectID)

    if project.KnowledgeSeedPath != "" {
        // Load knowledge from configured path (YAML/JSON)
        facts := loadKnowledgeSeed(project.KnowledgeSeedPath)
        for _, fact := range facts {
            n.knowledgeRepo.Upsert(ctx, dag.ProjectID, fact)
        }
    }
    return nil
}
```

#### 2. Knowledge Seed File Format

```yaml
# .ekaya/knowledge.yaml
terminology:
  - fact: "A tik represents 6 seconds of engagement time"
    context: "Billing unit - from billing_helpers.go:413"

  - fact: "Host is a content creator who receives payments"
    context: "User role - identified by host_id columns"

  - fact: "Visitor is a viewer who pays for engagements"
    context: "User role - identified by visitor_id columns"

business_rules:
  - fact: "Platform fees are 4.5% of total amount"
    context: "billing_helpers.go:373"

  - fact: "Tikr share is 30% of amount after platform fees"
    context: "billing_helpers.go:375"

  - fact: "Host earns approximately 66.35% of total transaction"
    context: "Calculation: (1 - 0.045) * (1 - 0.30) = 0.6635"

conventions:
  - fact: "All monetary amounts are stored in cents (USD)"
    context: "Currency convention across billing tables"

  - fact: "Minimum capture amount is $1.00 (100 cents)"
    context: "MinCaptureAmount constant"
```

#### 3. Include Knowledge in Glossary Discovery Prompt

**File:** `pkg/services/glossary_service.go`

```go
func (s *glossaryService) buildSuggestTermsPrompt(...) string {
    // Fetch project knowledge
    facts, _ := s.knowledgeRepo.List(ctx, projectID)

    // Include in prompt context
    knowledgeSection := "DOMAIN KNOWLEDGE:\n"
    for _, fact := range facts {
        knowledgeSection += fmt.Sprintf("- %s (%s)\n", fact.Key, fact.FactType)
    }

    return fmt.Sprintf(`
%s

Based on the domain knowledge above and the database schema below,
suggest business glossary terms specific to this platform.

DO NOT suggest generic SaaS metrics unless they apply.
Focus on terms that reflect the actual business model.
...
`, knowledgeSection)
}
```

### Medium-Term: Auto-Discovery from Documentation

#### 1. Add Documentation Scanner

**File:** `pkg/services/knowledge_discovery.go` (NEW)

```go
type KnowledgeDiscovery struct {
    llmClient llm.Client
}

func (s *KnowledgeDiscovery) ScanDocumentation(ctx context.Context, repoPath string) ([]KnowledgeFact, error) {
    // Find documentation files
    files := glob(repoPath, "*.md", "docs/**/*.md", "README*")

    // Extract facts via LLM
    prompt := `Analyze the following documentation and extract domain-specific facts:
    - Business terminology (unique terms, abbreviations)
    - Business rules (calculations, thresholds, percentages)
    - User roles and their meanings
    - Conventions (currency, time zones, soft deletes)

    Format as JSON: [{"fact_type": "...", "fact": "...", "context": "..."}]`

    for _, file := range files {
        content := readFile(file)
        response := s.llmClient.Generate(ctx, prompt + content)
        facts = append(facts, parseFactsFromResponse(response)...)
    }
    return facts
}
```

#### 2. Optional Code Comment Analysis

For projects with well-commented code:
```go
func (s *KnowledgeDiscovery) ScanCodeComments(ctx context.Context, repoPath string) ([]KnowledgeFact, error) {
    // Find Go/TypeScript files
    files := glob(repoPath, "**/*.go", "**/*.ts")

    // Extract constants and commented business rules
    for _, file := range files {
        ast := parseFile(file)
        // Look for const blocks with comments
        // Look for functions with business rule comments
    }
}
```

### Long-Term: Domain-Specific Glossary Templates

#### 1. Add Industry Templates

**File:** `pkg/services/glossary_templates.go`

```go
var IndustryTemplates = map[string][]GlossaryTermSuggestion{
    "video_streaming": {
        {Term: "Watch Time", Definition: "Total time viewers spend watching content"},
        {Term: "Engagement Rate", Definition: "Ratio of active viewers to total viewers"},
    },
    "marketplace": {
        {Term: "GMV", Definition: "Gross Merchandise Value"},
        {Term: "Take Rate", Definition: "Platform commission percentage"},
    },
    "creator_economy": {  // Tikr fits here
        {Term: "Creator Earnings", Definition: "Amount paid to content creators"},
        {Term: "Platform Revenue", Definition: "Platform's share of transactions"},
        {Term: "Engagement Session", Definition: "Single interaction between creator and viewer"},
    },
}
```

#### 2. Project Industry Classification

Allow projects to specify their industry for template selection:
```sql
ALTER TABLE engine_projects
ADD COLUMN industry_type text DEFAULT 'general';
```

Then use industry templates as starting context for glossary discovery.

## Testing

1. Create knowledge seed file with Tikr facts
2. Configure project to use seed file
3. Run ontology extraction
4. Verify:
   - ✓ "Tik" appears in glossary
   - ✓ "Host" and "Visitor" roles captured in entity occurrences
   - ✓ Fee structure captured in project knowledge
   - ✗ Generic terms like "Churn Rate" NOT generated

## Acceptance Criteria

- [x] Knowledge seeding mechanism exists
- [x] Knowledge seed file format defined
- [x] Seeded knowledge included in glossary discovery prompt
- [x] Domain-specific terms generated instead of generic SaaS metrics
- [x] Host/Visitor roles captured with business meaning
- [ ] Fee structure documented in project knowledge
- [x] Documentation scanner extracts facts from README/docs
