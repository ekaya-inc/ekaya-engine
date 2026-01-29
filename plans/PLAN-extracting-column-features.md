# PLAN: Extracting Column Features (DAG Stage 2)

## Context

The ontology extraction DAG currently has pattern detection code scattered across multiple stages, using hardcoded string comparisons (`_amount`, `_at`, `is_*`, etc.) to classify columns. This approach is fundamentally flawed:

1. **Static suffixes assume English naming conventions** - Different teams use different conventions
2. **Column names lie** - A column named `deleted_at` might not be a soft delete; a soft delete column might be named `removed_timestamp`
3. **Data doesn't lie** - A column with 95% NULL values and timestamps in the non-NULL rows IS a soft delete, regardless of name

**This plan eliminates ALL static string pattern matching on column names.** Column names are passed to the LLM as context, but ALL classification decisions are driven by:
- Data type
- Sample values
- Statistical distributions
- Cross-column correlations

## Design Principles

### Many Small LLM Requests > One Big Request

Even if a single large prompt has lower wall-clock time, **many small requests provide better UX**:

| Aspect | Large Prompt | Small Prompts |
|--------|--------------|---------------|
| User Experience | Spinner for 180s 😫 | Progress updates every 2-3s 🎉 |
| Context Usage | Token-stuffed, unfocused | Precisely scoped |
| Output Validation | Complex parsing, partial failures | Simple JSON, easy validation |
| Retry Cost | Re-run entire prompt | Re-run single column |
| Throughput | Sequential bottleneck | Parallel execution |

**Design rule:** Each LLM request classifies ONE column with ONE focused question.

### Deterministic Task Enumeration

At the start of each phase, we **count tasks deterministically** before processing:

```
Phase 1: Collecting column data...
         Found 38 columns across 12 tables

Phase 2: Classifying columns (0/38)
         → Classifying columns (12/38)
         → Classifying columns (38/38) ✓

Phase 3: Analyzing enum candidates (0/7)
         → Analyzing enum candidates (7/7) ✓

Phase 4: Resolving FK candidates (0/4)
         → Resolving FK candidates (4/4) ✓

Phase 5: Detecting monetary pairs (0/2)
         → Detecting monetary pairs (2/2) ✓
```

The UI shows **separate progress bars for each phase**, not one expanding counter.

### Task Queues Between Phases

Each phase enqueues tasks for the next phase based on classification results:

```
┌────────────────────────────────────────────────────────────────────┐
│ Phase 2: Column Classification                                      │
│ Input: 38 columns                                                   │
│ Output: Enqueue tasks for subsequent phases:                       │
│   - 7 columns → Enum Analysis Queue                                │
│   - 4 columns → FK Resolution Queue                                │
│   - 2 tables  → Monetary Pair Detection Queue                      │
│   - 3 columns → Soft Delete Validation Queue                       │
└────────────────────────────────────────────────────────────────────┘
```

## Design: Data-Driven Mini-DAG with Task Queues

The Column Feature Extraction stage runs a mini-DAG with **deterministic task enumeration** at each phase. Each phase completes before the next begins, and task counts are known upfront.

```
┌─────────────────────────────────────────────────────────────────────┐
│ PHASE 1: Data Collection (Deterministic, No LLM)                     │
│ UI: "Collecting column metadata..."                                  │
│                                                                      │
│  1. Query schema for all columns                                    │
│  2. For each column, gather: type, stats, samples                   │
│  3. Run regex patterns against sample values                        │
│  4. Determine classification path for each column                   │
│                                                                      │
│  Output: Column profiles with paths assigned                        │
│  Enqueue: All columns → Phase 2 queue                               │
│                                                                      │
│  UI Update: "Found 38 columns in 12 tables"                         │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│ PHASE 2: Column Classification (Parallel LLM, 1 request/column)     │
│ UI: "Classifying columns (0/38)"                                    │
│                                                                      │
│  For each column (parallel, up to N workers):                       │
│    - Build path-specific prompt (timestamp/boolean/enum/etc)        │
│    - Send focused LLM request for THIS column only                  │
│    - Parse response, store features                                 │
│    - Update progress: (1/38), (2/38), ...                           │
│                                                                      │
│  Based on classification results, enqueue follow-up tasks:          │
│    - Columns with low cardinality → Enum Analysis Queue             │
│    - Columns classified as identifiers → FK Resolution Queue        │
│    - Tables with numeric + currency columns → Monetary Queue        │
│    - Timestamps with high null rate → Soft Delete Queue             │
│                                                                      │
│  UI Update: "Classified 38 columns. Found 7 enums, 4 FK candidates" │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│ PHASE 3: Enum Value Analysis (Parallel LLM, 1 request/enum column)  │
│ UI: "Analyzing enum values (0/7)"                                   │
│                                                                      │
│  For each enum column (parallel):                                   │
│    - Query value distribution                                       │
│    - Correlate with timestamp columns (state machine detection)     │
│    - Send LLM request: "What do these values mean?"                 │
│    - Parse response, store enum labels                              │
│    - Update progress: (1/7), (2/7), ...                             │
│                                                                      │
│  UI Update: "Labeled 7 enum columns"                                │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│ PHASE 4: FK Target Resolution (Parallel LLM, 1 request/candidate)   │
│ UI: "Resolving FK relationships (0/4)"                              │
│                                                                      │
│  For each FK candidate (parallel):                                  │
│    - Run data overlap queries against potential targets             │
│    - Send LLM request: "Which target is most likely?"               │
│    - Parse response, store FK hints                                 │
│    - Update progress: (1/4), (2/4), ...                             │
│                                                                      │
│  UI Update: "Resolved 4 FK candidates"                              │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│ PHASE 5: Cross-Column Analysis (Parallel LLM, 1 request/table)      │
│ UI: "Analyzing column relationships (0/3)"                          │
│                                                                      │
│  For each table with candidates (parallel):                         │
│    - Monetary pairing: Which amounts pair with which currency?     │
│    - Soft delete validation: Is this really a soft delete?         │
│    - Send focused LLM requests                                      │
│    - Update progress: (1/3), (2/3), ...                             │
│                                                                      │
│  UI Update: "Completed cross-column analysis"                       │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│ PHASE 6: Store Results (Deterministic, No LLM)                       │
│ UI: "Saving column features..."                                      │
│                                                                      │
│  Persist all features to schema repository                          │
│                                                                      │
│  UI Update: "Column feature extraction complete"                    │
└─────────────────────────────────────────────────────────────────────┘
```

