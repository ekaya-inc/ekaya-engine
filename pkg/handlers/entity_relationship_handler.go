package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Request/Response Types
// ============================================================================

// SchemaRelationshipResponse represents a relationship between two columns.
// Uses field names compatible with the UI's RelationshipDetail type.
type SchemaRelationshipResponse struct {
	ID               string  `json:"id"`
	SourceTableName  string  `json:"source_table_name"`
	SourceColumnName string  `json:"source_column_name"`
	SourceColumnType string  `json:"source_column_type,omitempty"`
	TargetTableName  string  `json:"target_table_name"`
	TargetColumnName string  `json:"target_column_name"`
	TargetColumnType string  `json:"target_column_type,omitempty"`
	RelationshipType string  `json:"relationship_type"` // "fk", "inferred", or "manual"
	Cardinality      string  `json:"cardinality,omitempty"`
	Confidence       float64 `json:"confidence"`
	InferenceMethod  string  `json:"inference_method,omitempty"`
	IsValidated      bool    `json:"is_validated"`
	IsApproved       *bool   `json:"is_approved,omitempty"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// SchemaRelationshipListResponse for GET /relationships
type SchemaRelationshipListResponse struct {
	Relationships []SchemaRelationshipResponse `json:"relationships"`
	TotalCount    int                          `json:"total_count"`
	EmptyTables   []string                     `json:"empty_tables,omitempty"`
	OrphanTables  []string                     `json:"orphan_tables,omitempty"`
}

// DiscoverEntityRelationshipsResponse for POST /datasources/{dsid}/relationships/discover
type DiscoverEntityRelationshipsResponse struct {
	FKRelationships       int `json:"fk_relationships"`
	InferredRelationships int `json:"inferred_relationships"`
	TotalRelationships    int `json:"total_relationships"`
}

// ============================================================================
// Handler
// ============================================================================

// EntityRelationshipHandler handles relationship HTTP requests.
// List reads from engine_schema_relationships (schema layer).
// Discover writes to engine_schema_relationships via DeterministicRelationshipService.
type EntityRelationshipHandler struct {
	relationshipService services.DeterministicRelationshipService
	schemaService       services.SchemaService
	projectService      services.ProjectService
	logger              *zap.Logger
}

// NewEntityRelationshipHandler creates a new relationship handler.
func NewEntityRelationshipHandler(
	relationshipService services.DeterministicRelationshipService,
	schemaService services.SchemaService,
	projectService services.ProjectService,
	logger *zap.Logger,
) *EntityRelationshipHandler {
	return &EntityRelationshipHandler{
		relationshipService: relationshipService,
		schemaService:       schemaService,
		projectService:      projectService,
		logger:              logger,
	}
}

// RegisterRoutes registers the relationship handler's routes on the given mux.
func (h *EntityRelationshipHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	// Discovery endpoint - per datasource (write operation, requires provenance)
	mux.HandleFunc("POST /api/projects/{pid}/datasources/{dsid}/relationships/discover",
		authMiddleware.RequireAuthWithPathValidationAndProvenance("pid")(tenantMiddleware(h.Discover)))

	// List endpoint - per project (read-only, no provenance needed)
	// Reads from engine_schema_relationships via SchemaService
	mux.HandleFunc("GET /api/projects/{pid}/relationships",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.List)))
}

// Discover handles POST /api/projects/{pid}/datasources/{dsid}/relationships/discover
func (h *EntityRelationshipHandler) Discover(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	datasourceID, ok := ParseDatasourceID(w, r, h.logger)
	if !ok {
		return
	}

	// Discover FK relationships first (nil callback since no DAG progress to report)
	fkResult, err := h.relationshipService.DiscoverFKRelationships(r.Context(), projectID, datasourceID, nil)
	if err != nil {
		h.logger.Error("Failed to discover FK relationships",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "discover_relationships_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Discover pk_match relationships (nil callback since no DAG progress to report)
	pkMatchResult, err := h.relationshipService.DiscoverPKMatchRelationships(r.Context(), projectID, datasourceID, nil)
	if err != nil {
		h.logger.Error("Failed to discover pk_match relationships",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "discover_relationships_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := DiscoverEntityRelationshipsResponse{
		FKRelationships:       fkResult.FKRelationships,
		InferredRelationships: pkMatchResult.InferredRelationships,
		TotalRelationships:    fkResult.FKRelationships + pkMatchResult.InferredRelationships,
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// List handles GET /api/projects/{pid}/relationships
// Returns relationships from engine_schema_relationships (schema layer).
func (h *EntityRelationshipHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	// Get default datasource for the project
	datasourceID, err := h.projectService.GetDefaultDatasourceID(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get default datasource",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		// Return empty response if no datasource configured
		response := SchemaRelationshipListResponse{
			Relationships: []SchemaRelationshipResponse{},
			TotalCount:    0,
		}
		if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	// Get relationships from schema layer
	relResponse, err := h.schemaService.GetRelationshipsResponse(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to list relationships",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_relationships_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Convert to response format compatible with UI's RelationshipDetail type
	relResponses := make([]SchemaRelationshipResponse, 0, len(relResponse.Relationships))
	for _, rel := range relResponse.Relationships {
		// Map inference_method to relationship_type
		relType := mapInferenceMethodToType(rel.InferenceMethod)

		var cardinality string
		if rel.Cardinality != "" {
			cardinality = rel.Cardinality
		}

		var inferenceMethod string
		if rel.InferenceMethod != nil {
			inferenceMethod = *rel.InferenceMethod
		}

		relResponses = append(relResponses, SchemaRelationshipResponse{
			ID:               rel.ID.String(),
			SourceTableName:  rel.SourceTableName,
			SourceColumnName: rel.SourceColumnName,
			SourceColumnType: rel.SourceColumnType,
			TargetTableName:  rel.TargetTableName,
			TargetColumnName: rel.TargetColumnName,
			TargetColumnType: rel.TargetColumnType,
			RelationshipType: relType,
			Cardinality:      cardinality,
			Confidence:       rel.Confidence,
			InferenceMethod:  inferenceMethod,
			IsValidated:      rel.IsValidated,
			IsApproved:       rel.IsApproved,
			CreatedAt:        rel.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:        rel.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	response := SchemaRelationshipListResponse{
		Relationships: relResponses,
		TotalCount:    relResponse.TotalCount,
		EmptyTables:   relResponse.EmptyTables,
		OrphanTables:  relResponse.OrphanTables,
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

// mapInferenceMethodToType maps inference_method from schema layer to relationship_type for UI.
func mapInferenceMethodToType(inferenceMethod *string) string {
	if inferenceMethod == nil {
		return "inferred"
	}
	switch *inferenceMethod {
	case "fk", "foreign_key":
		return "fk"
	case "manual":
		return "manual"
	default:
		// pk_match, column_features, etc. are all "inferred"
		return "inferred"
	}
}
