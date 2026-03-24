package models

import "time"

// OntologyExportFormat identifies the ontology export bundle format.
const OntologyExportFormat = "ekaya-ontology-export"

// OntologyExportVersion is the current export bundle version.
const OntologyExportVersion = 1

// OntologyExportBundle is a versioned, import-oriented ontology export artifact.
type OntologyExportBundle struct {
	Format          string                        `json:"format"`
	Version         int                           `json:"version"`
	ExportedAt      time.Time                     `json:"exported_at"`
	Project         OntologyExportProject         `json:"project"`
	RequiredApps    []string                      `json:"required_apps"`
	Datasources     []OntologyExportDatasource    `json:"datasources"`
	Ontology        OntologyExportOntology        `json:"ontology"`
	ApprovedQueries []OntologyExportApprovedQuery `json:"approved_queries"`
	Security        OntologyExportSecurity        `json:"security"`
}

// OntologyExportProject contains project-scoped template metadata.
type OntologyExportProject struct {
	Name          string         `json:"name"`
	IndustryType  string         `json:"industry_type,omitempty"`
	DomainSummary *DomainSummary `json:"domain_summary,omitempty"`
}

// OntologyExportDatasource contains a portable datasource definition and schema snapshot.
type OntologyExportDatasource struct {
	Key            string                       `json:"key"`
	Name           string                       `json:"name"`
	DatasourceType string                       `json:"datasource_type"`
	Provider       string                       `json:"provider,omitempty"`
	Config         map[string]any               `json:"config,omitempty"`
	SelectedSchema OntologyExportSelectedSchema `json:"selected_schema"`
}

// OntologyExportSelectedSchema captures the selected schema snapshot for the datasource.
type OntologyExportSelectedSchema struct {
	Tables        []OntologyExportTable        `json:"tables"`
	Relationships []OntologyExportRelationship `json:"relationships"`
}

// OntologyExportTable is a portable table definition.
type OntologyExportTable struct {
	SchemaName string                 `json:"schema_name"`
	TableName  string                 `json:"table_name"`
	RowCount   *int64                 `json:"row_count,omitempty"`
	Columns    []OntologyExportColumn `json:"columns"`
}

// OntologyExportColumn is a portable column definition.
type OntologyExportColumn struct {
	ColumnName        string   `json:"column_name"`
	DataType          string   `json:"data_type"`
	IsNullable        bool     `json:"is_nullable"`
	IsPrimaryKey      bool     `json:"is_primary_key"`
	IsUnique          bool     `json:"is_unique"`
	OrdinalPosition   int      `json:"ordinal_position"`
	DefaultValue      *string  `json:"default_value,omitempty"`
	DistinctCount     *int64   `json:"distinct_count,omitempty"`
	NullCount         *int64   `json:"null_count,omitempty"`
	MinLength         *int64   `json:"min_length,omitempty"`
	MaxLength         *int64   `json:"max_length,omitempty"`
	EnumValues        []string `json:"enum_values,omitempty"`
	RowCount          *int64   `json:"row_count,omitempty"`
	NonNullCount      *int64   `json:"non_null_count,omitempty"`
	IsJoinable        *bool    `json:"is_joinable,omitempty"`
	JoinabilityReason *string  `json:"joinability_reason,omitempty"`
}

// OntologyExportTableRef identifies a table by natural key.
type OntologyExportTableRef struct {
	SchemaName string `json:"schema_name,omitempty"`
	TableName  string `json:"table_name"`
}

// OntologyExportColumnRef identifies a column by natural key.
type OntologyExportColumnRef struct {
	Table      OntologyExportTableRef `json:"table"`
	ColumnName string                 `json:"column_name"`
}

// OntologyExportRelationship is a portable relationship definition.
type OntologyExportRelationship struct {
	Source           OntologyExportColumnRef `json:"source"`
	Target           OntologyExportColumnRef `json:"target"`
	RelationshipType string                  `json:"relationship_type"`
	Cardinality      string                  `json:"cardinality"`
	Confidence       float64                 `json:"confidence"`
	InferenceMethod  *string                 `json:"inference_method,omitempty"`
	IsValidated      bool                    `json:"is_validated"`
	Validation       *ValidationResults      `json:"validation,omitempty"`
	IsApproved       *bool                   `json:"is_approved,omitempty"`
}

// OntologyExportOntology contains semantic project state.
type OntologyExportOntology struct {
	TableMetadata    []OntologyExportTableMetadata  `json:"table_metadata"`
	ColumnMetadata   []OntologyExportColumnMetadata `json:"column_metadata"`
	Questions        []OntologyExportQuestion       `json:"questions"`
	ProjectKnowledge []OntologyExportKnowledgeFact  `json:"project_knowledge"`
	GlossaryTerms    []OntologyExportGlossaryTerm   `json:"glossary_terms"`
}

