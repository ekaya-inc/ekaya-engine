package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

const projectParamSetupSteps = "setup_steps"

const (
	SetupStepDatasourceConfigured = "datasource_configured"
	SetupStepSchemaSelected       = "schema_selected"
	SetupStepAIConfigured         = "ai_configured"
	SetupStepOntologyExtracted    = "ontology_extracted"
	SetupStepQuestionsAnswered    = "questions_answered"
	SetupStepQueriesCreated       = "queries_created"
	SetupStepGlossarySetup        = "glossary_setup"
	SetupStepADLActivated         = "adl_activated"
	SetupStepAgentsQueriesCreated = "agents_queries_created"
	SetupStepTunnelActivated      = "tunnel_activated"
	SetupStepTunnelConnected      = "tunnel_connected"
)

type SetupStatus struct {
	Steps           map[string]bool `json:"steps"`
	IncompleteCount int             `json:"incomplete_count"`
	NextStep        string          `json:"next_step,omitempty"`
}

type SetupStateService interface {
	GetSetupStatus(ctx context.Context, projectID uuid.UUID) (*SetupStatus, error)
	EnsureInitialized(ctx context.Context, projectID uuid.UUID) error
	SetStepState(ctx context.Context, projectID uuid.UUID, stepID string, complete bool) error
	EnsureAppSteps(ctx context.Context, projectID uuid.UUID, appID string) error
	RemoveAppSteps(ctx context.Context, projectID uuid.UUID, appID string) error
	ReconcileStep(ctx context.Context, projectID uuid.UUID, stepID string) error
	ReconcileSteps(ctx context.Context, projectID uuid.UUID, stepIDs ...string) error
}

type SetupStateAware interface {
	SetSetupStateService(setupStateSvc SetupStateService)
}

type setupStepDefinition struct {
	ID       string
	AppID    string
	Required bool
}

type setupStateService struct {
	db               *database.DB
	datasourceSvc    DatasourceService
	schemaRepo       repositories.SchemaRepository
	aiConfigSvc      AIConfigService
	dagSvc           OntologyDAGService
	questionSvc      OntologyQuestionService
	queryRepo        repositories.QueryRepository
	glossaryRepo     repositories.GlossaryRepository
	installedAppRepo repositories.InstalledAppRepository
	logger           *zap.Logger

	tunnelConnected func(ctx context.Context, projectID uuid.UUID) bool
}

var setupStepRegistry = []setupStepDefinition{
	{ID: SetupStepDatasourceConfigured, AppID: models.AppIDMCPServer, Required: true},
	{ID: SetupStepSchemaSelected, AppID: models.AppIDOntologyForge, Required: true},
	{ID: SetupStepAIConfigured, AppID: models.AppIDOntologyForge, Required: true},
	{ID: SetupStepOntologyExtracted, AppID: models.AppIDOntologyForge, Required: true},
	{ID: SetupStepQuestionsAnswered, AppID: models.AppIDOntologyForge, Required: true},
	{ID: SetupStepQueriesCreated, AppID: models.AppIDOntologyForge, Required: false},
	{ID: SetupStepGlossarySetup, AppID: models.AppIDAIDataLiaison, Required: true},
	{ID: SetupStepADLActivated, AppID: models.AppIDAIDataLiaison, Required: true},
	{ID: SetupStepAgentsQueriesCreated, AppID: models.AppIDAIAgents, Required: true},
	{ID: SetupStepTunnelActivated, AppID: models.AppIDMCPTunnel, Required: true},
	{ID: SetupStepTunnelConnected, AppID: models.AppIDMCPTunnel, Required: true},
}

