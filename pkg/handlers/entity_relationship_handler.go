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

// EntityRelationshipResponse represents a relationship between two entities.
// Uses field names compatible with the UI's RelationshipDetail type.
type EntityRelationshipResponse struct {
	ID               string  `json:"id"`
	SourceEntityID   string  `json:"source_entity_id,omitempty"`
	TargetEntityID   string  `json:"target_entity_id,omitempty"`
	SourceTableName  string  `json:"source_table_name"`
	SourceColumnName string  `json:"source_column_name"`
	SourceColumnType string  `json:"source_column_type,omitempty"`
	TargetTableName  string  `json:"target_table_name"`
	TargetColumnName string  `json:"target_column_name"`
	TargetColumnType string  `json:"target_column_type,omitempty"`
	RelationshipType string  `json:"relationship_type"` // "fk" or "inferred"
	Cardinality      string  `json:"cardinality,omitempty"`
	Confidence       float64 `json:"confidence"`
	IsValidated      bool    `json:"is_validated"`
	IsApproved       *bool   `json:"is_approved,omitempty"`
	Status           string  `json:"status,omitempty"` // "confirmed" or "pending"
	Description      string  `json:"description,omitempty"`
	// Provenance fields
	IsStale   bool    `json:"is_stale"`
	CreatedBy string  `json:"created_by"`           // 'manual', 'mcp', 'inference'
	UpdatedBy *string `json:"updated_by,omitempty"` // nil if never updated
}

// EntityRelationshipListResponse for GET /relationships
type EntityRelationshipListResponse struct {
	Relationships []EntityRelationshipResponse `json:"relationships"`
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

// EntityRelationshipHandler handles entity relationship HTTP requests.
type EntityRelationshipHandler struct {
	relationshipService services.DeterministicRelationshipService
	logger              *zap.Logger
}

// NewEntityRelationshipHandler creates a new entity relationship handler.
func NewEntityRelationshipHandler(
	relationshipService services.DeterministicRelationshipService,
	logger *zap.Logger,
) *EntityRelationshipHandler {
	return &EntityRelationshipHandler{
		relationshipService: relationshipService,
		logger:              logger,
	}
}

// RegisterRoutes registers the entity relationship handler's routes on the given mux.
func (h *EntityRelationshipHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	// Discovery endpoint - per datasource (write operation, requires provenance)
	mux.HandleFunc("POST /api/projects/{pid}/datasources/{dsid}/relationships/discover",
		authMiddleware.RequireAuthWithPathValidationAndProvenance("pid")(tenantMiddleware(h.Discover)))

	// List endpoint - per project (read-only, no provenance needed)
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
func (h *EntityRelationshipHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	relationships, err := h.relationshipService.GetByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to list relationships",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_relationships_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Convert to response format compatible with UI's RelationshipDetail type
	relResponses := make([]EntityRelationshipResponse, 0, len(relationships))
	for _, rel := range relationships {
		// Map detection_method to relationship_type
		relType := "inferred"
		switch rel.DetectionMethod {
		case "foreign_key":
			relType = "fk"
		case "manual":
			relType = "manual"
		}

		// Map status to is_approved
		var isApproved *bool
		if rel.Status == "confirmed" {
			t := true
			isApproved = &t
		}

		relResponses = append(relResponses, EntityRelationshipResponse{
			ID:               rel.ID.String(),
			SourceEntityID:   rel.SourceEntityID.String(),
			TargetEntityID:   rel.TargetEntityID.String(),
			SourceTableName:  rel.SourceColumnTable,
			SourceColumnName: rel.SourceColumnName,
			SourceColumnType: rel.SourceColumnType,
			TargetTableName:  rel.TargetColumnTable,
			TargetColumnName: rel.TargetColumnName,
			TargetColumnType: rel.TargetColumnType,
			RelationshipType: relType,
			Confidence:       rel.Confidence,
			IsValidated:      rel.Status == "confirmed",
			IsApproved:       isApproved,
			Status:           rel.Status,
			Description:      deref(rel.Description),
			// Provenance fields - map Source/LastEditSource (method tracking) to API fields
			IsStale:   rel.IsStale,
			CreatedBy: rel.Source,
			UpdatedBy: rel.LastEditSource,
		})
	}

	response := EntityRelationshipListResponse{
		Relationships: relResponses,
		TotalCount:    len(relResponses),
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

// deref safely dereferences a string pointer, returning empty string if nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
