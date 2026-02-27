package jsonutil

import (
	"encoding/json"
	"testing"
)

func TestFlexibleStringValue(t *testing.T) {
	tests := []struct {
		name  string
		input json.RawMessage
		want  string
	}{
		{
			name:  "string value",
			input: json.RawMessage(`"hello"`),
			want:  "hello",
		},
		{
			name:  "integer value",
			input: json.RawMessage(`42`),
			want:  "42",
		},
		{
			name:  "float value",
			input: json.RawMessage(`3.14`),
			want:  "3.14",
		},
		{
			name:  "boolean true",
			input: json.RawMessage(`true`),
			want:  "true",
		},
		{
			name:  "boolean false",
			input: json.RawMessage(`false`),
			want:  "false",
		},
		{
			name:  "null value",
			input: json.RawMessage(`null`),
			want:  "",
		},
		{
			name:  "empty raw message",
			input: json.RawMessage{},
			want:  "",
		},
		{
			name:  "nil raw message",
			input: nil,
			want:  "",
		},
		{
			name:  "large integer preserves precision",
			input: json.RawMessage(`9007199254740992`),
			want:  "9007199254740992",
		},
		{
			name:  "nested object falls back to raw string",
			input: json.RawMessage(`{"key":"value"}`),
			want:  `{"key":"value"}`,
		},
		{
			name:  "array falls back to raw string",
			input: json.RawMessage(`[1,2,3]`),
			want:  `[1,2,3]`,
		},
		{
			name:  "negative integer",
			input: json.RawMessage(`-7`),
			want:  "-7",
		},
		{
			name:  "zero",
			input: json.RawMessage(`0`),
			want:  "0",
		},
		{
			name:  "empty string",
			input: json.RawMessage(`""`),
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FlexibleStringValue(tt.input)
			if got != tt.want {
				t.Errorf("FlexibleStringValue(%s) = %q, want %q", string(tt.input), got, tt.want)
			}
		})
	}
}
