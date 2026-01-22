//go:build integration

package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// Test IDs for knowledge seeding integration tests (unique range 0x601-0x6xx)
var (
	knowledgeSeedingTestProjectID = uuid.MustParse("00000000-0000-0000-0000-000000000601")
	knowledgeSeedingTestDSID      = uuid.MustParse("00000000-0000-0000-0000-000000000602")
)

// knowledgeSeedingTestContext holds all dependencies for knowledge seeding integration tests.
type knowledgeSeedingTestContext struct {
	t             *testing.T
	engineDB      *testhelpers.EngineDB
	knowledgeSvc  KnowledgeService
	glossarySvc   GlossaryService
	knowledgeRepo repositories.KnowledgeRepository
	glossaryRepo  repositories.GlossaryRepository
	ontologyRepo  repositories.OntologyRepository
	entityRepo    repositories.OntologyEntityRepository
	projectRepo   repositories.ProjectRepository
	projectID     uuid.UUID
	dsID          uuid.UUID
	tempDir       string
	seedFilePath  string
}

// setupKnowledgeSeedingTest creates a test context with real database.
func setupKnowledgeSeedingTest(t *testing.T) *knowledgeSeedingTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	logger := zap.NewNop()

	// Create repositories
	knowledgeRepo := repositories.NewKnowledgeRepository()
	glossaryRepo := repositories.NewGlossaryRepository()
	ontologyRepo := repositories.NewOntologyRepository()
	entityRepo := repositories.NewOntologyEntityRepository()
	projectRepo := repositories.NewProjectRepository()

	// Create temp dir for seed file
	tempDir := t.TempDir()
	seedFilePath := filepath.Join(tempDir, "knowledge.yaml")

	// Create knowledge service
	knowledgeSvc := NewKnowledgeService(knowledgeRepo, projectRepo, logger)

	tc := &knowledgeSeedingTestContext{
		t:             t,
		engineDB:      engineDB,
		knowledgeSvc:  knowledgeSvc,
		knowledgeRepo: knowledgeRepo,
		glossaryRepo:  glossaryRepo,
		ontologyRepo:  ontologyRepo,
		entityRepo:    entityRepo,
		projectRepo:   projectRepo,
		projectID:     knowledgeSeedingTestProjectID,
		dsID:          knowledgeSeedingTestDSID,
		tempDir:       tempDir,
		seedFilePath:  seedFilePath,
	}

	// Ensure project exists with seed path
	tc.ensureTestProject()

	return tc
}

// createTestContext creates a context with tenant scope and returns a cleanup function.
func (tc *knowledgeSeedingTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}

	ctx = database.SetTenantScope(ctx, scope)

	return ctx, func() {
		scope.Close()
	}
}

// ensureTestProject creates the test project with knowledge_seed_path configured.
func (tc *knowledgeSeedingTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	// Create project with knowledge_seed_path parameter
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status, parameters)
		VALUES ($1, $2, 'active', $3)
		ON CONFLICT (id) DO UPDATE SET parameters = $3
	`, tc.projectID, "Knowledge Seeding Integration Test Project", map[string]interface{}{
		"knowledge_seed_path": tc.seedFilePath,
	})
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}

	// Create test datasource
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config, created_at, updated_at)
		VALUES ($1, $2, 'Test Datasource', 'postgres', '{}', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, tc.dsID, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to ensure test datasource: %v", err)
	}
}

// cleanup removes all test data.
func (tc *knowledgeSeedingTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Delete in reverse order of dependencies
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_project_knowledge WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_business_glossary WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_entities WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, tc.projectID)
}

// writeTikrSeedFile creates a knowledge seed file with Tikr-specific domain knowledge.
func (tc *knowledgeSeedingTestContext) writeTikrSeedFile() {
	tc.t.Helper()

	seedContent := `# Tikr Domain Knowledge Seed File
terminology:
  - fact: "A tik represents 6 seconds of engagement time"
    context: "Billing unit - from billing_helpers.go:413"

  - fact: "Host is a content creator who receives payments"
    context: "User role - identified by host_id columns"

  - fact: "Visitor is a viewer who pays for engagements"
    context: "User role - identified by visitor_id columns"