### Routing Logic (Deterministic, No LLM)

Columns are routed to classification paths based on TYPE + DATA, not names:

```
TIMESTAMP/DATE type ──────────────────────► Timestamp Path
BOOLEAN type ─────────────────────────────► Boolean Path
INTEGER/BIGINT type ──┬── samples are 0,1 ► Boolean Path
                      ├── samples are Unix timestamps ► Timestamp Path
                      ├── low cardinality ► Enum Path
                      └── high cardinality ► Numeric Path
TEXT/VARCHAR type ────┬── samples match UUID ► UUID Path
                      ├── samples match external ID ► ExtID Path
                      ├── low cardinality ► Enum Path
                      └── high cardinality ► Text Path
UUID type ────────────────────────────────► UUID Path
JSONB/JSON type ──────────────────────────► JSON Path
```

## Classification Paths

### Path A: Timestamp Classification

**Entry criteria (data-driven):**
- Type is `timestamp`, `timestamptz`, `date`, `datetime`
- OR: Type is `bigint` AND sample values are valid Unix timestamps (10-19 digits, convert to reasonable dates 1970-2100)

**Data collected:**
```sql
SELECT
    COUNT(*) as total_rows,
    COUNT(column) as non_null_count,
    COUNT(*) - COUNT(column) as null_count,
    100.0 * (COUNT(*) - COUNT(column)) / COUNT(*) as null_rate
FROM table;
```

**Branching within path:**
| Null Rate | Interpretation | LLM Prompt Focus |
|-----------|----------------|------------------|
| 90-100% NULL | Likely soft delete or optional event | "When is this timestamp populated?" |
| 0-5% NULL | Likely required audit/event field | "What event does this timestamp capture?" |
| 5-90% NULL | Conditional timestamp | "Under what conditions is this populated?" |

**For bigint timestamps, detect scale:**
| Sample Value Length | Scale | Validation |
|--------------------|-------|------------|
| 10 digits | Seconds | Convert to date, check 1970-2100 |
| 13 digits | Milliseconds | Convert to date, check 1970-2100 |
| 16 digits | Microseconds | Convert to date, check 1970-2100 |
| 19 digits | Nanoseconds | Convert to date, check 1970-2100 |

**LLM prompt includes:**
- Column name, table name
- Null rate
- Sample values (converted to human-readable dates)
- Other timestamp columns in same table (for context)

**LLM determines:**
- Purpose: `audit_created`, `audit_updated`, `soft_delete`, `event_time`, `scheduled_time`, `expiration`, `cursor`
- Business description

---

### Path B: Boolean Classification

**Entry criteria (data-driven):**
- Type is `boolean`
- OR: Type is `integer`/`smallint` AND distinct values are exactly {0, 1} or {0} or {1}
- OR: Type is `text` AND distinct values are exactly {true, false}, {yes, no}, {Y, N}, {T, F} (case-insensitive)

**Data collected:**
```sql
SELECT
    column_value,
    COUNT(*) as count,
    100.0 * COUNT(*) / SUM(COUNT(*)) OVER () as percentage
FROM table
GROUP BY column_value;
```

**LLM prompt includes:**
- Column name, table name
- True/false distribution percentages
- Other columns in table (for context)

**LLM determines:**
- What does `true` mean for this column?
- What does `false` mean?
- Is this a feature flag, status indicator, or permission?

---

### Path C: Enum/State Classification

**Entry criteria (data-driven):**
- Distinct count ≤ 50 (configurable threshold)
- Cardinality ratio < 0.01 (less than 1% of rows have unique values)
- NOT already classified as boolean

**Data collected:**
```sql
-- Value distribution
SELECT
    column_value,
    COUNT(*) as count,
    100.0 * COUNT(*) / SUM(COUNT(*)) OVER () as percentage
FROM table
GROUP BY column_value
ORDER BY count DESC;

-- State correlation (if table has timestamp columns)
SELECT
    column_value,
    COUNT(*) as total,
    COUNT(completed_at) as has_completion,
    100.0 * COUNT(completed_at) / COUNT(*) as completion_rate
FROM table
GROUP BY column_value;
```

**Branching within path:**

If correlation analysis shows state-like behavior (some values have 0% completion, others have ~100%):
→ Send to State Machine LLM prompt

Otherwise:
→ Send to General Enum LLM prompt

**State Machine LLM prompt includes:**
- Column name, table name
- Value distribution with counts/percentages
- Correlation with timestamp columns
- Values with 0% completion (likely initial/pending states)
- Values with ~100% completion (likely terminal states)
- Values with <1% frequency (likely error/edge cases)

**LLM determines:**
- Label for each value (e.g., 0 → "pending", 1 → "approved", 2 → "rejected")
- State machine description (initial → intermediate → terminal)
- Whether values represent a workflow or just categories