// OntologyExportTableMetadata is portable table-level semantic metadata.
type OntologyExportTableMetadata struct {
	Table                OntologyExportTableRef  `json:"table"`
	TableType            *string                 `json:"table_type,omitempty"`
	Description          *string                 `json:"description,omitempty"`
	UsageNotes           *string                 `json:"usage_notes,omitempty"`
	IsEphemeral          bool                    `json:"is_ephemeral"`
	PreferredAlternative *OntologyExportTableRef `json:"preferred_alternative,omitempty"`
	Confidence           *float64                `json:"confidence,omitempty"`
	Features             TableMetadataFeatures   `json:"features"`
	Source               string                  `json:"source,omitempty"`
	LastEditSource       *string                 `json:"last_edit_source,omitempty"`
}

// OntologyExportColumnMetadata is portable column-level semantic metadata.
type OntologyExportColumnMetadata struct {
	Column                OntologyExportColumnRef `json:"column"`
	ClassificationPath    *string                 `json:"classification_path,omitempty"`
	Purpose               *string                 `json:"purpose,omitempty"`
	SemanticType          *string                 `json:"semantic_type,omitempty"`
	Role                  *string                 `json:"role,omitempty"`
	Description           *string                 `json:"description,omitempty"`
	Confidence            *float64                `json:"confidence,omitempty"`
	Features              ColumnMetadataFeatures  `json:"features"`
	NeedsEnumAnalysis     bool                    `json:"needs_enum_analysis"`
	NeedsFKResolution     bool                    `json:"needs_fk_resolution"`
	NeedsCrossColumnCheck bool                    `json:"needs_cross_column_check"`
	NeedsClarification    bool                    `json:"needs_clarification"`
	ClarificationQuestion *string                 `json:"clarification_question,omitempty"`
	IsSensitive           *bool                   `json:"is_sensitive,omitempty"`
	Source                string                  `json:"source,omitempty"`
	LastEditSource        *string                 `json:"last_edit_source,omitempty"`
}

// OntologyExportQuestionAffects captures portable question scope.
type OntologyExportQuestionAffects struct {
	Tables  []OntologyExportTableRef  `json:"tables,omitempty"`
	Columns []OntologyExportColumnRef `json:"columns,omitempty"`
}

// OntologyExportQuestion is a portable ontology question state snapshot.
type OntologyExportQuestion struct {
	Text            string                         `json:"text"`
	Priority        int                            `json:"priority"`
	IsRequired      bool                           `json:"is_required"`
	Category        string                         `json:"category,omitempty"`
	Reasoning       string                         `json:"reasoning,omitempty"`
	Affects         *OntologyExportQuestionAffects `json:"affects,omitempty"`
	DetectedPattern string                         `json:"detected_pattern,omitempty"`
	Status          QuestionStatus                 `json:"status"`
	StatusReason    string                         `json:"status_reason,omitempty"`
	Answer          string                         `json:"answer,omitempty"`
}

// OntologyExportKnowledgeFact is a portable project knowledge fact.
type OntologyExportKnowledgeFact struct {
	FactType       string  `json:"fact_type"`
	Value          string  `json:"value"`
	Context        string  `json:"context,omitempty"`
	Source         string  `json:"source,omitempty"`
	LastEditSource *string `json:"last_edit_source,omitempty"`
}

// OntologyExportGlossaryTerm is a portable glossary term definition.
type OntologyExportGlossaryTerm struct {
	Term             string         `json:"term"`
	Definition       string         `json:"definition"`
	DefiningSQL      string         `json:"defining_sql"`
	BaseTable        string         `json:"base_table,omitempty"`
	OutputColumns    []OutputColumn `json:"output_columns"`
	Aliases          []string       `json:"aliases"`
	EnrichmentStatus string         `json:"enrichment_status,omitempty"`
	EnrichmentError  string         `json:"enrichment_error,omitempty"`
	NeedsReview      bool           `json:"needs_review,omitempty"`
	ReviewReason     string         `json:"review_reason,omitempty"`
	Source           string         `json:"source,omitempty"`
	LastEditSource   *string        `json:"last_edit_source,omitempty"`
}

// OntologyExportApprovedQuery is a portable approved query definition.
type OntologyExportApprovedQuery struct {
	Key                   string           `json:"key"`
	DatasourceKey         string           `json:"datasource_key"`
	NaturalLanguagePrompt string           `json:"natural_language_prompt"`
	AdditionalContext     *string          `json:"additional_context,omitempty"`
	SQL                   string           `json:"sql"`
	Dialect               string           `json:"dialect"`
	Enabled               bool             `json:"enabled"`
	Parameters            []QueryParameter `json:"parameters"`
	OutputColumns         []OutputColumn   `json:"output_columns"`
	Constraints           *string          `json:"constraints,omitempty"`
	Tags                  []string         `json:"tags"`
	AllowsModification    bool             `json:"allows_modification"`
}

// OntologyExportSecurity documents secret-handling guarantees for the bundle.
type OntologyExportSecurity struct {
	IncludesDatasourceCredentials bool `json:"includes_datasource_credentials"`
	IncludesAIConfig              bool `json:"includes_ai_config"`
	IncludesAgentAPIKeys          bool `json:"includes_agent_api_keys"`
}
