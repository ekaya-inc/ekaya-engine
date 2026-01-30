# ISSUES: Ontology Benchmark API Testing - 2026-01-30

Issues discovered during comprehensive MCP API testing before production push.

---

## Critical Issues

### 1. Sensitive Data Exposure in sample_values

**Severity**: CRITICAL
**Tool**: `get_context` with `include: ["sample_values"]`
**Status**: In Progress (split into subtasks)

**Description**: When requesting sample_values, sensitive data is exposed. The `agent_data` column in `users` table shows LiveKit API keys and secrets in plaintext.

**Reproduction**:
```
get_context(depth='columns', tables=['users'], include=['sample_values'])
```

**Observed output**:
```json
{
  "column_name": "agent_data",
  "sample_values": [
    "",
    "{\"livekit_url\":\"wss://tikragents-xxx.livekit.cloud\",\"livekit_api_key\":\"API67e2wiyw3KvB\",\"livekit_api_secret\":\"MATPBGtZAPGGxyslrsjHaZjN3W6KsU2pIfdwNHMfR0i\",\"livekit_agent_id\":\"kitt\"}"
  ]
}
```

**Expected**: Columns containing sensitive patterns (api_key, api_secret, password, token, credential) should be redacted or excluded from sample_values.

---

#### 1.1 Add sensitive column detection with regex patterns

- [x] **Status**: Complete

Create a reusable sensitive column detector in the MCP tools package that identifies columns containing secrets based on naming patterns and content analysis.

---

#### 1.2 Integrate sensitive detection into get_context sample_values output

- [x] **Status**: Complete

Modify the `get_context` MCP tool to use the SensitiveDetector when returning sample_values, automatically redacting sensitive data.

---

#### 1.3 Add column-level sensitive flag to ontology schema and MCP tools

- [x] **Status**: Complete

Extend the ontology data model to support an explicit `is_sensitive` flag on columns, allowing manual override of automatic detection.

---

## High Priority Issues

### 2. create_approved_query Error Message Unclear

- [x] **Status**: Complete

**Severity**: HIGH (UX)
**Tool**: `create_approved_query`

**Description**: When `output_column_descriptions` is not provided, the error message is confusing.

**Recommendation**: Update error message to clearly indicate the required parameter name and format.

---

### 3. probe_relationship Returns Empty for Known Relationships

**Severity**: HIGH
**Tool**: `probe_relationship`
**Status**: Open

**Description**: When probing for relationships between entities that should be related (User -> Account), the tool returns empty results.

---

### 4. get_query_history Returns Empty

**Severity**: MEDIUM
**Tool**: `get_query_history`
**Status**: Open

**Description**: Query history appears to not be recording executed queries.

---

## Medium Priority Issues

### 5. Glossary Terms with Invalid SQL Functions

**Severity**: MEDIUM
**Tool**: Glossary enrichment
**Status**: Open

**Description**: 3 out of 10 glossary terms have SQL that fails validation due to PostgreSQL function compatibility.

---

### 6. refresh_schema Reports 634 Columns Added on Every Run

**Severity**: LOW
**Tool**: `refresh_schema`
**Status**: Open

**Description**: Running refresh_schema reports 634 columns added even when no schema changes occurred.

---

## Recommendations Before Production

### Must Fix (Blocking)
1. **Sensitive data exposure** - Add redaction for API keys/secrets in sample_values (see subtasks 1.1, 1.2, 1.3)

### Should Fix
2. **Error message clarity** - Improve create_approved_query error message
3. **Query history** - Verify logging is working

### Nice to Have
4. **probe_relationship** - Investigate empty results for known relationships
5. **Glossary SQL validation** - Improve PostgreSQL function recognition
6. **refresh_schema delta** - Fix column count reporting