---

### Path D: UUID Identifier Classification

**Entry criteria (data-driven):**
- Type is `uuid`
- OR: Type is `text`/`varchar`/`char` AND >95% of sample values match UUID regex

**UUID regex:** `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$` (case-insensitive)

**Data collected:**
- Is this column a primary key?
- Is this column unique?
- Cardinality ratio

**Branching within path:**
| Characteristic | Interpretation |
|----------------|----------------|
| Is PK | This is the entity's identifier |
| Is Unique, not PK | Alternate key or external reference |
| High cardinality, not unique | Likely FK to another entity |

**For likely FK columns, chain to FK Target Resolution:**
→ Run data overlap analysis against PK columns of other tables
→ Return top candidates with match rates

**LLM prompt includes:**
- Column name, table name
- PK/Unique status
- Cardinality
- FK candidates (if overlap analysis was run)

**LLM determines:**
- What entity does this identify?
- Is this an internal ID or external reference?
- For FKs: which target table is most likely?

---

### Path E: External ID Classification

**Entry criteria (data-driven):**
- Type is `text`/`varchar`
- Sample values match known external ID patterns (detected by regex, NOT by column name)

**Pattern detection on sample values:**
| Pattern (Regex) | Service |
|-----------------|---------|
| `^(pi_\|pm_\|ch_\|cus_\|sub_\|inv_\|price_)[a-zA-Z0-9]+$` | Stripe |
| `^[0-9a-f-]+@email\.amazonses\.com$` | AWS SES |
| `^(AC\|SM\|MM\|PN\|SK)[a-f0-9]{32}$` | Twilio |
| `^[a-zA-Z0-9_-]{20,}$` + high cardinality | Generic external ID |

**LLM prompt includes:**
- Column name, table name
- Detected pattern and service (if known)
- Sample values
- Cardinality

**LLM determines:**
- Confirm or override service detection
- What external entity does this reference?
- Business context for this integration

---

### Path F: Numeric Classification (Integer/Bigint/Numeric)

**Entry criteria (data-driven):**
- Type is `integer`, `bigint`, `smallint`, `numeric`, `decimal`, `float`, `double`
- NOT already routed to Boolean or Timestamp paths

**Data collected:**
```sql
SELECT
    MIN(column) as min_val,
    MAX(column) as max_val,
    AVG(column) as avg_val,
    COUNT(DISTINCT column) as distinct_count,
    COUNT(*) as row_count
FROM table;
```

**Branching within path:**

| Characteristic | Route |
|----------------|-------|
| Low cardinality (≤50 distinct) | → Enum Path |
| Is PK or Unique | → Identifier classification |
| Same table has currency-like column | → Monetary analysis |
| Otherwise | → General numeric (measure/attribute) |

**Monetary analysis (cross-column correlation):**

Detect currency column by DATA, not name:
```sql
-- Find text columns with low cardinality and ISO 4217-like values
SELECT column_name
FROM information_schema.columns c
JOIN (
    SELECT column_name,
           COUNT(DISTINCT column_value) as distinct_count,
           bool_and(column_value ~ '^[A-Z]{3}$') as all_match_iso
    FROM sample_values
    GROUP BY column_name
) stats USING (column_name)
WHERE c.table_name = $1
  AND c.data_type IN ('text', 'varchar', 'char')
  AND stats.distinct_count BETWEEN 3 AND 200
  AND stats.all_match_iso = true;
```

If currency column found, this numeric column may be monetary.

**LLM prompt for monetary:**
- Column name, table name
- Detected currency column and its values
- Sample numeric values
- Min/max/avg statistics

**LLM determines:**
- Is this a monetary amount?
- What unit? (cents, dollars, basis points)
- What does this amount represent?

---

### Path G: Text Classification

**Entry criteria (data-driven):**
- Type is `text`, `varchar`, `char`
- NOT already routed to UUID, External ID, or Enum paths

**Data collected:**
- Min/max string length
- Cardinality ratio
- Sample values

**Branching within path:**
| Characteristic | Interpretation |
|----------------|----------------|
| Max length < 10, low cardinality | Possible code/abbreviation |
| Max length > 1000 | Likely free text/description |
| Contains structured patterns (JSON, XML, email) | Structured text |

**LLM prompt includes:**
- Column name, table name
- Length statistics
- Sample values
- Cardinality

**LLM determines:**
- What kind of text is this?
- Is it structured or free-form?
- Business meaning

---

## Cross-Column Analysis

Some classifications require analyzing relationships between columns in the same table. These run as a separate phase after individual column classification:

### Monetary Pairing

**Trigger:** Table has both:
- A numeric column classified as potential monetary
- A text column with ISO 4217 currency codes

**Analysis:**
- Validate the pairing makes sense (e.g., not a percentage paired with currency)
- Check if multiple amount columns share one currency column

**LLM prompt:**
- List of numeric columns with their stats
- Currency column with its distinct values
- Ask: which numeric columns are monetary amounts paired with this currency?

### Soft Delete Detection

**Trigger:** Table has a timestamp column with >90% NULL values

**Analysis:**
- Check if non-NULL values correlate with missing/inactive records
- Look for other "active" indicators in the table

**LLM prompt:**
- Timestamp column name and null rate
- Sample of records with NULL vs non-NULL values
- Ask: is this a soft delete timestamp? What does a non-NULL value indicate?

### FK Relationship Hints

**Trigger:** Column classified as identifier (UUID or integer) with high cardinality, not PK

