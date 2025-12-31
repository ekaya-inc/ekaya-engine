package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Request/Response Types
// ============================================================================

// EntityListResponse for GET /entities
type EntityListResponse struct {
	Entities []EntityDetailResponse `json:"entities"`
	Total    int                    `json:"total"`
}

// EntityDetailResponse represents an entity with full details.
type EntityDetailResponse struct {
	ID              string                     `json:"id"`
	Name            string                     `json:"name"`
	Description     string                     `json:"description"`
	PrimarySchema   string                     `json:"primary_schema"`
	PrimaryTable    string                     `json:"primary_table"`
	PrimaryColumn   string                     `json:"primary_column"`
	Occurrences     []EntityOccurrenceResponse `json:"occurrences"`
	Aliases         []EntityAliasResponse      `json:"aliases"`
	OccurrenceCount int                        `json:"occurrence_count"`
	IsDeleted       bool                       `json:"is_deleted"`
	DeletionReason  *string                    `json:"deletion_reason,omitempty"`
}

// EntityAliasResponse represents an alias for an entity.
type EntityAliasResponse struct {
	ID     string  `json:"id"`
	Alias  string  `json:"alias"`
	Source *string `json:"source,omitempty"`
}

// UpdateEntityRequest for PUT /entities/{entityId}
type UpdateEntityRequest struct {
	Description string `json:"description"`
}

// DeleteEntityRequest for DELETE /entities/{entityId}
type DeleteEntityRequest struct {
	Reason string `json:"reason,omitempty"`
}

// AddAliasRequest for POST /entities/{entityId}/aliases
type AddAliasRequest struct {
	Alias  string `json:"alias"`
	Source string `json:"source,omitempty"`
}

// ============================================================================
// Handler
// ============================================================================

// EntityHandler handles entity HTTP requests.
type EntityHandler struct {
	entityService services.EntityService
	logger        *zap.Logger
}

// NewEntityHandler creates a new entity handler.
func NewEntityHandler(
	entityService services.EntityService,
	logger *zap.Logger,
) *EntityHandler {
	return &EntityHandler{
		entityService: entityService,
		logger:        logger,
	}
}

// RegisterRoutes registers the entity handler's routes on the given mux.
func (h *EntityHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/entities"

	mux.HandleFunc("GET "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.List)))
	mux.HandleFunc("GET "+base+"/{eid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Get)))
	mux.HandleFunc("PUT "+base+"/{eid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Update)))
	mux.HandleFunc("DELETE "+base+"/{eid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Delete)))
	mux.HandleFunc("POST "+base+"/{eid}/restore",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Restore)))
	mux.HandleFunc("POST "+base+"/{eid}/aliases",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.AddAlias)))
	mux.HandleFunc("DELETE "+base+"/{eid}/aliases/{aid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.RemoveAlias)))
}

