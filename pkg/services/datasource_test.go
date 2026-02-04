//go:build ignore

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// Test encryption key (32 bytes, base64 encoded) - same as crypto/credentials_test.go
const testEncryptionKey = "dGVzdC1rZXktZm9yLXVuaXQtdGVzdHMtMzItYnl0ZXM="

// mockDatasourceRepository is a configurable mock for testing.
type mockDatasourceRepository struct {
	datasource       *models.Datasource
	datasources      []*models.Datasource
	encryptedConfig  string
	encryptedConfigs []string
	createErr        error
	getErr           error
	listErr          error
	updateErr        error
	deleteErr        error

	// Capture inputs for verification
	capturedDS              *models.Datasource
	capturedEncryptedConfig string
}

func (m *mockDatasourceRepository) Create(ctx context.Context, ds *models.Datasource, encryptedConfig string) error {
	m.capturedDS = ds
	m.capturedEncryptedConfig = encryptedConfig
	if m.createErr != nil {
		return m.createErr
	}
	ds.ID = uuid.New()
	return nil
}

func (m *mockDatasourceRepository) GetByID(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, string, error) {
	if m.getErr != nil {
		return nil, "", m.getErr
	}
	if m.datasource != nil {
		return m.datasource, m.encryptedConfig, nil
	}
	return &models.Datasource{ID: id, ProjectID: projectID}, "", nil
}

func (m *mockDatasourceRepository) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, string, error) {
	if m.getErr != nil {
		return nil, "", m.getErr
	}
	if m.datasource != nil {
		return m.datasource, m.encryptedConfig, nil
	}
	return &models.Datasource{ProjectID: projectID, Name: name}, "", nil
}

func (m *mockDatasourceRepository) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, []string, error) {
	if m.listErr != nil {
		return nil, nil, m.listErr
	}
	return m.datasources, m.encryptedConfigs, nil
}

func (m *mockDatasourceRepository) Update(ctx context.Context, id uuid.UUID, name, dsType, provider, encryptedConfig string) error {
	m.capturedEncryptedConfig = encryptedConfig
	return m.updateErr
}

func (m *mockDatasourceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteErr
}

func (m *mockDatasourceRepository) GetProjectID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	// For tests, return a fixed UUID
	return uuid.MustParse("00000000-0000-0000-0000-000000000001"), nil
}

// mockConnectionTester is a mock implementation of datasource.ConnectionTester.
type mockConnectionTester struct {
	testErr error
}

func (m *mockConnectionTester) TestConnection(ctx context.Context) error {
	return m.testErr
}

func (m *mockConnectionTester) Close() error {
	return nil
}

// mockAdapterFactory is a mock implementation of datasource.DatasourceAdapterFactory.
type mockAdapterFactory struct {
	tester     *mockConnectionTester
	factoryErr error
}

func (m *mockAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	if m.factoryErr != nil {
		return nil, m.factoryErr
	}
	if m.tester == nil {
		return &mockConnectionTester{}, nil
	}
	return m.tester, nil
}

func (m *mockAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockAdapterFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return []datasource.DatasourceAdapterInfo{
		{Type: "postgres", DisplayName: "PostgreSQL"},
	}
}

// mockDatasourceTestOntologyRepo is a minimal mock for ontology cleanup in datasource tests.
type mockDatasourceTestOntologyRepo struct {
	deleteErr error
}

func (m *mockDatasourceTestOntologyRepo) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return errors.New("not implemented in mock")
}

func (m *mockDatasourceTestOntologyRepo) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockDatasourceTestOntologyRepo) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return errors.New("not implemented in mock")
}

func (m *mockDatasourceTestOntologyRepo) UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error {
	return errors.New("not implemented in mock")
}

func (m *mockDatasourceTestOntologyRepo) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	return errors.New("not implemented in mock")
}

func (m *mockDatasourceTestOntologyRepo) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	return errors.New("not implemented in mock")
}

func (m *mockDatasourceTestOntologyRepo) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, errors.New("not implemented in mock")
}

func (m *mockDatasourceTestOntologyRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return m.deleteErr
}

func newTestService(repo *mockDatasourceRepository) (DatasourceService, *crypto.CredentialEncryptor) {
	encryptor, _ := crypto.NewCredentialEncryptor(testEncryptionKey)
	factory := &mockAdapterFactory{}
	ontologyRepo := &mockDatasourceTestOntologyRepo{}
	// Pass nil for projectService in tests - auto-set default datasource is tested separately
	return NewDatasourceService(repo, ontologyRepo, encryptor, factory, nil, zap.NewNop()), encryptor
}

