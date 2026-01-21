package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// Unit Tests for Helper Functions
// ============================================================================

func TestIsStringType(t *testing.T) {
	tests := []struct {
		dataType string
		expected bool
	}{
		{"text", true},
		{"varchar", true},
		{"varchar(255)", true},
		{"character varying", true},
		{"character varying(100)", true},
		{"char", true},
		{"char(10)", true},
		{"nvarchar", true},
		{"nvarchar(max)", true},
		{"nchar", true},
		{"ntext", true},
		{"string", true},
		{"integer", false},
		{"bigint", false},
		{"numeric", false},
		{"timestamp", false},
		{"boolean", false},
		{"uuid", false},
		{"json", false},
		{"jsonb", false},
	}

	for _, tc := range tests {
		t.Run(tc.dataType, func(t *testing.T) {
			result := isStringType(tc.dataType)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestDefaultDataChangeDetectionConfig(t *testing.T) {
	cfg := DefaultDataChangeDetectionConfig()

	assert.Equal(t, 100, cfg.MaxDistinctValuesForEnum, "MaxDistinctValuesForEnum default")
	assert.Equal(t, 100, cfg.MaxEnumValueLength, "MaxEnumValueLength default")
	assert.Equal(t, 0.9, cfg.MinMatchRateForFK, "MinMatchRateForFK default")
}
