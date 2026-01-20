package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrimString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
		{"leading whitespace", "  test", "test"},
		{"trailing whitespace", "test  ", "test"},
		{"both sides whitespace", "  test  ", "test"},
		{"tabs", "\ttest\t", "test"},
		{"newlines", "\ntest\n", "test"},
		{"mixed whitespace", " \t\ntest\n\t ", "test"},
		{"no whitespace", "test", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
