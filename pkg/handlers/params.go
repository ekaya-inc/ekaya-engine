package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ParseProjectID extracts and validates the project ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
// Expects path parameter: pid
func ParseProjectID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
	return parseUUID(w, r, "pid", "invalid_project_id", "Invalid project ID format", logger)
}

// ParseDatasourceID extracts and validates the datasource ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
// Expects path parameter: dsid
func ParseDatasourceID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
	return parseUUID(w, r, "dsid", "invalid_datasource_id", "Invalid datasource ID format", logger)
}

// ParseEntityID extracts and validates the entity ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
// Expects path parameter: eid
func ParseEntityID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
	return parseUUID(w, r, "eid", "invalid_entity_id", "Invalid entity ID format", logger)
}

// ParseAliasID extracts and validates the alias ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
// Expects path parameter: aid
func ParseAliasID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
	return parseUUID(w, r, "aid", "invalid_alias_id", "Invalid alias ID format", logger)
}

// ParseQuestionID extracts and validates the question ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
// Expects path parameter: qid
func ParseQuestionID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
	return parseUUID(w, r, "qid", "invalid_question_id", "Invalid question ID format", logger)
}

// ParseQueryID extracts and validates the query ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
// Expects path parameter: qid
func ParseQueryID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
	return parseUUID(w, r, "qid", "invalid_query_id", "Invalid query ID format", logger)
}

// ParseProjectAndDatasourceIDs extracts and validates both project and datasource IDs.
// Returns both UUIDs and true on success, or uuid.Nil values and false on error.
// Expects path parameters: pid, dsid
func ParseProjectAndDatasourceIDs(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := ParseProjectID(w, r, logger)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}

	datasourceID, ok := ParseDatasourceID(w, r, logger)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}

	return projectID, datasourceID, true
}

// parseUUID is the internal helper that does the actual parsing work.
func parseUUID(w http.ResponseWriter, r *http.Request, pathParam, errorCode, errorMessage string, logger *zap.Logger) (uuid.UUID, bool) {
	idStr := r.PathValue(pathParam)
	id, err := uuid.Parse(idStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, errorCode, errorMessage); err != nil {
			logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, false
	}
	return id, true
}
