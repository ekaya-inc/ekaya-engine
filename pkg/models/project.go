// Package models contains domain types for ekaya-engine.
package models

import "github.com/google/uuid"

// Project represents a project in the system.
type Project struct {
	// ID is the unique identifier for the project.
	ID uuid.UUID `json:"id"`
	// Name is the display name of the project.
	Name string `json:"name"`
}
