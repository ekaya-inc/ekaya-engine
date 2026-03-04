use serde::{Deserialize, Serialize};

/// Category of PII or sensitive data detected.
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Category {
    Secrets,
    PiiIdentity,
    PiiContact,
    PiiFinancial,
}

/// Confidence level of the detection.
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Confidence {
    High,
    Medium,
    Low,
}

/// A finding from scanning a column name.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ColumnFinding {
    pub column_name: String,
    pub category: Category,
    pub pattern_name: String,
    pub confidence: Confidence,
}

/// A finding from scanning cell content.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContentFinding {
    pub column_name: String,
    pub category: Category,
    pub pattern_name: String,
    pub confidence: Confidence,
    pub redacted_sample: String,
}

/// Schema representation for a table (matches get_schema MCP tool output).
#[derive(Debug, Clone, Deserialize)]
pub struct TableSchema {
    pub schema: String,
    pub name: String,
    pub row_count: Option<i64>,
    pub columns: Vec<ColumnSchema>,
}

/// Schema representation for a column.
#[derive(Debug, Clone, Deserialize)]
pub struct ColumnSchema {
    pub name: String,
    pub data_type: String,
    #[serde(default)]
    pub is_primary_key: bool,
    #[serde(default)]
    pub is_nullable: bool,
    #[serde(default)]
    pub ordinal_position: i32,
}
