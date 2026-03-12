package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ekaya-inc/ekaya-engine/pkg/jsonutil"
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

	body, err := jsonutil.MarshalNormalized(data)
	if err != nil {
		return err
	}

	_, err = w.Write(append(body, '\n'))
	return err
}
