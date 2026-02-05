package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTableMetadata_GetRelationshipSummary(t *testing.T) {
	tests := []struct {
		name     string
		features TableMetadataFeatures
		wantNil  bool
	}{
		{
			name:     "nil relationship summary",
			features: TableMetadataFeatures{},
			wantNil:  true,
		},
		{
			name: "has relationship summary",
			features: TableMetadataFeatures{
				RelationshipSummary: &RelationshipSummaryFeatures{
					IncomingFKCount: 5,
					OutgoingFKCount: 2,
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &TableMetadata{Features: tt.features}
			result := m.GetRelationshipSummary()
			if tt.wantNil && result != nil {
				t.Errorf("GetRelationshipSummary() = %v, want nil", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("GetRelationshipSummary() = nil, want non-nil")
			}
			if !tt.wantNil && result.IncomingFKCount != 5 {
				t.Errorf("IncomingFKCount = %d, want %d", result.IncomingFKCount, 5)
			}
		})
	}
}

func TestTableMetadata_GetTemporalFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features TableMetadataFeatures
		wantNil  bool
	}{
		{
			name:     "nil temporal features",
			features: TableMetadataFeatures{},
			wantNil:  true,
		},
		{
			name: "has temporal features",
			features: TableMetadataFeatures{
				TemporalFeatures: &TableTemporalFeatures{
					HasSoftDelete:      true,
					HasAuditTimestamps: true,
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &TableMetadata{Features: tt.features}
			result := m.GetTemporalFeatures()
			if tt.wantNil && result != nil {
				t.Errorf("GetTemporalFeatures() = %v, want nil", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("GetTemporalFeatures() = nil, want non-nil")
			}
			if !tt.wantNil && !result.HasSoftDelete {
				t.Error("HasSoftDelete = false, want true")
			}
		})
	}
}

func TestTableMetadata_GetSizeFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features TableMetadataFeatures
		wantNil  bool
	}{
		{
			name:     "nil size features",
			features: TableMetadataFeatures{},
			wantNil:  true,
		},
		{
			name: "has size features",
			features: TableMetadataFeatures{
				SizeFeatures: &TableSizeFeatures{
					IsLargeTable:  true,
					GrowthPattern: "append_only",
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &TableMetadata{Features: tt.features}
			result := m.GetSizeFeatures()
			if tt.wantNil && result != nil {
				t.Errorf("GetSizeFeatures() = %v, want nil", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("GetSizeFeatures() = nil, want non-nil")
			}
			if !tt.wantNil && result.GrowthPattern != "append_only" {
				t.Errorf("GrowthPattern = %q, want %q", result.GrowthPattern, "append_only")
			}
		})
	}
}

func TestTableMetadata_SetFromAnalysis(t *testing.T) {
	m := &TableMetadata{}
	analyzedAt := time.Now()

	m.SetFromAnalysis(
		TableTypeTransactional,
		"Stores user order history",
		"Primary table for order data",
		false,
		0.95,
		analyzedAt,
		"claude-3-haiku",
	)

	// Verify fields are set
	if m.TableType == nil || *m.TableType != TableTypeTransactional {
		t.Errorf("TableType = %v, want %q", m.TableType, TableTypeTransactional)
	}
	if m.Description == nil || *m.Description != "Stores user order history" {
		t.Errorf("Description = %v, want %q", m.Description, "Stores user order history")
	}
	if m.UsageNotes == nil || *m.UsageNotes != "Primary table for order data" {
		t.Errorf("UsageNotes = %v, want %q", m.UsageNotes, "Primary table for order data")
	}
	if m.IsEphemeral {
		t.Error("IsEphemeral = true, want false")
	}
	if m.Confidence == nil || *m.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want %v", m.Confidence, 0.95)
	}
	if m.AnalyzedAt == nil {
		t.Error("AnalyzedAt should not be nil")
	}
	if m.LLMModelUsed == nil || *m.LLMModelUsed != "claude-3-haiku" {
		t.Errorf("LLMModelUsed = %v, want %q", m.LLMModelUsed, "claude-3-haiku")
	}
}

func TestTableMetadata_SetFromAnalysis_EmptyStringsNotSet(t *testing.T) {
	m := &TableMetadata{}

	m.SetFromAnalysis(
		"",          // Empty table type
		"",          // Empty description
		"",          // Empty usage notes
		false,       // isEphemeral
		0,           // Zero confidence
		time.Time{}, // Zero time
		"",          // Empty LLM model
	)

	if m.TableType != nil {
		t.Error("TableType should be nil for empty string")
	}
	if m.Description != nil {
		t.Error("Description should be nil for empty string")
	}
	if m.UsageNotes != nil {
		t.Error("UsageNotes should be nil for empty string")
	}
	if m.Confidence != nil {
		t.Error("Confidence should be nil for zero value")
	}
	if m.AnalyzedAt != nil {
		t.Error("AnalyzedAt should be nil for zero time")
	}
	if m.LLMModelUsed != nil {
		t.Error("LLMModelUsed should be nil for empty string")
	}
}

func TestTableMetadata_SetRelationshipSummary(t *testing.T) {
	m := &TableMetadata{}
	m.SetRelationshipSummary(3, 2)

	if m.Features.RelationshipSummary == nil {
		t.Fatal("RelationshipSummary should not be nil")
	}
	if m.Features.RelationshipSummary.IncomingFKCount != 3 {
		t.Errorf("IncomingFKCount = %d, want 3", m.Features.RelationshipSummary.IncomingFKCount)
	}
	if m.Features.RelationshipSummary.OutgoingFKCount != 2 {
		t.Errorf("OutgoingFKCount = %d, want 2", m.Features.RelationshipSummary.OutgoingFKCount)
	}
}

func TestTableMetadata_SetTemporalFeatures(t *testing.T) {
	m := &TableMetadata{}
	m.SetTemporalFeatures(true, true)

	if m.Features.TemporalFeatures == nil {
		t.Fatal("TemporalFeatures should not be nil")
	}
	if !m.Features.TemporalFeatures.HasSoftDelete {
		t.Error("HasSoftDelete = false, want true")
	}
	if !m.Features.TemporalFeatures.HasAuditTimestamps {
		t.Error("HasAuditTimestamps = false, want true")
	}
}

func TestTableMetadata_SetSizeFeatures(t *testing.T) {
	m := &TableMetadata{}
	m.SetSizeFeatures(true, "append_only")

	if m.Features.SizeFeatures == nil {
		t.Fatal("SizeFeatures should not be nil")
	}
	if !m.Features.SizeFeatures.IsLargeTable {
		t.Error("IsLargeTable = false, want true")
	}
	if m.Features.SizeFeatures.GrowthPattern != "append_only" {
		t.Errorf("GrowthPattern = %q, want %q", m.Features.SizeFeatures.GrowthPattern, "append_only")
	}
}

func TestTableMetadataFeatures_ScanValue(t *testing.T) {
	// Test JSON roundtrip through Scan and Value
	original := TableMetadataFeatures{
		RelationshipSummary: &RelationshipSummaryFeatures{
			IncomingFKCount: 5,
			OutgoingFKCount: 2,
		},
		TemporalFeatures: &TableTemporalFeatures{
			HasSoftDelete:      true,
			HasAuditTimestamps: true,
		},
		SizeFeatures: &TableSizeFeatures{
			IsLargeTable:  true,
			GrowthPattern: "mixed",
		},
	}

	// Convert to JSON via Value()
	jsonValue, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}

	// Scan back from JSON
	var scanned TableMetadataFeatures
	err = scanned.Scan(jsonValue)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	// Verify round-trip
	if scanned.RelationshipSummary == nil {
		t.Error("RelationshipSummary should not be nil after Scan")
	} else if scanned.RelationshipSummary.IncomingFKCount != 5 {
		t.Errorf("IncomingFKCount = %d, want 5", scanned.RelationshipSummary.IncomingFKCount)
	}

	if scanned.TemporalFeatures == nil {
		t.Error("TemporalFeatures should not be nil after Scan")
	} else if !scanned.TemporalFeatures.HasSoftDelete {
		t.Error("HasSoftDelete = false, want true")
	}

	if scanned.SizeFeatures == nil {
		t.Error("SizeFeatures should not be nil after Scan")
	} else if scanned.SizeFeatures.GrowthPattern != "mixed" {
		t.Errorf("GrowthPattern = %q, want %q", scanned.SizeFeatures.GrowthPattern, "mixed")
	}
}

