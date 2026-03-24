package models

import "time"

// OntologyImportMaxBytes is the maximum accepted ontology bundle size.
const OntologyImportMaxBytes = 5 << 20 // 5 MB

// OntologyCompletionProvenance identifies how the current ontology state was produced.
type OntologyCompletionProvenance string

const (
	OntologyCompletionProvenanceExtracted OntologyCompletionProvenance = "extracted"
	OntologyCompletionProvenanceImported  OntologyCompletionProvenance = "imported"
)

// IsValid returns true when the provenance value is supported.
func (p OntologyCompletionProvenance) IsValid() bool {
	switch p {
	case OntologyCompletionProvenanceExtracted, OntologyCompletionProvenanceImported:
		return true
	default:
		return false
	}
}

// OntologyImportResult is returned after a successful ontology bundle import.
type OntologyImportResult struct {
	ImportedAt           time.Time                    `json:"imported_at"`
	CompletionProvenance OntologyCompletionProvenance `json:"completion_provenance"`
}

// OntologyImportProblem is a structured validation failure for bundle import.
type OntologyImportProblem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// OntologyImportDatabaseTypeMismatch describes a datasource type mismatch.
type OntologyImportDatabaseTypeMismatch struct {
	BundleType string `json:"bundle_type"`
	TargetType string `json:"target_type"`
}

// OntologyImportRelationshipIssue identifies a relationship that could not be resolved.
type OntologyImportRelationshipIssue struct {
	Source  OntologyExportColumnRef `json:"source"`
	Target  OntologyExportColumnRef `json:"target"`
	Message string                  `json:"message"`
}

// OntologyImportValidationReport contains structured validation failures for UI display.
type OntologyImportValidationReport struct {
	Problems                []OntologyImportProblem             `json:"problems,omitempty"`
	DatabaseTypeMismatch    *OntologyImportDatabaseTypeMismatch `json:"database_type_mismatch,omitempty"`
	MissingTables           []OntologyExportTableRef            `json:"missing_tables,omitempty"`
	UnexpectedTables        []OntologyExportTableRef            `json:"unexpected_tables,omitempty"`
	MissingColumns          []OntologyExportColumnRef           `json:"missing_columns,omitempty"`
	UnexpectedColumns       []OntologyExportColumnRef           `json:"unexpected_columns,omitempty"`
	UnresolvedRelationships []OntologyImportRelationshipIssue   `json:"unresolved_relationships,omitempty"`
	MissingRequiredApps     []string                            `json:"missing_required_apps,omitempty"`
}

// HasProblems returns true when the validation report contains any blocking issue.
func (r OntologyImportValidationReport) HasProblems() bool {
	return len(r.Problems) > 0 ||
		r.DatabaseTypeMismatch != nil ||
		len(r.MissingTables) > 0 ||
		len(r.UnexpectedTables) > 0 ||
		len(r.MissingColumns) > 0 ||
		len(r.UnexpectedColumns) > 0 ||
		len(r.UnresolvedRelationships) > 0 ||
		len(r.MissingRequiredApps) > 0
}
