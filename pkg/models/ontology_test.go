package models

import (
	"encoding/json"
	"testing"
)

func TestEnumValue_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"string value", `{"value": "active", "label": "Active"}`, "active"},
		{"integer value", `{"value": 1, "label": "One"}`, "1"},
		{"zero value", `{"value": 0, "label": "Zero"}`, "0"},
		{"float value", `{"value": 3.14, "label": "Pi"}`, "3.14"},
		{"boolean true", `{"value": true, "label": "Yes"}`, "true"},
		{"boolean false", `{"value": false, "label": "No"}`, "false"},
		{"negative number", `{"value": -5, "label": "Negative"}`, "-5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ev EnumValue
			if err := json.Unmarshal([]byte(tt.input), &ev); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if ev.Value != tt.expected {
				t.Errorf("Value = %q, want %q", ev.Value, tt.expected)
			}
		})
	}
}

func TestEnumValue_UnmarshalJSON_Array(t *testing.T) {
	input := `[
		{"value": "active", "label": "Active"},
		{"value": 1, "label": "One"},
		{"value": true, "label": "Yes"}
	]`

	var evs []EnumValue
	if err := json.Unmarshal([]byte(input), &evs); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(evs) != 3 {
		t.Fatalf("Expected 3 values, got %d", len(evs))
	}

	expected := []string{"active", "1", "true"}
	for i, ev := range evs {
		if ev.Value != expected[i] {
			t.Errorf("evs[%d].Value = %q, want %q", i, ev.Value, expected[i])
		}
	}
}