// List handles GET /api/projects/{pid}/entities
func (h *EntityHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	entities, err := h.entityService.ListByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to list entities",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_entities_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Convert to response format
	entityResponses := make([]EntityDetailResponse, 0, len(entities))
	for _, e := range entities {
		entityResponses = append(entityResponses, h.toEntityDetailResponse(e))
	}

	response := EntityListResponse{
		Entities: entityResponses,
		Total:    len(entityResponses),
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Get handles GET /api/projects/{pid}/entities/{eid}
func (h *EntityHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	entityID, ok := h.parseEntityID(w, r)
	if !ok {
		return
	}

	entity, err := h.entityService.GetByID(r.Context(), entityID)
	if err != nil {
		h.logger.Error("Failed to get entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_entity_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if entity == nil {
		if err := ErrorResponse(w, http.StatusNotFound, "entity_not_found", "Entity not found"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := h.toEntityDetailResponse(entity)
	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Update handles PUT /api/projects/{pid}/entities/{eid}
func (h *EntityHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	entityID, ok := h.parseEntityID(w, r)
	if !ok {
		return
	}

	var req UpdateEntityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.entityService.Update(r.Context(), entityID, req.Description); err != nil {
		h.logger.Error("Failed to update entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		if err.Error() == "entity not found" {
			if err := ErrorResponse(w, http.StatusNotFound, "entity_not_found", "Entity not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		if err := ErrorResponse(w, http.StatusInternalServerError, "update_entity_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Return updated entity
	entity, err := h.entityService.GetByID(r.Context(), entityID)
	if err != nil {
		h.logger.Error("Failed to get updated entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_entity_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := h.toEntityDetailResponse(entity)
	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Delete handles DELETE /api/projects/{pid}/entities/{eid}
func (h *EntityHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	entityID, ok := h.parseEntityID(w, r)
	if !ok {
		return
	}

	var req DeleteEntityRequest
	// Body is optional for delete
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.entityService.Delete(r.Context(), entityID, req.Reason); err != nil {
		h.logger.Error("Failed to delete entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "delete_entity_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: map[string]string{"status": "deleted"}}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Restore handles POST /api/projects/{pid}/entities/{eid}/restore
func (h *EntityHandler) Restore(w http.ResponseWriter, r *http.Request) {
	_, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	entityID, ok := h.parseEntityID(w, r)
	if !ok {
		return
	}

	if err := h.entityService.Restore(r.Context(), entityID); err != nil {
		h.logger.Error("Failed to restore entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "restore_entity_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Return restored entity
	entity, err := h.entityService.GetByID(r.Context(), entityID)
	if err != nil {
		h.logger.Error("Failed to get restored entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_entity_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := h.toEntityDetailResponse(entity)
	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// AddAlias handles POST /api/projects/{pid}/entities/{eid}/aliases
func (h *EntityHandler) AddAlias(w http.ResponseWriter, r *http.Request) {
	_, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	entityID, ok := h.parseEntityID(w, r)
	if !ok {
		return
	}

	var req AddAliasRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	alias, err := h.entityService.AddAlias(r.Context(), entityID, req.Alias, req.Source)
	if err != nil {
		h.logger.Error("Failed to add alias",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		if err.Error() == "entity not found" {
			if err := ErrorResponse(w, http.StatusNotFound, "entity_not_found", "Entity not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		if err.Error() == "alias cannot be empty" {
			if err := ErrorResponse(w, http.StatusBadRequest, "invalid_alias", "Alias cannot be empty"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		if err := ErrorResponse(w, http.StatusInternalServerError, "add_alias_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := EntityAliasResponse{
		ID:     alias.ID.String(),
		Alias:  alias.Alias,
		Source: alias.Source,
	}

	if err := WriteJSON(w, http.StatusCreated, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// RemoveAlias handles DELETE /api/projects/{pid}/entities/{eid}/aliases/{aid}
func (h *EntityHandler) RemoveAlias(w http.ResponseWriter, r *http.Request) {
	_, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	_, ok = h.parseEntityID(w, r)
	if !ok {
		return
	}

	aliasID, ok := h.parseAliasID(w, r)
	if !ok {
		return
	}

	if err := h.entityService.RemoveAlias(r.Context(), aliasID); err != nil {
		h.logger.Error("Failed to remove alias",
			zap.String("alias_id", aliasID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "remove_alias_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: map[string]string{"status": "deleted"}}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Methods
// ============================================================================

func (h *EntityHandler) parseProjectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	projectIDStr := r.PathValue("pid")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, false
	}
	return projectID, true
}

func (h *EntityHandler) parseEntityID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	entityIDStr := r.PathValue("eid")
	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, false
	}
	return entityID, true
}

func (h *EntityHandler) parseAliasID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	aliasIDStr := r.PathValue("aid")
	aliasID, err := uuid.Parse(aliasIDStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_alias_id", "Invalid alias ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, false
	}
	return aliasID, true
}

func (h *EntityHandler) toEntityDetailResponse(e *services.EntityWithDetails) EntityDetailResponse {
	occurrences := make([]EntityOccurrenceResponse, 0, len(e.Occurrences))
	for _, occ := range e.Occurrences {
		occurrences = append(occurrences, EntityOccurrenceResponse{
			ID:         occ.ID.String(),
			SchemaName: occ.SchemaName,
			TableName:  occ.TableName,
			ColumnName: occ.ColumnName,
			Role:       occ.Role,
			Confidence: occ.Confidence,
		})
	}

	aliases := make([]EntityAliasResponse, 0, len(e.Aliases))
	for _, alias := range e.Aliases {
		aliases = append(aliases, EntityAliasResponse{
			ID:     alias.ID.String(),
			Alias:  alias.Alias,
			Source: alias.Source,
		})
	}

	return EntityDetailResponse{
		ID:              e.Entity.ID.String(),
		Name:            e.Entity.Name,
		Description:     e.Entity.Description,
		PrimarySchema:   e.Entity.PrimarySchema,
		PrimaryTable:    e.Entity.PrimaryTable,
		PrimaryColumn:   e.Entity.PrimaryColumn,
		Occurrences:     occurrences,
		Aliases:         aliases,
		OccurrenceCount: e.OccurrenceCount,
		IsDeleted:       e.Entity.IsDeleted,
		DeletionReason:  e.Entity.DeletionReason,
	}
}
