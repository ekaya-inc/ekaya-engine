package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// --- Request Types ---

// UpdateMetadataRequest for updating table or column metadata.
type UpdateMetadataRequest struct {
	BusinessName *string `json:"business_name,omitempty"`
	Description  *string `json:"description,omitempty"`
}

// SaveSelectionsRequest for bulk updating selection state.
type SaveSelectionsRequest struct {
	TableSelections  map[uuid.UUID]bool        `json:"table_selections"`  // table_id -> selected
	ColumnSelections map[uuid.UUID][]uuid.UUID `json:"column_selections"` // table_id -> [column_id, ...]
}

// --- Response Types ---

// SchemaResponse wraps the complete schema for a datasource.
type SchemaResponse struct {
	Tables        []TableResponse        `json:"tables"`
	TotalTables   int                    `json:"total_tables"`
	Relationships []RelationshipResponse `json:"relationships,omitempty"`
}

// TableResponse represents a table with its columns.
type TableResponse struct {
	ID           string           `json:"id"`
	SchemaName   string           `json:"schema_name"`
	TableName    string           `json:"table_name"`
	BusinessName string           `json:"business_name,omitempty"`
	Description  string           `json:"description,omitempty"`
	RowCount     int64            `json:"row_count"`
	IsSelected   bool             `json:"is_selected"`
	Columns      []ColumnResponse `json:"columns"`
}

// ColumnResponse represents a column within a table.
// Note: business_name and description are now in engine_ontology_column_metadata, not engine_schema_columns.
type ColumnResponse struct {
	ID              string `json:"id"`
	ColumnName      string `json:"column_name"`
	DataType        string `json:"data_type"`
	IsNullable      bool   `json:"is_nullable"`
	IsPrimaryKey    bool   `json:"is_primary_key"`
	IsSelected      bool   `json:"is_selected"`
	OrdinalPosition int    `json:"ordinal_position"`
	DistinctCount   *int64 `json:"distinct_count,omitempty"`
	NullCount       *int64 `json:"null_count,omitempty"`
}

// RelationshipResponse represents a relationship between columns.
type RelationshipResponse struct {
	ID               string  `json:"id"`
	SourceTableID    string  `json:"source_table_id"`
	SourceTableName  string  `json:"source_table_name"`
	SourceColumnID   string  `json:"source_column_id"`
	SourceColumnName string  `json:"source_column_name"`
	TargetTableID    string  `json:"target_table_id"`
	TargetTableName  string  `json:"target_table_name"`
	TargetColumnID   string  `json:"target_column_id"`
	TargetColumnName string  `json:"target_column_name"`
	RelationshipType string  `json:"relationship_type"`
	Cardinality      string  `json:"cardinality"`
	Confidence       float64 `json:"confidence"`
	IsApproved       *bool   `json:"is_approved,omitempty"`
}

// RefreshSchemaResponse contains statistics from a schema refresh operation.
type RefreshSchemaResponse struct {
	TablesUpserted       int   `json:"tables_upserted"`
	TablesDeleted        int64 `json:"tables_deleted"`
	ColumnsUpserted      int   `json:"columns_upserted"`
	ColumnsDeleted       int64 `json:"columns_deleted"`
	RelationshipsCreated int   `json:"relationships_created"`
	RelationshipsDeleted int64 `json:"relationships_deleted"`
}

// SchemaPromptResponse contains the schema formatted for LLM context.
type SchemaPromptResponse struct {
	Prompt string `json:"prompt"`
}

// GetRelationshipsResponse contains relationships with enriched data and table analysis.
type GetRelationshipsResponse struct {
	Relationships []RelationshipDetailResponse `json:"relationships"`
	TotalCount    int                          `json:"total_count"`
	EmptyTables   []string                     `json:"empty_tables,omitempty"`
	OrphanTables  []string                     `json:"orphan_tables,omitempty"`
}