business_rules:
  - fact: "Platform fees are 4.5% of total amount"
    context: "billing_helpers.go:373"

  - fact: "Tikr share is 30% of amount after platform fees"
    context: "billing_helpers.go:375"

  - fact: "Host earns approximately 66.35% of total transaction"
    context: "Calculation: (1 - 0.045) * (1 - 0.30) = 0.6635"

conventions:
  - fact: "All monetary amounts are stored in cents (USD)"
    context: "Currency convention across billing tables"

  - fact: "Minimum capture amount is $1.00 (100 cents)"
    context: "MinCaptureAmount constant"
`
	err := os.WriteFile(tc.seedFilePath, []byte(seedContent), 0600)
	require.NoError(tc.t, err, "Failed to write seed file")
}

// createTestOntologyWithEntities creates an ontology with Tikr-specific entities.
func (tc *knowledgeSeedingTestContext) createTestOntologyWithEntities(ctx context.Context) uuid.UUID {
	tc.t.Helper()

	ontologyID := uuid.New()

	// Create ontology
	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: tc.projectID,
		Version:   1,
		IsActive:  true,
		DomainSummary: &models.DomainSummary{
			Description: "Video engagement platform where viewers pay creators per engagement",
			Domains:     []string{"billing", "user", "engagement"},
		},
	}

	err := tc.ontologyRepo.Create(ctx, ontology)
	require.NoError(tc.t, err, "Failed to create test ontology")

	// Create User entity
	userEntityID := uuid.New()
	userEntity := &models.OntologyEntity{
		ID:            userEntityID,
		ProjectID:     tc.projectID,
		OntologyID:    ontologyID,
		Name:          "User",
		Description:   "Platform user who can be a host or visitor",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
		Domain:        "user",
	}
	err = tc.entityRepo.Create(ctx, userEntity)
	require.NoError(tc.t, err, "Failed to create user entity")

	// Create Engagement entity
	engagementEntityID := uuid.New()
	engagementEntity := &models.OntologyEntity{
		ID:            engagementEntityID,
		ProjectID:     tc.projectID,
		OntologyID:    ontologyID,
		Name:          "Engagement",
		Description:   "User engagement session measured in tiks",
		PrimarySchema: "public",
		PrimaryTable:  "billing_engagements",
		PrimaryColumn: "id",
		Domain:        "billing",
	}
	err = tc.entityRepo.Create(ctx, engagementEntity)
	require.NoError(tc.t, err, "Failed to create engagement entity")

	return ontologyID
}

// ============================================================================
// Integration Tests: Knowledge Seed File Loading
// ============================================================================

func TestKnowledgeSeeding_Integration_LoadSeedFile(t *testing.T) {
	tc := setupKnowledgeSeedingTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Write the seed file
	tc.writeTikrSeedFile()

	// Seed knowledge from file
	count, err := tc.knowledgeSvc.SeedKnowledgeFromFile(ctx, tc.projectID)
	require.NoError(t, err)
	assert.Equal(t, 8, count, "Should seed 8 facts (3 terminology + 3 business_rule + 2 convention)")

	// Verify facts were stored
	facts, err := tc.knowledgeRepo.GetByProject(ctx, tc.projectID)
	require.NoError(t, err)
	assert.Len(t, facts, 8)

	// Verify fact types
	factTypes := make(map[string]int)
	for _, fact := range facts {
		factTypes[fact.FactType]++
	}
	assert.Equal(t, 3, factTypes["terminology"], "Should have 3 terminology facts")
	assert.Equal(t, 3, factTypes["business_rule"], "Should have 3 business_rule facts")
	assert.Equal(t, 2, factTypes["convention"], "Should have 2 convention facts")
}

func TestKnowledgeSeeding_Integration_TikTerminologyCaptured(t *testing.T) {
	tc := setupKnowledgeSeedingTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Write the seed file
	tc.writeTikrSeedFile()

	// Seed knowledge from file
	_, err := tc.knowledgeSvc.SeedKnowledgeFromFile(ctx, tc.projectID)
	require.NoError(t, err)

	// Verify 'Tik' terminology is captured
	terminologyFacts, err := tc.knowledgeRepo.GetByType(ctx, tc.projectID, "terminology")
	require.NoError(t, err)

	// Check that tik terminology is present
	var foundTik bool
	for _, fact := range terminologyFacts {
		if fact.Key == "A tik represents 6 seconds of engagement time" {
			foundTik = true
			assert.Contains(t, fact.Context, "billing_helpers.go", "Context should reference source file")
			break
		}
	}
	assert.True(t, foundTik, "'Tik' terminology should be captured in knowledge")
}

func TestKnowledgeSeeding_Integration_HostVisitorRolesCaptured(t *testing.T) {
	tc := setupKnowledgeSeedingTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Write the seed file
	tc.writeTikrSeedFile()

	// Seed knowledge from file
	_, err := tc.knowledgeSvc.SeedKnowledgeFromFile(ctx, tc.projectID)
	require.NoError(t, err)

	// Verify Host and Visitor roles are captured
	terminologyFacts, err := tc.knowledgeRepo.GetByType(ctx, tc.projectID, "terminology")
	require.NoError(t, err)

	var foundHost, foundVisitor bool
	for _, fact := range terminologyFacts {
		if fact.Key == "Host is a content creator who receives payments" {
			foundHost = true
			assert.Contains(t, fact.Context, "host_id", "Host context should reference column naming")
		}
		if fact.Key == "Visitor is a viewer who pays for engagements" {
			foundVisitor = true
			assert.Contains(t, fact.Context, "visitor_id", "Visitor context should reference column naming")
		}
	}
	assert.True(t, foundHost, "'Host' role should be captured in knowledge")
	assert.True(t, foundVisitor, "'Visitor' role should be captured in knowledge")
}

func TestKnowledgeSeeding_Integration_FeeStructureCaptured(t *testing.T) {
	tc := setupKnowledgeSeedingTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Write the seed file
	tc.writeTikrSeedFile()

	// Seed knowledge from file
	_, err := tc.knowledgeSvc.SeedKnowledgeFromFile(ctx, tc.projectID)
	require.NoError(t, err)

	// Verify fee structure is captured in business rules
	businessRules, err := tc.knowledgeRepo.GetByType(ctx, tc.projectID, "business_rule")
	require.NoError(t, err)

	var foundPlatformFee, foundTikrShare, foundHostEarnings bool
	for _, fact := range businessRules {
		if fact.Key == "Platform fees are 4.5% of total amount" {
			foundPlatformFee = true
			assert.Contains(t, fact.Context, "billing_helpers.go", "Platform fee context should reference source")
		}
		if fact.Key == "Tikr share is 30% of amount after platform fees" {
			foundTikrShare = true
			assert.Contains(t, fact.Context, "billing_helpers.go", "Tikr share context should reference source")
		}
		if fact.Key == "Host earns approximately 66.35% of total transaction" {
			foundHostEarnings = true
			assert.Contains(t, fact.Context, "0.6635", "Host earnings context should include calculation")
		}
	}
	assert.True(t, foundPlatformFee, "Platform fee rule (4.5%) should be captured")
	assert.True(t, foundTikrShare, "Tikr share rule (30%) should be captured")
	assert.True(t, foundHostEarnings, "Host earnings rule (~66.35%) should be captured")
}

func TestKnowledgeSeeding_Integration_NoSeedPath_ReturnsZero(t *testing.T) {
	tc := setupKnowledgeSeedingTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Clear the seed path from project
	scope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		UPDATE engine_projects SET parameters = '{}' WHERE id = $1
	`, tc.projectID)
	require.NoError(t, err)

	// Attempt to seed knowledge - should return 0 without error
	count, err := tc.knowledgeSvc.SeedKnowledgeFromFile(ctx, tc.projectID)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "Should return 0 when no seed path is configured")
}

