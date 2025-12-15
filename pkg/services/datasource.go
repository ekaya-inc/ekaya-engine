package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// DatasourceService defines the interface for datasource operations.
type DatasourceService interface {
	// Create creates a new datasource with encrypted config.
	Create(ctx context.Context, projectID uuid.UUID, name, dsType string, config map[string]any) (*models.Datasource, error)

	// Get retrieves a datasource by ID within a project with decrypted config.
	Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error)

	// GetByName retrieves a datasource by name with decrypted config.
	GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error)

	// List retrieves all datasources for a project with decrypted configs.
	List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, error)

	// Update modifies a datasource with encrypted config.
	Update(ctx context.Context, id uuid.UUID, name, dsType string, config map[string]any) error

	// Delete removes a datasource.
	Delete(ctx context.Context, id uuid.UUID) error

	// TestConnection tests connectivity to a datasource without saving it.
	TestConnection(ctx context.Context, dsType string, config map[string]any) error
}

// datasourceService implements DatasourceService.
type datasourceService struct {
	repo           repositories.DatasourceRepository
	encryptor      *crypto.CredentialEncryptor
	adapterFactory datasource.DatasourceAdapterFactory
	logger         *zap.Logger
}

// NewDatasourceService creates a new datasource service with dependencies.
func NewDatasourceService(
	repo repositories.DatasourceRepository,
	encryptor *crypto.CredentialEncryptor,
	adapterFactory datasource.DatasourceAdapterFactory,
	logger *zap.Logger,
) DatasourceService {
	return &datasourceService{
		repo:           repo,
		encryptor:      encryptor,
		adapterFactory: adapterFactory,
		logger:         logger,
	}
}

// Create creates a new datasource with encrypted config.
func (s *datasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType string, config map[string]any) (*models.Datasource, error) {
	// Validate inputs
	if name == "" {
		return nil, fmt.Errorf("datasource name is required")
	}
	if dsType == "" {
		return nil, fmt.Errorf("datasource type is required")
	}
	if config == nil {
		config = make(map[string]any)
	}

	// Encrypt config
	encryptedConfig, err := s.encryptConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt config: %w", err)
	}

	// Create datasource
	ds := &models.Datasource{
		ProjectID:      projectID,
		Name:           name,
		DatasourceType: dsType,
		Config:         config,
	}

	if err := s.repo.Create(ctx, ds, encryptedConfig); err != nil {
		return nil, err
	}

	s.logger.Info("Created datasource",
		zap.String("id", ds.ID.String()),
		zap.String("project_id", projectID.String()),
		zap.String("name", name),
		zap.String("type", dsType),
	)

	return ds, nil
}

// Get retrieves a datasource by ID within a project with decrypted config.
func (s *datasourceService) Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error) {
	ds, encryptedConfig, err := s.repo.GetByID(ctx, projectID, id)
	if err != nil {
		return nil, err
	}

	// Decrypt config
	config, err := s.decryptConfig(encryptedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt config: %w", err)
	}
	ds.Config = config

	return ds, nil
}

// GetByName retrieves a datasource by name with decrypted config.
func (s *datasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	ds, encryptedConfig, err := s.repo.GetByName(ctx, projectID, name)
	if err != nil {
		return nil, err
	}

	// Decrypt config
	config, err := s.decryptConfig(encryptedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt config: %w", err)
	}
	ds.Config = config

	return ds, nil
}

// List retrieves all datasources for a project with decrypted configs.
func (s *datasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, error) {
	datasources, encryptedConfigs, err := s.repo.List(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Decrypt all configs
	for i, ds := range datasources {
		config, err := s.decryptConfig(encryptedConfigs[i])
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt config for datasource %s: %w", ds.ID, err)
		}
		ds.Config = config
	}

	return datasources, nil
}

// Update modifies a datasource with encrypted config.
func (s *datasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType string, config map[string]any) error {
	// Validate inputs
	if name == "" {
		return fmt.Errorf("datasource name is required")
	}
	if dsType == "" {
		return fmt.Errorf("datasource type is required")
	}
	if config == nil {
		config = make(map[string]any)
	}

	// Encrypt config
	encryptedConfig, err := s.encryptConfig(config)
	if err != nil {
		return fmt.Errorf("failed to encrypt config: %w", err)
	}

	if err := s.repo.Update(ctx, id, name, dsType, encryptedConfig); err != nil {
		return err
	}

	s.logger.Info("Updated datasource",
		zap.String("id", id.String()),
	)

	return nil
}

// Delete removes a datasource.
func (s *datasourceService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	s.logger.Info("Deleted datasource",
		zap.String("id", id.String()),
	)

	return nil
}

// TestConnection tests connectivity to a datasource without saving it.
func (s *datasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any) error {
	adapter, err := s.adapterFactory.NewConnectionTester(ctx, dsType, config)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer adapter.Close()

	if err := adapter.TestConnection(ctx); err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	s.logger.Info("Connection test successful", zap.String("type", dsType))
	return nil
}

// encryptConfig serializes config to JSON and encrypts it.
func (s *datasourceService) encryptConfig(config map[string]any) (string, error) {
	jsonBytes, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}
	return s.encryptor.Encrypt(string(jsonBytes))
}

// decryptConfig decrypts and deserializes config from encrypted string.
func (s *datasourceService) decryptConfig(encrypted string) (map[string]any, error) {
	decrypted, err := s.encryptor.Decrypt(encrypted)
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(decrypted), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return config, nil
}

// Ensure datasourceService implements DatasourceService at compile time.
var _ DatasourceService = (*datasourceService)(nil)