func NewSetupStateService(
	db *database.DB,
	datasourceSvc DatasourceService,
	schemaRepo repositories.SchemaRepository,
	aiConfigSvc AIConfigService,
	dagSvc OntologyDAGService,
	questionSvc OntologyQuestionService,
	queryRepo repositories.QueryRepository,
	glossaryRepo repositories.GlossaryRepository,
	installedAppRepo repositories.InstalledAppRepository,
	logger *zap.Logger,
) *setupStateService {
	return &setupStateService{
		db:               db,
		datasourceSvc:    datasourceSvc,
		schemaRepo:       schemaRepo,
		aiConfigSvc:      aiConfigSvc,
		dagSvc:           dagSvc,
		questionSvc:      questionSvc,
		queryRepo:        queryRepo,
		glossaryRepo:     glossaryRepo,
		installedAppRepo: installedAppRepo,
		logger:           logger.Named("setup-state"),
	}
}

func (s *setupStateService) SetTunnelConnectedResolver(resolver func(ctx context.Context, projectID uuid.UUID) bool) {
	s.tunnelConnected = resolver
}

func (s *setupStateService) GetSetupStatus(ctx context.Context, projectID uuid.UUID) (*SetupStatus, error) {
	if err := s.EnsureInitialized(ctx, projectID); err != nil {
		return nil, err
	}

	var status *SetupStatus
	err := s.withProjectScope(ctx, projectID, func(ctx context.Context, execer projectSettingsExecer) error {
		parameters, err := loadProjectParameters(ctx, execer, projectID)
		if err != nil {
			return err
		}

		steps, _, err := loadSetupSteps(parameters)
		if err != nil {
			return err
		}

		definitions, err := s.includedSteps(ctx, projectID)
		if err != nil {
			return err
		}

		filtered := make(map[string]bool, len(definitions))
		incompleteCount := 0
		nextRequired := ""
		nextOptional := ""

		for _, definition := range definitions {
			complete := steps[definition.ID]
			filtered[definition.ID] = complete
			if complete {
				continue
			}

			if definition.Required {
				incompleteCount++
				if nextRequired == "" {
					nextRequired = definition.ID
				}
				continue
			}

			if nextOptional == "" {
				nextOptional = definition.ID
			}
		}

		nextStep := nextRequired
		if nextStep == "" {
			nextStep = nextOptional
		}

		status = &SetupStatus{
			Steps:           filtered,
			IncompleteCount: incompleteCount,
			NextStep:        nextStep,
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return status, nil
}

func (s *setupStateService) EnsureInitialized(ctx context.Context, projectID uuid.UUID) error {
	return s.withProjectScope(ctx, projectID, func(ctx context.Context, execer projectSettingsExecer) error {
		parameters, err := loadProjectParameters(ctx, execer, projectID)
		if err != nil {
			return err
		}

		steps, present, err := loadSetupSteps(parameters)
		if err != nil {
			return err
		}

		definitions, err := s.includedSteps(ctx, projectID)
		if err != nil {
			return err
		}

		if !present {
			bootstrapped := make(map[string]bool, len(definitions))
			for _, definition := range definitions {
				complete, err := s.evaluateStep(ctx, projectID, definition.ID)
				if err != nil {
					return fmt.Errorf("bootstrap setup step %s: %w", definition.ID, err)
				}
				bootstrapped[definition.ID] = complete
			}

			parameters[projectParamSetupSteps] = bootstrapped
			return updateProjectParameters(ctx, execer, projectID, parameters)
		}

		included := make(map[string]bool, len(definitions))
		for _, definition := range definitions {
			included[definition.ID] = true
		}

		changed := false
		for stepID := range steps {
			if !included[stepID] {
				delete(steps, stepID)
				changed = true
			}
		}

		for _, definition := range definitions {
			if _, ok := steps[definition.ID]; ok {
				continue
			}
			steps[definition.ID] = false
			changed = true
		}

		if !changed {
			return nil
		}

		parameters[projectParamSetupSteps] = steps
		return updateProjectParameters(ctx, execer, projectID, parameters)
	})
}

func (s *setupStateService) SetStepState(ctx context.Context, projectID uuid.UUID, stepID string, complete bool) error {
	return s.updateStoredSteps(ctx, projectID, func(ctx context.Context, steps map[string]bool) (bool, error) {
		if _, ok := steps[stepID]; !ok {
			definitions, err := s.includedSteps(ctx, projectID)
			if err != nil {
				return false, err
			}
			included := false
			for _, definition := range definitions {
				if definition.ID == stepID {
					included = true
					break
				}
			}
			if !included {
				return false, nil
			}
		}

		if current, ok := steps[stepID]; ok && current == complete {
			return false, nil
		}

		steps[stepID] = complete
		return true, nil
	})
}

func (s *setupStateService) EnsureAppSteps(ctx context.Context, projectID uuid.UUID, appID string) error {
	return s.updateStoredSteps(ctx, projectID, func(ctx context.Context, steps map[string]bool) (bool, error) {
		changed := false
		for _, definition := range setupStepRegistry {
			if definition.AppID != appID {
				continue
			}
			if _, ok := steps[definition.ID]; ok {
				continue
			}
			complete, err := s.evaluateStep(ctx, projectID, definition.ID)
			if err != nil {
				return false, err
			}
			steps[definition.ID] = complete
			changed = true
		}
		return changed, nil
	})
}

func (s *setupStateService) RemoveAppSteps(ctx context.Context, projectID uuid.UUID, appID string) error {
	return s.updateStoredSteps(ctx, projectID, func(_ context.Context, steps map[string]bool) (bool, error) {
		changed := false
		for _, definition := range setupStepRegistry {
			if definition.AppID != appID {
				continue
			}
			if _, ok := steps[definition.ID]; !ok {
				continue
			}
			delete(steps, definition.ID)
			changed = true
		}
		return changed, nil
	})
}

func (s *setupStateService) ReconcileStep(ctx context.Context, projectID uuid.UUID, stepID string) error {
	return s.updateStoredSteps(ctx, projectID, func(ctx context.Context, steps map[string]bool) (bool, error) {
		if _, ok := steps[stepID]; !ok {
			return false, nil
		}

		complete, err := s.evaluateStep(ctx, projectID, stepID)
		if err != nil {
			return false, err
		}
		if steps[stepID] == complete {
			return false, nil
		}
		steps[stepID] = complete
		return true, nil
	})
}

func (s *setupStateService) ReconcileSteps(ctx context.Context, projectID uuid.UUID, stepIDs ...string) error {
	for _, stepID := range stepIDs {
		if err := s.ReconcileStep(ctx, projectID, stepID); err != nil {
			return err
		}
	}
	return nil
}

func (s *setupStateService) updateStoredSteps(
	ctx context.Context,
	projectID uuid.UUID,
	update func(ctx context.Context, steps map[string]bool) (bool, error),
) error {
	return s.withProjectScope(ctx, projectID, func(ctx context.Context, execer projectSettingsExecer) error {
		parameters, err := loadProjectParameters(ctx, execer, projectID)
		if err != nil {
			return err
		}

		steps, present, err := loadSetupSteps(parameters)
		if err != nil {
			return err
		}
		if !present {
			return nil
		}

		changed, err := update(ctx, steps)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}

		parameters[projectParamSetupSteps] = steps
		return updateProjectParameters(ctx, execer, projectID, parameters)
	})
}

func (s *setupStateService) evaluateStep(ctx context.Context, projectID uuid.UUID, stepID string) (bool, error) {
	switch stepID {
	case SetupStepDatasourceConfigured:
		datasources, err := s.datasourceSvc.List(ctx, projectID)
		if err != nil {
			return false, err
		}
		for _, datasource := range datasources {
			if !datasource.DecryptionFailed {
				return true, nil
			}
		}
		return false, nil

	case SetupStepSchemaSelected:
		tableNames, err := s.schemaRepo.GetSelectedTableNamesByProject(ctx, projectID)
		if err != nil {
			return false, err
		}
		return len(tableNames) > 0, nil

	case SetupStepAIConfigured:
		config, err := s.aiConfigSvc.Get(ctx, projectID)
		if err != nil {
			return false, err
		}
		return config != nil && config.ConfigType != models.AIConfigNone, nil

	case SetupStepOntologyExtracted:
		datasourceID, ok, err := s.firstUsableDatasourceID(ctx, projectID)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}

		status, err := s.dagSvc.GetOntologyStatus(ctx, projectID, datasourceID)
		if err != nil {
			return false, err
		}
		return status != nil && status.HasOntology, nil

	case SetupStepQuestionsAnswered:
		ontologyExtracted, err := s.evaluateStep(ctx, projectID, SetupStepOntologyExtracted)
		if err != nil {
			return false, err
		}
		if !ontologyExtracted {
			return false, nil
		}

		counts, err := s.questionSvc.GetPendingCounts(ctx, projectID)
		if err != nil {
			return false, err
		}
		if counts == nil {
			return true, nil
		}
		return counts.Required == 0, nil

	case SetupStepQueriesCreated, SetupStepAgentsQueriesCreated:
		return s.queryRepo.HasApprovedEnabledQueriesByProject(ctx, projectID)

	case SetupStepGlossarySetup:
		terms, err := s.glossaryRepo.GetByProject(ctx, projectID)
		if err != nil {
			return false, err
		}
		return len(terms) > 0, nil

	case SetupStepADLActivated:
		return s.isAppActivated(ctx, projectID, models.AppIDAIDataLiaison)

	case SetupStepTunnelActivated:
		return s.isAppActivated(ctx, projectID, models.AppIDMCPTunnel)

	case SetupStepTunnelConnected:
		if s.tunnelConnected == nil {
			return false, nil
		}
		return s.tunnelConnected(ctx, projectID), nil

	default:
		return false, fmt.Errorf("unknown setup step: %s", stepID)
	}
}

func (s *setupStateService) includedSteps(ctx context.Context, projectID uuid.UUID) ([]setupStepDefinition, error) {
	apps, err := s.installedAppRepo.List(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list installed apps for setup steps: %w", err)
	}

	installed := map[string]bool{
		models.AppIDMCPServer: true,
	}
	for _, app := range apps {
		installed[app.AppID] = true
	}

	steps := make([]setupStepDefinition, 0, len(setupStepRegistry))
	for _, definition := range setupStepRegistry {
		if installed[definition.AppID] {
			steps = append(steps, definition)
		}
	}

	return steps, nil
}

func (s *setupStateService) firstUsableDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, bool, error) {
	datasources, err := s.datasourceSvc.List(ctx, projectID)
	if err != nil {
		return uuid.Nil, false, err
	}
	for _, datasource := range datasources {
		if !datasource.DecryptionFailed {
			return datasource.ID, true, nil
		}
	}
	return uuid.Nil, false, nil
}

