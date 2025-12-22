package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// thinkTagPattern matches <think>...</think> tags that may appear at the start of LLM responses.
var thinkTagPattern = regexp.MustCompile(`(?s)^[\s]*<think>.*?</think>[\s]*`)

// thinkContentPattern extracts the content inside <think>...</think> tags.
var thinkContentPattern = regexp.MustCompile(`(?s)<think>(.*?)</think>`)

// ExtractThinking extracts the content from <think>...</think> tags in an LLM response.
// Returns empty string if no thinking tags are found.
func ExtractThinking(response string) string {
	matches := thinkContentPattern.FindStringSubmatch(response)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// ExtractJSON extracts JSON content from an LLM response that may contain
// <think> tags, markdown code blocks, or other formatting.
func ExtractJSON(response string) (string, error) {
	// Strip <think>...</think> tags from the start of the response
	cleaned := thinkTagPattern.ReplaceAllString(response, "")

	// Find the first occurrence of { or [ to determine JSON type
	objStart := strings.IndexByte(cleaned, '{')
	arrStart := strings.IndexByte(cleaned, '[')

	// Try whichever comes first (or the one that exists)
	if objStart >= 0 && (arrStart < 0 || objStart < arrStart) {
		if jsonStr, ok := extractBalancedJSON(cleaned, '{', '}'); ok {
			if json.Valid([]byte(jsonStr)) {
				return jsonStr, nil
			}
		}
	}

	if arrStart >= 0 {
		if jsonStr, ok := extractBalancedJSON(cleaned, '[', ']'); ok {
			if json.Valid([]byte(jsonStr)) {
				return jsonStr, nil
			}
		}
	}

	// Last resort: check if the entire cleaned response is valid JSON
	trimmed := strings.TrimSpace(cleaned)
	if json.Valid([]byte(trimmed)) {
		return trimmed, nil
	}

	return "", fmt.Errorf("no valid JSON found in response")
}

// extractBalancedJSON finds the first balanced JSON structure starting with openChar.
// It handles nested structures by counting bracket depth.
func extractBalancedJSON(s string, openChar, closeChar byte) (string, bool) {
	// Find the first occurrence of the opening bracket
	start := strings.IndexByte(s, openChar)
	if start == -1 {
		return "", false
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(s); i++ {
		c := s[i]

		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inString {
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if c == openChar {
			depth++
		} else if c == closeChar {
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}

	return "", false
}

// ParseJSONResponse extracts JSON from a response and unmarshals it into the target.
func ParseJSONResponse[T any](response string) (T, error) {
	var result T

	jsonStr, err := ExtractJSON(response)
	if err != nil {
		return result, err
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return result, fmt.Errorf("unmarshal JSON: %w", err)
	}

	return result, nil
}
