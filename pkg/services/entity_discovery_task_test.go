package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

func TestParseEntityDiscoveryOutput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		expectCount int
	}{
		{
			name: "valid JSON",
			input: `{
				"entities": [
					{
						"name": "user",
						"description": "A person who uses the system",
						"primary_schema": "public",
						"primary_table": "users",
						"primary_column": "id",
						"occurrences": [
							{
								"schema_name": "public",
								"table_name": "orders",
								"column_name": "user_id",
								"association": null
							}
						]
					}
				]
			}`,
			expectError: false,
			expectCount: 1,
		},
		{
			name: "valid JSON with markdown code blocks",
			input: "```json\n" + `{
				"entities": [
					{
						"name": "user",
						"description": "A person who uses the system",
						"primary_schema": "public",
						"primary_table": "users",
						"primary_column": "id",
						"occurrences": []
					}
				]
			}` + "\n```",
			expectError: false,
			expectCount: 1,
		},
		{
			name: "empty entities array",
			input: `{
				"entities": []
			}`,
			expectError: true,
			expectCount: 0,
		},
		{
			name:        "invalid JSON",
			input:       `{"entities": [{"name": "user"`,
			expectError: true,
			expectCount: 0,
		},
		{
			name: "missing required fields",
			input: `{
				"entities": [
					{
						"name": "user",
						"description": "A person"
					}
				]
			}`,
			expectError: true,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &EntityDiscoveryTask{}
			output, err := task.parseEntityDiscoveryOutput(tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, output.Entities, tt.expectCount)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON with markdown code blocks",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON with generic code blocks",
			input:    "```\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON with whitespace",
			input:    "  \n  {\"key\": \"value\"}  \n  ",
			expected: `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildPrompt(t *testing.T) {
	task := &EntityDiscoveryTask{
		candidates: []ColumnFilterResult{
			{
				SchemaName:    "public",
				TableName:     "users",
				ColumnName:    "id",
				DataType:      "bigint",
				DistinctCount: 100,
				IsPrimaryKey:  true,
				IsUnique:      false,
				IsCandidate:   true,
				Reason:        "primary key",
			},
			{
				SchemaName:    "public",
				TableName:     "orders",
				ColumnName:    "user_id",
				DataType:      "bigint",
				DistinctCount: 95,
				IsPrimaryKey:  false,
				IsUnique:      false,
				IsCandidate:   true,
				Reason:        "entity reference name pattern",
			},
		},
		excluded: []ColumnFilterResult{
			{
				SchemaName:  "public",
				TableName:   "orders",
				ColumnName:  "created_at",
				DataType:    "timestamp",
				IsCandidate: false,
				Reason:      "excluded type (timestamp)",
			},
		},
		components: []ConnectedComponent{
			{
				Tables: []string{"public.users", "public.orders"},
				Size:   2,
			},
		},
		islands: []string{"public.audit_logs"},
	}

	prompt := task.buildPrompt(nil)

	// Check that prompt contains expected sections
	assert.Contains(t, prompt, "# Entity Discovery Task")
	assert.Contains(t, prompt, "## Candidate Columns (Entity References)")
	assert.Contains(t, prompt, "public.users.id")
	assert.Contains(t, prompt, "public.orders.user_id")
	assert.Contains(t, prompt, "## Existing Foreign Key Relationships")
	assert.Contains(t, prompt, "## Graph Connectivity Analysis")
	assert.Contains(t, prompt, "Component 1")
	assert.Contains(t, prompt, "Island tables")
	assert.Contains(t, prompt, "public.audit_logs")
	assert.Contains(t, prompt, "## Excluded Columns (For Context Only)")
	assert.Contains(t, prompt, "created_at")
	assert.Contains(t, prompt, "## Your Task")
	assert.Contains(t, prompt, "Output Format:")
}

func TestEntityDiscoveryOutputValidation(t *testing.T) {
	tests := []struct {
		name        string
		entity      DiscoveredEntity
		expectValid bool
	}{
		{
			name: "valid entity",
			entity: DiscoveredEntity{
				Name:          "user",
				Description:   "A person",
				PrimarySchema: "public",
				PrimaryTable:  "users",
				PrimaryColumn: "id",
				Occurrences:   []EntityOccurrence{},
			},
			expectValid: true,
		},
		{
			name: "missing name",
			entity: DiscoveredEntity{
				Description:   "A person",
				PrimarySchema: "public",
				PrimaryTable:  "users",
				PrimaryColumn: "id",
			},
			expectValid: false,
		},
		{
			name: "missing primary_table",
			entity: DiscoveredEntity{
				Name:          "user",
				Description:   "A person",
				PrimarySchema: "public",
				PrimaryColumn: "id",
			},
			expectValid: false,
		},
		{
			name: "missing primary_column",
			entity: DiscoveredEntity{
				Name:          "user",
				Description:   "A person",
				PrimarySchema: "public",
				PrimaryTable:  "users",
			},
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := EntityDiscoveryOutput{
				Entities: []DiscoveredEntity{tt.entity},
			}

			// Serialize to JSON and back to test validation
			jsonBytes, err := json.Marshal(output)
			require.NoError(t, err)

			task := &EntityDiscoveryTask{}
			_, err = task.parseEntityDiscoveryOutput(string(jsonBytes))

			if tt.expectValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// ============================================================================
// Provenance Tests
// ============================================================================

// mockEntityRepoForTask is a mock entity repository that tracks created entities
type mockEntityRepoForTask struct {
	createdEntities []*models.OntologyEntity
}

func (m *mockEntityRepoForTask) Create(ctx context.Context, entity *models.OntologyEntity) error {
	if entity.ID == uuid.Nil {
		entity.ID = uuid.New()
	}
	m.createdEntities = append(m.createdEntities, entity)
	return nil
}

// Stub implementations for the interface
func (m *mockEntityRepoForTask) GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityRepoForTask) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityRepoForTask) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityRepoForTask) GetPromotedByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityRepoForTask) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityRepoForTask) GetByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityRepoForTask) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockEntityRepoForTask) DeleteInferenceEntitiesByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockEntityRepoForTask) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}
func (m *mockEntityRepoForTask) Update(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}
func (m *mockEntityRepoForTask) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}
func (m *mockEntityRepoForTask) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}
func (m *mockEntityRepoForTask) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}
func (m *mockEntityRepoForTask) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}
func (m *mockEntityRepoForTask) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	return nil, nil
}
func (m *mockEntityRepoForTask) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	return nil
}
func (m *mockEntityRepoForTask) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}
func (m *mockEntityRepoForTask) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}
func (m *mockEntityRepoForTask) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}
func (m *mockEntityRepoForTask) CountOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockEntityRepoForTask) GetOccurrenceTablesByEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]string, error) {
	return nil, nil
}

func (m *mockEntityRepoForTask) MarkInferenceEntitiesStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForTask) ClearStaleFlag(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForTask) GetStaleEntities(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}

var _ repositories.OntologyEntityRepository = (*mockEntityRepoForTask)(nil)

func TestEntityDiscoveryTask_PersistEntities_SetsConfidence(t *testing.T) {
	// Test that LLM-based entity discovery (EntityDiscoveryTask) sets confidence=0.7

	projectID := uuid.New()
	ontologyID := uuid.New()

	entityRepo := &mockEntityRepoForTask{}

	task := &EntityDiscoveryTask{
		entityRepo: entityRepo,
		projectID:  projectID,
		ontologyID: ontologyID,
	}

	output := &EntityDiscoveryOutput{
		Entities: []DiscoveredEntity{
			{
				Name:          "user",
				Description:   "A person who uses the system",
				PrimarySchema: "public",
				PrimaryTable:  "users",
				PrimaryColumn: "id",
			},
		},
	}

	// Execute
	err := task.persistEntities(context.Background(), output)

	// Verify: no error
	require.NoError(t, err)
	require.Len(t, entityRepo.createdEntities, 1)

	// Verify: LLM-discovered entities should have confidence=0.7
	entity := entityRepo.createdEntities[0]
	assert.Equal(t, 0.7, entity.Confidence, "LLM-discovered entities (EntityDiscoveryTask) should have confidence=0.7")
	assert.Equal(t, "user", entity.Name)
	assert.Equal(t, "A person who uses the system", entity.Description)
	assert.Equal(t, projectID, entity.ProjectID)
	assert.Equal(t, ontologyID, entity.OntologyID)
}
