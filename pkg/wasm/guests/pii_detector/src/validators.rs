/// Validates a credit card number using the Luhn algorithm.
/// Strips spaces and dashes before checking.
pub fn validate_luhn(input: &str) -> bool {
    let digits: Vec<u32> = input
        .chars()
        .filter(|c| c.is_ascii_digit())
        .filter_map(|c| c.to_digit(10))
        .collect();

    if digits.len() < 13 || digits.len() > 19 {
        return false;
    }

    let mut sum = 0u32;
    let mut double = false;

    for &d in digits.iter().rev() {
        let mut val = d;
        if double {
            val *= 2;
            if val > 9 {
                val -= 9;
            }
        }
        sum += val;
        double = !double;
    }

    sum % 10 == 0
}

/// Validates SSN format: area (001-899, not 666), group (01-99), serial (0001-9999).
pub fn validate_ssn(input: &str) -> bool {
    let parts: Vec<&str> = input.split('-').collect();
    if parts.len() != 3 {
        return false;
    }

    let area: u16 = match parts[0].parse() {
        Ok(v) => v,
        Err(_) => return false,
    };
    let group: u16 = match parts[1].parse() {
        Ok(v) => v,
        Err(_) => return false,
    };
    let serial: u16 = match parts[2].parse() {
        Ok(v) => v,
        Err(_) => return false,
    };

    // Area: 001-899 excluding 666
    if area == 0 || area == 666 || area >= 900 {
        return false;
    }
    // Group: 01-99
    if group == 0 {
        return false;
    }
    // Serial: 0001-9999
    if serial == 0 {
        return false;
    }

    true
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_luhn_valid_cards() {
        // Visa test number
        assert!(validate_luhn("4111111111111111"));
        // With spaces
        assert!(validate_luhn("4111 1111 1111 1111"));
        // With dashes
        assert!(validate_luhn("4111-1111-1111-1111"));
        // Mastercard test number
        assert!(validate_luhn("5500000000000004"));
        // Amex test number
        assert!(validate_luhn("378282246310005"));
    }

    #[test]
    fn test_luhn_invalid_cards() {
        assert!(!validate_luhn("4111111111111112"));
        assert!(!validate_luhn("1234567890123456"));
    }

    #[test]
    fn test_luhn_too_short() {
        assert!(!validate_luhn("411111"));
    }

    #[test]
    fn test_ssn_valid() {
        assert!(validate_ssn("123-45-6789"));
        assert!(validate_ssn("001-01-0001"));
        assert!(validate_ssn("899-99-9999"));
    }

    #[test]
    fn test_ssn_invalid_area() {
        // Area 000 is invalid
        assert!(!validate_ssn("000-45-6789"));
        // Area 666 is invalid
        assert!(!validate_ssn("666-45-6789"));
        // Area 900+ is invalid
        assert!(!validate_ssn("900-45-6789"));
        assert!(!validate_ssn("999-45-6789"));
    }

    #[test]
    fn test_ssn_invalid_group() {
        // Group 00 is invalid
        assert!(!validate_ssn("123-00-6789"));
    }

    #[test]
    fn test_ssn_invalid_serial() {
        // Serial 0000 is invalid
        assert!(!validate_ssn("123-45-0000"));
    }

    #[test]
    fn test_ssn_bad_format() {
        assert!(!validate_ssn("12345-6789"));
        assert!(!validate_ssn("abc-de-fghi"));
    }
}
