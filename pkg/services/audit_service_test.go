package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockAuditRepository is a mock implementation of AuditRepository for testing.
type mockAuditRepository struct {
	entries []*models.AuditLogEntry
}

func (m *mockAuditRepository) Create(ctx context.Context, entry *models.AuditLogEntry) error {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockAuditRepository) GetByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.AuditLogEntry, error) {
	var result []*models.AuditLogEntry
	for _, e := range m.entries {
		if e.ProjectID == projectID {
			result = append(result, e)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockAuditRepository) GetByEntity(ctx context.Context, entityType string, entityID uuid.UUID) ([]*models.AuditLogEntry, error) {
	var result []*models.AuditLogEntry
	for _, e := range m.entries {
		if e.EntityType == entityType && e.EntityID == entityID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockAuditRepository) GetByUser(ctx context.Context, projectID uuid.UUID, userID uuid.UUID, limit int) ([]*models.AuditLogEntry, error) {
	var result []*models.AuditLogEntry
	for _, e := range m.entries {
		if e.ProjectID == projectID && e.UserID != nil && *e.UserID == userID {
			result = append(result, e)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func TestAuditService_LogCreate(t *testing.T) {
	repo := &mockAuditRepository{}
	svc := NewAuditService(repo, zap.NewNop())

	projectID := uuid.New()
	entityID := uuid.New()
	userID := uuid.New()

	// Create context with provenance
	ctx := models.WithProvenance(context.Background(), models.ProvenanceContext{
		Source: models.SourceManual,
		UserID: userID,
	})

	// Log a create action
	err := svc.LogCreate(ctx, projectID, models.AuditEntityTypeEntity, entityID)
	require.NoError(t, err)

	// Verify the entry was created
	require.Len(t, repo.entries, 1)
	entry := repo.entries[0]
	assert.Equal(t, projectID, entry.ProjectID)
	assert.Equal(t, models.AuditEntityTypeEntity, entry.EntityType)
	assert.Equal(t, entityID, entry.EntityID)
	assert.Equal(t, models.AuditActionCreate, entry.Action)
	assert.Equal(t, models.ProvenanceManual, entry.Source)
	assert.Equal(t, userID, *entry.UserID)
}

func TestAuditService_LogUpdate(t *testing.T) {
	repo := &mockAuditRepository{}
	svc := NewAuditService(repo, zap.NewNop())

	projectID := uuid.New()
	entityID := uuid.New()
	userID := uuid.New()

	ctx := models.WithProvenance(context.Background(), models.ProvenanceContext{
		Source: models.SourceMCP,
		UserID: userID,
	})

	changes := map[string]models.FieldChange{
		"description": {Old: "old", New: "new"},
	}

	// Log an update action
	err := svc.LogUpdate(ctx, projectID, models.AuditEntityTypeRelationship, entityID, changes)
	require.NoError(t, err)

	// Verify the entry was created
	require.Len(t, repo.entries, 1)
	entry := repo.entries[0]
	assert.Equal(t, models.AuditActionUpdate, entry.Action)
	assert.Equal(t, models.ProvenanceMCP, entry.Source)
	assert.Len(t, entry.ChangedFields, 1)
	assert.Equal(t, "old", entry.ChangedFields["description"].Old)
	assert.Equal(t, "new", entry.ChangedFields["description"].New)
}

func TestAuditService_LogDelete(t *testing.T) {
	repo := &mockAuditRepository{}
	svc := NewAuditService(repo, zap.NewNop())

	projectID := uuid.New()
	entityID := uuid.New()
	userID := uuid.New()

	ctx := models.WithProvenance(context.Background(), models.ProvenanceContext{
		Source: models.SourceInference,
		UserID: userID,
	})

	// Log a delete action
	err := svc.LogDelete(ctx, projectID, models.AuditEntityTypeGlossaryTerm, entityID)
	require.NoError(t, err)

	// Verify the entry was created
	require.Len(t, repo.entries, 1)
	entry := repo.entries[0]
	assert.Equal(t, models.AuditActionDelete, entry.Action)
	assert.Equal(t, models.ProvenanceInference, entry.Source)
}

func TestAuditService_LogWithoutProvenance(t *testing.T) {
	repo := &mockAuditRepository{}
	svc := NewAuditService(repo, zap.NewNop())

	// Context without provenance
	ctx := context.Background()

	// Should not fail, just warn and skip
	err := svc.LogCreate(ctx, uuid.New(), models.AuditEntityTypeEntity, uuid.New())
	require.NoError(t, err)

	// No entry should be created
	assert.Len(t, repo.entries, 0)
}

func TestAuditService_GetByProject(t *testing.T) {
	repo := &mockAuditRepository{}
	svc := NewAuditService(repo, zap.NewNop())

	projectID := uuid.New()
	userID := uuid.New()

	ctx := models.WithProvenance(context.Background(), models.ProvenanceContext{
		Source: models.SourceManual,
		UserID: userID,
	})

	// Create several entries
	for i := 0; i < 5; i++ {
		err := svc.LogCreate(ctx, projectID, models.AuditEntityTypeEntity, uuid.New())
		require.NoError(t, err)
	}

	// Retrieve with limit
	entries, err := svc.GetByProject(ctx, projectID, 3)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
}

func TestAuditService_GetByEntity(t *testing.T) {
	repo := &mockAuditRepository{}
	svc := NewAuditService(repo, zap.NewNop())

	projectID := uuid.New()
	entityID := uuid.New()
	userID := uuid.New()

	ctx := models.WithProvenance(context.Background(), models.ProvenanceContext{
		Source: models.SourceManual,
		UserID: userID,
	})

	// Create entries for our target entity
	err := svc.LogCreate(ctx, projectID, models.AuditEntityTypeEntity, entityID)
	require.NoError(t, err)
	err = svc.LogUpdate(ctx, projectID, models.AuditEntityTypeEntity, entityID, nil)
	require.NoError(t, err)

	// Create entry for different entity
	err = svc.LogCreate(ctx, projectID, models.AuditEntityTypeEntity, uuid.New())
	require.NoError(t, err)

	// Retrieve for specific entity
	entries, err := svc.GetByEntity(ctx, models.AuditEntityTypeEntity, entityID)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}
