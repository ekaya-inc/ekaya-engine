package models

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestParseEnumFileContent_YAML(t *testing.T) {
	yamlContent := `enums:
  - table: billing_transactions
    column: transaction_state
    values:
      "0": "UNSPECIFIED - Not set"
      "1": "STARTED - Transaction started"
      "2": "ENDED - Transaction ended"
  - table: "*"
    column: offer_type
    values:
      "1": "FREE - Free Engagement"
      "2": "PAID - Preauthorized per-minute"
`

	defs, err := ParseEnumFileContent([]byte(yamlContent), ".yaml")
	if err != nil {
		t.Fatalf("ParseEnumFileContent failed: %v", err)
	}

	if len(defs) != 2 {
		t.Fatalf("Expected 2 enum definitions, got %d", len(defs))
	}

	// Check first definition
	if defs[0].Table != "billing_transactions" {
		t.Errorf("defs[0].Table = %q, want 'billing_transactions'", defs[0].Table)
	}
	if defs[0].Column != "transaction_state" {
		t.Errorf("defs[0].Column = %q, want 'transaction_state'", defs[0].Column)
	}
	if len(defs[0].Values) != 3 {
		t.Errorf("defs[0].Values len = %d, want 3", len(defs[0].Values))
	}
	if defs[0].Values["1"] != "STARTED - Transaction started" {
		t.Errorf("defs[0].Values[\"1\"] = %q, want 'STARTED - Transaction started'", defs[0].Values["1"])
	}

	// Check second definition (wildcard table)
	if defs[1].Table != "*" {
		t.Errorf("defs[1].Table = %q, want '*'", defs[1].Table)
	}
	if defs[1].Column != "offer_type" {
		t.Errorf("defs[1].Column = %q, want 'offer_type'", defs[1].Column)
	}
	if len(defs[1].Values) != 2 {
		t.Errorf("defs[1].Values len = %d, want 2", len(defs[1].Values))
	}
}

func TestParseEnumFileContent_JSON(t *testing.T) {
	jsonContent := `{
  "enums": [
    {
      "table": "billing_transactions",
      "column": "transaction_state",
      "values": {
        "0": "UNSPECIFIED - Not set",
        "1": "STARTED - Transaction started"
      }
    }
  ]
}`

	defs, err := ParseEnumFileContent([]byte(jsonContent), ".json")
	if err != nil {
		t.Fatalf("ParseEnumFileContent failed: %v", err)
	}

	if len(defs) != 1 {
		t.Fatalf("Expected 1 enum definition, got %d", len(defs))
	}

	if defs[0].Table != "billing_transactions" {
		t.Errorf("defs[0].Table = %q, want 'billing_transactions'", defs[0].Table)
	}
	if len(defs[0].Values) != 2 {
		t.Errorf("defs[0].Values len = %d, want 2", len(defs[0].Values))
	}
}

func TestParseEnumFileContent_Empty(t *testing.T) {
	yamlContent := `enums: []`

	defs, err := ParseEnumFileContent([]byte(yamlContent), ".yaml")
	if err != nil {
		t.Fatalf("ParseEnumFileContent failed: %v", err)
	}

	if len(defs) != 0 {
		t.Errorf("Expected 0 enum definitions, got %d", len(defs))
	}
}

func TestParseEnumFileContent_NoEnumsKey(t *testing.T) {
	yamlContent := `# Empty file with no enums key`

	defs, err := ParseEnumFileContent([]byte(yamlContent), ".yaml")
	if err != nil {
		t.Fatalf("ParseEnumFileContent failed: %v", err)
	}

	// Should return nil/empty when no enums key is present
	if defs != nil && len(defs) != 0 {
		t.Errorf("Expected nil or empty enum definitions, got %d", len(defs))
	}
}

func TestParseEnumFileContent_InvalidYAML(t *testing.T) {
	invalidContent := `enums:
  - table: [invalid
    column: test`

	_, err := ParseEnumFileContent([]byte(invalidContent), ".yaml")
	if err == nil {
		t.Fatal("Expected error for invalid YAML, got nil")
	}
}

func TestParseEnumFileContent_InvalidJSON(t *testing.T) {
	invalidContent := `{"enums": [{"invalid]}`

	_, err := ParseEnumFileContent([]byte(invalidContent), ".json")
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestParseEnumFileContent_AutoDetect(t *testing.T) {
	yamlContent := `enums:
  - table: test_table
    column: status
    values:
      "1": "ACTIVE"
`

	// Test with unknown extension - should auto-detect as YAML
	defs, err := ParseEnumFileContent([]byte(yamlContent), ".txt")
	if err != nil {
		t.Fatalf("ParseEnumFileContent with auto-detect failed: %v", err)
	}

	if len(defs) != 1 {
		t.Fatalf("Expected 1 enum definition, got %d", len(defs))
	}
	if defs[0].Table != "test_table" {
		t.Errorf("defs[0].Table = %q, want 'test_table'", defs[0].Table)
	}
}

func TestParseEnumFile(t *testing.T) {
	// Create a temporary file for testing
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "enums.yaml")

	yamlContent := `enums:
  - table: users
    column: role
    values:
      "1": "ADMIN - System administrator"
      "2": "USER - Regular user"
`

	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	defs, err := ParseEnumFile(tmpFile)
	if err != nil {
		t.Fatalf("ParseEnumFile failed: %v", err)
	}

	if len(defs) != 1 {
		t.Fatalf("Expected 1 enum definition, got %d", len(defs))
	}
	if defs[0].Table != "users" {
		t.Errorf("defs[0].Table = %q, want 'users'", defs[0].Table)
	}
	if defs[0].Values["1"] != "ADMIN - System administrator" {
		t.Errorf("defs[0].Values[\"1\"] = %q, want 'ADMIN - System administrator'", defs[0].Values["1"])
	}
}

func TestParseEnumFile_NotFound(t *testing.T) {
	_, err := ParseEnumFile("/nonexistent/path/enums.yaml")
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}
}

func TestParseEnumFileContent_YMLExtension(t *testing.T) {
	yamlContent := `enums:
  - table: test
    column: status
    values:
      "1": "OK"
`

	// Test with .yml extension (alternative YAML extension)
	defs, err := ParseEnumFileContent([]byte(yamlContent), ".yml")
	if err != nil {
		t.Fatalf("ParseEnumFileContent with .yml extension failed: %v", err)
	}

	if len(defs) != 1 {
		t.Fatalf("Expected 1 enum definition, got %d", len(defs))
	}
}