// ============================================================================
// Integration Tests: Glossary Discovery with Domain Knowledge
// ============================================================================

// mockLLMClientForKnowledgeSeeding captures prompts and returns domain-specific terms.
type mockLLMClientForKnowledgeSeeding struct {
	capturedPrompt        string
	capturedSystemMessage string
}

func (m *mockLLMClientForKnowledgeSeeding) GenerateResponse(_ context.Context, prompt string, systemMessage string, _ float64, _ bool) (*llm.GenerateResponseResult, error) {
	m.capturedPrompt = prompt
	m.capturedSystemMessage = systemMessage

	// Return domain-specific terms (NOT generic SaaS metrics)
	return &llm.GenerateResponseResult{
		Content: `{"terms": [
			{
				"term": "Tik Count",
				"definition": "Number of 6-second engagement units consumed during a session",
				"aliases": ["Engagement Tiks"]
			},
			{
				"term": "Host Earnings",
				"definition": "Amount earned by content creators after platform fees and Tikr share",
				"aliases": ["Creator Revenue"]
			},
			{
				"term": "Engagement Revenue",
				"definition": "Total revenue from viewer-to-host engagement sessions",
				"aliases": ["Visitor Spend"]
			}
		]}`,
		PromptTokens:     100,
		CompletionTokens: 200,
	}, nil
}

