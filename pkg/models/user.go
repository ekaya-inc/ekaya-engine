package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user's membership in a project.
type User struct {
	ProjectID uuid.UUID `json:"project_id"`
	UserID    uuid.UUID `json:"user_id"`
	Role      string    `json:"role"` // 'admin', 'data', 'user'
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Role constants for user roles within a project.
const (
	RoleAdmin = "admin"
	RoleData  = "data"
	RoleUser  = "user"
)

// ValidRoles contains all valid role values.
var ValidRoles = []string{RoleAdmin, RoleData, RoleUser}

// IsValidRole checks if the given role is valid.
func IsValidRole(role string) bool {
	for _, r := range ValidRoles {
		if r == role {
			return true
		}
	}
	return false
}
