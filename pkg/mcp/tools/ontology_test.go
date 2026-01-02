package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// TestOntologyToolDeps_Structure verifies the OntologyToolDeps struct has all required fields.
func TestOntologyToolDeps_Structure(t *testing.T) {
	// Create a zero-value instance to verify struct is properly defined
	deps := &OntologyToolDeps{}

	// Verify all fields exist and have correct types
	assert.Nil(t, deps.DB, "DB field should be nil by default")
	assert.Nil(t, deps.MCPConfigService, "MCPConfigService field should be nil by default")
	assert.Nil(t, deps.ProjectService, "ProjectService field should be nil by default")
	assert.Nil(t, deps.OntologyRepo, "OntologyRepo field should be nil by default")
	assert.Nil(t, deps.EntityRepo, "EntityRepo field should be nil by default")
	assert.Nil(t, deps.SchemaRepo, "SchemaRepo field should be nil by default")
	assert.Nil(t, deps.Logger, "Logger field should be nil by default")
}

// TestOntologyToolDeps_Initialization verifies the struct can be initialized with dependencies.
func TestOntologyToolDeps_Initialization(t *testing.T) {
	// Create mock dependencies (just for compilation check)
	var db *database.DB
	var mcpConfigService services.MCPConfigService
	var projectService services.ProjectService
	var ontologyRepo repositories.OntologyRepository
	var entityRepo repositories.OntologyEntityRepository
	var schemaRepo repositories.SchemaRepository
	logger := zap.NewNop()

	// Verify struct can be initialized with all dependencies
	deps := &OntologyToolDeps{
		DB:               db,
		MCPConfigService: mcpConfigService,
		ProjectService:   projectService,
		OntologyRepo:     ontologyRepo,
		EntityRepo:       entityRepo,
		SchemaRepo:       schemaRepo,
		Logger:           logger,
	}

	assert.NotNil(t, deps, "OntologyToolDeps should be initialized")
	assert.Equal(t, logger, deps.Logger, "Logger should be set correctly")
}
