package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestColumnMetadata_GetTimestampFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features ColumnMetadataFeatures
		wantNil  bool
	}{
		{
			name:     "nil timestamp features",
			features: ColumnMetadataFeatures{},
			wantNil:  true,
		},
		{
			name: "has timestamp features",
			features: ColumnMetadataFeatures{
				TimestampFeatures: &TimestampFeatures{
					TimestampPurpose: TimestampPurposeSoftDelete,
					IsSoftDelete:     true,
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ColumnMetadata{Features: tt.features}
			result := m.GetTimestampFeatures()
			if tt.wantNil && result != nil {
				t.Errorf("GetTimestampFeatures() = %v, want nil", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("GetTimestampFeatures() = nil, want non-nil")
			}
			if !tt.wantNil && result.TimestampPurpose != TimestampPurposeSoftDelete {
				t.Errorf("TimestampPurpose = %q, want %q", result.TimestampPurpose, TimestampPurposeSoftDelete)
			}
		})
	}
}

func TestColumnMetadata_GetBooleanFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features ColumnMetadataFeatures
		wantNil  bool
	}{
		{
			name:     "nil boolean features",
			features: ColumnMetadataFeatures{},
			wantNil:  true,
		},
		{
			name: "has boolean features",
			features: ColumnMetadataFeatures{
				BooleanFeatures: &BooleanFeatures{
					TrueMeaning:  "active",
					FalseMeaning: "inactive",
					BooleanType:  BooleanTypeStatusIndicator,
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ColumnMetadata{Features: tt.features}
			result := m.GetBooleanFeatures()
			if tt.wantNil && result != nil {
				t.Errorf("GetBooleanFeatures() = %v, want nil", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("GetBooleanFeatures() = nil, want non-nil")
			}
			if !tt.wantNil && result.TrueMeaning != "active" {
				t.Errorf("TrueMeaning = %q, want %q", result.TrueMeaning, "active")
			}
		})
	}
}

func TestColumnMetadata_GetEnumFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features ColumnMetadataFeatures
		wantNil  bool
	}{
		{
			name:     "nil enum features",
			features: ColumnMetadataFeatures{},
			wantNil:  true,
		},
		{
			name: "has enum features",
			features: ColumnMetadataFeatures{
				EnumFeatures: &EnumFeatures{
					IsStateMachine: true,
					Values: []ColumnEnumValue{
						{Value: "pending", Label: "Pending", Category: EnumCategoryInitial},
						{Value: "completed", Label: "Completed", Category: EnumCategoryTerminalSuccess},
					},
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ColumnMetadata{Features: tt.features}
			result := m.GetEnumFeatures()
			if tt.wantNil && result != nil {
				t.Errorf("GetEnumFeatures() = %v, want nil", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("GetEnumFeatures() = nil, want non-nil")
			}
			if !tt.wantNil && !result.IsStateMachine {
				t.Error("IsStateMachine = false, want true")
			}
		})
	}
}

func TestColumnMetadata_GetIdentifierFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features ColumnMetadataFeatures
		wantNil  bool
	}{
		{
			name:     "nil identifier features",
			features: ColumnMetadataFeatures{},
			wantNil:  true,
		},
		{
			name: "has identifier features",
			features: ColumnMetadataFeatures{
				IdentifierFeatures: &IdentifierFeatures{
					IdentifierType:   IdentifierTypeForeignKey,
					FKTargetTable:    "users",
					FKTargetColumn:   "id",
					EntityReferenced: "User",
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ColumnMetadata{Features: tt.features}
			result := m.GetIdentifierFeatures()
			if tt.wantNil && result != nil {
				t.Errorf("GetIdentifierFeatures() = %v, want nil", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("GetIdentifierFeatures() = nil, want non-nil")
			}
			if !tt.wantNil && result.IdentifierType != IdentifierTypeForeignKey {
				t.Errorf("IdentifierType = %q, want %q", result.IdentifierType, IdentifierTypeForeignKey)
			}
		})
	}
}

func TestColumnMetadata_GetMonetaryFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features ColumnMetadataFeatures
		wantNil  bool
	}{
		{
			name:     "nil monetary features",
			features: ColumnMetadataFeatures{},
			wantNil:  true,
		},
		{
			name: "has monetary features",
			features: ColumnMetadataFeatures{
				MonetaryFeatures: &MonetaryFeatures{
					IsMonetary:   true,
					CurrencyUnit: CurrencyUnitCents,
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ColumnMetadata{Features: tt.features}
			result := m.GetMonetaryFeatures()
			if tt.wantNil && result != nil {
				t.Errorf("GetMonetaryFeatures() = %v, want nil", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("GetMonetaryFeatures() = nil, want non-nil")
			}
			if !tt.wantNil && result.CurrencyUnit != CurrencyUnitCents {
				t.Errorf("CurrencyUnit = %q, want %q", result.CurrencyUnit, CurrencyUnitCents)
			}
		})
	}
}

func TestColumnMetadata_SetFeatures_Nil(t *testing.T) {
	m := &ColumnMetadata{}
	m.SetFeatures(nil)

	// Should not panic and should not modify anything
	if m.ClassificationPath != nil {
		t.Error("ClassificationPath should be nil after SetFeatures(nil)")
	}
}

func TestColumnMetadata_SetFeatures_CoreFields(t *testing.T) {
	m := &ColumnMetadata{}
	columnID := uuid.New()
	analyzedAt := time.Now()

	features := &ColumnFeatures{
		ColumnID:           columnID,
		ClassificationPath: ClassificationPathEnum,
		Purpose:            PurposeEnum,
		SemanticType:       "order_status",
		Role:               RoleAttribute,
		Description:        "Order processing status",
		Confidence:         0.95,
		AnalyzedAt:         analyzedAt,
		LLMModelUsed:       "claude-3-haiku",
	}

	m.SetFeatures(features)

	// Verify core classification fields
	if m.ClassificationPath == nil || *m.ClassificationPath != "enum" {
		t.Errorf("ClassificationPath = %v, want %q", m.ClassificationPath, "enum")
	}
	if m.Purpose == nil || *m.Purpose != PurposeEnum {
		t.Errorf("Purpose = %v, want %q", m.Purpose, PurposeEnum)
	}
	if m.SemanticType == nil || *m.SemanticType != "order_status" {
		t.Errorf("SemanticType = %v, want %q", m.SemanticType, "order_status")
	}
	if m.Role == nil || *m.Role != RoleAttribute {
		t.Errorf("Role = %v, want %q", m.Role, RoleAttribute)
	}
	if m.Description == nil || *m.Description != "Order processing status" {
		t.Errorf("Description = %v, want %q", m.Description, "Order processing status")
	}
	if m.Confidence == nil || *m.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want %v", m.Confidence, 0.95)
	}

	// Verify analysis metadata
	if m.AnalyzedAt == nil {
		t.Error("AnalyzedAt should not be nil")
	}
	if m.LLMModelUsed == nil || *m.LLMModelUsed != "claude-3-haiku" {
		t.Errorf("LLMModelUsed = %v, want %q", m.LLMModelUsed, "claude-3-haiku")
	}
}

