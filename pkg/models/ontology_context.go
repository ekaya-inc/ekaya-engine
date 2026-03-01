package models

// ============================================================================
// Ontology Context Response Types
// ============================================================================
// These types define the structured JSON responses for the get_ontology MCP tool
// at different depth levels: domain, tables, and columns.

// OntologyDomainContext represents the top-level domain view (~200-500 tokens).
// Returns high-level business context.
type OntologyDomainContext struct {
	Domain DomainInfo `json:"domain"`
}

// DomainInfo provides high-level business context about the database.
type DomainInfo struct {
	Description    string              `json:"description"`
	PrimaryDomains []string            `json:"primary_domains"`
	TableCount     int                 `json:"table_count"`
	ColumnCount    int                 `json:"column_count"`
	Conventions    *ProjectConventions `json:"conventions,omitempty"`
}

// OntologyTablesContext returns table-level summaries with column overview.
// Variable size depending on table filter.
type OntologyTablesContext struct {
	Tables map[string]TableSummary `json:"tables"`
}

// TableSummary provides table-level information with column overview.
type TableSummary struct {
	Schema        string              `json:"schema"`
	BusinessName  string              `json:"business_name"`
	Description   string              `json:"description"`
	Domain        string              `json:"domain"`
	RowCount      int64               `json:"row_count"`
	ColumnCount   int                 `json:"column_count"`
	Synonyms      []string            `json:"synonyms,omitempty"`
	Columns       []ColumnOverview    `json:"columns"`
	Relationships []TableRelationship `json:"relationships,omitempty"`

	// Table metadata (from engine_ontology_table_metadata)
	UsageNotes           string `json:"usage_notes,omitempty"`
	IsEphemeral          bool   `json:"is_ephemeral,omitempty"`
	PreferredAlternative string `json:"preferred_alternative,omitempty"`
}

// ColumnOverview provides basic column information for table summary.
type ColumnOverview struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Role           string `json:"role"`
	IsPrimaryKey   bool   `json:"is_primary_key"`
	HasEnumValues  bool   `json:"has_enum_values"`
	HasDescription bool   `json:"has_description"`
	FKAssociation  string `json:"fk_association,omitempty"` // e.g., host, visitor, payer, payee
}

// TableRelationship represents a relationship at table level.
type TableRelationship struct {
	Column      string `json:"column"`
	References  string `json:"references"`
	Cardinality string `json:"cardinality"`
}

// OntologyColumnsContext returns detailed column information.
// Always filtered by table list to manage response size.
type OntologyColumnsContext struct {
	Tables map[string]TableDetail `json:"tables"`
}

// TableDetail provides full table and column details.
type TableDetail struct {
	Schema       string         `json:"schema"`
	BusinessName string         `json:"business_name"`
	Description  string         `json:"description"`
	Columns      []ColumnDetail `json:"columns"`

	// Table metadata (from engine_ontology_table_metadata)
	UsageNotes           string `json:"usage_notes,omitempty"`
	IsEphemeral          bool   `json:"is_ephemeral,omitempty"`
	PreferredAlternative string `json:"preferred_alternative,omitempty"`
}