func TestTableMetadataFeatures_Scan_Nil(t *testing.T) {
	var f TableMetadataFeatures
	err := f.Scan(nil)
	if err != nil {
		t.Fatalf("Scan(nil) error = %v", err)
	}

	// Should result in empty features
	if f.RelationshipSummary != nil {
		t.Error("RelationshipSummary should be nil")
	}
	if f.TemporalFeatures != nil {
		t.Error("TemporalFeatures should be nil")
	}
	if f.SizeFeatures != nil {
		t.Error("SizeFeatures should be nil")
	}
}

func TestTableMetadataFeatures_Scan_String(t *testing.T) {
	jsonStr := `{"relationship_summary":{"incoming_fk_count":3,"outgoing_fk_count":1}}`

	var f TableMetadataFeatures
	err := f.Scan(jsonStr)
	if err != nil {
		t.Fatalf("Scan(string) error = %v", err)
	}

	if f.RelationshipSummary == nil {
		t.Error("RelationshipSummary should not be nil")
	} else if f.RelationshipSummary.IncomingFKCount != 3 {
		t.Errorf("IncomingFKCount = %d, want 3", f.RelationshipSummary.IncomingFKCount)
	}
}

func TestTableMetadataFeatures_Scan_Bytes(t *testing.T) {
	jsonBytes := []byte(`{"temporal_features":{"has_soft_delete":true,"has_audit_timestamps":false}}`)

	var f TableMetadataFeatures
	err := f.Scan(jsonBytes)
	if err != nil {
		t.Fatalf("Scan([]byte) error = %v", err)
	}

	if f.TemporalFeatures == nil {
		t.Error("TemporalFeatures should not be nil")
	} else if !f.TemporalFeatures.HasSoftDelete {
		t.Error("HasSoftDelete = false, want true")
	}
}

func TestTableMetadataFeatures_Scan_InvalidType(t *testing.T) {
	var f TableMetadataFeatures
	err := f.Scan(12345) // Invalid type

	// Should not error, just return empty features
	if err != nil {
		t.Fatalf("Scan(int) should not error, got = %v", err)
	}

	if f.RelationshipSummary != nil {
		t.Error("RelationshipSummary should be nil for invalid type")
	}
}

func TestTableMetadataFeatures_Value_Empty(t *testing.T) {
	f := TableMetadataFeatures{}
	val, err := f.Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}

	// Should produce valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal(val.([]byte), &parsed)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
}

func TestTableTypeConstants(t *testing.T) {
	// Verify constants match expected values
	expectedTypes := map[string]string{
		"transactional": TableTypeTransactional,
		"reference":     TableTypeReference,
		"logging":       TableTypeLogging,
		"ephemeral":     TableTypeEphemeral,
		"junction":      TableTypeJunction,
	}

	for expected, actual := range expectedTypes {
		if actual != expected {
			t.Errorf("TableType constant = %q, want %q", actual, expected)
		}
	}
}
