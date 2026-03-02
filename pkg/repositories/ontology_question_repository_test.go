//go:build integration

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// questionTestContext holds test dependencies for ontology question repository tests.
type questionTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	repo      OntologyQuestionRepository
	projectID uuid.UUID
}

// setupQuestionTest initializes the test context with shared testcontainer.
func setupQuestionTest(t *testing.T) *questionTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &questionTestContext{
		t:         t,
		engineDB:  engineDB,
		repo:      NewOntologyQuestionRepository(),
		projectID: uuid.MustParse("00000000-0000-0000-0000-000000000501"),
	}
	tc.ensureTestProject()
	return tc
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *questionTestContext) ensureTestProject() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Question Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}
}

// cleanup removes test questions.
func (tc *questionTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_questions WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope.
func (tc *questionTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

func TestCreate_ContentHashDeduplication(t *testing.T) {
	tc := setupQuestionTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a question
	q1 := &models.OntologyQuestion{
		ID:        uuid.New(),
		ProjectID: tc.projectID,
		Category:  models.QuestionCategoryTerminology,
		Text:      "What does 'tik' mean in tiks_count?",
		Priority:  2,
		Status:    models.QuestionStatusPending,
	}

	if err := tc.repo.Create(ctx, q1); err != nil {
		t.Fatalf("failed to create q1: %v", err)
	}

	// Verify content_hash was computed
	if q1.ContentHash == "" {
		t.Error("ContentHash should have been computed")
	}

	// Try to create a duplicate question (same category + text)
	q2 := &models.OntologyQuestion{
		ID:        uuid.New(),
		ProjectID: tc.projectID,
		Category:  models.QuestionCategoryTerminology,
		Text:      "What does 'tik' mean in tiks_count?",
		Priority:  1, // Different priority doesn't matter
		Status:    models.QuestionStatusPending,
	}

	// Create should succeed (ON CONFLICT DO NOTHING)
	if err := tc.repo.Create(ctx, q2); err != nil {
		t.Fatalf("Create duplicate should not return error: %v", err)
	}

	// But there should still be only 1 question
	questions, err := tc.repo.ListPending(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("ListPending failed: %v", err)
	}

	if len(questions) != 1 {
		t.Errorf("expected 1 question (duplicate ignored), got %d", len(questions))
	}

	// Verify the original question was kept (by ID)
	if questions[0].ID != q1.ID {
		t.Errorf("expected original question ID %s, got %s", q1.ID, questions[0].ID)
	}
}

func TestCreateBatch_ContentHashDeduplication(t *testing.T) {
	tc := setupQuestionTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create batch with duplicates
	questions := []*models.OntologyQuestion{
		{
			ID:        uuid.New(),
			ProjectID: tc.projectID,
			Category:  models.QuestionCategoryEnumeration,
			Text:      "What do status values mean?",
			Priority:  1,
			Status:    models.QuestionStatusPending,
		},
		{
			ID:        uuid.New(),
			ProjectID: tc.projectID,
			Category:  models.QuestionCategoryEnumeration,
			Text:      "What do status values mean?", // Duplicate
			Priority:  2,
			Status:    models.QuestionStatusPending,
		},
		{
			ID:        uuid.New(),
			ProjectID: tc.projectID,
			Category:  models.QuestionCategoryRelationship,
			Text:      "Is this a self-reference?", // Different question
			Priority:  2,
			Status:    models.QuestionStatusPending,
		},
	}

	if err := tc.repo.CreateBatch(ctx, questions); err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}

	// Should have 2 questions (one duplicate ignored)
	result, err := tc.repo.ListPending(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("ListPending failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 questions (1 duplicate ignored), got %d", len(result))
	}
}

func TestCreate_ContentHashIsReturned(t *testing.T) {
	tc := setupQuestionTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	q := &models.OntologyQuestion{
		ID:        uuid.New(),
		ProjectID: tc.projectID,
		Category:  models.QuestionCategoryDataQuality,
		Text:      "Column has 80% NULL - expected?",
		Priority:  3,
		Status:    models.QuestionStatusPending,
	}

	if err := tc.repo.Create(ctx, q); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve and verify content_hash is stored
	retrieved, err := tc.repo.GetByID(ctx, q.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.ContentHash == "" {
		t.Error("ContentHash should be stored in database")
	}

	// Verify the computed hash matches
	expectedHash := q.ComputeContentHash()
	if retrieved.ContentHash != expectedHash {
		t.Errorf("ContentHash mismatch: got %s, want %s", retrieved.ContentHash, expectedHash)
	}
}
