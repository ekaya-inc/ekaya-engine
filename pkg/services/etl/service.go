package etl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// Service is the main ETL orchestrator that wires together parsing, inference,
// ontology matching, and loading.
type Service struct {
	datasourceService services.DatasourceService
	adapterFactory    datasource.DatasourceAdapterFactory
	installedAppSvc   services.InstalledAppService
	ontologyMatcher   *OntologyMatcher
	loader            *Loader
	watcher           *Watcher
	etlRepo           repositories.ETLRepository
	logger            *zap.Logger
}

// NewService creates a new ETL service.
func NewService(
	datasourceService services.DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	installedAppSvc services.InstalledAppService,
	ontologyMatcher *OntologyMatcher,
	etlRepo repositories.ETLRepository,
	logger *zap.Logger,
) *Service {
	s := &Service{
		datasourceService: datasourceService,
		adapterFactory:    adapterFactory,
		installedAppSvc:   installedAppSvc,
		ontologyMatcher:   ontologyMatcher,
		loader:            NewLoader(logger),
		watcher:           NewWatcher(logger),
		etlRepo:           etlRepo,
		logger:            logger,
	}

	// Register file handlers
	s.watcher.RegisterHandler([]string{".csv", ".tsv", ".txt"}, s.handleFile)
	s.watcher.RegisterHandler([]string{".xlsx", ".xlsm", ".xltx"}, s.handleFile)

	return s
}

// Preview parses a file and returns the inferred schema and sample rows without loading.
func (s *Service) Preview(ctx context.Context, projectID uuid.UUID, appID string, fileName string, data []byte) (*models.PreviewResult, error) {
	settings, err := s.getSettings(ctx, projectID, appID)
	if err != nil {
		return nil, err
	}

	parsedSheets, err := s.parseFile(appID, fileName, data)
	if err != nil {
		return nil, err
	}

	// Use the first sheet/result for preview
	if len(parsedSheets) == 0 {
		return nil, fmt.Errorf("no data found in file")
	}
	parsed := parsedSheets[0]

	schema := InferSchema(parsed.Headers, parsed.Rows, settings.SampleRows)

	// Sample rows for preview (up to 10)
	sampleCount := 10
	if len(parsed.Rows) < sampleCount {
		sampleCount = len(parsed.Rows)
	}

	result := &models.PreviewResult{
		FileName:       fileName,
		InferredSchema: schema,
		SampleRows:     parsed.Rows[:sampleCount],
		TotalRows:      len(parsed.Rows),
	}

	// Ontology match if enabled
	if settings.UseOntology {
		tableName := SanitizeTableName(fileName)
		match, err := s.ontologyMatcher.Match(ctx, projectID, tableName, schema)
		if err != nil {
			s.logger.Warn("ontology matching failed", zap.Error(err))
		} else {
			result.OntologyMatch = match
		}
	}

	return result, nil
}

// LoadFile parses and loads a file into the project's datasource.
func (s *Service) LoadFile(ctx context.Context, projectID uuid.UUID, appID string, fileName string, data []byte) (*models.LoadResult, error) {
	settings, err := s.getSettings(ctx, projectID, appID)
	if err != nil {
		return nil, err
	}

	parsedSheets, err := s.parseFile(appID, fileName, data)
	if err != nil {
		return nil, err
	}

	// Get the project's datasource
	executor, dsType, err := s.getExecutor(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to datasource: %w", err)
	}
	defer executor.Close()
	_ = dsType // Could be used for dialect-specific SQL generation

	// Load each sheet
	var combinedResult models.LoadResult
	for _, parsed := range parsedSheets {
		tableName := s.resolveTableName(fileName, parsed.SheetName)
		schema := InferSchema(parsed.Headers, parsed.Rows, settings.SampleRows)

		// Ontology matching
		if settings.UseOntology {
			match, err := s.ontologyMatcher.Match(ctx, projectID, tableName, schema)
			if err == nil && !match.IsNewTable {
				tableName = match.MatchedTable
				// Apply ontology-matched types
				for i, col := range schema {
					for _, m := range match.ColumnMappings {
						if m.InferredName == col.Name && m.MappedType != "" {
							schema[i].SQLType = m.MappedType
							if m.MappedName != "" {
								schema[i].Name = m.MappedName
							}
						}
					}
				}
			}
		}

		// Record load status
		loadStatus := &models.LoadStatus{
			ID:        uuid.New(),
			ProjectID: projectID,
			AppID:     appID,
			FileName:  fileName,
			TableName: tableName,
			StartedAt: time.Now(),
			Status:    models.ETLStatusRunning,
		}
		_ = s.etlRepo.CreateLoadStatus(ctx, loadStatus)

		// Create table if needed
		if settings.AutoCreateTables {
			if err := s.loader.CreateTable(ctx, executor, tableName, schema); err != nil {
				loadStatus.Status = models.ETLStatusFailed
				loadStatus.Errors = []string{err.Error()}
				now := time.Now()
				loadStatus.CompletedAt = &now
				_ = s.etlRepo.UpdateLoadStatus(ctx, loadStatus)
				return nil, err
			}
		}

		// Load rows
		result := s.loader.LoadRows(ctx, executor, tableName, schema, parsed.Rows, settings.BatchSize)

		// Update load status
		now := time.Now()
		loadStatus.TableName = result.TableName
		loadStatus.RowsAttempted = result.RowsAttempted
		loadStatus.RowsLoaded = result.RowsLoaded
		loadStatus.RowsSkipped = result.RowsSkipped
		loadStatus.Errors = result.Errors
		loadStatus.CompletedAt = &now
		if result.RowsSkipped > 0 || len(result.Errors) > 0 {
			loadStatus.Status = models.ETLStatusCompleted
		} else {
			loadStatus.Status = models.ETLStatusCompleted
		}
		_ = s.etlRepo.UpdateLoadStatus(ctx, loadStatus)

		combinedResult.TableName = result.TableName
		combinedResult.RowsAttempted += result.RowsAttempted
		combinedResult.RowsLoaded += result.RowsLoaded
		combinedResult.RowsSkipped += result.RowsSkipped
		combinedResult.Errors = append(combinedResult.Errors, result.Errors...)
	}

	return &combinedResult, nil
}

