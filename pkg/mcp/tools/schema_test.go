package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSchemaToolDeps_Structure verifies the SchemaToolDeps struct has all required fields.
func TestSchemaToolDeps_Structure(t *testing.T) {
	// Create a zero-value instance to verify struct is properly defined
	deps := &SchemaToolDeps{}

	// Verify all fields exist and have correct types
	assert.Nil(t, deps.DB, "DB field should be nil by default")
	assert.Nil(t, deps.MCPConfigService, "MCPConfigService field should be nil by default")
	assert.Nil(t, deps.ProjectService, "ProjectService field should be nil by default")
	assert.Nil(t, deps.SchemaService, "SchemaService field should be nil by default")
	assert.Nil(t, deps.Logger, "Logger field should be nil by default")
}