**Analysis:**
Run data overlap against potential target tables:
```sql
SELECT
    target_table,
    COUNT(DISTINCT s.column_value) as matched,
    (SELECT COUNT(DISTINCT column_value) FROM source) as total,
    100.0 * COUNT(DISTINCT s.column_value) /
        (SELECT COUNT(DISTINCT column_value) FROM source) as match_rate
FROM source_values s
JOIN target_pk t ON s.column_value = t.pk_value
GROUP BY target_table
ORDER BY match_rate DESC
LIMIT 5;
```

**Output:** FK candidate hints for downstream FK Discovery stage

---

## Implementation Tasks

### Task 1: Define Data Models and Task Queues ✓

**File:** `pkg/models/column_features.go` (new)

```go
// ColumnDataProfile holds raw data collected for a column (Phase 1 output)
type ColumnDataProfile struct {
    ColumnID      uuid.UUID
    ColumnName    string
    TableName     string
    DataType      string

    // From DDL
    IsPrimaryKey  bool
    IsUnique      bool
    IsNullable    bool

    // Statistics
    RowCount      int64
    DistinctCount int64
    NullCount     int64
    NullRate      float64      // null_count / row_count
    Cardinality   float64      // distinct_count / row_count

    // For numeric columns
    MinValue      *float64
    MaxValue      *float64
    AvgValue      *float64

    // For text columns
    MinLength     *int64
    MaxLength     *int64

    // Sample values (up to 50 distinct)
    SampleValues  []string

    // Pattern detection results (from sample analysis)
    DetectedPatterns []DetectedPattern

    // Routing decision (determined in Phase 1, processed in Phase 2)
    ClassificationPath string // "timestamp", "boolean", "enum", "uuid", "external_id", "numeric", "text"
}

// DetectedPattern represents a regex pattern match on sample values
type DetectedPattern struct {
    PatternName   string   // e.g., "uuid", "stripe_id", "iso4217_currency"
    MatchRate     float64  // percentage of samples matching
    MatchedValues []string // examples of matched values
}

// ColumnFeatures holds the final classification results
type ColumnFeatures struct {
    ColumnID       uuid.UUID

    // Classification path taken
    ClassificationPath string

    // Common fields
    Purpose        string  // "identifier", "timestamp", "flag", "measure", "enum", "text", "json"
    SemanticType   string  // more specific: "soft_delete_timestamp", "currency_cents", etc.
    Role           string  // "primary_key", "foreign_key", "attribute", "measure"
    Description    string  // LLM-generated business description
    Confidence     float64 // 0.0-1.0

    // Path-specific results (populated based on classification)
    TimestampFeatures  *TimestampFeatures
    BooleanFeatures    *BooleanFeatures
    EnumFeatures       *EnumFeatures
    IdentifierFeatures *IdentifierFeatures
    MonetaryFeatures   *MonetaryFeatures

    // Flags for follow-up phases (set during Phase 2)
    NeedsEnumAnalysis     bool  // → enqueue to Phase 3
    NeedsFKResolution     bool  // → enqueue to Phase 4
    NeedsCrossColumnCheck bool  // → enqueue to Phase 5

    // Analysis metadata
    AnalyzedAt     time.Time
    LLMModelUsed   string
}

// FeatureExtractionProgress tracks progress across all phases
type FeatureExtractionProgress struct {
    CurrentPhase     string
    PhaseDescription string

    // Phase-specific counts (known at phase start)
    TotalItems       int
    CompletedItems   int

    // Summary of work discovered
    TotalColumns         int
    EnumCandidates       int
    FKCandidates         int
    CrossColumnCandidates int
}
```

---

### Task 2: Implement Phase 1 - Data Collection (Deterministic) ✓

**File:** `pkg/services/column_feature_extraction.go`

Phase 1 runs with NO LLM calls. It collects all data and determines routing.

```go
// Phase1Result contains everything needed for subsequent phases
type Phase1Result struct {
    Profiles     []*ColumnDataProfile
    TotalColumns int

    // Queues for subsequent phases (counts known after Phase 1)
    Phase2Queue []uuid.UUID // All columns → classification
}

// runPhase1DataCollection gathers data and routes columns
func (s *columnFeatureExtractionService) runPhase1DataCollection(
    ctx context.Context,
    projectID, datasourceID uuid.UUID,
    progress *FeatureExtractionProgress,
) (*Phase1Result, error) {
    progress.CurrentPhase = "phase1"
    progress.PhaseDescription = "Collecting column metadata..."

    // Get all columns with their stats (already collected during schema extraction)
    columns, err := s.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
    if err != nil {
        return nil, err
    }

    profiles := make([]*ColumnDataProfile, 0, len(columns))
    for _, col := range columns {
        profile := s.buildColumnProfile(col)
        profile.DetectedPatterns = s.detectPatternsInSamples(col.SampleValues)
        profile.ClassificationPath = s.routeToClassificationPath(profile)
        profiles = append(profiles, profile)
    }

    progress.TotalColumns = len(profiles)
    progress.PhaseDescription = fmt.Sprintf("Found %d columns in schema", len(profiles))

    return &Phase1Result{
        Profiles:     profiles,
        TotalColumns: len(profiles),
        Phase2Queue:  extractColumnIDs(profiles),
    }, nil
}

// routeToClassificationPath determines path based on TYPE + DATA (no column names)
func (s *columnFeatureExtractionService) routeToClassificationPath(
    profile *ColumnDataProfile,
) string {
    switch {
    case isTimestampType(profile.DataType):
        return "timestamp"
    case isBooleanType(profile.DataType):
        return "boolean"
    case isIntegerType(profile.DataType):
        if profile.hasOnlyBooleanValues() {
            return "boolean"
        }
        if profile.hasUnixTimestampPattern() {
            return "timestamp"
        }
        if profile.Cardinality < 0.01 && profile.DistinctCount <= 50 {
            return "enum"
        }
        return "numeric"
    case isUUIDType(profile.DataType):
        return "uuid"
    case isTextType(profile.DataType):
        if profile.matchesPattern("uuid") {
            return "uuid"
        }
        if profile.matchesExternalIDPattern() {
            return "external_id"
        }
        if profile.Cardinality < 0.01 && profile.DistinctCount <= 50 {
            return "enum"
        }
        return "text"
    case isJSONType(profile.DataType):
        return "json"
    default:
        return "unknown"
    }
}
```

