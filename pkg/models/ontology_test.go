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
		// Object format with various value types
		{"object with string value", `{"value": "active", "label": "Active"}`, "active"},
		{"object with integer value", `{"value": 1, "label": "One"}`, "1"},
		{"object with zero value", `{"value": 0, "label": "Zero"}`, "0"},
		{"object with float value", `{"value": 3.14, "label": "Pi"}`, "3.14"},
		{"object with boolean true", `{"value": true, "label": "Yes"}`, "true"},
		{"object with boolean false", `{"value": false, "label": "No"}`, "false"},
		{"object with negative number", `{"value": -5, "label": "Negative"}`, "-5"},
		// Plain string format (LLM sometimes returns just strings)
		{"plain string", `"AUD"`, "AUD"},
		{"plain string with spaces", `"United States Dollar"`, "United States Dollar"},
		{"plain empty string", `""`, ""},
		// Plain number format
		{"plain integer", `42`, "42"},
		{"plain zero", `0`, "0"},
		{"plain negative", `-10`, "-10"},
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

func TestEnumValue_UnmarshalJSON_ArrayOfObjects(t *testing.T) {
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

func TestEnumValue_UnmarshalJSON_ArrayOfStrings(t *testing.T) {
	// LLMs sometimes return enum values as plain strings instead of objects
	input := `["AUD", "EUR", "USD", "GBP"]`

	var evs []EnumValue
	if err := json.Unmarshal([]byte(input), &evs); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(evs) != 4 {
		t.Fatalf("Expected 4 values, got %d", len(evs))
	}

	expected := []string{"AUD", "EUR", "USD", "GBP"}
	for i, ev := range evs {
		if ev.Value != expected[i] {
			t.Errorf("evs[%d].Value = %q, want %q", i, ev.Value, expected[i])
		}
		// Plain strings should have empty label and description
		if ev.Label != "" {
			t.Errorf("evs[%d].Label = %q, want empty", i, ev.Label)
		}
		if ev.Description != "" {
			t.Errorf("evs[%d].Description = %q, want empty", i, ev.Description)
		}
	}
}

func TestEnumValue_UnmarshalJSON_MixedArray(t *testing.T) {
	// Mix of objects, strings, and numbers in same array
	input := `[
		{"value": "active", "label": "Active", "description": "Currently active"},
		"pending",
		42,
		{"value": 0, "label": "Zero"}
	]`

	var evs []EnumValue
	if err := json.Unmarshal([]byte(input), &evs); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(evs) != 4 {
		t.Fatalf("Expected 4 values, got %d", len(evs))
	}

	// First: object with all fields
	if evs[0].Value != "active" || evs[0].Label != "Active" || evs[0].Description != "Currently active" {
		t.Errorf("evs[0] = %+v, want {active, Active, Currently active}", evs[0])
	}
	// Second: plain string
	if evs[1].Value != "pending" || evs[1].Label != "" {
		t.Errorf("evs[1] = %+v, want {pending, , }", evs[1])
	}
	// Third: plain number
	if evs[2].Value != "42" || evs[2].Label != "" {
		t.Errorf("evs[2] = %+v, want {42, , }", evs[2])
	}
	// Fourth: object with value and label
	if evs[3].Value != "0" || evs[3].Label != "Zero" {
		t.Errorf("evs[3] = %+v, want {0, Zero, }", evs[3])
	}
}