func (s *setupStateService) isAppActivated(ctx context.Context, projectID uuid.UUID, appID string) (bool, error) {
	app, err := s.installedAppRepo.Get(ctx, projectID, appID)
	if err != nil {
		return false, err
	}
	return app != nil && app.ActivatedAt != nil, nil
}

func (s *setupStateService) withProjectScope(
	ctx context.Context,
	projectID uuid.UUID,
	fn func(context.Context, projectSettingsExecer) error,
) error {
	if scope, ok := database.GetTenantScope(ctx); ok {
		return fn(ctx, scope.Conn)
	}

	scope, err := s.db.WithTenant(ctx, projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant scope for setup state: %w", err)
	}
	defer scope.Close()

	scopedCtx := database.SetTenantScope(ctx, scope)
	return fn(scopedCtx, scope.Conn)
}

func loadSetupSteps(parameters map[string]any) (map[string]bool, bool, error) {
	rawSteps, ok := parameters[projectParamSetupSteps]
	if !ok || rawSteps == nil {
		return map[string]bool{}, false, nil
	}

	payload, err := json.Marshal(rawSteps)
	if err != nil {
		return nil, true, fmt.Errorf("encode setup steps: %w", err)
	}

	var steps map[string]bool
	if err := json.Unmarshal(payload, &steps); err != nil {
		return nil, true, fmt.Errorf("decode setup steps: %w", err)
	}
	if steps == nil {
		return map[string]bool{}, true, nil
	}

	return steps, true, nil
}

var _ SetupStateService = (*setupStateService)(nil)
