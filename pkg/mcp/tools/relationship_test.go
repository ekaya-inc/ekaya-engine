package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestRegisterRelationshipTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	RegisterRelationshipTools(mcpServer, &RelationshipToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			Logger: zap.NewNop(),
		},
	})

	payload, err := json.Marshal(mcpServer.HandleMessage(t.Context(), []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)))
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(payload, &response))

	found := make(map[string]bool)
	for _, tool := range response.Result.Tools {
		found[tool.Name] = true
	}

	for _, expected := range []string{
		"list_relationships",
		"create_relationship",
		"update_relationship",
		"delete_relationship",
	} {
		assert.True(t, found[expected], "tool %s should be registered", expected)
	}
}

func TestUpdateRelationshipTool_Structure(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	RegisterRelationshipTools(mcpServer, &RelationshipToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			Logger: zap.NewNop(),
		},
	})

	payload, err := json.Marshal(mcpServer.HandleMessage(t.Context(), []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)))
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				InputSchema struct {
					Properties map[string]any `json:"properties"`
					Required   []string       `json:"required"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(payload, &response))

	var updateTool *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		} `json:"inputSchema"`
	}
	for i := range response.Result.Tools {
		if response.Result.Tools[i].Name == "update_relationship" {
			updateTool = &response.Result.Tools[i]
			break
		}
	}

	require.NotNil(t, updateTool)
	assert.Contains(t, updateTool.Description, "cardinality")
	assert.Contains(t, updateTool.InputSchema.Required, "relationship_id")
	assert.Contains(t, updateTool.InputSchema.Properties, "cardinality")
	assert.Contains(t, updateTool.InputSchema.Properties, "is_approved")
}

func TestRelationshipToolItemFromModel_UsesEffectiveSource(t *testing.T) {
	lastEdit := models.ProvenanceMCP
	createdBy := uuid.New()
	updatedBy := uuid.New()

	item := relationshipToolItemFromModel(&models.SchemaRelationship{
		ID:               uuid.New(),
		RelationshipType: models.RelationshipTypeInferred,
		Cardinality:      models.Cardinality1To1,
		Confidence:       0.93,
		Source:           models.ProvenanceInferred,
		LastEditSource:   &lastEdit,
		CreatedBy:        &createdBy,
		UpdatedBy:        &updatedBy,
	})

	require.NotNil(t, item)
	assert.Equal(t, models.ProvenanceMCP, item.EffectiveSource)
	assert.Equal(t, createdBy.String(), *item.CreatedBy)
	assert.Equal(t, updatedBy.String(), *item.UpdatedBy)
}

type allDatasourceRelationshipSchemaService struct {
	*mockSchemaService
	lastProjectID    uuid.UUID
	lastDatasourceID uuid.UUID
}

func (m *allDatasourceRelationshipSchemaService) GetRelationshipsResponse(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
) (*models.RelationshipsResponse, error) {
	m.lastProjectID = projectID
	m.lastDatasourceID = datasourceID

	if datasourceID != uuid.Nil {
		return &models.RelationshipsResponse{}, nil
	}

	return &models.RelationshipsResponse{
		Relationships: []*models.RelationshipDetail{
			{
				ID:               uuid.MustParse("00000000-0000-0000-0000-000000000321"),
				SourceTableName:  "orders",
				SourceColumnName: "user_id",
				TargetTableName:  "users",
				TargetColumnName: "id",
				RelationshipType: models.RelationshipTypeInferred,
				Cardinality:      models.CardinalityNTo1,
				Confidence:       0.9,
				Source:           models.ProvenanceInferred,
				EffectiveSource:  models.ProvenanceInferred,
			},
		},
	}, nil
}

func TestGetRelationshipToolItemByID_SearchesAllDatasources(t *testing.T) {
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000111")
	relationshipID := uuid.MustParse("00000000-0000-0000-0000-000000000321")
	schemaService := &allDatasourceRelationshipSchemaService{mockSchemaService: &mockSchemaService{}}

	item, err := getRelationshipToolItemByID(context.Background(), &RelationshipToolDeps{
		SchemaService: schemaService,
	}, projectID, relationshipID)
	require.NoError(t, err)
	require.NotNil(t, item)

	assert.Equal(t, projectID, schemaService.lastProjectID)
	assert.Equal(t, uuid.Nil, schemaService.lastDatasourceID)
	assert.Equal(t, relationshipID.String(), item.ID)
	assert.Equal(t, "orders", item.SourceTableName)
	assert.Equal(t, "users", item.TargetTableName)
}
