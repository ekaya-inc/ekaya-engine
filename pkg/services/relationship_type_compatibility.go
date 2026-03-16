package services

import "strings"

// areTypesCompatibleForFK checks if source and target column types are compatible for FK relationships.
// Supports exact match, UUID compatibility (text <-> uuid <-> varchar <-> character varying),
// and integer compatibility (int <-> integer <-> bigint <-> smallint <-> serial).
func areTypesCompatibleForFK(sourceType, targetType string) bool {
	source := strings.ToLower(sourceType)
	target := strings.ToLower(targetType)

	if idx := strings.Index(source, "("); idx > 0 {
		source = source[:idx]
	}
	if idx := strings.Index(target, "("); idx > 0 {
		target = target[:idx]
	}
	source = strings.TrimSpace(source)
	target = strings.TrimSpace(target)

	if source == target {
		return true
	}

	uuidTypes := map[string]bool{
		"uuid":              true,
		"text":              true,
		"varchar":           true,
		"character varying": true,
	}
	if uuidTypes[source] && uuidTypes[target] {
		return true
	}

	intTypes := map[string]bool{
		"int":       true,
		"int2":      true,
		"int4":      true,
		"int8":      true,
		"integer":   true,
		"bigint":    true,
		"smallint":  true,
		"serial":    true,
		"bigserial": true,
	}
	return intTypes[source] && intTypes[target]
}
