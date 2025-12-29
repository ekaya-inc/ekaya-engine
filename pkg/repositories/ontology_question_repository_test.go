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
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	repo       OntologyQuestionRepository
	projectID  uuid.UUID
	ontologyID uuid.UUID
}

// setupQuestionTest initializes the test context with shared testcontainer.
func setupQuestionTest(t *testing.T) *questionTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &questionTestContext{
		t:          t,
		engineDB:   engineDB,
		repo:       NewOntologyQuestionRepository(),
		projectID:  uuid.MustParse("00000000-0000-0000-0000-000000000501"),
		ontologyID: uuid.MustParse("00000000-0000-0000-0000-000000000502"),
	}
	tc.ensureTestProject()
	tc.ensureTestOntology()
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

// ensureTestOntology creates the test ontology if it doesn't exist.
func (tc *questionTestContext) ensureTestOntology() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for ontology setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, version, is_active)
		VALUES ($1, $2, 1, true)
		ON CONFLICT (id) DO NOTHING
	`, tc.ontologyID, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to ensure test ontology: %v", err)
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

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_questions WHERE ontology_id = $1", tc.ontologyID)
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

func TestListByOntologyID(t *testing.T) {
	tc := setupQuestionTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test questions for this ontology
	q1 := &models.OntologyQuestion{
		ID:         uuid.New(),
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Text:       "What is 'marker_at' used for?",
		Priority:   2,
		IsRequired: false,
		Status:     models.QuestionStatusPending,
	}
	q2 := &models.OntologyQuestion{
		ID:         uuid.New(),
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Text:       "What do the status values mean?",
		Priority:   1,
		IsRequired: true,
		Status:     models.QuestionStatusPending,
	}

	// Create questions
	if err := tc.repo.Create(ctx, q1); err != nil {
		t.Fatalf("failed to create q1: %v", err)
	}
	if err := tc.repo.Create(ctx, q2); err != nil {
		t.Fatalf("failed to create q2: %v", err)
	}

	// Test ListByOntologyID
	questions, err := tc.repo.ListByOntologyID(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("ListByOntologyID failed: %v", err)
	}

	if len(questions) != 2 {
		t.Errorf("expected 2 questions, got %d", len(questions))
	}

	// Verify questions are returned (ordered by created_at ASC)
	foundQ1 := false
	foundQ2 := false
	for _, q := range questions {
		if q.Text == q1.Text {
			foundQ1 = true
		}
		if q.Text == q2.Text {
			foundQ2 = true
		}
	}
	if !foundQ1 {
		t.Error("q1 not found in results")
	}
	if !foundQ2 {
		t.Error("q2 not found in results")
	}
}

func TestListByOntologyID_EmptyResult(t *testing.T) {
	tc := setupQuestionTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Query for a non-existent ontology
	nonExistentOntologyID := uuid.New()
	questions, err := tc.repo.ListByOntologyID(ctx, nonExistentOntologyID)
	if err != nil {
		t.Fatalf("ListByOntologyID failed: %v", err)
	}

	if len(questions) != 0 {
		t.Errorf("expected 0 questions for non-existent ontology, got %d", len(questions))
	}
}

func TestListByOntologyID_OnlyReturnsMatchingOntology(t *testing.T) {
	tc := setupQuestionTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create another ontology using the same scope/context (version 2 to avoid unique constraint)
	otherOntologyID := uuid.MustParse("00000000-0000-0000-0000-000000000503")
	setupScope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	if err != nil {
		t.Fatalf("failed to create scope: %v", err)
	}
	_, err = setupScope.Conn.Exec(context.Background(), `
		INSERT INTO engine_ontologies (id, project_id, version, is_active)
		VALUES ($1, $2, 2, false)
		ON CONFLICT (id) DO NOTHING
	`, otherOntologyID, tc.projectID)
	setupScope.Close()
	if err != nil {
		t.Fatalf("failed to create other ontology: %v", err)
	}

	// Create question for main ontology
	q1 := &models.OntologyQuestion{
		ID:         uuid.New(),
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Text:       "Question for main ontology",
		Priority:   2,
		Status:     models.QuestionStatusPending,
	}
	if err := tc.repo.Create(ctx, q1); err != nil {
		t.Fatalf("failed to create q1: %v", err)
	}

	// Create question for other ontology
	q2 := &models.OntologyQuestion{
		ID:         uuid.New(),
		ProjectID:  tc.projectID,
		OntologyID: otherOntologyID,
		Text:       "Question for other ontology",
		Priority:   2,
		Status:     models.QuestionStatusPending,
	}
	if err := tc.repo.Create(ctx, q2); err != nil {
		t.Fatalf("failed to create q2: %v", err)
	}

	// Query for main ontology only
	questions, err := tc.repo.ListByOntologyID(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("ListByOntologyID failed: %v", err)
	}

	if len(questions) != 1 {
		t.Errorf("expected 1 question for main ontology, got %d", len(questions))
	}

	if len(questions) > 0 && questions[0].Text != q1.Text {
		t.Errorf("expected q1 text, got %s", questions[0].Text)
	}

	// Clean up other ontology's questions
	cleanupScope, _ := tc.engineDB.DB.WithoutTenant(context.Background())
	_, _ = cleanupScope.Conn.Exec(context.Background(), "DELETE FROM engine_ontology_questions WHERE ontology_id = $1", otherOntologyID)
	cleanupScope.Close()
}
