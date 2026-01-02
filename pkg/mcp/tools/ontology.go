// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// OntologyToolDeps contains dependencies for ontology tools.
type OntologyToolDeps struct {
	DB               *database.DB
	MCPConfigService services.MCPConfigService
	ProjectService   services.ProjectService
	OntologyRepo     repositories.OntologyRepository
	EntityRepo       repositories.OntologyEntityRepository
	SchemaRepo       repositories.SchemaRepository
	Logger           *zap.Logger
}
