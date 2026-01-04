package models

// ============================================================================
// Ontology Context Response Types
// ============================================================================
// These types define the structured JSON responses for the get_ontology MCP tool
// at different depth levels: domain, entities, tables, and columns.

// OntologyDomainContext represents the top-level domain view (~200-500 tokens).
// Returns high-level business context with entity list and relationships.
type OntologyDomainContext struct {
	Domain        DomainInfo         `json:"domain"`
	Entities      []EntityBrief      `json:"entities"`
	Relationships []RelationshipEdge `json:"relationships,omitempty"`
}

// DomainInfo provides high-level business context about the database.
type DomainInfo struct {
	Description    string              `json:"description"`
	PrimaryDomains []string            `json:"primary_domains"`
	TableCount     int                 `json:"table_count"`
	ColumnCount    int                 `json:"column_count"`
	Conventions    *ProjectConventions `json:"conventions,omitempty"`
}

// EntityBrief provides a concise entity summary for domain-level view.
type EntityBrief struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	PrimaryTable    string `json:"primary_table"`
	OccurrenceCount int    `json:"occurrence_count"`
}

// OntologyEntitiesContext returns entity details with occurrences (~500-1500 tokens).
// Provides entity summaries with key columns and where they appear in the schema.
type OntologyEntitiesContext struct {
	Entities      map[string]EntityDetail      `json:"entities"`
	Relationships []OntologyEntityRelationship `json:"relationships,omitempty"`
}

// EntityDetail provides full entity information including occurrences.
type EntityDetail struct {
	PrimaryTable string             `json:"primary_table"`
	Description  string             `json:"description"`
	Synonyms     []string           `json:"synonyms,omitempty"`
	KeyColumns   []KeyColumnInfo    `json:"key_columns,omitempty"`
	Occurrences  []EntityOccurrence `json:"occurrences"`
}

// KeyColumnInfo represents a key column in entity detail view.
type KeyColumnInfo struct {
	Name     string   `json:"name"`
	Synonyms []string `json:"synonyms,omitempty"`
}

// EntityOccurrence describes where an entity appears in the schema.
type EntityOccurrence struct {
	Table  string  `json:"table"`
	Column string  `json:"column"`
	Role   *string `json:"role,omitempty"`
}

// OntologyEntityRelationship represents a relationship between entities in the ontology context.
type OntologyEntityRelationship struct {
	FromEntity  string `json:"from_entity"`
	FromTable   string `json:"from_table"`
	ToEntity    string `json:"to_entity"`
	ToTable     string `json:"to_table"`
	ViaColumn   string `json:"via_column"`
	Cardinality string `json:"cardinality"`
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
}

// ColumnOverview provides basic column information for table summary.
type ColumnOverview struct {
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	Role          string  `json:"role"`
	IsPrimaryKey  bool    `json:"is_primary_key"`
	Entity        *string `json:"entity,omitempty"`
	EntityRole    *string `json:"entity_role,omitempty"`
	HasEnumValues bool    `json:"has_enum_values"`
	FKRole        string  `json:"fk_role,omitempty"` // semantic role: host, visitor, payer, payee, etc.
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
}