Pattern detection regexes (applied to DATA, not column names):
```go
var samplePatterns = map[string]*regexp.Regexp{
    "uuid":           regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
    "stripe_id":      regexp.MustCompile(`^(pi_|pm_|ch_|cus_|sub_|inv_|price_)[a-zA-Z0-9]+$`),
    "aws_ses":        regexp.MustCompile(`^[0-9a-f-]+@email\.amazonses\.com$`),
    "twilio_sid":     regexp.MustCompile(`^(AC|SM|MM|PN|SK)[a-f0-9]{32}$`),
    "iso4217":        regexp.MustCompile(`^[A-Z]{3}$`),
    "unix_seconds":   regexp.MustCompile(`^[0-9]{10}$`),
    "unix_millis":    regexp.MustCompile(`^[0-9]{13}$`),
    "unix_micros":    regexp.MustCompile(`^[0-9]{16}$`),
    "unix_nanos":     regexp.MustCompile(`^[0-9]{19}$`),
    "email":          regexp.MustCompile(`^[^@]+@[^@]+\.[^@]+$`),
    "url":            regexp.MustCompile(`^https?://`),
}
```

---

### Task 3: Implement Phase 2 - Column Classification (Parallel LLM) ✓

**File:** `pkg/services/column_feature_extraction.go`

Each column gets ONE focused LLM request. Progress updates after each completion.

```go
// Phase2Result contains classification results and queues for follow-up phases
type Phase2Result struct {
    Features []*ColumnFeatures

    // Queues for subsequent phases (populated based on classification results)
    Phase3EnumQueue       []uuid.UUID // Columns needing enum value analysis
    Phase4FKQueue         []uuid.UUID // Columns needing FK resolution
    Phase5CrossColumnQueue []string   // Tables needing cross-column analysis
}

// runPhase2ColumnClassification classifies each column with a focused LLM request
func (s *columnFeatureExtractionService) runPhase2ColumnClassification(
    ctx context.Context,
    projectID uuid.UUID,
    profiles []*ColumnDataProfile,
    progress *FeatureExtractionProgress,
) (*Phase2Result, error) {
    progress.CurrentPhase = "phase2"
    progress.PhaseDescription = "Classifying columns"
    progress.TotalItems = len(profiles)
    progress.CompletedItems = 0

    // Build work items - ONE LLM request per column
    workItems := make([]llm.WorkItem[*ColumnFeatures], 0, len(profiles))
    for _, profile := range profiles {
        p := profile // capture
        workItems = append(workItems, llm.WorkItem[*ColumnFeatures]{
            ID: p.ColumnID.String(),
            Execute: func(ctx context.Context) (*ColumnFeatures, error) {
                return s.classifySingleColumn(ctx, projectID, p)
            },
        })
    }

    // Process in parallel with progress updates
    results := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
        progress.CompletedItems = completed
        s.reportProgress(progress)
    })

    // Collect results and build queues for next phases
    result := &Phase2Result{
        Features:               make([]*ColumnFeatures, 0, len(results)),
        Phase3EnumQueue:        make([]uuid.UUID, 0),
        Phase4FKQueue:          make([]uuid.UUID, 0),
        Phase5CrossColumnQueue: make([]string, 0),
    }

    tablesNeedingCrossColumn := make(map[string]bool)

    for _, r := range results {
        if r.Err != nil {
            s.logger.Error("Column classification failed", zap.Error(r.Err))
            continue
        }
        features := r.Result
        result.Features = append(result.Features, features)

        // Enqueue follow-up work based on classification
        if features.NeedsEnumAnalysis {
            result.Phase3EnumQueue = append(result.Phase3EnumQueue, features.ColumnID)
        }
        if features.NeedsFKResolution {
            result.Phase4FKQueue = append(result.Phase4FKQueue, features.ColumnID)
        }
        if features.NeedsCrossColumnCheck {
            tablesNeedingCrossColumn[features.TableName] = true
        }
    }

    for table := range tablesNeedingCrossColumn {
        result.Phase5CrossColumnQueue = append(result.Phase5CrossColumnQueue, table)
    }

    progress.EnumCandidates = len(result.Phase3EnumQueue)
    progress.FKCandidates = len(result.Phase4FKQueue)
    progress.CrossColumnCandidates = len(result.Phase5CrossColumnQueue)
    progress.PhaseDescription = fmt.Sprintf(
        "Classified %d columns. Found %d enums, %d FK candidates, %d tables for cross-column analysis",
        len(result.Features), len(result.Phase3EnumQueue), len(result.Phase4FKQueue), len(result.Phase5CrossColumnQueue),
    )

    return result, nil
}

