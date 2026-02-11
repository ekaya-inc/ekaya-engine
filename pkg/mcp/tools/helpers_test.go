package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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

func TestExtractArrayParam(t *testing.T) {
	t.Run("native array", func(t *testing.T) {
		args := map[string]any{
			"tags": []any{"a", "b"},
		}
		result, err := extractArrayParam(args, "tags", nil)
		require.NoError(t, err)
		assert.Equal(t, []any{"a", "b"}, result)
	})

	t.Run("stringified parsable string array", func(t *testing.T) {
		args := map[string]any{
			"tags": `["a","b"]`,
		}
		result, err := extractArrayParam(args, "tags", nil)
		require.NoError(t, err)
		assert.Equal(t, []any{"a", "b"}, result)
	})

	t.Run("stringified parsable object array", func(t *testing.T) {
		args := map[string]any{
			"parameters": `[{"name":"limit","type":"integer","example":20}]`,
		}
		result, err := extractArrayParam(args, "parameters", nil)
		require.NoError(t, err)
		require.Len(t, result, 1)
		obj, ok := result[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "limit", obj["name"])
		assert.Equal(t, "integer", obj["type"])
		assert.Equal(t, float64(20), obj["example"]) // JSON numbers are float64
	})

	t.Run("unparsable string returns error with guidance", func(t *testing.T) {
		args := map[string]any{
			"tags": "not-an-array",
		}
		result, err := extractArrayParam(args, "tags", nil)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "parameter \"tags\"")
		assert.Contains(t, err.Error(), "could not be parsed")
		assert.Contains(t, err.Error(), "native JSON array")
	})

	t.Run("wrong type (number) returns error with type info", func(t *testing.T) {
		args := map[string]any{
			"tags": 123,
		}
		result, err := extractArrayParam(args, "tags", nil)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "parameter \"tags\"")
		assert.Contains(t, err.Error(), "int")
	})

	t.Run("wrong type (bool) returns error with type info", func(t *testing.T) {
		args := map[string]any{
			"tags": true,
		}
		result, err := extractArrayParam(args, "tags", nil)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "bool")
	})

	t.Run("absent key returns nil nil", func(t *testing.T) {
		args := map[string]any{}
		result, err := extractArrayParam(args, "tags", nil)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("string fallback logs warning", func(t *testing.T) {
		core, logs := observer.New(zap.WarnLevel)
		logger := zap.New(core)

		args := map[string]any{
			"tags": `["a","b"]`,
		}
		result, err := extractArrayParam(args, "tags", logger)
		require.NoError(t, err)
		assert.Equal(t, []any{"a", "b"}, result)

		require.Equal(t, 1, logs.Len())
		logEntry := logs.All()[0]
		assert.Equal(t, zap.WarnLevel, logEntry.Level)
		assert.Contains(t, logEntry.Message, "stringified JSON")
		assert.Equal(t, "tags", logEntry.ContextMap()["param"])
	})

	t.Run("native array does not log warning", func(t *testing.T) {
		core, logs := observer.New(zap.WarnLevel)
		logger := zap.New(core)

		args := map[string]any{
			"tags": []any{"a", "b"},
		}
		result, err := extractArrayParam(args, "tags", logger)
		require.NoError(t, err)
		assert.Equal(t, []any{"a", "b"}, result)
		assert.Equal(t, 0, logs.Len())
	})

	t.Run("empty native array", func(t *testing.T) {
		args := map[string]any{
			"tags": []any{},
		}
		result, err := extractArrayParam(args, "tags", nil)
		require.NoError(t, err)
		assert.Equal(t, []any{}, result)
	})

	t.Run("empty stringified array", func(t *testing.T) {
		args := map[string]any{
			"tags": `[]`,
		}
		result, err := extractArrayParam(args, "tags", nil)
		require.NoError(t, err)
		assert.Equal(t, []any{}, result)
	})
}

func TestExtractStringSlice(t *testing.T) {
	t.Run("native string array", func(t *testing.T) {
		args := map[string]any{
			"tags": []any{"billing", "analytics"},
		}
		result, err := extractStringSlice(args, "tags", nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"billing", "analytics"}, result)
	})

	t.Run("stringified string array", func(t *testing.T) {
		args := map[string]any{
			"tags": `["billing","analytics"]`,
		}
		result, err := extractStringSlice(args, "tags", nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"billing", "analytics"}, result)
	})

	t.Run("absent key returns nil nil", func(t *testing.T) {
		args := map[string]any{}
		result, err := extractStringSlice(args, "tags", nil)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("unparsable string returns error", func(t *testing.T) {
		args := map[string]any{
			"tags": "not-json",
		}
		result, err := extractStringSlice(args, "tags", nil)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "parameter \"tags\"")
	})

	t.Run("non-string element returns error", func(t *testing.T) {
		args := map[string]any{
			"tags": []any{"valid", 123, "also-valid"},
		}
		result, err := extractStringSlice(args, "tags", nil)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "element 1")
		assert.Contains(t, err.Error(), "string")
	})

	t.Run("empty array returns empty slice", func(t *testing.T) {
		args := map[string]any{
			"tags": []any{},
		}
		result, err := extractStringSlice(args, "tags", nil)
		require.NoError(t, err)
		assert.Equal(t, []string{}, result)
	})
}