func (m *mockLLMClientForKnowledgeSeeding) CreateEmbedding(_ context.Context, _ string, _ string) ([]float32, error) {
	return nil, nil
}

func (m *mockLLMClientForKnowledgeSeeding) CreateEmbeddings(_ context.Context, _ []string, _ string) ([][]float32, error) {
	return nil, nil
}

func (m *mockLLMClientForKnowledgeSeeding) GetModel() string {
	return "test-model"
}

func (m *mockLLMClientForKnowledgeSeeding) GetEndpoint() string {
	return "https://test.endpoint"
}

// mockLLMFactoryForKnowledgeSeeding returns the mock client.
type mockLLMFactoryForKnowledgeSeeding struct {
	client llm.LLMClient
}

func (f *mockLLMFactoryForKnowledgeSeeding) CreateForProject(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return f.client, nil
}

func (f *mockLLMFactoryForKnowledgeSeeding) CreateEmbeddingClient(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return f.client, nil
}

func (f *mockLLMFactoryForKnowledgeSeeding) CreateStreamingClient(_ context.Context, _ uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

func TestKnowledgeSeeding_Integration_DomainKnowledgeIncludedInGlossaryPrompt(t *testing.T) {
	tc := setupKnowledgeSeedingTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Write the seed file and load knowledge
	tc.writeTikrSeedFile()
	_, err := tc.knowledgeSvc.SeedKnowledgeFromFile(ctx, tc.projectID)
	require.NoError(t, err)

	// Create ontology with entities (SuggestTerms finds the active ontology automatically)
	_ = tc.createTestOntologyWithEntities(ctx)

	// Create mock LLM client to capture prompts
	mockLLM := &mockLLMClientForKnowledgeSeeding{}
	mockFactory := &mockLLMFactoryForKnowledgeSeeding{client: mockLLM}

	// Create glossary service with knowledge repository
	glossarySvc := NewGlossaryService(
		tc.glossaryRepo,
		tc.ontologyRepo,
		tc.entityRepo,
		tc.knowledgeRepo,
		nil, // datasourceSvc
		nil, // adapterFactory
		mockFactory,
		nil, // getTenantCtx
		zap.NewNop(),
	)

	// Call SuggestTerms which should include knowledge in the prompt
	_, err = glossarySvc.SuggestTerms(ctx, tc.projectID)
	require.NoError(t, err)

	// Verify domain knowledge was included in the prompt
	assert.Contains(t, mockLLM.capturedPrompt, "Domain Knowledge", "Prompt should include domain knowledge section")
	assert.Contains(t, mockLLM.capturedPrompt, "tik represents 6 seconds", "Prompt should include tik terminology")
	assert.Contains(t, mockLLM.capturedPrompt, "Host is a content creator", "Prompt should include host role")
	assert.Contains(t, mockLLM.capturedPrompt, "Visitor is a viewer", "Prompt should include visitor role")
	assert.Contains(t, mockLLM.capturedPrompt, "Platform fees are 4.5%", "Prompt should include fee structure")
	assert.Contains(t, mockLLM.capturedPrompt, "Tikr share is 30%", "Prompt should include Tikr share rule")

	// Verify system message guides against generic metrics
	assert.Contains(t, mockLLM.capturedSystemMessage, "DO NOT suggest generic SaaS metrics",
		"System message should warn against generic metrics")
}

func TestKnowledgeSeeding_Integration_GlossaryDiscovery_DomainSpecificTerms(t *testing.T) {
	tc := setupKnowledgeSeedingTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Write the seed file and load knowledge
	tc.writeTikrSeedFile()
	_, err := tc.knowledgeSvc.SeedKnowledgeFromFile(ctx, tc.projectID)
	require.NoError(t, err)

	// Create ontology with entities
	ontologyID := tc.createTestOntologyWithEntities(ctx)

	// Create mock LLM client that returns domain-specific terms
	mockLLM := &mockLLMClientForKnowledgeSeeding{}
	mockFactory := &mockLLMFactoryForKnowledgeSeeding{client: mockLLM}

	// Create glossary service
	glossarySvc := NewGlossaryService(
		tc.glossaryRepo,
		tc.ontologyRepo,
		tc.entityRepo,
		tc.knowledgeRepo,
		nil, // datasourceSvc
		nil, // adapterFactory
		mockFactory,
		nil, // getTenantCtx
		zap.NewNop(),
	)

	// Discover glossary terms using the ontologyID
	count, err := glossarySvc.DiscoverGlossaryTerms(ctx, tc.projectID, ontologyID)
	require.NoError(t, err)
	assert.Equal(t, 3, count, "Should discover 3 domain-specific terms")

	// Verify discovered terms are domain-specific, NOT generic SaaS metrics
	terms, err := tc.glossaryRepo.GetByProject(ctx, tc.projectID)
	require.NoError(t, err)
	require.Len(t, terms, 3)

	termNames := make([]string, len(terms))
	for i, term := range terms {
		termNames[i] = term.Term
	}

	// Verify domain-specific terms were generated
	assert.Contains(t, termNames, "Tik Count", "Should generate 'Tik Count' term")
	assert.Contains(t, termNames, "Host Earnings", "Should generate 'Host Earnings' term")
	assert.Contains(t, termNames, "Engagement Revenue", "Should generate 'Engagement Revenue' term")

	// Verify NO generic SaaS metrics
	for _, name := range termNames {
		assert.NotEqual(t, "Churn Rate", name, "Should NOT generate generic 'Churn Rate'")
		assert.NotEqual(t, "Active Subscribers", name, "Should NOT generate generic 'Active Subscribers'")
		assert.NotEqual(t, "Average Order Value", name, "Should NOT generate generic 'AOV'")
		assert.NotEqual(t, "Monthly Active Users", name, "Should NOT generate generic 'MAU'")
		assert.NotEqual(t, "Customer Lifetime Value", name, "Should NOT generate generic 'CLV'")
	}
}

func TestKnowledgeSeeding_Integration_ConventionsCaptured(t *testing.T) {
	tc := setupKnowledgeSeedingTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Write the seed file
	tc.writeTikrSeedFile()

	// Seed knowledge from file
	_, err := tc.knowledgeSvc.SeedKnowledgeFromFile(ctx, tc.projectID)
	require.NoError(t, err)

	// Verify conventions are captured
	conventions, err := tc.knowledgeRepo.GetByType(ctx, tc.projectID, "convention")
	require.NoError(t, err)

	var foundCurrency, foundMinCapture bool
	for _, fact := range conventions {
		if fact.Key == "All monetary amounts are stored in cents (USD)" {
			foundCurrency = true
		}
		if fact.Key == "Minimum capture amount is $1.00 (100 cents)" {
			foundMinCapture = true
		}
	}
	assert.True(t, foundCurrency, "Currency convention should be captured")
	assert.True(t, foundMinCapture, "Minimum capture convention should be captured")
}

func TestKnowledgeSeeding_Integration_IdempotentSeeding(t *testing.T) {
	tc := setupKnowledgeSeedingTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Write the seed file
	tc.writeTikrSeedFile()

	// Seed knowledge twice
	count1, err := tc.knowledgeSvc.SeedKnowledgeFromFile(ctx, tc.projectID)
	require.NoError(t, err)
	assert.Equal(t, 8, count1)

	count2, err := tc.knowledgeSvc.SeedKnowledgeFromFile(ctx, tc.projectID)
	require.NoError(t, err)
	assert.Equal(t, 8, count2, "Second seeding should also report 8 facts (upsert)")

	// Verify only 8 facts exist (not 16 - upsert should deduplicate)
	facts, err := tc.knowledgeRepo.GetByProject(ctx, tc.projectID)
	require.NoError(t, err)
	assert.Len(t, facts, 8, "Should have exactly 8 facts after idempotent seeding")
}