// classifySingleColumn sends ONE focused LLM request for ONE column
func (s *columnFeatureExtractionService) classifySingleColumn(
    ctx context.Context,
    projectID uuid.UUID,
    profile *ColumnDataProfile,
) (*ColumnFeatures, error) {
    classifier := s.getClassifier(profile.ClassificationPath)
    return classifier.Classify(ctx, profile)
}
```

---

### Task 4: Implement Path-Specific Classifiers ✓

**File:** `pkg/services/column_feature_extraction.go` (classifiers added to existing file)

Each classifier builds a focused prompt for its classification path.

```go
// ColumnClassifier interface for path-specific classification
type ColumnClassifier interface {
    Classify(ctx context.Context, profile *ColumnDataProfile) (*ColumnFeatures, error)
}

// TimestampClassifier - focused prompt for timestamp columns
type TimestampClassifier struct {
    llmClient llm.Client
    logger    *zap.Logger
}

func (c *TimestampClassifier) Classify(ctx context.Context, profile *ColumnDataProfile) (*ColumnFeatures, error) {
    prompt := c.buildPrompt(profile)
    response, err := c.llmClient.Complete(ctx, prompt)
    if err != nil {
        return nil, err
    }
    return c.parseResponse(profile, response)
}

func (c *TimestampClassifier) buildPrompt(profile *ColumnDataProfile) string {
    // Focused prompt for timestamp classification
    // See "LLM Prompt Examples" section
}

// BooleanClassifier - focused prompt for boolean columns
type BooleanClassifier struct { ... }

// EnumClassifier - initial classification, may flag for deeper analysis
type EnumClassifier struct { ... }

// UUIDClassifier - focused prompt for UUID columns
type UUIDClassifier struct { ... }

// ExternalIDClassifier - focused prompt for external service IDs
type ExternalIDClassifier struct { ... }

// NumericClassifier - focused prompt for numeric columns
type NumericClassifier struct { ... }