// GetLoadHistory returns load history for a project.
func (s *Service) GetLoadHistory(ctx context.Context, projectID uuid.UUID, appID string, limit int) ([]*models.LoadStatus, error) {
	return s.etlRepo.ListLoadStatus(ctx, projectID, appID, limit)
}

// StartWatcher starts the file watcher for a project/app if configured.
func (s *Service) StartWatcher(ctx context.Context, projectID uuid.UUID, appID string) error {
	settings, err := s.getSettings(ctx, projectID, appID)
	if err != nil {
		return err
	}

	if settings.WatchDirectory == "" {
		return nil // No watch directory configured
	}

	return s.watcher.Watch(ctx, projectID, appID, settings.WatchDirectory)
}

// StopWatcher stops the file watcher for a project/app.
func (s *Service) StopWatcher(projectID uuid.UUID, appID string) {
	s.watcher.StopWatch(projectID, appID)
}

// Close shuts down all watchers.
func (s *Service) Close() {
	s.watcher.Close()
}

// handleFile is called by the watcher when a new file is detected.
func (s *Service) handleFile(ctx context.Context, projectID uuid.UUID, appID, filePath string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		s.logger.Error("failed to read watched file",
			zap.String("file", filePath),
			zap.Error(err),
		)
		return
	}

	fileName := filepath.Base(filePath)
	result, err := s.LoadFile(ctx, projectID, appID, fileName, data)
	if err != nil {
		s.logger.Error("failed to load watched file",
			zap.String("file", filePath),
			zap.Error(err),
		)
		return
	}

	s.logger.Info("loaded watched file",
		zap.String("file", filePath),
		zap.String("table", result.TableName),
		zap.Int("rows_loaded", result.RowsLoaded),
		zap.Int("rows_skipped", result.RowsSkipped),
	)
}

func (s *Service) parseFile(appID, fileName string, data []byte) ([]models.SheetData, error) {
	ext := strings.ToLower(filepath.Ext(fileName))

	switch {
	case ext == ".csv" || ext == ".tsv" || ext == ".txt":
		parsed, err := ParseCSV(data)
		if err != nil {
			return nil, err
		}
		return []models.SheetData{{
			SheetName: "",
			Headers:   parsed.Headers,
			Rows:      parsed.Rows,
		}}, nil

	case ext == ".xlsx" || ext == ".xlsm" || ext == ".xltx":
		return ParseXLSXFromBytes(data)

	default:
		return nil, fmt.Errorf("unsupported file type: %s", ext)
	}
}

func (s *Service) resolveTableName(fileName, sheetName string) string {
	tableName := SanitizeTableName(fileName)
	if sheetName != "" {
		sheetSuffix := sanitizeColumnName(sheetName)
		if sheetSuffix != "" && sheetSuffix != "column" {
			tableName = tableName + "_" + sheetSuffix
		}
	}
	// Truncate to 63 chars
	if len(tableName) > 63 {
		tableName = tableName[:63]
	}
	return tableName
}

func (s *Service) getSettings(ctx context.Context, projectID uuid.UUID, appID string) (*models.ETLSettings, error) {
	rawSettings, err := s.installedAppSvc.GetSettings(ctx, projectID, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app settings: %w", err)
	}

	settings := models.DefaultETLSettings()

	if v, ok := rawSettings["watch_directory"].(string); ok {
		settings.WatchDirectory = v
	}
	if v, ok := rawSettings["auto_create_tables"].(bool); ok {
		settings.AutoCreateTables = v
	}
	if v, ok := rawSettings["batch_size"].(float64); ok && v > 0 {
		settings.BatchSize = int(v)
	}
	if v, ok := rawSettings["sample_rows"].(float64); ok && v > 0 {
		settings.SampleRows = int(v)
	}
	if v, ok := rawSettings["on_conflict"].(string); ok {
		settings.OnConflict = v
	}
	if v, ok := rawSettings["use_ontology"].(bool); ok {
		settings.UseOntology = v
	}

	return &settings, nil
}

func (s *Service) getExecutor(ctx context.Context, projectID uuid.UUID) (datasource.QueryExecutor, string, error) {
	// Get the project's datasource
	datasources, err := s.datasourceService.List(ctx, projectID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list datasources: %w", err)
	}
	if len(datasources) == 0 {
		return nil, "", fmt.Errorf("no datasource configured for project")
	}

	ds := datasources[0]
	executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config, projectID, ds.ID, "etl-service")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create query executor: %w", err)
	}

	return executor, ds.DatasourceType, nil
}
