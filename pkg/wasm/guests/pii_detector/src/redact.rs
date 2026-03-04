use crate::types::Category;

/// Redact a matched value based on its category and pattern name.
/// Returns a redacted string that preserves enough structure for recognition
/// but hides the sensitive data.
pub fn redact(value: &str, category: &Category, pattern_name: &str) -> String {
    match (category, pattern_name) {
        (Category::PiiIdentity, "ssn") => redact_ssn(value),
        (Category::PiiFinancial, "credit_card_number") => redact_credit_card(value),
        (Category::PiiContact, "email_address") => redact_email(value),
        (Category::Secrets, "stripe_key") => redact_api_key(value),
        (Category::Secrets, "aws_access_key") => redact_api_key(value),
        (Category::Secrets, "jwt_token") => redact_jwt(value),
        (Category::Secrets, "pem_private_key") => "[PRIVATE KEY DETECTED]".to_string(),
        (Category::Secrets, "connection_uri") => redact_connection_uri(value),
        (Category::Secrets, "json_secret_key") => redact_json_secret(value),
        _ => redact_generic(value),
    }
}

/// SSN: `***-**-6789`
fn redact_ssn(value: &str) -> String {
    let parts: Vec<&str> = value.split('-').collect();
    if parts.len() == 3 {
        format!("***-**-{}", parts[2])
    } else {
        redact_generic(value)
    }
}

/// Credit card: `****-****-****-1111`
fn redact_credit_card(value: &str) -> String {
    let digits: String = value.chars().filter(|c| c.is_ascii_digit()).collect();
    if digits.len() >= 4 {
        let last4 = &digits[digits.len() - 4..];
        format!("****-****-****-{}", last4)
    } else {
        redact_generic(value)
    }
}

/// Email: `****@company.com`
fn redact_email(value: &str) -> String {
    if let Some(at_pos) = value.find('@') {
        format!("****{}", &value[at_pos..])
    } else {
        redact_generic(value)
    }
}

/// API key: show prefix + `****`
fn redact_api_key(value: &str) -> String {
    if value.len() > 8 {
        format!("{}****", &value[..8])
    } else {
        redact_generic(value)
    }
}

/// JWT: `eyJ****...`
fn redact_jwt(value: &str) -> String {
    if value.len() > 3 {
        format!("{}****...", &value[..3])
    } else {
        redact_generic(value)
    }
}

/// Connection URI: `postgres://****`
fn redact_connection_uri(value: &str) -> String {
    if let Some(pos) = value.find("://") {
        format!("{}://****", &value[..pos])
    } else {
        redact_generic(value)
    }
}

/// JSON secret: redact value but keep key structure visible.
fn redact_json_secret(value: &str) -> String {
    // Try to find the key name and redact the value part
    // Pattern: "key": "value" or key=value
    if let Some(colon_pos) = value.find(':') {
        let key_part = &value[..=colon_pos];
        format!("{} \"****\"", key_part.trim_end_matches(':'))
    } else if let Some(eq_pos) = value.find('=') {
        let key_part = &value[..=eq_pos];
        format!("{}****", key_part)
    } else {
        redact_generic(value)
    }
}

/// Generic redaction: show first 3 chars + ****
fn redact_generic(value: &str) -> String {
    if value.len() > 3 {
        format!("{}****", &value[..3])
    } else {
        "****".to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_redact_ssn() {
        assert_eq!(
            redact("123-45-6789", &Category::PiiIdentity, "ssn"),
            "***-**-6789"
        );
    }

    #[test]
    fn test_redact_credit_card() {
        assert_eq!(
            redact("4111111111111111", &Category::PiiFinancial, "credit_card_number"),
            "****-****-****-1111"
        );
        assert_eq!(
            redact("4111 1111 1111 1111", &Category::PiiFinancial, "credit_card_number"),
            "****-****-****-1111"
        );
    }

    #[test]
    fn test_redact_email() {
        assert_eq!(
            redact("alice@company.com", &Category::PiiContact, "email_address"),
            "****@company.com"
        );
    }

    #[test]
    fn test_redact_stripe_key() {
        assert_eq!(
            redact("sk_live_abc123def456", &Category::Secrets, "stripe_key"),
            "sk_live_****"
        );
    }

    #[test]
    fn test_redact_jwt() {
        let jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123";
        assert_eq!(
            redact(jwt, &Category::Secrets, "jwt_token"),
            "eyJ****..."
        );
    }

    #[test]
    fn test_redact_pem() {
        assert_eq!(
            redact("-----BEGIN RSA PRIVATE KEY-----\nMIIE...", &Category::Secrets, "pem_private_key"),
            "[PRIVATE KEY DETECTED]"
        );
    }

    #[test]
    fn test_redact_connection_uri() {
        assert_eq!(
            redact("postgres://user:pass@host:5432/db", &Category::Secrets, "connection_uri"),
            "postgres://****"
        );
    }

    #[test]
    fn test_redact_json_secret() {
        let result = redact(
            r#""api_key": "sk_live_abc123def456""#,
            &Category::Secrets,
            "json_secret_key",
        );
        assert!(result.contains("****"));
        assert!(!result.contains("sk_live_abc123def456"));
    }
}