// TextClassifier - focused prompt for text columns
type TextClassifier struct { ... }
```

---

### Task 5: Implement Phase 3 - Enum Value Analysis (Parallel LLM) ✓

**File:** `pkg/services/column_feature_extraction.go`

Only runs for columns that were flagged in Phase 2. Each enum column gets ONE request.

```go
// runPhase3EnumAnalysis analyzes enum values for columns flagged in Phase 2
func (s *columnFeatureExtractionService) runPhase3EnumAnalysis(
    ctx context.Context,
    projectID uuid.UUID,
    enumQueue []uuid.UUID,
    features []*ColumnFeatures,
    progress *FeatureExtractionProgress,
) error {
    if len(enumQueue) == 0 {
        return nil // Skip phase if no enum candidates
    }

    progress.CurrentPhase = "phase3"
    progress.PhaseDescription = "Analyzing enum values"
    progress.TotalItems = len(enumQueue)
    progress.CompletedItems = 0

    // Build work items - ONE request per enum column
    workItems := make([]llm.WorkItem[*EnumAnalysisResult], 0, len(enumQueue))
    for _, columnID := range enumQueue {
        cid := columnID
        workItems = append(workItems, llm.WorkItem[*EnumAnalysisResult]{
            ID: cid.String(),
            Execute: func(ctx context.Context) (*EnumAnalysisResult, error) {
                return s.analyzeEnumColumn(ctx, projectID, cid)
            },
        })
    }

    // Process in parallel
    results := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
        progress.CompletedItems = completed
        s.reportProgress(progress)
    })

    // Merge results into features
    for _, r := range results {
        if r.Err != nil {
            continue
        }
        s.mergeEnumAnalysis(features, r.Result)
    }

    progress.PhaseDescription = fmt.Sprintf("Analyzed %d enum columns", len(enumQueue))
    return nil
}
```

---

### Task 6: Implement Phase 4 - FK Resolution (Parallel LLM)

**File:** `pkg/services/column_feature_extraction.go`

```go
// runPhase4FKResolution resolves FK candidates flagged in Phase 2
func (s *columnFeatureExtractionService) runPhase4FKResolution(
    ctx context.Context,
    projectID uuid.UUID,
    fkQueue []uuid.UUID,
    features []*ColumnFeatures,
    progress *FeatureExtractionProgress,
) error {
    if len(fkQueue) == 0 {
        return nil
    }

    progress.CurrentPhase = "phase4"
    progress.PhaseDescription = "Resolving FK relationships"
    progress.TotalItems = len(fkQueue)
    progress.CompletedItems = 0

    // Build work items
    workItems := make([]llm.WorkItem[*FKResolutionResult], 0, len(fkQueue))
    for _, columnID := range fkQueue {
        cid := columnID
        workItems = append(workItems, llm.WorkItem[*FKResolutionResult]{
            ID: cid.String(),
            Execute: func(ctx context.Context) (*FKResolutionResult, error) {
                // 1. Run data overlap queries (deterministic)
                // 2. Send LLM request with overlap results
                return s.resolveFK(ctx, projectID, cid)
            },
        })
    }

    results := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
        progress.CompletedItems = completed
        s.reportProgress(progress)
    })

    for _, r := range results {
        if r.Err != nil {
            continue
        }
        s.mergeFKResolution(features, r.Result)
    }

    progress.PhaseDescription = fmt.Sprintf("Resolved %d FK candidates", len(fkQueue))
    return nil
}
```

---

### Task 7: Implement Phase 5 - Cross-Column Analysis (Parallel LLM)

**File:** `pkg/services/column_feature_extraction.go`

```go
// runPhase5CrossColumnAnalysis analyzes column relationships per table
func (s *columnFeatureExtractionService) runPhase5CrossColumnAnalysis(
    ctx context.Context,
    projectID uuid.UUID,
    tableQueue []string,
    features []*ColumnFeatures,
    progress *FeatureExtractionProgress,
) error {
    if len(tableQueue) == 0 {
        return nil
    }

    progress.CurrentPhase = "phase5"
    progress.PhaseDescription = "Analyzing column relationships"
    progress.TotalItems = len(tableQueue)
    progress.CompletedItems = 0

    // Build work items - ONE request per table
    workItems := make([]llm.WorkItem[*CrossColumnResult], 0, len(tableQueue))
    for _, tableName := range tableQueue {
        tn := tableName
        workItems = append(workItems, llm.WorkItem[*CrossColumnResult]{
            ID: tn,
            Execute: func(ctx context.Context) (*CrossColumnResult, error) {
                return s.analyzeCrossColumn(ctx, projectID, tn, features)
            },
        })
    }

    results := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
        progress.CompletedItems = completed
        s.reportProgress(progress)
    })

    for _, r := range results {
        if r.Err != nil {
            continue
        }
        s.mergeCrossColumnAnalysis(features, r.Result)
    }

    progress.PhaseDescription = fmt.Sprintf("Analyzed %d tables for column relationships", len(tableQueue))
    return nil
}
```

---

### Task 8: Implement Main Orchestrator

**File:** `pkg/services/column_feature_extraction.go`

```go
func (s *columnFeatureExtractionService) ExtractColumnFeatures(
    ctx context.Context,
    projectID, datasourceID uuid.UUID,
    progressCallback dag.ProgressCallback,
) (int, error) {
    progress := &FeatureExtractionProgress{}

    reportProgress := func() {
        if progressCallback != nil {
            progressCallback(
                progress.CompletedItems,
                progress.TotalItems,
                progress.PhaseDescription,
            )
        }
    }
    s.reportProgress = reportProgress

    // Phase 1: Data Collection (deterministic, no LLM)
    phase1, err := s.runPhase1DataCollection(ctx, projectID, datasourceID, progress)
    if err != nil {
        return 0, fmt.Errorf("phase 1 failed: %w", err)
    }
    reportProgress()

    // Phase 2: Column Classification (parallel LLM, 1 request/column)
    phase2, err := s.runPhase2ColumnClassification(ctx, projectID, phase1.Profiles, progress)
    if err != nil {
        return 0, fmt.Errorf("phase 2 failed: %w", err)
    }
    reportProgress()

    // Phase 3: Enum Value Analysis (parallel LLM, 1 request/enum)
    if err := s.runPhase3EnumAnalysis(ctx, projectID, phase2.Phase3EnumQueue, phase2.Features, progress); err != nil {
        return 0, fmt.Errorf("phase 3 failed: %w", err)
    }
    reportProgress()

    // Phase 4: FK Resolution (parallel LLM, 1 request/FK candidate)
    if err := s.runPhase4FKResolution(ctx, projectID, phase2.Phase4FKQueue, phase2.Features, progress); err != nil {
        return 0, fmt.Errorf("phase 4 failed: %w", err)
    }
    reportProgress()

    // Phase 5: Cross-Column Analysis (parallel LLM, 1 request/table)
    if err := s.runPhase5CrossColumnAnalysis(ctx, projectID, phase2.Phase5CrossColumnQueue, phase2.Features, progress); err != nil {
        return 0, fmt.Errorf("phase 5 failed: %w", err)
    }
    reportProgress()

    // Phase 6: Store Results (deterministic, no LLM)
    progress.CurrentPhase = "phase6"
    progress.PhaseDescription = "Saving column features..."
    reportProgress()

    if err := s.storeFeatures(ctx, projectID, phase2.Features); err != nil {
        return 0, fmt.Errorf("phase 6 failed: %w", err)
    }

    progress.PhaseDescription = "Column feature extraction complete"
    reportProgress()

    return len(phase2.Features), nil
}
```

---

### Task 9: Remove All Static Column Name Patterns

**Files to modify:**

1. `pkg/services/column_enrichment.go`
   - Remove: `detectSoftDeletePattern()`, `detectMonetaryColumnPattern()`, `detectUUIDTextColumnPattern()`, `detectTimestampScalePattern()`, `detectExternalIDPattern()`
   - Remove: `monetaryColumnPatterns` variable
   - Update: `convertToColumnDetails()` to read from stored features instead

2. `pkg/services/column_filter.go`
   - Remove: `isExcludedName()`, `isEntityReferenceName()`
   - Remove: `useLegacyPatternMatching` parameter
   - Update: Use `ColumnFeatures.Purpose` for filtering

3. `pkg/services/deterministic_question_generation.go`
   - Remove any column name pattern checks
   - Use stored features for question generation

---

### Task 10: Update UI to Show Phase-Based Progress

**File:** `ui/src/components/ontology/ExtractionProgress.tsx` (or similar)

The UI needs to display multi-phase progress:

```typescript
interface ExtractionPhase {
  id: string;
  name: string;
  status: 'pending' | 'in_progress' | 'complete' | 'failed';
  totalItems?: number;
  completedItems?: number;
  currentItem?: string;
}

interface ColumnFeatureExtractionProgress {
  phases: ExtractionPhase[];
}

// Example phases:
const phases: ExtractionPhase[] = [
  { id: 'phase1', name: 'Collecting column metadata', status: 'complete', totalItems: 38, completedItems: 38 },
  { id: 'phase2', name: 'Classifying columns', status: 'in_progress', totalItems: 38, completedItems: 23, currentItem: 'billing_transactions.transaction_state' },
  { id: 'phase3', name: 'Analyzing enum values', status: 'pending', totalItems: 7 },
  { id: 'phase4', name: 'Resolving FK candidates', status: 'pending', totalItems: 4 },
  { id: 'phase5', name: 'Cross-column analysis', status: 'pending', totalItems: 2 },
  { id: 'phase6', name: 'Saving results', status: 'pending' },
];
```

---

## LLM Prompt Examples

### Timestamp Classification Prompt

```
You are analyzing a database column to determine its purpose.

