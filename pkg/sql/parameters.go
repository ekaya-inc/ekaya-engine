package sql

import (
	"fmt"
	"regexp"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// parameterRegex matches {{parameter_name}} placeholders in SQL templates.
// Parameter names must start with a letter or underscore, followed by any
// number of alphanumeric characters or underscores.
var parameterRegex = regexp.MustCompile(`\{\{([a-zA-Z_]\w*)\}\}`)

// ExtractParameters finds all {{param}} placeholders in SQL and returns
// a deduplicated list of parameter names in order of first appearance.
//
// Example:
//
//	sql := "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}}"
//	params := ExtractParameters(sql)
//	// params == []string{"customer_id", "min_total"}
//
// If the same parameter appears multiple times, it's only included once:
//
//	sql := "SELECT * FROM transactions WHERE sender_id = {{user_id}} OR receiver_id = {{user_id}}"
//	params := ExtractParameters(sql)
//	// params == []string{"user_id"}
func ExtractParameters(sqlQuery string) []string {
	matches := parameterRegex.FindAllStringSubmatch(sqlQuery, -1)
	seen := make(map[string]bool)
	var params []string

	for _, match := range matches {
		name := match[1]
		if !seen[name] {
			seen[name] = true
			params = append(params, name)
		}
	}

	return params
}

// ValidateParameterDefinitions checks that all template parameters used in the SQL
// have corresponding parameter definitions.
//
// Returns an error if:
//   - A {{param}} placeholder is used in SQL but not defined in params
//
// Example:
//
//	sql := "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}}"
//	params := []models.QueryParameter{
//	    {Name: "customer_id", Type: "uuid", Required: true},
//	    // min_total is missing!
//	}
//	err := ValidateParameterDefinitions(sql, params)
//	// err != nil: "parameter {{min_total}} used in SQL but not defined"
func ValidateParameterDefinitions(sqlQuery string, params []models.QueryParameter) error {
	extracted := ExtractParameters(sqlQuery)
	defined := make(map[string]bool)

	for _, p := range params {
		defined[p.Name] = true
	}

	for _, name := range extracted {
		if !defined[name] {
			return fmt.Errorf("parameter {{%s}} used in SQL but not defined", name)
		}
	}

	return nil
}

// SubstituteParameters replaces {{param}} placeholders with PostgreSQL positional
// parameters ($1, $2, etc.) and returns the prepared SQL along with ordered parameter
// values for binding.
//
// The function:
//  1. Replaces each unique {{param}} with $N (where N is the position)
//  2. Reuses the same $N for parameters that appear multiple times
//  3. Applies default values for parameters not supplied
//  4. Returns ordered values matching the positional indices
//
// Example:
//
//	sql := "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}}"
//	paramDefs := []models.QueryParameter{
//	    {Name: "customer_id", Type: "uuid", Required: true},
//	    {Name: "min_total", Type: "decimal", Required: false, Default: 0.00},
//	}
//	suppliedValues := map[string]any{
//	    "customer_id": "550e8400-e29b-41d4-a716-446655440000",
//	}
//
//	preparedSQL, orderedValues, err := SubstituteParameters(sql, paramDefs, suppliedValues)
//	// preparedSQL == "SELECT * FROM orders WHERE customer_id = $1 AND total > $2"
//	// orderedValues == []any{"550e8400-e29b-41d4-a716-446655440000", 0.00}
//
// For parameters used multiple times:
//
//	sql := "SELECT * FROM transactions WHERE sender_id = {{user_id}} OR receiver_id = {{user_id}}"
//	paramDefs := []models.QueryParameter{
//	    {Name: "user_id", Type: "uuid", Required: true},
//	}
//	suppliedValues := map[string]any{
//	    "user_id": "550e8400-e29b-41d4-a716-446655440000",
//	}
//
//	preparedSQL, orderedValues, err := SubstituteParameters(sql, paramDefs, suppliedValues)
//	// preparedSQL == "SELECT * FROM transactions WHERE sender_id = $1 OR receiver_id = $1"
//	// orderedValues == []any{"550e8400-e29b-41d4-a716-446655440000"}
func SubstituteParameters(
	sqlQuery string,
	paramDefs []models.QueryParameter,
	suppliedValues map[string]any,
) (string, []any, error) {
	// Build lookup for parameter definitions
	defLookup := make(map[string]models.QueryParameter)
	for _, p := range paramDefs {
		defLookup[p.Name] = p
	}

	// Track parameter order for positional binding
	var orderedValues []any
	paramIndex := 1
	paramPositions := make(map[string]int)

	result := parameterRegex.ReplaceAllStringFunc(sqlQuery, func(match string) string {
		name := parameterRegex.FindStringSubmatch(match)[1]

		// Check if already assigned position (same param used multiple times)
		if pos, exists := paramPositions[name]; exists {
			return fmt.Sprintf("$%d", pos)
		}

		def, defExists := defLookup[name]
		if !defExists {
			// This should have been caught by ValidateParameterDefinitions
			// Return the original match to avoid breaking the SQL
			return match
		}

		value, supplied := suppliedValues[name]

		// Use supplied value or fall back to default
		if !supplied {
			value = def.Default
		}

		paramPositions[name] = paramIndex
		orderedValues = append(orderedValues, value)
		pos := paramIndex
		paramIndex++

		return fmt.Sprintf("$%d", pos)
	})

	return result, orderedValues, nil
}