func TestColumnMetadata_SetFeatures_ProcessingFlags(t *testing.T) {
	m := &ColumnMetadata{}

	features := &ColumnFeatures{
		ClassificationPath:    ClassificationPathUUID,
		NeedsEnumAnalysis:     true,
		NeedsFKResolution:     true,
		NeedsCrossColumnCheck: true,
		NeedsClarification:    true,
		ClarificationQuestion: "Is this a customer or merchant ID?",
	}

	m.SetFeatures(features)

	if !m.NeedsEnumAnalysis {
		t.Error("NeedsEnumAnalysis = false, want true")
	}
	if !m.NeedsFKResolution {
		t.Error("NeedsFKResolution = false, want true")
	}
	if !m.NeedsCrossColumnCheck {
		t.Error("NeedsCrossColumnCheck = false, want true")
	}
	if !m.NeedsClarification {
		t.Error("NeedsClarification = false, want true")
	}
	if m.ClarificationQuestion == nil || *m.ClarificationQuestion != "Is this a customer or merchant ID?" {
		t.Errorf("ClarificationQuestion = %v, want %q", m.ClarificationQuestion, "Is this a customer or merchant ID?")
	}
}

func TestColumnMetadata_SetFeatures_TypeSpecificFeatures(t *testing.T) {
	m := &ColumnMetadata{}

	timestampFeatures := &TimestampFeatures{
		TimestampPurpose: TimestampPurposeAuditCreated,
		IsAuditField:     true,
	}
	booleanFeatures := &BooleanFeatures{
		TrueMeaning: "enabled",
		BooleanType: BooleanTypeFeatureFlag,
	}
	enumFeatures := &EnumFeatures{
		IsStateMachine: true,
		Values:         []ColumnEnumValue{{Value: "active", Label: "Active"}},
	}
	identifierFeatures := &IdentifierFeatures{
		IdentifierType: IdentifierTypePrimaryKey,
	}
	monetaryFeatures := &MonetaryFeatures{
		IsMonetary:   true,
		CurrencyUnit: CurrencyUnitDollars,
	}

	features := &ColumnFeatures{
		ClassificationPath: ClassificationPathTimestamp,
		TimestampFeatures:  timestampFeatures,
		BooleanFeatures:    booleanFeatures,
		EnumFeatures:       enumFeatures,
		IdentifierFeatures: identifierFeatures,
		MonetaryFeatures:   monetaryFeatures,
	}

	m.SetFeatures(features)

	// Verify all type-specific features are copied
	if m.Features.TimestampFeatures == nil {
		t.Error("TimestampFeatures should not be nil")
	} else if m.Features.TimestampFeatures.TimestampPurpose != TimestampPurposeAuditCreated {
		t.Errorf("TimestampPurpose = %q, want %q", m.Features.TimestampFeatures.TimestampPurpose, TimestampPurposeAuditCreated)
	}

	if m.Features.BooleanFeatures == nil {
		t.Error("BooleanFeatures should not be nil")
	} else if m.Features.BooleanFeatures.TrueMeaning != "enabled" {
		t.Errorf("TrueMeaning = %q, want %q", m.Features.BooleanFeatures.TrueMeaning, "enabled")
	}

	if m.Features.EnumFeatures == nil {
		t.Error("EnumFeatures should not be nil")
	} else if !m.Features.EnumFeatures.IsStateMachine {
		t.Error("IsStateMachine = false, want true")
	}

	if m.Features.IdentifierFeatures == nil {
		t.Error("IdentifierFeatures should not be nil")
	} else if m.Features.IdentifierFeatures.IdentifierType != IdentifierTypePrimaryKey {
		t.Errorf("IdentifierType = %q, want %q", m.Features.IdentifierFeatures.IdentifierType, IdentifierTypePrimaryKey)
	}

	if m.Features.MonetaryFeatures == nil {
		t.Error("MonetaryFeatures should not be nil")
	} else if m.Features.MonetaryFeatures.CurrencyUnit != CurrencyUnitDollars {
		t.Errorf("CurrencyUnit = %q, want %q", m.Features.MonetaryFeatures.CurrencyUnit, CurrencyUnitDollars)
	}
}

