use crate::patterns::{compile, content_patterns};
use crate::redact::redact;
use crate::types::ContentFinding;

/// Scan a set of cell values for a given column and return findings.
/// `column_name` is the name of the column being scanned.
/// `values` is a slice of cell values (as strings) to check.
pub fn scan_content(column_name: &str, values: &[&str]) -> Vec<ContentFinding> {
    let patterns = content_patterns();
    let mut findings = Vec::new();
    let mut seen: std::collections::HashSet<String> = std::collections::HashSet::new();

    for pat in &patterns {
        let re = match compile(pat.regex) {
            Some(r) => r,
            None => continue,
        };

        for value in values {
            if value.is_empty() {
                continue;
            }

            if let Some(m) = re.find(value) {
                let matched = m.as_str();

                // Apply validator if present
                if let Some(validator) = pat.validator {
                    if !validator(matched) {
                        continue;
                    }
                }

                // Deduplicate by pattern name within this call
                let key = pat.name.to_string();
                if seen.contains(&key) {
                    continue;
                }
                seen.insert(key);

                let redacted = redact(matched, &pat.category, pat.name);

                findings.push(ContentFinding {
                    column_name: column_name.to_string(),
                    category: pat.category.clone(),
                    pattern_name: pat.name.to_string(),
                    confidence: pat.confidence.clone(),
                    redacted_sample: redacted,
                });
            }
        }
    }

    findings
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::{Category, Confidence};

    #[test]
    fn test_detects_ssn() {
        let findings = scan_content("data", &["123-45-6789"]);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].category, Category::PiiIdentity);
        assert_eq!(findings[0].pattern_name, "ssn");
        assert_eq!(findings[0].redacted_sample, "***-**-6789");
    }

    #[test]
    fn test_rejects_invalid_ssn() {
        // Area 000 is invalid
        let findings = scan_content("data", &["000-45-6789"]);
        assert!(findings.is_empty());
    }

    #[test]
    fn test_detects_email() {
        let findings = scan_content("data", &["test@example.com"]);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].category, Category::PiiContact);
        assert_eq!(findings[0].redacted_sample, "****@example.com");
    }

    #[test]
    fn test_detects_valid_credit_card() {
        let findings = scan_content("data", &["4111111111111111"]);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].category, Category::PiiFinancial);
        assert_eq!(findings[0].confidence, Confidence::High);
    }

    #[test]
    fn test_rejects_invalid_luhn() {
        let findings = scan_content("data", &["4111111111111112"]);
        // Should not detect as credit card due to failed Luhn
        let cc_findings: Vec<_> = findings
            .iter()
            .filter(|f| f.pattern_name == "credit_card_number")
            .collect();
        assert!(cc_findings.is_empty());
    }

    #[test]
    fn test_detects_stripe_key() {
        let findings = scan_content("data", &["sk_live_abc123def456"]);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].category, Category::Secrets);
        assert_eq!(findings[0].pattern_name, "stripe_key");
    }

    #[test]
    fn test_detects_jwt() {
        let jwt = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U";
        let findings = scan_content("token_col", &[jwt]);
        assert!(findings.iter().any(|f| f.pattern_name == "jwt_token"));
    }

    #[test]
    fn test_detects_connection_uri() {
        let findings = scan_content("config", &["postgres://user:pass@host:5432/db"]);
        assert!(findings.iter().any(|f| f.pattern_name == "connection_uri"));
    }

    #[test]
    fn test_detects_aws_key() {
        let findings = scan_content("key", &["AKIAIOSFODNN7EXAMPLE"]);
        assert!(findings.iter().any(|f| f.pattern_name == "aws_access_key"));
    }

    #[test]
    fn test_detects_json_secret() {
        let findings = scan_content("agent_data", &[
            r#"{"livekit_api_key": "sk_live_abc123def456"}"#,
        ]);
        // Should detect both json_secret_key and stripe_key
        let pattern_names: Vec<&str> = findings.iter().map(|f| f.pattern_name.as_str()).collect();
        assert!(
            pattern_names.contains(&"json_secret_key") || pattern_names.contains(&"stripe_key"),
            "expected json_secret_key or stripe_key, got {:?}",
            pattern_names
        );
    }

    #[test]
    fn test_no_findings_for_clean_data() {
        let findings = scan_content("amount", &["100.50", "200.75", "50.00"]);
        assert!(findings.is_empty());
    }

    #[test]
    fn test_deduplicates_within_call() {
        let findings = scan_content("emails", &[
            "alice@example.com",
            "bob@example.com",
            "charlie@example.com",
        ]);
        // Only one email finding per call
        let email_findings: Vec<_> = findings
            .iter()
            .filter(|f| f.pattern_name == "email_address")
            .collect();
        assert_eq!(email_findings.len(), 1);
    }

    #[test]
    fn test_redacted_sample_never_contains_full_value() {
        let findings = scan_content("data", &["123-45-6789"]);
        assert!(!findings[0].redacted_sample.contains("123-45-6789"));
    }
}
