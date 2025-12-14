package handlers

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse writes a JSON error response and returns any encoding error.
func ErrorResponse(w http.ResponseWriter, statusCode int, errorCode, message string) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(map[string]string{
		"error":   errorCode,
		"message": message,
	})
}

// WriteJSON writes a JSON response and returns any encoding error.
func WriteJSON(w http.ResponseWriter, statusCode int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	return json.NewEncoder(w).Encode(data)
}