func TestColumnMetadata_SetFeatures_EmptyStringsNotSet(t *testing.T) {
	m := &ColumnMetadata{}

	features := &ColumnFeatures{
		// Empty strings should not set the pointers
		ClassificationPath: "",
		Purpose:            "",
		SemanticType:       "",
		Role:               "",
		Description:        "",
		Confidence:         0, // Zero confidence should not set
	}

	m.SetFeatures(features)

	if m.ClassificationPath != nil {
		t.Error("ClassificationPath should be nil for empty string")
	}
	if m.Purpose != nil {
		t.Error("Purpose should be nil for empty string")
	}
	if m.SemanticType != nil {
		t.Error("SemanticType should be nil for empty string")
	}
	if m.Role != nil {
		t.Error("Role should be nil for empty string")
	}
	if m.Description != nil {
		t.Error("Description should be nil for empty string")
	}
	if m.Confidence != nil {
		t.Error("Confidence should be nil for zero value")
	}
}

func TestColumnMetadata_SetFeatures_ZeroTimeNotSet(t *testing.T) {
	m := &ColumnMetadata{}

	features := &ColumnFeatures{
		ClassificationPath: ClassificationPathText,
		AnalyzedAt:         time.Time{}, // Zero time
		LLMModelUsed:       "",          // Empty string
	}

	m.SetFeatures(features)

	if m.AnalyzedAt != nil {
		t.Error("AnalyzedAt should be nil for zero time")
	}
	if m.LLMModelUsed != nil {
		t.Error("LLMModelUsed should be nil for empty string")
	}
}

