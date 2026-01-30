package models

import (
	"testing"
)

func TestIsValidClassificationPath(t *testing.T) {
	tests := []struct {
		name     string
		path     ClassificationPath
		expected bool
	}{
		{"valid timestamp", ClassificationPathTimestamp, true},
		{"valid boolean", ClassificationPathBoolean, true},
		{"valid enum", ClassificationPathEnum, true},
		{"valid uuid", ClassificationPathUUID, true},
		{"valid external_id", ClassificationPathExternalID, true},
		{"valid numeric", ClassificationPathNumeric, true},
		{"valid text", ClassificationPathText, true},
		{"valid json", ClassificationPathJSON, true},
		{"valid unknown", ClassificationPathUnknown, true},
		{"invalid path", ClassificationPath("invalid"), false},
		{"empty path", ClassificationPath(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidClassificationPath(tt.path)
			if result != tt.expected {
				t.Errorf("IsValidClassificationPath(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestColumnDataProfile_HasOnlyBooleanValues(t *testing.T) {
	tests := []struct {
		name          string
		sampleValues  []string
		distinctCount int64
		expected      bool
	}{
		{
			name:          "empty samples",
			sampleValues:  []string{},
			distinctCount: 0,
			expected:      false,
		},
		{
			name:          "0 and 1 values",
			sampleValues:  []string{"0", "1"},
			distinctCount: 2,
			expected:      true,
		},
		{
			name:          "only 0",
			sampleValues:  []string{"0"},
			distinctCount: 1,
			expected:      true,
		},
		{
			name:          "only 1",
			sampleValues:  []string{"1"},
			distinctCount: 1,
			expected:      true,
		},
		{
			name:          "true and false",
			sampleValues:  []string{"true", "false"},
			distinctCount: 2,
			expected:      true,
		},
		{
			name:          "TRUE and FALSE uppercase",
			sampleValues:  []string{"TRUE", "FALSE"},
			distinctCount: 2,
			expected:      true,
		},
		{
			name:          "yes and no",
			sampleValues:  []string{"yes", "no"},
			distinctCount: 2,
			expected:      true,
		},
		{
			name:          "Y and N",
			sampleValues:  []string{"Y", "N"},
			distinctCount: 2,
			expected:      true,
		},
		{
			name:          "T and F",
			sampleValues:  []string{"T", "F"},
			distinctCount: 2,
			expected:      true,
		},
		{
			name:          "more than 2 distinct values",
			sampleValues:  []string{"0", "1", "2"},
			distinctCount: 3,
			expected:      false,
		},
		{
			name:          "non-boolean text values",
			sampleValues:  []string{"pending", "approved"},
			distinctCount: 2,
			expected:      false,
		},
		{
			name:          "mixed boolean and non-boolean",
			sampleValues:  []string{"true", "maybe"},
			distinctCount: 2,
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &ColumnDataProfile{
				SampleValues:  tt.sampleValues,
				DistinctCount: tt.distinctCount,
			}
			result := profile.HasOnlyBooleanValues()
			if result != tt.expected {
				t.Errorf("HasOnlyBooleanValues() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestColumnDataProfile_MatchesPattern(t *testing.T) {
	profile := &ColumnDataProfile{
		DetectedPatterns: []DetectedPattern{
			{PatternName: PatternUUID, MatchRate: 0.98},
			{PatternName: PatternStripeID, MatchRate: 0.85},
			{PatternName: PatternISO4217, MatchRate: 0.50},
		},
	}

	tests := []struct {
		name        string
		patternName string
		expected    bool
	}{
		{"uuid pattern above threshold", PatternUUID, true},
		{"stripe_id pattern below threshold", PatternStripeID, false},
		{"iso4217 pattern well below threshold", PatternISO4217, false},
		{"non-existent pattern", "nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := profile.MatchesPattern(tt.patternName)
			if result != tt.expected {
				t.Errorf("MatchesPattern(%q) = %v, want %v", tt.patternName, result, tt.expected)
			}
		})
	}
}

func TestColumnDataProfile_MatchesPatternWithThreshold(t *testing.T) {
	profile := &ColumnDataProfile{
		DetectedPatterns: []DetectedPattern{
			{PatternName: PatternUUID, MatchRate: 0.80},
		},
	}

	tests := []struct {
		name        string
		patternName string
		threshold   float64
		expected    bool
	}{
		{"above threshold", PatternUUID, 0.75, true},
		{"at threshold", PatternUUID, 0.80, true},
		{"below threshold", PatternUUID, 0.85, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := profile.MatchesPatternWithThreshold(tt.patternName, tt.threshold)
			if result != tt.expected {
				t.Errorf("MatchesPatternWithThreshold(%q, %v) = %v, want %v",
					tt.patternName, tt.threshold, result, tt.expected)
			}
		})
	}
}

func TestFeatureExtractionProgress_Percentage(t *testing.T) {
	tests := []struct {
		name           string
		totalItems     int
		completedItems int
		expected       int
	}{
		{"empty progress", 0, 0, 0},
		{"no items completed", 10, 0, 0},
		{"half completed", 10, 5, 50},
		{"all completed", 10, 10, 100},
		{"3 of 4 completed", 4, 3, 75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			progress := &FeatureExtractionProgress{
				TotalItems:     tt.totalItems,
				CompletedItems: tt.completedItems,
			}
			result := progress.Percentage()
			if result != tt.expected {
				t.Errorf("Percentage() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFeatureExtractionProgress_Percentage_Nil(t *testing.T) {
	var progress *FeatureExtractionProgress
	result := progress.Percentage()
	if result != 0 {
		t.Errorf("Percentage() on nil receiver = %v, want 0", result)
	}
}

func TestNewFeatureExtractionProgress(t *testing.T) {
	progress := NewFeatureExtractionProgress()

	if progress.CurrentPhase != PhaseIDDataCollection {
		t.Errorf("CurrentPhase = %q, want %q", progress.CurrentPhase, PhaseIDDataCollection)
	}

	if len(progress.Phases) != 6 {
		t.Errorf("len(Phases) = %d, want 6", len(progress.Phases))
	}

	expectedPhases := []struct {
		phaseID string
		name    string
	}{
		{PhaseIDDataCollection, "Collecting column metadata"},
		{PhaseIDColumnClassification, "Classifying columns"},
		{PhaseIDEnumAnalysis, "Analyzing enum values"},
		{PhaseIDFKResolution, "Resolving FK candidates"},
		{PhaseIDCrossColumnAnalysis, "Cross-column analysis"},
		{PhaseIDStoreResults, "Saving results"},
	}

	for i, expected := range expectedPhases {
		if progress.Phases[i].PhaseID != expected.phaseID {
			t.Errorf("Phases[%d].PhaseID = %q, want %q", i, progress.Phases[i].PhaseID, expected.phaseID)
		}
		if progress.Phases[i].PhaseName != expected.name {
			t.Errorf("Phases[%d].PhaseName = %q, want %q", i, progress.Phases[i].PhaseName, expected.name)
		}
		if progress.Phases[i].Status != PhaseStatusPending {
			t.Errorf("Phases[%d].Status = %q, want %q", i, progress.Phases[i].Status, PhaseStatusPending)
		}
	}
}

func TestFeatureExtractionProgress_SetPhaseStatus(t *testing.T) {
	progress := NewFeatureExtractionProgress()

	progress.SetPhaseStatus(PhaseIDColumnClassification, PhaseStatusInProgress)

	// Find the phase and check status
	var found bool
	for _, phase := range progress.Phases {
		if phase.PhaseID == PhaseIDColumnClassification {
			found = true
			if phase.Status != PhaseStatusInProgress {
				t.Errorf("Phase status = %q, want %q", phase.Status, PhaseStatusInProgress)
			}
		}
	}
	if !found {
		t.Error("Phase not found")
	}
}

func TestFeatureExtractionProgress_SetPhaseProgress(t *testing.T) {
	progress := NewFeatureExtractionProgress()

	progress.SetPhaseProgress(PhaseIDColumnClassification, 5, 10, "processing users.id")

	// Find the phase and check progress
	var found bool
	for _, phase := range progress.Phases {
		if phase.PhaseID == PhaseIDColumnClassification {
			found = true
			if phase.CompletedItems != 5 {
				t.Errorf("CompletedItems = %d, want 5", phase.CompletedItems)
			}
			if phase.TotalItems != 10 {
				t.Errorf("TotalItems = %d, want 10", phase.TotalItems)
			}
			if phase.CurrentItem != "processing users.id" {
				t.Errorf("CurrentItem = %q, want %q", phase.CurrentItem, "processing users.id")
			}
		}
	}
	if !found {
		t.Error("Phase not found")
	}
}

func TestFeatureExtractionProgress_SetPhaseStatus_NonExistent(t *testing.T) {
	progress := NewFeatureExtractionProgress()

	// Should not panic when setting status for non-existent phase
	progress.SetPhaseStatus("nonexistent", PhaseStatusComplete)

	// Verify no phase was added or modified incorrectly
	for _, phase := range progress.Phases {
		if phase.PhaseID == "nonexistent" {
			t.Error("Non-existent phase should not have been added")
		}
	}
}

func TestClassificationPathConstants(t *testing.T) {
	// Verify all path constants are distinct and valid
	paths := map[ClassificationPath]bool{
		ClassificationPathTimestamp:  false,
		ClassificationPathBoolean:    false,
		ClassificationPathEnum:       false,
		ClassificationPathUUID:       false,
		ClassificationPathExternalID: false,
		ClassificationPathNumeric:    false,
		ClassificationPathText:       false,
		ClassificationPathJSON:       false,
		ClassificationPathUnknown:    false,
	}

	for path := range paths {
		if paths[path] {
			t.Errorf("Duplicate classification path: %q", path)
		}
		paths[path] = true
	}

	// Verify all constants are in ValidClassificationPaths
	if len(ValidClassificationPaths) != len(paths) {
		t.Errorf("ValidClassificationPaths has %d items, expected %d",
			len(ValidClassificationPaths), len(paths))
	}
}

func TestPatternConstants(t *testing.T) {
	// Ensure pattern constants are non-empty and unique
	patterns := []string{
		PatternUUID,
		PatternStripeID,
		PatternAWSSES,
		PatternTwilioSID,
		PatternISO4217,
		PatternUnixSeconds,
		PatternUnixMillis,
		PatternUnixMicros,
		PatternUnixNanos,
		PatternEmail,
		PatternURL,
		PatternGenericExtID,
	}

	seen := make(map[string]bool)
	for _, p := range patterns {
		if p == "" {
			t.Error("Found empty pattern constant")
		}
		if seen[p] {
			t.Errorf("Duplicate pattern constant: %q", p)
		}
		seen[p] = true
	}
}

func TestPurposeAndRoleConstants(t *testing.T) {
	// Verify purpose constants
	purposes := []string{
		PurposeIdentifier,
		PurposeTimestamp,
		PurposeFlag,
		PurposeMeasure,
		PurposeEnum,
		PurposeText,
		PurposeJSON,
	}

	for _, p := range purposes {
		if p == "" {
			t.Error("Found empty purpose constant")
		}
	}

	// Verify role constants
	roles := []string{
		RolePrimaryKey,
		RoleForeignKey,
		RoleAttribute,
		RoleMeasure,
	}

	for _, r := range roles {
		if r == "" {
			t.Error("Found empty role constant")
		}
	}
}

func TestTimestampFeatureConstants(t *testing.T) {
	purposes := []string{
		TimestampPurposeAuditCreated,
		TimestampPurposeAuditUpdated,
		TimestampPurposeSoftDelete,
		TimestampPurposeEventTime,
		TimestampPurposeScheduled,
		TimestampPurposeExpiration,
		TimestampPurposeCursor,
	}

	for _, p := range purposes {
		if p == "" {
			t.Error("Found empty timestamp purpose constant")
		}
	}

	scales := []string{
		TimestampScaleSeconds,
		TimestampScaleMilliseconds,
		TimestampScaleMicroseconds,
		TimestampScaleNanoseconds,
	}

	for _, s := range scales {
		if s == "" {
			t.Error("Found empty timestamp scale constant")
		}
	}
}

func TestBooleanTypeConstants(t *testing.T) {
	types := []string{
		BooleanTypeFeatureFlag,
		BooleanTypeStatusIndicator,
		BooleanTypePermission,
		BooleanTypePreference,
		BooleanTypeState,
	}

	seen := make(map[string]bool)
	for _, bt := range types {
		if bt == "" {
			t.Error("Found empty boolean type constant")
		}
		if seen[bt] {
			t.Errorf("Duplicate boolean type constant: %q", bt)
		}
		seen[bt] = true
	}
}

func TestEnumCategoryConstants(t *testing.T) {
	categories := []string{
		EnumCategoryInitial,
		EnumCategoryInProgress,
		EnumCategoryTerminal,
		EnumCategoryTerminalSuccess,
		EnumCategoryTerminalError,
	}

	seen := make(map[string]bool)
	for _, c := range categories {
		if c == "" {
			t.Error("Found empty enum category constant")
		}
		if seen[c] {
			t.Errorf("Duplicate enum category constant: %q", c)
		}
		seen[c] = true
	}
}

func TestIdentifierTypeConstants(t *testing.T) {
	types := []string{
		IdentifierTypeInternalUUID,
		IdentifierTypeExternalUUID,
		IdentifierTypePrimaryKey,
		IdentifierTypeForeignKey,
		IdentifierTypeExternalService,
	}

	seen := make(map[string]bool)
	for _, it := range types {
		if it == "" {
			t.Error("Found empty identifier type constant")
		}
		if seen[it] {
			t.Errorf("Duplicate identifier type constant: %q", it)
		}
		seen[it] = true
	}
}

func TestExternalServiceConstants(t *testing.T) {
	services := []string{
		ExternalServiceStripe,
		ExternalServiceTwilio,
		ExternalServiceAWSSES,
	}

	seen := make(map[string]bool)
	for _, s := range services {
		if s == "" {
			t.Error("Found empty external service constant")
		}
		if seen[s] {
			t.Errorf("Duplicate external service constant: %q", s)
		}
		seen[s] = true
	}
}

func TestCurrencyUnitConstants(t *testing.T) {
	units := []string{
		CurrencyUnitCents,
		CurrencyUnitDollars,
		CurrencyUnitBasisPoints,
	}

	seen := make(map[string]bool)
	for _, u := range units {
		if u == "" {
			t.Error("Found empty currency unit constant")
		}
		if seen[u] {
			t.Errorf("Duplicate currency unit constant: %q", u)
		}
		seen[u] = true
	}
}

func TestPhaseStatusConstants(t *testing.T) {
	statuses := []string{
		PhaseStatusPending,
		PhaseStatusInProgress,
		PhaseStatusComplete,
		PhaseStatusFailed,
	}

	seen := make(map[string]bool)
	for _, s := range statuses {
		if s == "" {
			t.Error("Found empty phase status constant")
		}
		if seen[s] {
			t.Errorf("Duplicate phase status constant: %q", s)
		}
		seen[s] = true
	}
}

func TestPhaseIDConstants(t *testing.T) {
	phases := []string{
		PhaseIDDataCollection,
		PhaseIDColumnClassification,
		PhaseIDEnumAnalysis,
		PhaseIDFKResolution,
		PhaseIDCrossColumnAnalysis,
		PhaseIDStoreResults,
	}

	seen := make(map[string]bool)
	for _, p := range phases {
		if p == "" {
			t.Error("Found empty phase ID constant")
		}
		if seen[p] {
			t.Errorf("Duplicate phase ID constant: %q", p)
		}
		seen[p] = true
	}
}
