package services

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestGetGenerationStatus_ReturnsIdleByDefault(t *testing.T) {
	svc := &glossaryService{}
	projectID := uuid.New()

	status := svc.GetGenerationStatus(projectID)

	require.NotNil(t, status)
	assert.Equal(t, "idle", status.Status)
	assert.Equal(t, "No generation in progress", status.Message)
	assert.Empty(t, status.Error)
	assert.Nil(t, status.StartedAt)
}

func TestGetGenerationStatus_ReturnStoredStatus(t *testing.T) {
	svc := &glossaryService{}
	projectID := uuid.New()

	// Store a status directly via the package-level helper
	setGenerationStatus(projectID, "discovering", "Discovering glossary terms from ontology...", "", nil)

	status := svc.GetGenerationStatus(projectID)

	require.NotNil(t, status)
	assert.Equal(t, "discovering", status.Status)
	assert.Equal(t, "Discovering glossary terms from ontology...", status.Message)
	assert.Empty(t, status.Error)

	// Cleanup
	generationStatus.Delete(projectID)
}

func TestSetGenerationStatus_FailedWithError(t *testing.T) {
	svc := &glossaryService{}
	projectID := uuid.New()

	setGenerationStatus(projectID, "failed", "Discovery failed", "connection refused", nil)

	status := svc.GetGenerationStatus(projectID)

	require.NotNil(t, status)
	assert.Equal(t, "failed", status.Status)
	assert.Equal(t, "Discovery failed", status.Message)
	assert.Equal(t, "connection refused", status.Error)

	// Cleanup
	generationStatus.Delete(projectID)
}

func TestGenerationStatus_IsolatedPerProject(t *testing.T) {
	svc := &glossaryService{}
	project1 := uuid.New()
	project2 := uuid.New()

	setGenerationStatus(project1, "discovering", "Project 1 discovering...", "", nil)
	setGenerationStatus(project2, "enriching", "Project 2 enriching...", "", nil)

	status1 := svc.GetGenerationStatus(project1)
	status2 := svc.GetGenerationStatus(project2)

	assert.Equal(t, "discovering", status1.Status)
	assert.Equal(t, "enriching", status2.Status)

	// A third project with no status should be idle
	status3 := svc.GetGenerationStatus(uuid.New())
	assert.Equal(t, "idle", status3.Status)

	// Cleanup
	generationStatus.Delete(project1)
	generationStatus.Delete(project2)
}

func TestSetGenerationStatus_OverwritesPrevious(t *testing.T) {
	svc := &glossaryService{}
	projectID := uuid.New()

	setGenerationStatus(projectID, "discovering", "Step 1...", "", nil)
	assert.Equal(t, "discovering", svc.GetGenerationStatus(projectID).Status)

	setGenerationStatus(projectID, "enriching", "Step 2...", "", nil)
	assert.Equal(t, "enriching", svc.GetGenerationStatus(projectID).Status)

	setGenerationStatus(projectID, "completed", "Done", "", nil)
	status := svc.GetGenerationStatus(projectID)
	assert.Equal(t, "completed", status.Status)
	assert.Equal(t, "Done", status.Message)
	assert.Empty(t, status.Error)

	// Cleanup
	generationStatus.Delete(projectID)
}

func TestGetGenerationStatus_ReturnsCorrectType(t *testing.T) {
	svc := &glossaryService{}
	projectID := uuid.New()

	// Verify the returned value is always *models.GlossaryGenerationStatus
	status := svc.GetGenerationStatus(projectID)
	var _ *models.GlossaryGenerationStatus = status
	require.NotNil(t, status)
}