func TestColumnMetadataFeatures_ScanValue(t *testing.T) {
	// Test JSON roundtrip through Scan and Value
	original := ColumnMetadataFeatures{
		TimestampFeatures: &TimestampFeatures{
			TimestampPurpose: TimestampPurposeSoftDelete,
			IsSoftDelete:     true,
		},
		EnumFeatures: &EnumFeatures{
			IsStateMachine: true,
			Values: []ColumnEnumValue{
				{Value: "active", Label: "Active", Category: EnumCategoryInitial},
			},
		},
	}

	// Convert to JSON via Value()
	jsonValue, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}

	// Scan back from JSON
	var scanned ColumnMetadataFeatures
	err = scanned.Scan(jsonValue)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	// Verify round-trip
	if scanned.TimestampFeatures == nil {
		t.Error("TimestampFeatures should not be nil after Scan")
	} else if scanned.TimestampFeatures.TimestampPurpose != TimestampPurposeSoftDelete {
		t.Errorf("TimestampPurpose = %q, want %q", scanned.TimestampFeatures.TimestampPurpose, TimestampPurposeSoftDelete)
	}

	if scanned.EnumFeatures == nil {
		t.Error("EnumFeatures should not be nil after Scan")
	} else if len(scanned.EnumFeatures.Values) != 1 {
		t.Errorf("len(EnumFeatures.Values) = %d, want 1", len(scanned.EnumFeatures.Values))
	}
}

func TestColumnMetadataFeatures_Scan_Nil(t *testing.T) {
	var f ColumnMetadataFeatures
	err := f.Scan(nil)
	if err != nil {
		t.Fatalf("Scan(nil) error = %v", err)
	}

	// Should result in empty features
	if f.TimestampFeatures != nil {
		t.Error("TimestampFeatures should be nil")
	}
	if f.BooleanFeatures != nil {
		t.Error("BooleanFeatures should be nil")
	}
}

func TestColumnMetadataFeatures_Scan_String(t *testing.T) {
	jsonStr := `{"timestamp_features":{"timestamp_purpose":"soft_delete","is_soft_delete":true}}`

	var f ColumnMetadataFeatures
	err := f.Scan(jsonStr)
	if err != nil {
		t.Fatalf("Scan(string) error = %v", err)
	}

	if f.TimestampFeatures == nil {
		t.Error("TimestampFeatures should not be nil")
	} else if f.TimestampFeatures.TimestampPurpose != TimestampPurposeSoftDelete {
		t.Errorf("TimestampPurpose = %q, want %q", f.TimestampFeatures.TimestampPurpose, TimestampPurposeSoftDelete)
	}
}

func TestColumnMetadataFeatures_Scan_Bytes(t *testing.T) {
	jsonBytes := []byte(`{"boolean_features":{"true_meaning":"enabled","false_meaning":"disabled"}}`)

	var f ColumnMetadataFeatures
	err := f.Scan(jsonBytes)
	if err != nil {
		t.Fatalf("Scan([]byte) error = %v", err)
	}

	if f.BooleanFeatures == nil {
		t.Error("BooleanFeatures should not be nil")
	} else if f.BooleanFeatures.TrueMeaning != "enabled" {
		t.Errorf("TrueMeaning = %q, want %q", f.BooleanFeatures.TrueMeaning, "enabled")
	}
}

func TestColumnMetadataFeatures_Scan_InvalidType(t *testing.T) {
	var f ColumnMetadataFeatures
	err := f.Scan(12345) // Invalid type

	// Should not error, just return empty features
	if err != nil {
		t.Fatalf("Scan(int) should not error, got = %v", err)
	}

	if f.TimestampFeatures != nil {
		t.Error("TimestampFeatures should be nil for invalid type")
	}
}

func TestColumnMetadataFeatures_Value_Empty(t *testing.T) {
	f := ColumnMetadataFeatures{}
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