// RelationshipDetailResponse provides enriched relationship data with column types.
type RelationshipDetailResponse struct {
	ID               string  `json:"id"`
	SourceTableName  string  `json:"source_table_name"`
	SourceColumnName string  `json:"source_column_name"`
	SourceColumnType string  `json:"source_column_type"`
	TargetTableName  string  `json:"target_table_name"`
	TargetColumnName string  `json:"target_column_name"`
	TargetColumnType string  `json:"target_column_type"`
	RelationshipType string  `json:"relationship_type"`
	Cardinality      *string `json:"cardinality"`
	Confidence       float64 `json:"confidence"`
	InferenceMethod  *string `json:"inference_method,omitempty"`
	IsValidated      bool    `json:"is_validated"`
	IsApproved       *bool   `json:"is_approved"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// DiscoverRelationshipsResponse contains results from relationship discovery.
type DiscoverRelationshipsResponse struct {
	RelationshipsCreated       int      `json:"relationships_created"`
	TablesAnalyzed             int      `json:"tables_analyzed"`
	ColumnsAnalyzed            int      `json:"columns_analyzed"`
	TablesWithoutRelationships int      `json:"tables_without_relationships"`
	EmptyTables                int      `json:"empty_tables"`
	EmptyTableNames            []string `json:"empty_table_names,omitempty"`
	OrphanTableNames           []string `json:"orphan_table_names,omitempty"`
}

// --- Handler ---

// SchemaHandler handles schema-related HTTP requests.
type SchemaHandler struct {
	schemaService    services.SchemaService
	discoveryService services.RelationshipDiscoveryService
	logger           *zap.Logger
}

// NewSchemaHandler creates a new schema handler.
func NewSchemaHandler(schemaService services.SchemaService, logger *zap.Logger) *SchemaHandler {
	return &SchemaHandler{
		schemaService: schemaService,
		logger:        logger,
	}
}

// NewSchemaHandlerWithDiscovery creates a schema handler with discovery support.
func NewSchemaHandlerWithDiscovery(schemaService services.SchemaService, discoveryService services.RelationshipDiscoveryService, logger *zap.Logger) *SchemaHandler {
	return &SchemaHandler{
		schemaService:    schemaService,
		discoveryService: discoveryService,
		logger:           logger,
	}
}

// RegisterRoutes registers the schema handler's routes on the given mux.
func (h *SchemaHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	// Schema operations
	mux.HandleFunc("GET /api/projects/{pid}/datasources/{dsid}/schema",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetSchema)))
	mux.HandleFunc("GET /api/projects/{pid}/datasources/{dsid}/schema/selected",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetSelectedSchema)))
	mux.HandleFunc("GET /api/projects/{pid}/datasources/{dsid}/schema/prompt",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetSchemaPrompt)))
	mux.HandleFunc("POST /api/projects/{pid}/datasources/{dsid}/schema/refresh",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.RefreshSchema)))

	// Table operations
	mux.HandleFunc("GET /api/projects/{pid}/datasources/{dsid}/schema/tables/{tableName}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetTable)))
	mux.HandleFunc("PUT /api/projects/{pid}/datasources/{dsid}/schema/tables/{tableId}/metadata",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.UpdateTableMetadata)))

	// Column metadata is now managed through MCP tools (update_column) and stored in engine_ontology_column_metadata.
	// The PUT /schema/columns/{columnId}/metadata endpoint has been removed.

	// Selection operations
	mux.HandleFunc("POST /api/projects/{pid}/datasources/{dsid}/schema/selections",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.SaveSelections)))

	// Relationship operations
	mux.HandleFunc("GET /api/projects/{pid}/datasources/{dsid}/schema/relationships",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetRelationships)))
	mux.HandleFunc("POST /api/projects/{pid}/datasources/{dsid}/schema/relationships",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.AddRelationship)))
	mux.HandleFunc("DELETE /api/projects/{pid}/datasources/{dsid}/schema/relationships/{relId}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.RemoveRelationship)))

	// Relationship discovery operations
	mux.HandleFunc("POST /api/projects/{pid}/datasources/{dsid}/schema/relationships/discover",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.DiscoverRelationships)))
}

// GetSchema handles GET /api/projects/{pid}/datasources/{dsid}/schema
// Returns the complete schema for a datasource.
func (h *SchemaHandler) GetSchema(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	schema, err := h.schemaService.GetDatasourceSchema(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to get schema",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_schema_failed", "Failed to get schema"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := h.toSchemaResponse(schema)
	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetSelectedSchema handles GET /api/projects/{pid}/datasources/{dsid}/schema/selected
// Returns only the selected tables and columns.
func (h *SchemaHandler) GetSelectedSchema(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	schema, err := h.schemaService.GetSelectedDatasourceSchema(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to get selected schema",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_selected_schema_failed", "Failed to get selected schema"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := h.toSchemaResponse(schema)
	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetSchemaPrompt handles GET /api/projects/{pid}/datasources/{dsid}/schema/prompt
// Returns the schema formatted for LLM context.
func (h *SchemaHandler) GetSchemaPrompt(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	// Default to selected_only=true
	selectedOnly := true
	if r.URL.Query().Get("selected_only") == "false" {
		selectedOnly = false
	}

	prompt, err := h.schemaService.GetDatasourceSchemaForPrompt(r.Context(), projectID, datasourceID, selectedOnly)
	if err != nil {
		h.logger.Error("Failed to get schema prompt",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_schema_prompt_failed", "Failed to get schema prompt"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := SchemaPromptResponse{Prompt: prompt}
	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// RefreshSchema handles POST /api/projects/{pid}/datasources/{dsid}/schema/refresh
// Syncs tables, columns, and relationships from the datasource.
func (h *SchemaHandler) RefreshSchema(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	// UI-initiated refresh does not auto-select new tables (users can manually select what they need)
	result, err := h.schemaService.RefreshDatasourceSchema(r.Context(), projectID, datasourceID, false)
	if err != nil {
		h.logger.Error("Failed to refresh schema",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "refresh_schema_failed", "Failed to refresh schema"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := RefreshSchemaResponse{
		TablesUpserted:       result.TablesUpserted,
		TablesDeleted:        result.TablesDeleted,
		ColumnsUpserted:      result.ColumnsUpserted,
		ColumnsDeleted:       result.ColumnsDeleted,
		RelationshipsCreated: result.RelationshipsCreated,
		RelationshipsDeleted: result.RelationshipsDeleted,
	}
	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetTable handles GET /api/projects/{pid}/datasources/{dsid}/schema/tables/{tableName}
// Returns a single table with its columns.
func (h *SchemaHandler) GetTable(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	// URL-decode the table name (may contain schema.table format)
	tableName, err := url.PathUnescape(r.PathValue("tableName"))
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_table_name", "Invalid table name format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	table, err := h.schemaService.GetDatasourceTable(r.Context(), projectID, datasourceID, tableName)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "table_not_found", "Table not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to get table",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.String("table_name", tableName),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_table_failed", "Failed to get table"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := h.toTableResponse(table)
	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// UpdateTableMetadata handles PUT /api/projects/{pid}/datasources/{dsid}/schema/tables/{tableId}/metadata
// Updates the business_name and/or description for a table.
func (h *SchemaHandler) UpdateTableMetadata(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	tableID, err := uuid.Parse(r.PathValue("tableId"))
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_table_id", "Invalid table ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req UpdateMetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.schemaService.UpdateTableMetadata(r.Context(), projectID, tableID, req.BusinessName, req.Description); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "table_not_found", "Table not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to update table metadata",
			zap.String("project_id", projectID.String()),
			zap.String("table_id", tableID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "update_metadata_failed", "Failed to update table metadata"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// SaveSelections handles POST /api/projects/{pid}/datasources/{dsid}/schema/selections
// Bulk updates is_selected flags for tables and columns.
func (h *SchemaHandler) SaveSelections(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	var req SaveSelectionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.schemaService.SaveSelections(r.Context(), projectID, datasourceID, req.TableSelections, req.ColumnSelections); err != nil {
		h.logger.Error("Failed to save selections",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "save_selections_failed", "Failed to save selections"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetRelationships handles GET /api/projects/{pid}/datasources/{dsid}/schema/relationships
// Returns all relationships for a datasource with enriched data and table analysis.
func (h *SchemaHandler) GetRelationships(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	relResponse, err := h.schemaService.GetRelationshipsResponse(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to get relationships",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_relationships_failed", "Failed to get relationships"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Convert model response to handler response
	data := GetRelationshipsResponse{
		TotalCount:   relResponse.TotalCount,
		EmptyTables:  relResponse.EmptyTables,
		OrphanTables: relResponse.OrphanTables,
	}

	data.Relationships = make([]RelationshipDetailResponse, len(relResponse.Relationships))
	for i, rel := range relResponse.Relationships {
		data.Relationships[i] = h.toRelationshipDetailResponse(rel)
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// AddRelationship handles POST /api/projects/{pid}/datasources/{dsid}/schema/relationships
// Creates a user-defined relationship between two columns.
func (h *SchemaHandler) AddRelationship(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	var req models.AddRelationshipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Validate required fields
	if req.SourceTableName == "" || req.SourceColumnName == "" || req.TargetTableName == "" || req.TargetColumnName == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_fields", "All relationship fields are required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	relationship, err := h.schemaService.AddManualRelationship(r.Context(), projectID, datasourceID, &req)
	if err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			if err := ErrorResponse(w, http.StatusConflict, "relationship_exists", "A relationship between these columns already exists"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "table_or_column_not_found", "Table or column not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to add relationship",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "add_relationship_failed", "Failed to add relationship"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := h.toSchemaRelationshipResponse(relationship)
	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusCreated, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// RemoveRelationship handles DELETE /api/projects/{pid}/datasources/{dsid}/schema/relationships/{relId}
// Marks a relationship as removed (is_approved=false).
func (h *SchemaHandler) RemoveRelationship(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	relationshipID, err := uuid.Parse(r.PathValue("relId"))
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_relationship_id", "Invalid relationship ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.schemaService.RemoveRelationship(r.Context(), projectID, relationshipID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "relationship_not_found", "Relationship not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to remove relationship",
			zap.String("project_id", projectID.String()),
			zap.String("relationship_id", relationshipID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "remove_relationship_failed", "Failed to remove relationship"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// DiscoverRelationships handles POST /api/projects/{pid}/datasources/{dsid}/schema/relationships/discover
// Runs automated relationship discovery to infer relationships from data.
func (h *SchemaHandler) DiscoverRelationships(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	// Check if discovery service is available
	if h.discoveryService == nil {
		if err := ErrorResponse(w, http.StatusServiceUnavailable, "discovery_not_available", "Relationship discovery service is not configured"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	results, err := h.discoveryService.DiscoverRelationships(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to discover relationships",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "discover_relationships_failed", "Failed to discover relationships"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := DiscoverRelationshipsResponse{
		RelationshipsCreated:       results.RelationshipsCreated,
		TablesAnalyzed:             results.TablesAnalyzed,
		ColumnsAnalyzed:            results.ColumnsAnalyzed,
		TablesWithoutRelationships: results.TablesWithoutRelationships,
		EmptyTables:                results.EmptyTables,
		EmptyTableNames:            results.EmptyTableNames,
		OrphanTableNames:           results.OrphanTableNames,
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// --- Model to Response Converters ---

// toSchemaResponse converts a DatasourceSchema model to a SchemaResponse.
func (h *SchemaHandler) toSchemaResponse(schema *models.DatasourceSchema) SchemaResponse {
	tables := make([]TableResponse, len(schema.Tables))
	for i, t := range schema.Tables {
		tables[i] = h.toTableResponse(t)
	}

	relationships := make([]RelationshipResponse, len(schema.Relationships))
	for i, r := range schema.Relationships {
		relationships[i] = h.toDatasourceRelationshipResponse(r)
	}

	return SchemaResponse{
		Tables:        tables,
		TotalTables:   len(tables),
		Relationships: relationships,
	}
}

// toTableResponse converts a DatasourceTable model to a TableResponse.
func (h *SchemaHandler) toTableResponse(table *models.DatasourceTable) TableResponse {
	columns := make([]ColumnResponse, len(table.Columns))
	for i, c := range table.Columns {
		columns[i] = h.toColumnResponse(c)
	}

	return TableResponse{
		ID:           table.ID.String(),
		SchemaName:   table.SchemaName,
		TableName:    table.TableName,
		BusinessName: table.BusinessName,
		Description:  table.Description,
		RowCount:     table.RowCount,
		IsSelected:   table.IsSelected,
		Columns:      columns,
	}
}

// toColumnResponse converts a DatasourceColumn model to a ColumnResponse.
func (h *SchemaHandler) toColumnResponse(col *models.DatasourceColumn) ColumnResponse {
	return ColumnResponse{
		ID:              col.ID.String(),
		ColumnName:      col.ColumnName,
		DataType:        col.DataType,
		IsNullable:      col.IsNullable,
		IsPrimaryKey:    col.IsPrimaryKey,
		IsSelected:      col.IsSelected,
		OrdinalPosition: col.OrdinalPosition,
		DistinctCount:   col.DistinctCount,
		NullCount:       col.NullCount,
	}
}

// toDatasourceRelationshipResponse converts a DatasourceRelationship model to a RelationshipResponse.
func (h *SchemaHandler) toDatasourceRelationshipResponse(rel *models.DatasourceRelationship) RelationshipResponse {
	return RelationshipResponse{
		ID:               rel.ID.String(),
		SourceTableID:    rel.SourceTableID.String(),
		SourceTableName:  rel.SourceTableName,
		SourceColumnID:   rel.SourceColumnID.String(),
		SourceColumnName: rel.SourceColumnName,
		TargetTableID:    rel.TargetTableID.String(),
		TargetTableName:  rel.TargetTableName,
		TargetColumnID:   rel.TargetColumnID.String(),
		TargetColumnName: rel.TargetColumnName,
		RelationshipType: rel.RelationshipType,
		Cardinality:      rel.Cardinality,
		Confidence:       rel.Confidence,
		IsApproved:       rel.IsApproved,
	}
}

// toSchemaRelationshipResponse converts a SchemaRelationship model to a RelationshipResponse.
// This is used for relationships returned from AddManualRelationship and GetRelationshipsForDatasource.
func (h *SchemaHandler) toSchemaRelationshipResponse(rel *models.SchemaRelationship) RelationshipResponse {
	return RelationshipResponse{
		ID:               rel.ID.String(),
		SourceTableID:    rel.SourceTableID.String(),
		SourceColumnID:   rel.SourceColumnID.String(),
		TargetTableID:    rel.TargetTableID.String(),
		TargetColumnID:   rel.TargetColumnID.String(),
		RelationshipType: rel.RelationshipType,
		Cardinality:      rel.Cardinality,
		Confidence:       rel.Confidence,
		IsApproved:       rel.IsApproved,
	}
}

// toRelationshipDetailResponse converts a RelationshipDetail model to a RelationshipDetailResponse.
func (h *SchemaHandler) toRelationshipDetailResponse(rel *models.RelationshipDetail) RelationshipDetailResponse {
	var cardinality *string
	if rel.Cardinality != "" {
		cardinality = &rel.Cardinality
	}

	return RelationshipDetailResponse{
		ID:               rel.ID.String(),
		SourceTableName:  rel.SourceTableName,
		SourceColumnName: rel.SourceColumnName,
		SourceColumnType: rel.SourceColumnType,
		TargetTableName:  rel.TargetTableName,
		TargetColumnName: rel.TargetColumnName,
		TargetColumnType: rel.TargetColumnType,
		RelationshipType: rel.RelationshipType,
		Cardinality:      cardinality,
		Confidence:       rel.Confidence,
		InferenceMethod:  rel.InferenceMethod,
		IsValidated:      rel.IsValidated,
		IsApproved:       rel.IsApproved,
		CreatedAt:        rel.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:        rel.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
