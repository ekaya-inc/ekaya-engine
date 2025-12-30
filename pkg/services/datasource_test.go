package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
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

func (m *mockDatasourceRepository) Update(ctx context.Context, id uuid.UUID, name, dsType, encryptedConfig string) error {
	m.capturedEncryptedConfig = encryptedConfig
	return m.updateErr
}

func (m *mockDatasourceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteErr
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

func newTestService(repo *mockDatasourceRepository) (DatasourceService, *crypto.CredentialEncryptor) {
	encryptor, _ := crypto.NewCredentialEncryptor(testEncryptionKey)
	factory := &mockAdapterFactory{}
	// Pass nil for projectService in tests - auto-set default datasource is tested separately
	return NewDatasourceService(repo, encryptor, factory, nil, zap.NewNop()), encryptor
}

func TestDatasourceService_Create_Success(t *testing.T) {
	repo := &mockDatasourceRepository{}
	service, _ := newTestService(repo)

	config := map[string]any{
		"host":     "localhost",
		"port":     5432,
		"database": "testdb",
	}

	ds, err := service.Create(context.Background(), uuid.New(), "my-postgres", "postgres", config)
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

	_, err := service.Create(context.Background(), uuid.New(), "", "postgres", nil)
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

	_, err := service.Create(context.Background(), uuid.New(), "my-ds", "", nil)
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

	ds, err := service.Create(context.Background(), uuid.New(), "my-ds", "postgres", nil)
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

	_, err := service.Create(context.Background(), uuid.New(), "existing-name", "postgres", nil)
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
	err := service.Update(context.Background(), uuid.New(), "updated-name", "postgres", config)
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
	err := service.Update(context.Background(), uuid.New(), "", "postgres", nil)
	if err == nil || err.Error() != "datasource name is required" {
		t.Errorf("expected name validation error, got: %v", err)
	}

	// Empty type
	err = service.Update(context.Background(), uuid.New(), "name", "", nil)
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
	_, err := service.Create(context.Background(), projectID, "complex-db", "postgres", originalConfig)
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
	service := NewDatasourceService(repo, encryptor, factory, nil, zap.NewNop())

	config := map[string]any{
		"host":     "localhost",
		"user":     "testuser",
		"database": "testdb",
	}

	err := service.TestConnection(context.Background(), "postgres", config)
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
	service := NewDatasourceService(repo, encryptor, factory, nil, zap.NewNop())

	err := service.TestConnection(context.Background(), "unknown", nil)
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
	service := NewDatasourceService(repo, encryptor, factory, nil, zap.NewNop())

	config := map[string]any{
		"host":     "localhost",
		"user":     "testuser",
		"database": "testdb",
	}

	err := service.TestConnection(context.Background(), "postgres", config)
	if err == nil {
		t.Fatal("expected connection error")
	}
}
