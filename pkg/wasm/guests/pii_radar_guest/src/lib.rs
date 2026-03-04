use extism_pdk::*;
use pii_detector::{
    scan_columns, scan_content, Category, ColumnSchema, Confidence, ContentFinding, TableSchema,
};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[host_fn]
extern "ExtismHost" {
    fn tool_invoke(input: String) -> String;
    fn state_get(input: String) -> String;
    fn state_set(input: String) -> String;
}

// --- Host response types ---

#[derive(Deserialize)]
struct ToolResponse {
    result: Option<serde_json::Value>,
    #[serde(default)]
    is_error: Option<bool>,
    error: Option<String>,
}

#[derive(Deserialize)]
struct StateGetResponse {
    data: Option<serde_json::Value>,
    version: i64,
}

#[derive(Deserialize)]
struct StateSetResponse {
    version: Option<i64>,
    error: Option<String>,
}

// --- Input ---

#[derive(Deserialize)]
struct RunInput {
    now: String,
}

// --- State ---

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
struct ScanState {
    #[serde(default)]
    high_watermarks: HashMap<String, HighWatermark>,
    #[serde(default)]
    findings: Vec<Finding>,
    #[serde(default)]
    last_scan_at: Option<String>,
    #[serde(default)]
    scan_count: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct HighWatermark {
    pk_column: String,
    value: String,
    #[serde(default)]
    completed: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct Finding {
    table: String,
    column: String,
    category: Category,
    pattern_name: String,
    confidence: Confidence,
    detection_type: String,
    redacted_sample: String,
    first_detected_at: String,
    last_seen_at: String,
    #[serde(default)]
    occurrence_count: u64,
}

// --- Report ---

#[derive(Serialize)]
struct Report {
    status: String,
    tables_scanned: usize,
    tables_total: usize,
    rows_scanned_this_run: u64,
    new_findings: Vec<ReportFinding>,
    all_findings_summary: FindingsSummary,
    scan_progress: HashMap<String, ScanProgress>,
    errors: Vec<String>,
}

#[derive(Serialize)]
struct ReportFinding {
    table: String,
    column: String,
    category: Category,
    pattern_name: String,
    confidence: Confidence,
    detection_type: String,
    redacted_sample: String,
}

#[derive(Serialize)]
struct FindingsSummary {
    total: usize,
    by_category: HashMap<String, usize>,
    by_confidence: HashMap<String, usize>,
}

#[derive(Serialize)]
struct ScanProgress {
    completed: bool,
    scanned_rows: u64,
}

// --- Schema response types ---

#[derive(Deserialize)]
struct SchemaResponse {
    #[serde(default)]
    tables: Vec<TableSchema>,
}

#[derive(Deserialize)]
#[allow(dead_code)]
struct QueryResponse {
    #[serde(default)]
    columns: Vec<QueryColumn>,
    #[serde(default)]
    rows: Vec<serde_json::Value>,
    #[serde(default)]
    row_count: u64,
    #[serde(default)]
    truncated: bool,
}

#[derive(Deserialize)]
#[allow(dead_code)]
struct QueryColumn {
    name: String,
    #[serde(rename = "type")]
    col_type: Option<String>,
}

// --- Helper functions ---

fn invoke_tool(tool: &str, arguments: serde_json::Value) -> Result<serde_json::Value, Error> {
    let req = serde_json::json!({
        "tool": tool,
        "arguments": arguments,
    })
    .to_string();

    let resp_str = unsafe { tool_invoke(req)? };
    let resp: ToolResponse = serde_json::from_str(&resp_str)?;

    if let Some(err) = &resp.error {
        return Err(Error::msg(format!("tool {} error: {}", tool, err)));
    }
    if resp.is_error.unwrap_or(false) {
        let result_str = resp
            .result
            .as_ref()
            .map(|v| v.to_string())
            .unwrap_or_default();
        return Err(Error::msg(format!("tool {} returned error: {}", tool, result_str)));
    }

    resp.result.ok_or_else(|| Error::msg(format!("tool {} returned no result", tool)))
}

fn load_state() -> Result<(ScanState, i64), Error> {
    let resp_str = unsafe { state_get(String::new())? };
    let resp: StateGetResponse = serde_json::from_str(&resp_str)?;

    let state = match resp.data {
        Some(serde_json::Value::Null) | None => ScanState::default(),
        Some(v) => serde_json::from_value(v).unwrap_or_default(),
    };

    Ok((state, resp.version))
}

fn save_state(state: &ScanState, version: i64) -> Result<i64, Error> {
    let data = serde_json::to_value(state)?;
    let req = serde_json::json!({
        "data": data,
        "version": version,
    })
    .to_string();

    let resp_str = unsafe { state_set(req)? };
    let resp: StateSetResponse = serde_json::from_str(&resp_str)?;

    if let Some(err) = &resp.error {
        return Err(Error::msg(format!("state_set error: {}", err)));
    }

    resp.version.ok_or_else(|| Error::msg("state_set returned no version"))
}

fn full_table_name(schema: &str, name: &str) -> String {
    format!("{}.{}", schema, name)
}

fn find_primary_key(columns: &[ColumnSchema]) -> Option<&ColumnSchema> {
    columns.iter().find(|c| c.is_primary_key)
}

fn category_str(cat: &Category) -> String {
    serde_json::to_value(cat)
        .ok()
        .and_then(|v| v.as_str().map(|s| s.to_string()))
        .unwrap_or_else(|| format!("{:?}", cat))
}

fn confidence_str(conf: &Confidence) -> String {
    serde_json::to_value(conf)
        .ok()
        .and_then(|v| v.as_str().map(|s| s.to_string()))
        .unwrap_or_else(|| format!("{:?}", conf))
}

fn merge_finding(state: &mut ScanState, table: &str, finding: &ContentFinding, now: &str) {
    // Look for existing finding with same table+column+pattern_name
    for existing in state.findings.iter_mut() {
        if existing.table == table
            && existing.column == finding.column_name
            && existing.pattern_name == finding.pattern_name
        {
            existing.last_seen_at = now.to_string();
            existing.occurrence_count += 1;
            return;
        }
    }

    // New finding
    state.findings.push(Finding {
        table: table.to_string(),
        column: finding.column_name.clone(),
        category: finding.category.clone(),
        pattern_name: finding.pattern_name.clone(),
        confidence: finding.confidence.clone(),
        detection_type: "content".to_string(),
        redacted_sample: finding.redacted_sample.clone(),
        first_detected_at: now.to_string(),
        last_seen_at: now.to_string(),
        occurrence_count: 1,
    });
}

fn merge_column_finding(
    state: &mut ScanState,
    table: &str,
    col_name: &str,
    category: &Category,
    pattern_name: &str,
    confidence: &Confidence,
    now: &str,
) {
    for existing in state.findings.iter_mut() {
        if existing.table == table
            && existing.column == col_name
            && existing.pattern_name == pattern_name
        {
            existing.last_seen_at = now.to_string();
            existing.occurrence_count += 1;
            return;
        }
    }

    state.findings.push(Finding {
        table: table.to_string(),
        column: col_name.to_string(),
        category: category.clone(),
        pattern_name: pattern_name.to_string(),
        confidence: confidence.clone(),
        detection_type: "column".to_string(),
        redacted_sample: String::new(),
        first_detected_at: now.to_string(),
        last_seen_at: now.to_string(),
        occurrence_count: 1,
    });
}

// --- Main entry point ---

#[plugin_fn]
pub fn run(input: String) -> FnResult<String> {
    let run_input: RunInput = serde_json::from_str(&input)
        .unwrap_or(RunInput { now: "unknown".to_string() });
    let now = &run_input.now;

    let mut errors: Vec<String> = Vec::new();
    let mut new_findings: Vec<ReportFinding> = Vec::new();
    let mut rows_scanned_this_run: u64 = 0;
    let mut scan_progress: HashMap<String, ScanProgress> = HashMap::new();

    // 1. Load state
    let (mut state, version) = load_state()?;

    // 2. Get schema
    let schema_result = invoke_tool("get_schema", serde_json::json!({}));
    let schema: SchemaResponse = match schema_result {
        Ok(val) => serde_json::from_value(val)?,
        Err(e) => {
            return Ok(serde_json::to_string(&Report {
                status: "error".to_string(),
                tables_scanned: 0,
                tables_total: 0,
                rows_scanned_this_run: 0,
                new_findings: vec![],
                all_findings_summary: FindingsSummary {
                    total: 0,
                    by_category: HashMap::new(),
                    by_confidence: HashMap::new(),
                },
                scan_progress: HashMap::new(),
                errors: vec![format!("failed to get schema: {}", e)],
            })?);
        }
    };

    let tables_total = schema.tables.len();
    let mut tables_scanned = 0;

    // 3. Phase 1: Column name scan
    for table in &schema.tables {
        let table_name = full_table_name(&table.schema, &table.name);
        let col_findings = scan_columns(&table.columns);

        for cf in &col_findings {
            let is_new = !state.findings.iter().any(|f| {
                f.table == table_name
                    && f.column == cf.column_name
                    && f.pattern_name == cf.pattern_name
            });

            merge_column_finding(
                &mut state,
                &table_name,
                &cf.column_name,
                &cf.category,
                &cf.pattern_name,
                &cf.confidence,
                now,
            );

            if is_new {
                new_findings.push(ReportFinding {
                    table: table_name.clone(),
                    column: cf.column_name.clone(),
                    category: cf.category.clone(),
                    pattern_name: cf.pattern_name.clone(),
                    confidence: cf.confidence.clone(),
                    detection_type: "column".to_string(),
                    redacted_sample: String::new(),
                });
            }
        }
    }

    // 4. Phase 2: Content scan
    for table in &schema.tables {
        let table_name = full_table_name(&table.schema, &table.name);

        let pk = match find_primary_key(&table.columns) {
            Some(pk) => pk,
            None => {
                // No primary key, can't do incremental scanning.
                // Mark as not completed in progress.
                scan_progress.insert(
                    table_name.clone(),
                    ScanProgress {
                        completed: false,
                        scanned_rows: 0,
                    },
                );
                continue;
            }
        };

        // Get high watermark from state
        let hwm = state.high_watermarks.get(&table_name).cloned();
        let hwm_value = hwm.as_ref().map(|h| h.value.clone());

        // Build SQL query
        let qualified = format!("{}.{}", table.schema, table.name);
        let sql = if let Some(ref hwm_val) = hwm_value {
            format!(
                "SELECT * FROM {} WHERE {} > '{}' ORDER BY {} LIMIT 1000",
                qualified, pk.name, hwm_val, pk.name
            )
        } else {
            format!(
                "SELECT * FROM {} ORDER BY {} LIMIT 1000",
                qualified, pk.name
            )
        };

        // Execute query
        let query_result = invoke_tool("query", serde_json::json!({"sql": sql}));
        let query_resp: QueryResponse = match query_result {
            Ok(val) => match serde_json::from_value(val) {
                Ok(r) => r,
                Err(e) => {
                    errors.push(format!("failed to parse query response for {}: {}", table_name, e));
                    continue;
                }
            },
            Err(e) => {
                errors.push(format!("query failed for {}: {}", table_name, e));
                continue;
            }
        };

        let num_rows = query_resp.rows.len() as u64;
        rows_scanned_this_run += num_rows;

        // Extract column values and scan content
        let col_names: Vec<String> = query_resp.columns.iter().map(|c| c.name.clone()).collect();

        for col_name in &col_names {
            let values: Vec<String> = query_resp
                .rows
                .iter()
                .filter_map(|row| {
                    let v = row.get(col_name)?;
                    match v {
                        serde_json::Value::String(s) => Some(s.clone()),
                        serde_json::Value::Number(n) => Some(n.to_string()),
                        serde_json::Value::Null => None,
                        other => Some(other.to_string()),
                    }
                })
                .collect();

            let value_refs: Vec<&str> = values.iter().map(|s| s.as_str()).collect();
            let content_findings = scan_content(col_name, &value_refs);

            for cf in &content_findings {
                let is_new = !state.findings.iter().any(|f| {
                    f.table == table_name
                        && f.column == cf.column_name
                        && f.pattern_name == cf.pattern_name
                });

                merge_finding(&mut state, &table_name, cf, now);

                if is_new {
                    new_findings.push(ReportFinding {
                        table: table_name.clone(),
                        column: cf.column_name.clone(),
                        category: cf.category.clone(),
                        pattern_name: cf.pattern_name.clone(),
                        confidence: cf.confidence.clone(),
                        detection_type: "content".to_string(),
                        redacted_sample: cf.redacted_sample.clone(),
                    });
                }
            }
        }

        // Update high watermark
        let new_hwm_value = if !query_resp.rows.is_empty() {
            let last_row = &query_resp.rows[query_resp.rows.len() - 1];
            last_row.get(&pk.name).and_then(|v| match v {
                serde_json::Value::String(s) => Some(s.clone()),
                serde_json::Value::Number(n) => Some(n.to_string()),
                _ => None,
            })
        } else {
            None
        };

        let completed = num_rows < 1000;

        if let Some(val) = new_hwm_value {
            state.high_watermarks.insert(
                table_name.clone(),
                HighWatermark {
                    pk_column: pk.name.clone(),
                    value: val,
                    completed,
                },
            );
        } else if hwm.is_none() {
            // No rows returned and no previous HWM: table is empty, mark completed
            state.high_watermarks.insert(
                table_name.clone(),
                HighWatermark {
                    pk_column: pk.name.clone(),
                    value: String::new(),
                    completed: true,
                },
            );
        }

        scan_progress.insert(
            table_name.clone(),
            ScanProgress {
                completed,
                scanned_rows: num_rows,
            },
        );

        tables_scanned += 1;
    }

    // 5. Update state metadata
    state.last_scan_at = Some(now.to_string());
    state.scan_count += 1;

    // 6. Save state
    save_state(&state, version)?;

    // 7. Build summary
    let mut by_category: HashMap<String, usize> = HashMap::new();
    let mut by_confidence: HashMap<String, usize> = HashMap::new();
    for f in &state.findings {
        *by_category.entry(category_str(&f.category)).or_insert(0) += 1;
        *by_confidence.entry(confidence_str(&f.confidence)).or_insert(0) += 1;
    }

    let report = Report {
        status: "completed".to_string(),
        tables_scanned,
        tables_total,
        rows_scanned_this_run,
        new_findings,
        all_findings_summary: FindingsSummary {
            total: state.findings.len(),
            by_category,
            by_confidence,
        },
        scan_progress,
        errors,
    };

    Ok(serde_json::to_string(&report)?)
}
