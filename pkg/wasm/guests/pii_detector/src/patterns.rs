use crate::types::{Category, Confidence};
use regex_lite::Regex;

/// A column name pattern that indicates PII or secrets.
pub struct ColumnPattern {
    pub name: &'static str,
    pub category: Category,
    pub confidence: Confidence,
    pub regex: &'static str,
    /// Column names matching this regex are excluded (e.g., password_hash).
    pub exclude_regex: Option<&'static str>,
}

/// A content pattern that detects PII or secrets in cell values.
pub struct ContentPattern {
    pub name: &'static str,
    pub category: Category,
    pub confidence: Confidence,
    pub regex: &'static str,
    /// Optional validation function applied after regex match.
    pub validator: Option<fn(&str) -> bool>,
}

/// All column name patterns for detecting PII/secrets.
pub fn column_patterns() -> Vec<ColumnPattern> {
    vec![
        // --- Secrets ---
        ColumnPattern {
            name: "api_key",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(api[_]?key|apikey)(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "secret_key",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(secret[_]?key|secretkey)(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "access_token",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(access[_]?token|accesstoken)(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "private_key",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(private[_]?key|privatekey)(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "password",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)password(_|$)",
            exclude_regex: Some(r"(?i)password[_]?(hash|reset|changed|updated|expires|expiry|policy|history|length|salt)"),
        },
        ColumnPattern {
            name: "credential",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)credentials?(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "connection_string",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(connection[_]?string|conn[_]?str|connstring)(_|$)",
            exclude_regex: None,
        },
        // --- PII Identity ---
        ColumnPattern {
            name: "ssn",
            category: Category::PiiIdentity,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(ssn|social[_]?security[_]?(number)?)(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "passport",
            category: Category::PiiIdentity,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)passport([_]?(number|num|no|id))?(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "drivers_license",
            category: Category::PiiIdentity,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(drivers?[_]?licen[sc]e|dl[_]?(number|num|no))(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "national_id",
            category: Category::PiiIdentity,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(national[_]?id)(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "date_of_birth",
            category: Category::PiiIdentity,
            confidence: Confidence::Medium,
            regex: r"(?i)(^|_)(date[_]?of[_]?birth|dob|birth[_]?date|birthday)(_|$)",
            exclude_regex: None,
        },
        // --- PII Contact ---
        ColumnPattern {
            name: "email",
            category: Category::PiiContact,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)e?mail([_]?(address|addr))?(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "phone",
            category: Category::PiiContact,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(phone|telephone|mobile|cell)[_]?(number|num|no)?(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "address",
            category: Category::PiiContact,
            confidence: Confidence::Medium,
            regex: r"(?i)(^|_)(street[_]?address|home[_]?address|mailing[_]?address|physical[_]?address|address[_]?(line|1|2))(_|$)",
            exclude_regex: None,
        },
        // --- PII Financial ---
        ColumnPattern {
            name: "credit_card",
            category: Category::PiiFinancial,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(credit[_]?card|card[_]?number|cc[_]?(number|num|no))(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "bank_account",
            category: Category::PiiFinancial,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(bank[_]?account|account[_]?number|acct[_]?(number|num|no))(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "routing_number",
            category: Category::PiiFinancial,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)(routing[_]?(number|num|no))(_|$)",
            exclude_regex: None,
        },
        ColumnPattern {
            name: "iban",
            category: Category::PiiFinancial,
            confidence: Confidence::High,
            regex: r"(?i)(^|_)iban(_|$)",
            exclude_regex: None,
        },
    ]
}

/// All content patterns for detecting PII/secrets in cell values.
pub fn content_patterns() -> Vec<ContentPattern> {
    vec![
        // --- Secrets ---
        ContentPattern {
            name: "stripe_key",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"(sk_live_|sk_test_|pk_live_|pk_test_)[a-zA-Z0-9]{10,}",
            validator: None,
        },
        ContentPattern {
            name: "aws_access_key",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"AKIA[0-9A-Z]{16}",
            validator: None,
        },
        ContentPattern {
            name: "jwt_token",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"eyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}",
            validator: None,
        },
        ContentPattern {
            name: "pem_private_key",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----",
            validator: None,
        },
        ContentPattern {
            name: "connection_uri",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r"(postgres|postgresql|mysql|mongodb|redis)://[^\s]{5,}",
            validator: None,
        },
        ContentPattern {
            name: "json_secret_key",
            category: Category::Secrets,
            confidence: Confidence::High,
            regex: r#"["']?(api[_]?key|secret[_]?key|access[_]?token|private[_]?key|password|credential|livekit_api_key)["']?\s*[:=]\s*["']?[a-zA-Z0-9_\-]{8,}"#,
            validator: None,
        },
        // --- PII Identity ---
        ContentPattern {
            name: "ssn",
            category: Category::PiiIdentity,
            confidence: Confidence::High,
            regex: r"\b\d{3}-\d{2}-\d{4}\b",
            validator: Some(crate::validators::validate_ssn),
        },
        // --- PII Contact ---
        ContentPattern {
            name: "email_address",
            category: Category::PiiContact,
            confidence: Confidence::High,
            regex: r"\b[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\b",
            validator: None,
        },
        ContentPattern {
            name: "us_phone",
            category: Category::PiiContact,
            confidence: Confidence::Medium,
            regex: r"\b(\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b",
            validator: None,
        },
        // --- PII Financial ---
        ContentPattern {
            name: "credit_card_number",
            category: Category::PiiFinancial,
            confidence: Confidence::High,
            regex: r"\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b",
            validator: Some(crate::validators::validate_luhn),
        },
        ContentPattern {
            name: "iban",
            category: Category::PiiFinancial,
            confidence: Confidence::High,
            regex: r"\b[A-Z]{2}\d{2}[A-Z0-9]{4}\d{7}([A-Z0-9]?){0,16}\b",
            validator: None,
        },
    ]
}

/// Compile a regex pattern, returning None if it fails.
pub fn compile(pattern: &str) -> Option<Regex> {
    Regex::new(pattern).ok()
}