Table: billing_transactions
Column: marker_at (stored in column named "marker_at")
Data type: bigint
Statistics:
- 0 NULL values (0% null rate)
- Values are Unix timestamps in nanoseconds
- Sample values (converted to dates):
  - 2024-01-15 14:23:45.123456789
  - 2024-01-15 14:23:46.234567890
  - 2024-01-15 14:23:47.345678901

Other timestamp columns in this table:
- created_at (0% null, audit timestamp)
- updated_at (0% null, audit timestamp)
- completed_at (35% null)

Based on the DATA (not the column name), what is the purpose of this timestamp?

Options:
- audit_created: Records when the row was created
- audit_updated: Records when the row was last modified
- soft_delete: Records when the row was soft-deleted (high null rate expected)
- event_time: Records when a business event occurred
- scheduled_time: Records when something is scheduled to happen
- expiration: Records when something expires
- cursor: Used for pagination/ordering (nanosecond precision suggests this)

Respond with JSON:
{
  "purpose": "cursor",
  "confidence": 0.85,
  "description": "Nanosecond-precision timestamp used for cursor-based pagination. The high precision ensures unique ordering even for rapid sequential operations."
}
```

### Enum Classification Prompt

```
You are analyzing a database column to determine the meaning of its values.

Table: billing_transactions
Column: transaction_state (stored in column named "transaction_state")
Data type: integer

Value distribution:
| Value | Count  | Percentage | Has completed_at |
|-------|--------|------------|------------------|
| 0     | 45,000 | 47.4%      | 0%               |
| 5     | 38,000 | 40.0%      | 100%             |
| 4     | 12,000 | 12.6%      | 98%              |
| 1     | 2,300  | 2.4%       | 0%               |
| 6     | 890    | 0.9%       | 100%             |
| 2     | 150    | 0.2%       | 0%               |
| 3     | 89     | 0.1%       | 0%               |
| 7     | 45     | 0.05%      | 100%             |

The correlation with completed_at suggests this is a state machine:
- Values 0, 1, 2, 3 have 0% completion → likely pending/in-progress states
- Values 4, 5, 6, 7 have ~100% completion → likely terminal states
- Value 0 is most common → likely initial state
- Value 5 is second most common terminal → likely success state
- Value 7 is rarest terminal → likely error/edge case

Based on the DATA patterns, provide labels for each value:

Respond with JSON:
{
  "is_state_machine": true,
  "values": [
    {"value": "0", "label": "pending", "category": "initial"},
    {"value": "1", "label": "processing", "category": "in_progress"},
    {"value": "2", "label": "review_required", "category": "in_progress"},
    {"value": "3", "label": "on_hold", "category": "in_progress"},
    {"value": "4", "label": "partially_completed", "category": "terminal"},
    {"value": "5", "label": "completed", "category": "terminal_success"},
    {"value": "6", "label": "refunded", "category": "terminal"},
    {"value": "7", "label": "failed", "category": "terminal_error"}
  ],
  "description": "Transaction processing state machine. Transactions start in pending (0), move through processing states, and end in a terminal state (4-7)."
}
```

---

## Success Criteria

### Data-Driven Classification
1. **Zero static column name patterns** in classification logic
2. **All routing decisions based on data type + data characteristics**
3. **Column names passed to LLM as context only**
4. **Cross-column analysis uses data correlation, not naming**
5. **Features stored once, consumed by all downstream stages**

### UX and Performance
6. **Each LLM request classifies ONE column** (no mega-prompts)
7. **Progress updates after EVERY column** (not batch updates)
8. **Task counts known at phase start** (deterministic enumeration)
9. **Separate UI progress indicators for each phase**
10. **No spinner stalls > 5 seconds** without progress update

### Throughput
11. **Parallel execution within each phase** (up to N workers)
12. **Failed column retries don't block other columns**
13. **Total extraction time scales linearly with column count**

## UI Progress Flow

The UI should display progress like this:

```
Extracting Column Features
├─ ✓ Collecting column metadata (found 38 columns)
├─ ● Classifying columns (23/38)
│    └─ Currently: billing_transactions.transaction_state
├─ ○ Analyzing enum values (0/7)
├─ ○ Resolving FK candidates (0/4)
├─ ○ Cross-column analysis (0/2)
└─ ○ Saving results

Legend: ✓ complete  ● in progress  ○ pending
```

Each phase shows:
- Phase name
- Progress counter (completed/total)
- Currently processing item (for in-progress phase)

## Testing Approach

### Unit Tests
1. **Routing logic tests** - Verify data-driven routing without column names
2. **Pattern detection tests** - Verify regex patterns on sample values
3. **Classifier tests** - Mock LLM responses, verify feature extraction

### Integration Tests
1. **Progress callback tests** - Verify callbacks fire after each column
2. **Phase transition tests** - Verify queues populated correctly
3. **Parallel execution tests** - Verify concurrent processing works

### Edge Cases
1. Column named `deleted_at` that ISN'T a soft delete (0% NULL rate)
   - Expected: Classified as timestamp, NOT soft delete
2. Soft delete column named `removed_timestamp` (95% NULL rate)
   - Expected: Classified as soft delete based on data
3. Integer column with values 0-7 that ISN'T a state machine
   - Expected: LLM analyzes context, may classify as enum without state semantics

### Performance Tests
1. **38 column schema** - Should complete in < 60s with progress updates every 1-2s
2. **200 column schema** - Should scale linearly, parallel execution
3. **Retry behavior** - Failed column doesn't block others
