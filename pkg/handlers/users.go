package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// AddUserRequest is the request body for adding a user.
type AddUserRequest struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

// RemoveUserRequest is the request body for removing a user.
type RemoveUserRequest struct {
	UserID string `json:"userId"`
}

// UpdateUserRequest is the request body for updating a user's role.
type UpdateUserRequest struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

// UsersHandler handles user-related HTTP requests.
type UsersHandler struct {
	userService services.UserService
	logger      *zap.Logger
}

// NewUsersHandler creates a new users handler.
func NewUsersHandler(userService services.UserService, logger *zap.Logger) *UsersHandler {
	return &UsersHandler{
		userService: userService,
		logger:      logger,
	}
}

// RegisterRoutes registers the users handler's routes on the given mux.
func (h *UsersHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	// POST /api/projects/{pid}/users - add user (auth + tenant context)
	mux.HandleFunc("POST /api/projects/{pid}/users",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			tenantMiddleware(h.Add)))

	// DELETE /api/projects/{pid}/users - remove user (auth + tenant context)
	mux.HandleFunc("DELETE /api/projects/{pid}/users",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			tenantMiddleware(h.Remove)))

	// PUT /api/projects/{pid}/users - update user role (auth + tenant context)
	mux.HandleFunc("PUT /api/projects/{pid}/users",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			tenantMiddleware(h.Update)))
}

// Add handles POST /api/projects/{pid}/users
// Adds a user to the project with the specified role.
func (h *UsersHandler) Add(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")

	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req AddUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.UserID == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_user_id", "User ID is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Role == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_role", "Role is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if !models.IsValidRole(req.Role) {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_role", "Invalid role. Must be one of: admin, data, user"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.userService.Add(r.Context(), projectID, userID, req.Role); err != nil {
		h.logger.Error("Failed to add user",
			zap.String("project_id", projectID.String()),
			zap.String("user_id", userID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "add_failed", "Failed to add user"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// Remove handles DELETE /api/projects/{pid}/users
// Removes a user from the project.
func (h *UsersHandler) Remove(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")

	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req RemoveUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.UserID == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_user_id", "User ID is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.userService.Remove(r.Context(), projectID, userID); err != nil {
		if errors.Is(err, apperrors.ErrLastAdmin) {
			if err := ErrorResponse(w, http.StatusBadRequest, "last_admin", "Cannot remove the last admin"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "User not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to remove user",
			zap.String("project_id", projectID.String()),
			zap.String("user_id", userID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "remove_failed", "Failed to remove user"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Update handles PUT /api/projects/{pid}/users
// Updates a user's role in the project.
func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")

	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.UserID == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_user_id", "User ID is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Role == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_role", "Role is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if !models.IsValidRole(req.Role) {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_role", "Invalid role. Must be one of: admin, data, user"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.userService.Update(r.Context(), projectID, userID, req.Role); err != nil {
		if errors.Is(err, apperrors.ErrLastAdmin) {
			if err := ErrorResponse(w, http.StatusBadRequest, "last_admin", "Cannot demote the last admin"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "User not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to update user",
			zap.String("project_id", projectID.String()),
			zap.String("user_id", userID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "update_failed", "Failed to update user"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}