func TestDatasourceService_Create_Success(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, _ := newTestService(repo)

	config := map[string]any{
		"host":     "localhost",
		"port":     5432,
		"database": "testdb",
	}

	ds, err := service.Create(context.Background(), uuid.New(), "my-postgres", "postgres", "", config)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if ds.Name != "my-postgres" {
		t.Errorf("expected name 'my-postgres', got %q", ds.Name)
	}
	if ds.DatasourceType != "postgres" {
		t.Errorf("expected type 'postgres', got %q", ds.DatasourceType)
	}
	if ds.Config["host"] != "localhost" {
		t.Errorf("expected config host 'localhost', got %v", ds.Config["host"])
	}

	// Verify encrypted config was passed to repo
	if repo.capturedEncryptedConfig == "" {
		t.Error("expected encrypted config to be set")
	}
	// Encrypted config should not be plaintext
	if repo.capturedEncryptedConfig == `{"host":"localhost","port":5432,"database":"testdb"}` {
		t.Error("config was not encrypted")
	}
}

func TestDatasourceService_Create_EmptyName(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, _ := newTestService(repo)

	_, err := service.Create(context.Background(), uuid.New(), "", "postgres", "", nil)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if err.Error() != "datasource name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDatasourceService_Create_EmptyType(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, _ := newTestService(repo)

	_, err := service.Create(context.Background(), uuid.New(), "my-ds", "", "", nil)
	if err == nil {
		t.Fatal("expected error for empty type")
	}
	if err.Error() != "datasource type is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDatasourceService_Create_NilConfig(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, _ := newTestService(repo)

	ds, err := service.Create(context.Background(), uuid.New(), "my-ds", "postgres", "", nil)
	if err != nil {
		t.Fatalf("Create with nil config failed: %v", err)
	}
	if ds.Config == nil {
		t.Error("expected config to be initialized to empty map")
	}
}

