use crate::patterns::{column_patterns, compile};
use crate::types::{ColumnFinding, ColumnSchema};

/// Scan column names against known PII/secret patterns.
/// Returns findings for columns whose names match sensitive patterns.
pub fn scan_columns(columns: &[ColumnSchema]) -> Vec<ColumnFinding> {
    let patterns = column_patterns();
    let mut findings = Vec::new();

    for col in columns {
        for pat in &patterns {
            let re = match compile(pat.regex) {
                Some(r) => r,
                None => continue,
            };

            if !re.is_match(&col.name) {
                continue;
            }

            // Check exclusion pattern
            if let Some(excl) = pat.exclude_regex {
                if let Some(excl_re) = compile(excl) {
                    if excl_re.is_match(&col.name) {
                        continue;
                    }
                }
            }

            findings.push(ColumnFinding {
                column_name: col.name.clone(),
                category: pat.category.clone(),
                pattern_name: pat.name.to_string(),
                confidence: pat.confidence.clone(),
            });
        }
    }

    findings
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::{Category, Confidence, ColumnSchema};

    fn col(name: &str) -> ColumnSchema {
        ColumnSchema {
            name: name.to_string(),
            data_type: "varchar".to_string(),
            is_primary_key: false,
            is_nullable: true,
            ordinal_position: 1,
        }
    }

    #[test]
    fn test_detects_api_key() {
        let cols = vec![col("api_key")];
        let findings = scan_columns(&cols);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].category, Category::Secrets);
        assert_eq!(findings[0].pattern_name, "api_key");
    }

    #[test]
    fn test_detects_email() {
        let cols = vec![col("email")];
        let findings = scan_columns(&cols);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].category, Category::PiiContact);
    }

    #[test]
    fn test_detects_user_password() {
        let cols = vec![col("user_password")];
        let findings = scan_columns(&cols);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].pattern_name, "password");
    }

    #[test]
    fn test_excludes_password_hash() {
        let cols = vec![col("password_hash")];
        let findings = scan_columns(&cols);
        assert!(findings.is_empty(), "password_hash should be excluded");
    }

    #[test]
    fn test_excludes_password_reset() {
        let cols = vec![col("password_reset")];
        let findings = scan_columns(&cols);
        assert!(findings.is_empty(), "password_reset should be excluded");
    }

    #[test]
    fn test_no_detection_for_normal_columns() {
        let cols = vec![col("first_name"), col("order_total"), col("created_at")];
        let findings = scan_columns(&cols);
        assert!(findings.is_empty());
    }

    #[test]
    fn test_detects_ssn() {
        let cols = vec![col("ssn")];
        let findings = scan_columns(&cols);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].category, Category::PiiIdentity);
    }

    #[test]
    fn test_detects_credit_card() {
        let cols = vec![col("credit_card")];
        let findings = scan_columns(&cols);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].category, Category::PiiFinancial);
    }

    #[test]
    fn test_mixed_columns() {
        let cols = vec![
            col("id"),
            col("user_password"),
            col("api_key"),
            col("email"),
            col("first_name"),
            col("order_total"),
        ];
        let findings = scan_columns(&cols);
        assert_eq!(findings.len(), 3); // password, api_key, email
        let names: Vec<&str> = findings.iter().map(|f| f.pattern_name.as_str()).collect();
        assert!(names.contains(&"password"));
        assert!(names.contains(&"api_key"));
        assert!(names.contains(&"email"));
    }

    #[test]
    fn test_confidence_levels() {
        let cols = vec![col("date_of_birth")];
        let findings = scan_columns(&cols);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].confidence, Confidence::Medium);
    }
}
