package models

import (
	"encoding/json"
	"testing"
)

func TestEnumDefinition_JSON(t *testing.T) {
	def := EnumDefinition{
		Table:  "billing_transactions",
		Column: "transaction_state",
		Values: map[string]string{
			"0": "UNSPECIFIED - Not set",
			"1": "STARTED - Transaction started",
			"2": "ENDED - Transaction ended",
		},
	}

	// Marshal
	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	var decoded EnumDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Table != def.Table {
		t.Errorf("Table = %q, want %q", decoded.Table, def.Table)
	}
	if decoded.Column != def.Column {
		t.Errorf("Column = %q, want %q", decoded.Column, def.Column)
	}
	if len(decoded.Values) != len(def.Values) {
		t.Errorf("Values len = %d, want %d", len(decoded.Values), len(def.Values))
	}
	for k, v := range def.Values {
		if decoded.Values[k] != v {
			t.Errorf("Values[%q] = %q, want %q", k, decoded.Values[k], v)
		}
	}
}

func TestEnumDefinition_WildcardTable(t *testing.T) {
	// Verify wildcard table syntax works
	def := EnumDefinition{
		Table:  "*",
		Column: "offer_type",
		Values: map[string]string{
			"1": "FREE - Free Engagement",
			"2": "PAID - Preauthorized per-minute",
		},
	}

	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded EnumDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Table != "*" {
		t.Errorf("Table = %q, want '*'", decoded.Table)
	}
}

func TestProjectConfig_JSON(t *testing.T) {
	cfg := ProjectConfig{
		EnumDefinitions: []EnumDefinition{
			{
				Table:  "billing_transactions",
				Column: "transaction_state",
				Values: map[string]string{
					"1": "STARTED",
					"2": "ENDED",
				},
			},
			{
				Table:  "*",
				Column: "offer_type",
				Values: map[string]string{
					"1": "FREE",
					"2": "PAID",
				},
			},
		},
	}

	// Marshal
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	var decoded ProjectConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.EnumDefinitions) != 2 {
		t.Fatalf("EnumDefinitions len = %d, want 2", len(decoded.EnumDefinitions))
	}

	if decoded.EnumDefinitions[0].Table != "billing_transactions" {
		t.Errorf("EnumDefinitions[0].Table = %q, want 'billing_transactions'", decoded.EnumDefinitions[0].Table)
	}
	if decoded.EnumDefinitions[1].Table != "*" {
		t.Errorf("EnumDefinitions[1].Table = %q, want '*'", decoded.EnumDefinitions[1].Table)
	}
}

func TestProjectConfig_Empty(t *testing.T) {
	cfg := ProjectConfig{}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Empty config should marshal to {} due to omitempty
	expected := "{}"
	if string(data) != expected {
		t.Errorf("Marshal = %q, want %q", string(data), expected)
	}

	// Unmarshal empty JSON
	var decoded ProjectConfig
	if err := json.Unmarshal([]byte("{}"), &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.EnumDefinitions != nil {
		t.Errorf("EnumDefinitions = %v, want nil", decoded.EnumDefinitions)
	}
}

func TestJSONBMap_Value(t *testing.T) {
	tests := []struct {
		name     string
		input    JSONBMap
		wantJSON string
	}{
		{"nil map", nil, "{}"},
		{"empty map", JSONBMap{}, "{}"},
		{"with values", JSONBMap{"key": "value"}, `{"key":"value"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.Value()
			if err != nil {
				t.Fatalf("Value() error = %v", err)
			}
			gotBytes, ok := got.([]byte)
			if !ok {
				t.Fatalf("Value() returned %T, want []byte", got)
			}
			if string(gotBytes) != tt.wantJSON {
				t.Errorf("Value() = %q, want %q", string(gotBytes), tt.wantJSON)
			}
		})
	}
}

func TestJSONBMap_Scan(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    JSONBMap
		wantErr bool
	}{
		{"nil value", nil, JSONBMap{}, false},
		{"empty json", []byte("{}"), JSONBMap{}, false},
		{"with values", []byte(`{"foo":"bar"}`), JSONBMap{"foo": "bar"}, false},
		{"invalid type", "not bytes", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var j JSONBMap
			err := j.Scan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Scan() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(j) != len(tt.want) {
					t.Errorf("Scan() len = %d, want %d", len(j), len(tt.want))
				}
			}
		})
	}
}
