package models

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestProvenanceSource_String(t *testing.T) {
	tests := []struct {
		source   ProvenanceSource
		expected string
	}{
		{SourceInference, "inference"},
		{SourceMCP, "mcp"},
		{SourceManual, "manual"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.source.String(); got != tt.expected {
				t.Errorf("ProvenanceSource.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestProvenanceSource_IsValid(t *testing.T) {
	tests := []struct {
		source   ProvenanceSource
		expected bool
	}{
		{SourceInference, true},
		{SourceMCP, true},
		{SourceManual, true},
		{ProvenanceSource("invalid"), false},
		{ProvenanceSource(""), false},
		{ProvenanceSource("system"), false},
	}

	for _, tt := range tests {
		name := string(tt.source)
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			if got := tt.source.IsValid(); got != tt.expected {
				t.Errorf("ProvenanceSource.IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWithProvenance_And_GetProvenance(t *testing.T) {
	userID := uuid.New()

	tests := []struct {
		name      string
		ctx       context.Context
		wantFound bool
		wantProv  ProvenanceContext
	}{
		{
			name: "manual provenance",
			ctx: WithProvenance(context.Background(), ProvenanceContext{
				Source: SourceManual,
				UserID: userID,
			}),
			wantFound: true,
			wantProv: ProvenanceContext{
				Source: SourceManual,
				UserID: userID,
			},
		},
		{
			name: "mcp provenance",
			ctx: WithProvenance(context.Background(), ProvenanceContext{
				Source: SourceMCP,
				UserID: userID,
			}),
			wantFound: true,
			wantProv: ProvenanceContext{
				Source: SourceMCP,
				UserID: userID,
			},
		},
		{
			name: "inference provenance",
			ctx: WithProvenance(context.Background(), ProvenanceContext{
				Source: SourceInference,
				UserID: userID,
			}),
			wantFound: true,
			wantProv: ProvenanceContext{
				Source: SourceInference,
				UserID: userID,
			},
		},
		{
			name:      "no provenance",
			ctx:       context.Background(),
			wantFound: false,
			wantProv:  ProvenanceContext{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := GetProvenance(tt.ctx)
			if found != tt.wantFound {
				t.Errorf("GetProvenance() found = %v, want %v", found, tt.wantFound)
			}
			if got.Source != tt.wantProv.Source {
				t.Errorf("GetProvenance() Source = %v, want %v", got.Source, tt.wantProv.Source)
			}
			if got.UserID != tt.wantProv.UserID {
				t.Errorf("GetProvenance() UserID = %v, want %v", got.UserID, tt.wantProv.UserID)
			}
		})
	}
}

func TestMustGetProvenance(t *testing.T) {
	userID := uuid.New()

	t.Run("panics when no provenance", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustGetProvenance() did not panic when provenance is missing")
			}
		}()
		MustGetProvenance(context.Background())
	})

	t.Run("returns provenance when present", func(t *testing.T) {
		ctx := WithProvenance(context.Background(), ProvenanceContext{
			Source: SourceManual,
			UserID: userID,
		})
		got := MustGetProvenance(ctx)
		if got.Source != SourceManual {
			t.Errorf("MustGetProvenance() Source = %v, want %v", got.Source, SourceManual)
		}
		if got.UserID != userID {
			t.Errorf("MustGetProvenance() UserID = %v, want %v", got.UserID, userID)
		}
	})
}

func TestWithManualProvenance(t *testing.T) {
	userID := uuid.New()
	ctx := WithManualProvenance(context.Background(), userID)

	got, found := GetProvenance(ctx)
	if !found {
		t.Fatal("WithManualProvenance() did not set provenance")
	}
	if got.Source != SourceManual {
		t.Errorf("WithManualProvenance() Source = %v, want %v", got.Source, SourceManual)
	}
	if got.UserID != userID {
		t.Errorf("WithManualProvenance() UserID = %v, want %v", got.UserID, userID)
	}
}

func TestWithMCPProvenance(t *testing.T) {
	userID := uuid.New()
	ctx := WithMCPProvenance(context.Background(), userID)

	got, found := GetProvenance(ctx)
	if !found {
		t.Fatal("WithMCPProvenance() did not set provenance")
	}
	if got.Source != SourceMCP {
		t.Errorf("WithMCPProvenance() Source = %v, want %v", got.Source, SourceMCP)
	}
	if got.UserID != userID {
		t.Errorf("WithMCPProvenance() UserID = %v, want %v", got.UserID, userID)
	}
}

func TestWithInferenceProvenance(t *testing.T) {
	userID := uuid.New()
	ctx := WithInferenceProvenance(context.Background(), userID)

	got, found := GetProvenance(ctx)
	if !found {
		t.Fatal("WithInferenceProvenance() did not set provenance")
	}
	if got.Source != SourceInference {
		t.Errorf("WithInferenceProvenance() Source = %v, want %v", got.Source, SourceInference)
	}
	if got.UserID != userID {
		t.Errorf("WithInferenceProvenance() UserID = %v, want %v", got.UserID, userID)
	}
}

func TestProvenanceContext_Overwrites(t *testing.T) {
	userID1 := uuid.New()
	userID2 := uuid.New()

	// Set initial provenance
	ctx := WithManualProvenance(context.Background(), userID1)

	// Overwrite with different provenance
	ctx = WithMCPProvenance(ctx, userID2)

	got, found := GetProvenance(ctx)
	if !found {
		t.Fatal("Provenance should be present after overwrite")
	}
	if got.Source != SourceMCP {
		t.Errorf("After overwrite, Source = %v, want %v", got.Source, SourceMCP)
	}
	if got.UserID != userID2 {
		t.Errorf("After overwrite, UserID = %v, want %v", got.UserID, userID2)
	}
}

func TestProvenanceConstants_MatchStringConstants(t *testing.T) {
	// Verify the ProvenanceSource constants match the existing string constants
	// defined in ontology_entity.go
	if string(SourceManual) != ProvenanceManual {
		t.Errorf("SourceManual = %q, but ProvenanceManual = %q", SourceManual, ProvenanceManual)
	}
	if string(SourceMCP) != ProvenanceMCP {
		t.Errorf("SourceMCP = %q, but ProvenanceMCP = %q", SourceMCP, ProvenanceMCP)
	}
	if string(SourceInference) != ProvenanceInference {
		t.Errorf("SourceInference = %q, but ProvenanceInference = %q", SourceInference, ProvenanceInference)
	}
}
