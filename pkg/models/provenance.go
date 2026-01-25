// Package models contains domain types for ekaya-engine.
package models

import (
	"context"

	"github.com/google/uuid"
)

// ProvenanceSource represents how an ontology object was created or modified.
type ProvenanceSource string

// Provenance source constants. These represent HOW an operation was performed.
// Note: ProvenanceManual, ProvenanceMCP, ProvenanceInference string constants
// are defined in ontology_entity.go for backward compatibility.
const (
	SourceInference ProvenanceSource = "inference" // Engine auto-detected or LLM-generated
	SourceMCP       ProvenanceSource = "mcp"       // Claude via MCP tools
	SourceManual    ProvenanceSource = "manual"    // Direct manual edit via UI
)

// String returns the string representation of a ProvenanceSource.
func (s ProvenanceSource) String() string {
	return string(s)
}

// IsValid returns true if the source is a valid provenance source.
func (s ProvenanceSource) IsValid() bool {
	switch s {
	case SourceInference, SourceMCP, SourceManual:
		return true
	default:
		return false
	}
}

// ProvenanceContext carries source and actor information through operations.
// This context is used to track WHO performed an action and HOW it was performed.
type ProvenanceContext struct {
	// Source indicates how the operation was performed (inference, mcp, manual)
	Source ProvenanceSource

	// UserID is the UUID of the user who triggered the operation.
	// Extracted from JWT claims. Required for all operations.
	UserID uuid.UUID
}

// provenanceKey is the context key for storing provenance information.
type provenanceKey struct{}

// WithProvenance returns a new context with provenance information attached.
func WithProvenance(ctx context.Context, p ProvenanceContext) context.Context {
	return context.WithValue(ctx, provenanceKey{}, p)
}

// GetProvenance retrieves provenance information from the context.
// Returns the provenance context and true if present, otherwise a zero value and false.
func GetProvenance(ctx context.Context) (ProvenanceContext, bool) {
	p, ok := ctx.Value(provenanceKey{}).(ProvenanceContext)
	return p, ok
}

// MustGetProvenance retrieves provenance information from the context.
// Panics if provenance is not present. Use only when provenance is guaranteed
// to be set (e.g., after middleware validation).
func MustGetProvenance(ctx context.Context) ProvenanceContext {
	p, ok := GetProvenance(ctx)
	if !ok {
		panic("provenance context required but not present")
	}
	return p
}

// WithManualProvenance returns a context with manual (UI) provenance set.
// Use this for HTTP handlers serving UI requests.
func WithManualProvenance(ctx context.Context, userID uuid.UUID) context.Context {
	return WithProvenance(ctx, ProvenanceContext{
		Source: SourceManual,
		UserID: userID,
	})
}

// WithMCPProvenance returns a context with MCP provenance set.
// Use this for MCP tool handlers.
func WithMCPProvenance(ctx context.Context, userID uuid.UUID) context.Context {
	return WithProvenance(ctx, ProvenanceContext{
		Source: SourceMCP,
		UserID: userID,
	})
}

// WithInferenceProvenance returns a context with inference provenance set.
// Use this for DAG task handlers and automatic LLM-based operations.
// The userID should be the user who triggered the extraction workflow.
func WithInferenceProvenance(ctx context.Context, userID uuid.UUID) context.Context {
	return WithProvenance(ctx, ProvenanceContext{
		Source: SourceInference,
		UserID: userID,
	})
}