func TestDatasourceService_Create_RepoError(t *testing.T) {
	repo := &mockDatasourceRepository{
		createErr: errors.New("duplicate name"),
	}
	service, _ := newTestService(repo)

	_, err := service.Create(context.Background(), uuid.New(), "existing-name", "postgres", "", nil)
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestDatasourceService_Get_DecryptsConfig(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, encryptor := newTestService(repo)

	// Pre-encrypt config for the mock to return
	originalConfig := `{"host":"db.example.com","password":"secret123"}`
	encryptedConfig, err := encryptor.Encrypt(originalConfig)
	if err != nil {
		t.Fatalf("failed to encrypt test config: %v", err)
	}

	dsID := uuid.New()
	projectID := uuid.New()
	repo.datasource = &models.Datasource{
		ID:             dsID,
		ProjectID:      projectID,
		Name:           "prod-db",
		DatasourceType: "postgres",
	}
	repo.encryptedConfig = encryptedConfig

	ds, err := service.Get(context.Background(), projectID, dsID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Verify config was decrypted
	if ds.Config["host"] != "db.example.com" {
		t.Errorf("expected host 'db.example.com', got %v", ds.Config["host"])
	}
	if ds.Config["password"] != "secret123" {
		t.Errorf("expected password 'secret123', got %v", ds.Config["password"])
	}
}

func TestDatasourceService_Get_RepoError(t *testing.T) {
	repo := &mockDatasourceRepository{
		getErr: errors.New("not found"),
	}
	service, _ := newTestService(repo)

	_, err := service.Get(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestDatasourceService_GetByName_DecryptsConfig(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, encryptor := newTestService(repo)

	encryptedConfig, _ := encryptor.Encrypt(`{"host":"localhost"}`)
	projectID := uuid.New()
	repo.datasource = &models.Datasource{
		ID:             uuid.New(),
		ProjectID:      projectID,
		Name:           "local-db",
		DatasourceType: "postgres",
	}
	repo.encryptedConfig = encryptedConfig

	ds, err := service.GetByName(context.Background(), projectID, "local-db")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}

	if ds.Config["host"] != "localhost" {
		t.Errorf("expected host 'localhost', got %v", ds.Config["host"])
	}
}

func TestDatasourceService_List_DecryptsAllConfigs(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, encryptor := newTestService(repo)

	// Setup two datasources with encrypted configs
	enc1, _ := encryptor.Encrypt(`{"host":"db1.example.com"}`)
	enc2, _ := encryptor.Encrypt(`{"host":"db2.example.com"}`)

	projectID := uuid.New()
	repo.datasources = []*models.Datasource{
		{ID: uuid.New(), ProjectID: projectID, Name: "db1", DatasourceType: "postgres"},
		{ID: uuid.New(), ProjectID: projectID, Name: "db2", DatasourceType: "postgres"},
	}
	repo.encryptedConfigs = []string{enc1, enc2}

	datasources, err := service.List(context.Background(), projectID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(datasources) != 2 {
		t.Fatalf("expected 2 datasources, got %d", len(datasources))
	}

	if datasources[0].Config["host"] != "db1.example.com" {
		t.Errorf("expected first host 'db1.example.com', got %v", datasources[0].Config["host"])
	}
	if datasources[1].Config["host"] != "db2.example.com" {
		t.Errorf("expected second host 'db2.example.com', got %v", datasources[1].Config["host"])
	}
}

func TestDatasourceService_List_Empty(t *testing.T) {
	repo := &mockDatasourceRepository{
		datasources:      []*models.Datasource{},
		encryptedConfigs: []string{},
	}
	service, _ := newTestService(repo)

	datasources, err := service.List(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(datasources) != 0 {
		t.Errorf("expected 0 datasources, got %d", len(datasources))
	}
}

func TestDatasourceService_Update_EncryptsConfig(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, _ := newTestService(repo)

	config := map[string]any{"host": "new-host.example.com"}
	err := service.Update(context.Background(), uuid.New(), "updated-name", "postgres", "", config)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify encrypted config was passed
	if repo.capturedEncryptedConfig == "" {
		t.Error("expected encrypted config to be captured")
	}
	if repo.capturedEncryptedConfig == `{"host":"new-host.example.com"}` {
		t.Error("config was not encrypted")
	}
}

func TestDatasourceService_Update_Validation(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, _ := newTestService(repo)

	// Empty name
	err := service.Update(context.Background(), uuid.New(), "", "postgres", "", nil)
	if err == nil || err.Error() != "datasource name is required" {
		t.Errorf("expected name validation error, got: %v", err)
	}

	// Empty type
	err = service.Update(context.Background(), uuid.New(), "name", "", "", nil)
	if err == nil || err.Error() != "datasource type is required" {
		t.Errorf("expected type validation error, got: %v", err)
	}
}

func TestDatasourceService_Delete_Success(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, _ := newTestService(repo)

	err := service.Delete(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestDatasourceService_Delete_RepoError(t *testing.T) {
	repo := &mockDatasourceRepository{
		deleteErr: errors.New("not found"),
	}
	service, _ := newTestService(repo)

	err := service.Delete(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestDatasourceService_EncryptionRoundTrip(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, encryptor := newTestService(repo)

	// Create with complex config
	originalConfig := map[string]any{
		"host":     "db.example.com",
		"port":     float64(5432), // JSON numbers are float64
		"user":     "admin",
		"password": "super-secret-password!@#$%",
		"options": map[string]any{
			"sslmode": "require",
			"timeout": float64(30),
		},
	}

	projectID := uuid.New()
	_, err := service.Create(context.Background(), projectID, "complex-db", "postgres", "", originalConfig)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Decrypt what was stored
	decrypted, err := encryptor.Decrypt(repo.capturedEncryptedConfig)
	if err != nil {
		t.Fatalf("failed to decrypt captured config: %v", err)
	}

	// Verify it contains expected values
	if decrypted == "" {
		t.Error("decrypted config is empty")
	}
	// The password should be in the decrypted JSON
	if !contains(decrypted, "super-secret-password") {
		t.Error("decrypted config does not contain password")
	}
}

func TestDatasourceService_Interface(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, _ := newTestService(repo)
	var _ DatasourceService = service
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDatasourceService_TestConnection_Success(t *testing.T) {
	repo := &mockDatasourceRepository{}
	encryptor, _ := crypto.NewCredentialEncryptor(testEncryptionKey)
	factory := &mockAdapterFactory{
		tester: &mockConnectionTester{},
	}
	ontologyRepo := &mockDatasourceTestOntologyRepo{}
	service := NewDatasourceService(repo, ontologyRepo, encryptor, factory, nil, zap.NewNop())

	config := map[string]any{
		"host":     "localhost",
		"user":     "testuser",
		"database": "testdb",
	}

	err := service.TestConnection(context.Background(), "postgres", config, uuid.Nil)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
}

func TestDatasourceService_TestConnection_FactoryError(t *testing.T) {
	repo := &mockDatasourceRepository{}
	encryptor, _ := crypto.NewCredentialEncryptor(testEncryptionKey)
	factory := &mockAdapterFactory{
		factoryErr: errors.New("unsupported datasource type"),
	}
	ontologyRepo := &mockDatasourceTestOntologyRepo{}
	service := NewDatasourceService(repo, ontologyRepo, encryptor, factory, nil, zap.NewNop())

	err := service.TestConnection(context.Background(), "unknown", nil, uuid.Nil)
	if err == nil {
		t.Fatal("expected error from factory")
	}
}

func TestDatasourceService_TestConnection_ConnectionError(t *testing.T) {
	repo := &mockDatasourceRepository{}
	encryptor, _ := crypto.NewCredentialEncryptor(testEncryptionKey)
	factory := &mockAdapterFactory{
		tester: &mockConnectionTester{
			testErr: errors.New("connection refused"),
		},
	}
	ontologyRepo := &mockDatasourceTestOntologyRepo{}
	service := NewDatasourceService(repo, ontologyRepo, encryptor, factory, nil, zap.NewNop())

	config := map[string]any{
		"host":     "localhost",
		"user":     "testuser",
		"database": "testdb",
	}

	err := service.TestConnection(context.Background(), "postgres", config, uuid.Nil)
	if err == nil {
		t.Fatal("expected connection error")
	}
}

// mockProjectServiceForDatasource tracks default datasource ID for testing.
type mockProjectServiceForDatasource struct {
	defaultDatasourceID uuid.UUID
}

func (m *mockProjectServiceForDatasource) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
	return nil, errors.New("not implemented")
}
func (m *mockProjectServiceForDatasource) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error) {
	return nil, errors.New("not implemented")
}
func (m *mockProjectServiceForDatasource) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, errors.New("not implemented")
}
func (m *mockProjectServiceForDatasource) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, errors.New("not implemented")
}
func (m *mockProjectServiceForDatasource) Delete(ctx context.Context, id uuid.UUID) error {
	return errors.New("not implemented")
}
func (m *mockProjectServiceForDatasource) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	return m.defaultDatasourceID, nil
}
func (m *mockProjectServiceForDatasource) SetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID, datasourceID uuid.UUID) error {
	m.defaultDatasourceID = datasourceID
	return nil
}
func (m *mockProjectServiceForDatasource) SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string) {
}
func (m *mockProjectServiceForDatasource) GetAuthServerURL(ctx context.Context, projectID uuid.UUID) (string, error) {
	return "", errors.New("not implemented")
}
func (m *mockProjectServiceForDatasource) UpdateAuthServerURL(ctx context.Context, projectID uuid.UUID, authServerURL string) error {
	return errors.New("not implemented")
}

func (m *mockProjectServiceForDatasource) GetAutoApproveSettings(ctx context.Context, projectID uuid.UUID) (*AutoApproveSettings, error) {
	return nil, nil
}

func (m *mockProjectServiceForDatasource) SetAutoApproveSettings(ctx context.Context, projectID uuid.UUID, settings *AutoApproveSettings) error {
	return nil
}

func (m *mockProjectServiceForDatasource) GetOntologySettings(ctx context.Context, projectID uuid.UUID) (*OntologySettings, error) {
	return &OntologySettings{UseLegacyPatternMatching: true}, nil
}

func (m *mockProjectServiceForDatasource) SetOntologySettings(ctx context.Context, projectID uuid.UUID, settings *OntologySettings) error {
	return nil
}

// TestDatasourceService_Delete_ClearsDefaultDatasourceID verifies that when the default
// datasource is deleted, the default_datasource_id is cleared so that a new datasource
// can become the default.
//
// Bug scenario:
// 1. User connects datasource A → default_datasource_id = A
// 2. User disconnects datasource A → default_datasource_id still = A (BUG: stale!)
// 3. User connects datasource B → default NOT updated (because currentDefault != uuid.Nil)
// 4. hasEnabledQueries() queries with datasource A (doesn't exist) → returns false
// 5. MCP tools are hidden even though approved_queries is enabled with queries on B
func TestDatasourceService_Delete_ClearsDefaultDatasourceID(t *testing.T) {
	projectID := uuid.New()
	projectService := &mockProjectServiceForDatasource{}

	// Create service with project service for auto-default tracking
	encryptor, _ := crypto.NewCredentialEncryptor(testEncryptionKey)
	factory := &mockAdapterFactory{}
	ontologyRepo := &mockDatasourceTestOntologyRepo{}

	// Track created datasources for the mock repo
	createdDatasources := make(map[uuid.UUID]*models.Datasource)

	repo := &mockDatasourceRepository{}
	// Override Create to track datasources
	originalCreate := repo.Create
	_ = originalCreate // silence unused warning
	repo.createErr = nil

	service := NewDatasourceService(repo, ontologyRepo, encryptor, factory, projectService, zap.NewNop())

	// Step 1: Create datasource A - should become default
	dsA, err := service.Create(context.Background(), projectID, "datasource-a", "postgres", "", map[string]any{"host": "a"})
	if err != nil {
		t.Fatalf("failed to create datasource A: %v", err)
	}
	createdDatasources[dsA.ID] = dsA

	// Verify A is now the default
	if projectService.defaultDatasourceID != dsA.ID {
		t.Fatalf("expected datasource A (%s) to be default, got %s", dsA.ID, projectService.defaultDatasourceID)
	}

	// Step 2: Delete datasource A
	// Update mock repo to return projectID for GetProjectID
	err = service.Delete(context.Background(), dsA.ID)
	if err != nil {
		t.Fatalf("failed to delete datasource A: %v", err)
	}

	// Step 3: Create datasource B
	dsB, err := service.Create(context.Background(), projectID, "datasource-b", "postgres", "", map[string]any{"host": "b"})
	if err != nil {
		t.Fatalf("failed to create datasource B: %v", err)
	}

	// Step 4: Verify B is now the default
	// THIS IS THE FAILING ASSERTION - currently default will still be A (stale)
	if projectService.defaultDatasourceID != dsB.ID {
		t.Errorf("BUG: expected datasource B (%s) to be default after A was deleted, but got stale reference to %s",
			dsB.ID, projectService.defaultDatasourceID)
	}
}

func TestDatasourceService_List_PartialDecryptionFailure(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, encryptor := newTestService(repo)

	// Create a second encryptor with a different key
	wrongEncryptor, err := crypto.NewCredentialEncryptor("wrong-key")
	if err != nil {
		t.Fatalf("failed to create wrong encryptor: %v", err)
	}

	// Setup two datasources: one encrypted with correct key, one with wrong key
	goodConfig, _ := encryptor.Encrypt(`{"host":"good.example.com"}`)
	badConfig, _ := wrongEncryptor.Encrypt(`{"host":"bad.example.com"}`)

	projectID := uuid.New()
	repo.datasources = []*models.Datasource{
		{ID: uuid.New(), ProjectID: projectID, Name: "good-db", DatasourceType: "postgres"},
		{ID: uuid.New(), ProjectID: projectID, Name: "bad-db", DatasourceType: "postgres"},
	}
	repo.encryptedConfigs = []string{goodConfig, badConfig}

	datasources, err := service.List(context.Background(), projectID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(datasources) != 2 {
		t.Fatalf("expected 2 datasources, got %d", len(datasources))
	}

	// First datasource should have decrypted successfully
	if datasources[0].DecryptionFailed {
		t.Error("expected first datasource to decrypt successfully")
	}
	if datasources[0].Config["host"] != "good.example.com" {
		t.Errorf("expected first host 'good.example.com', got %v", datasources[0].Config["host"])
	}

	// Second datasource should have decryption failure
	if !datasources[1].DecryptionFailed {
		t.Error("expected second datasource to have decryption failure")
	}
	if datasources[1].ErrorMessage != "datasource credentials were encrypted with a different key" {
		t.Errorf("unexpected error message: %s", datasources[1].ErrorMessage)
	}
	// Config should be nil/empty when decryption fails
	if datasources[1].Config != nil {
		t.Error("expected config to be nil when decryption fails")
	}
}

func TestDatasourceService_List_AllDecryptionFailures(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, _ := newTestService(repo)

	// Create a different encryptor with wrong key
	wrongEncryptor, err := crypto.NewCredentialEncryptor("wrong-key")
	if err != nil {
		t.Fatalf("failed to create wrong encryptor: %v", err)
	}

	// Setup datasource encrypted with wrong key
	badConfig, _ := wrongEncryptor.Encrypt(`{"host":"bad.example.com"}`)

	projectID := uuid.New()
	repo.datasources = []*models.Datasource{
		{ID: uuid.New(), ProjectID: projectID, Name: "bad-db", DatasourceType: "postgres"},
	}
	repo.encryptedConfigs = []string{badConfig}

	datasources, err := service.List(context.Background(), projectID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(datasources) != 1 {
		t.Fatalf("expected 1 datasource, got %d", len(datasources))
	}

	// Datasource should have decryption failure
	if !datasources[0].DecryptionFailed {
		t.Error("expected datasource to have decryption failure")
	}
	if datasources[0].ErrorMessage != "datasource credentials were encrypted with a different key" {
		t.Errorf("unexpected error message: %s", datasources[0].ErrorMessage)
	}
}
