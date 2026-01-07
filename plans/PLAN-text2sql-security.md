# PLAN: Text2SQL - SQL Security & Validation Architecture

> **Navigation:** [Overview](PLAN-text2sql-overview.md) | [Vector Infrastructure](PLAN-text2sql-vector.md) | [Service](PLAN-text2sql-service.md) | [Enhancements](PLAN-text2sql-enhancements.md) | [Security](PLAN-text2sql-security.md) | [Ontology Linking](PLAN-text2sql-ontology-linking.md) | [Memory System](PLAN-text2sql-memory.md) | [Implementation](PLAN-text2sql-implementation.md)

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

