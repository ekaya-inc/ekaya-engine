package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

const (
	ontologyExportDatasourceKey    = "primary"
	ontologyExportQuestionPageSize = 500
)

var nonFilenameChars = regexp.MustCompile(`[^a-z0-9]+`)

// OntologyExportService assembles portable ontology export bundles.
type OntologyExportService interface {
	BuildBundle(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyExportBundle, error)
	MarshalBundle(bundle *models.OntologyExportBundle) ([]byte, error)
	SuggestedFilename(bundle *models.OntologyExportBundle) string
}

type ontologyExportProjectRepository interface {
	Get(ctx context.Context, id uuid.UUID) (*models.Project, error)
}

type ontologyExportDatasourceService interface {
	Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error)
}

type ontologyExportSchemaRepository interface {
	ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error)
	ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error)
	ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error)
}

type ontologyExportTableMetadataRepository interface {
	List(ctx context.Context, projectID uuid.UUID) ([]*models.TableMetadata, error)
}

type ontologyExportColumnMetadataRepository interface {
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error)
}

type ontologyExportQuestionRepository interface {
	List(ctx context.Context, projectID uuid.UUID, filters repositories.QuestionListFilters) (*repositories.QuestionListResult, error)
}

type ontologyExportKnowledgeRepository interface {
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error)
}

type ontologyExportGlossaryRepository interface {
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)
}

type ontologyExportQueryRepository interface {
	ListByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error)
}

type ontologyExportService struct {
	projectRepo        ontologyExportProjectRepository
	datasourceService  ontologyExportDatasourceService
	schemaRepo         ontologyExportSchemaRepository
	tableMetadataRepo  ontologyExportTableMetadataRepository
	columnMetadataRepo ontologyExportColumnMetadataRepository
	questionRepo       ontologyExportQuestionRepository
	knowledgeRepo      ontologyExportKnowledgeRepository
	glossaryRepo       ontologyExportGlossaryRepository
	queryRepo          ontologyExportQueryRepository
	logger             *zap.Logger
}

type schemaRefResolver struct {
	tableByID     map[uuid.UUID]models.OntologyExportTableRef
	columnByID    map[uuid.UUID]models.OntologyExportColumnRef
	tableByName   map[string][]models.OntologyExportTableRef
	columnByValue map[string][]models.OntologyExportColumnRef
}

// NewOntologyExportService creates a new ontology export service.
func NewOntologyExportService(
	projectRepo ontologyExportProjectRepository,
	datasourceService ontologyExportDatasourceService,
	schemaRepo ontologyExportSchemaRepository,
	tableMetadataRepo ontologyExportTableMetadataRepository,
	columnMetadataRepo ontologyExportColumnMetadataRepository,
	questionRepo ontologyExportQuestionRepository,
	knowledgeRepo ontologyExportKnowledgeRepository,
	glossaryRepo ontologyExportGlossaryRepository,
	queryRepo ontologyExportQueryRepository,
	logger *zap.Logger,
) OntologyExportService {
	return &ontologyExportService{
		projectRepo:        projectRepo,
		datasourceService:  datasourceService,
		schemaRepo:         schemaRepo,
		tableMetadataRepo:  tableMetadataRepo,
		columnMetadataRepo: columnMetadataRepo,
		questionRepo:       questionRepo,
		knowledgeRepo:      knowledgeRepo,
		glossaryRepo:       glossaryRepo,
		queryRepo:          queryRepo,
		logger:             logger,
	}
}

func (s *ontologyExportService) BuildBundle(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyExportBundle, error) {
	project, err := s.projectRepo.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}

	ds, err := s.datasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("load datasource: %w", err)
	}

	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("load schema tables: %w", err)
	}

	columns, err := s.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("load schema columns: %w", err)
	}

	relationships, err := s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("load schema relationships: %w", err)
	}

	selectedTables := filterSelectedTables(tables)
	selectedColumns := filterSelectedColumns(columns, selectedTables)
	resolver := buildSchemaRefResolver(selectedTables, selectedColumns)

	tableMetadata, err := s.tableMetadataRepo.List(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load table metadata: %w", err)
	}

	columnMetadata, err := s.columnMetadataRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load column metadata: %w", err)
	}

	questions, err := s.loadAllQuestions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load ontology questions: %w", err)
	}

	knowledgeFacts, err := s.knowledgeRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load project knowledge: %w", err)
	}

	glossaryTerms, err := s.glossaryRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load glossary terms: %w", err)
	}

	queries, err := s.queryRepo.ListByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("load queries: %w", err)
	}

	exportedQueries := buildApprovedQueryExports(queries)

	bundle := &models.OntologyExportBundle{
		Format:       models.OntologyExportFormat,
		Version:      models.OntologyExportVersion,
		ExportedAt:   time.Now().UTC(),
		RequiredApps: buildRequiredApps(exportedQueries),
		Project: models.OntologyExportProject{
			Name:          project.Name,
			IndustryType:  project.IndustryType,
			DomainSummary: project.DomainSummary,
		},
		Datasources: []models.OntologyExportDatasource{
			{
				Key:            ontologyExportDatasourceKey,
				Name:           ds.Name,
				DatasourceType: ds.DatasourceType,
				Provider:       ds.Provider,
				Config:         sanitizeDatasourceConfig(ds.Config),
				SelectedSchema: models.OntologyExportSelectedSchema{
					Tables:        buildExportTables(selectedTables, selectedColumns),
					Relationships: buildExportRelationships(relationships, resolver),
				},
			},
		},
		Ontology: models.OntologyExportOntology{
			TableMetadata:    buildExportTableMetadata(tableMetadata, resolver),
			ColumnMetadata:   buildExportColumnMetadata(columnMetadata, resolver),
			Questions:        buildExportQuestions(questions, resolver),
			ProjectKnowledge: buildExportKnowledge(knowledgeFacts),
			GlossaryTerms:    buildExportGlossaryTerms(glossaryTerms),
		},
		ApprovedQueries: exportedQueries,
		Security: models.OntologyExportSecurity{
			IncludesDatasourceCredentials: false,
			IncludesAIConfig:              false,
			IncludesAgentAPIKeys:          false,
		},
	}

	return bundle, nil
}

func (s *ontologyExportService) MarshalBundle(bundle *models.OntologyExportBundle) ([]byte, error) {
	if bundle == nil {
		return nil, fmt.Errorf("bundle is required")
	}
	return json.MarshalIndent(bundle, "", "  ")
}

func (s *ontologyExportService) SuggestedFilename(bundle *models.OntologyExportBundle) string {
	if bundle == nil {
		return "ontology-export.json"
	}

	slug := slugifyFilename(bundle.Project.Name)
	if slug == "" {
		slug = "ontology"
	}

	return fmt.Sprintf("%s-export.json", slug)
}

func (s *ontologyExportService) loadAllQuestions(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error) {
	var result []*models.OntologyQuestion
	offset := 0

	for {
		page, err := s.questionRepo.List(ctx, projectID, repositories.QuestionListFilters{
			Limit:  ontologyExportQuestionPageSize,
			Offset: offset,
		})
		if err != nil {
			return nil, err
		}

		result = append(result, page.Questions...)
		offset += len(page.Questions)
		if offset >= page.TotalCount || len(page.Questions) == 0 {
			break
		}
	}

	return result, nil
}

func filterSelectedTables(tables []*models.SchemaTable) map[uuid.UUID]*models.SchemaTable {
	selected := make(map[uuid.UUID]*models.SchemaTable)
	for _, table := range tables {
		if table == nil || !table.IsSelected {
			continue
		}
		selected[table.ID] = table
	}
	return selected
}

func filterSelectedColumns(columns []*models.SchemaColumn, selectedTables map[uuid.UUID]*models.SchemaTable) map[uuid.UUID]*models.SchemaColumn {
	selected := make(map[uuid.UUID]*models.SchemaColumn)
	for _, column := range columns {
		if column == nil || !column.IsSelected {
			continue
		}
		if _, ok := selectedTables[column.SchemaTableID]; !ok {
			continue
		}
		selected[column.ID] = column
	}
	return selected
}

func buildSchemaRefResolver(
	selectedTables map[uuid.UUID]*models.SchemaTable,
	selectedColumns map[uuid.UUID]*models.SchemaColumn,
) schemaRefResolver {
	resolver := schemaRefResolver{
		tableByID:     make(map[uuid.UUID]models.OntologyExportTableRef, len(selectedTables)),
		columnByID:    make(map[uuid.UUID]models.OntologyExportColumnRef, len(selectedColumns)),
		tableByName:   make(map[string][]models.OntologyExportTableRef),
		columnByValue: make(map[string][]models.OntologyExportColumnRef),
	}

	for id, table := range selectedTables {
		ref := models.OntologyExportTableRef{
			SchemaName: table.SchemaName,
			TableName:  table.TableName,
		}
		resolver.tableByID[id] = ref
		resolver.tableByName[table.TableName] = append(resolver.tableByName[table.TableName], ref)
	}

	for id, column := range selectedColumns {
		tableRef, ok := resolver.tableByID[column.SchemaTableID]
		if !ok {
			continue
		}
		ref := models.OntologyExportColumnRef{
			Table:      tableRef,
			ColumnName: column.ColumnName,
		}
		resolver.columnByID[id] = ref
		key := tableRef.TableName + "." + column.ColumnName
		resolver.columnByValue[key] = append(resolver.columnByValue[key], ref)
	}

	for name := range resolver.tableByName {
		sort.Slice(resolver.tableByName[name], func(i, j int) bool {
			return compareTableRefs(resolver.tableByName[name][i], resolver.tableByName[name][j]) < 0
		})
	}
	for key := range resolver.columnByValue {
		sort.Slice(resolver.columnByValue[key], func(i, j int) bool {
			return compareColumnRefs(resolver.columnByValue[key][i], resolver.columnByValue[key][j]) < 0
		})
	}

	return resolver
}

func buildExportTables(
	selectedTables map[uuid.UUID]*models.SchemaTable,
	selectedColumns map[uuid.UUID]*models.SchemaColumn,
) []models.OntologyExportTable {
	orderedTables := make([]*models.SchemaTable, 0, len(selectedTables))
	for _, table := range selectedTables {
		orderedTables = append(orderedTables, table)
	}
	sort.Slice(orderedTables, func(i, j int) bool {
		if orderedTables[i].SchemaName != orderedTables[j].SchemaName {
			return orderedTables[i].SchemaName < orderedTables[j].SchemaName
		}
		return orderedTables[i].TableName < orderedTables[j].TableName
	})

	columnsByTableID := make(map[uuid.UUID][]*models.SchemaColumn)
	for _, column := range selectedColumns {
		columnsByTableID[column.SchemaTableID] = append(columnsByTableID[column.SchemaTableID], column)
	}

	exported := make([]models.OntologyExportTable, 0, len(orderedTables))
	for _, table := range orderedTables {
		tableColumns := columnsByTableID[table.ID]
		sort.Slice(tableColumns, func(i, j int) bool {
			if tableColumns[i].OrdinalPosition != tableColumns[j].OrdinalPosition {
				return tableColumns[i].OrdinalPosition < tableColumns[j].OrdinalPosition
			}
			return tableColumns[i].ColumnName < tableColumns[j].ColumnName
		})

		columns := make([]models.OntologyExportColumn, 0, len(tableColumns))
		for _, column := range tableColumns {
			enumValues := append([]string(nil), column.EnumValues...)
			sort.Strings(enumValues)

			columns = append(columns, models.OntologyExportColumn{
				ColumnName:        column.ColumnName,
				DataType:          column.DataType,
				IsNullable:        column.IsNullable,
				IsPrimaryKey:      column.IsPrimaryKey,
				IsUnique:          column.IsUnique,
				OrdinalPosition:   column.OrdinalPosition,
				DefaultValue:      column.DefaultValue,
				DistinctCount:     column.DistinctCount,
				NullCount:         column.NullCount,
				MinLength:         column.MinLength,
				MaxLength:         column.MaxLength,
				EnumValues:        enumValues,
				RowCount:          column.RowCount,
				NonNullCount:      column.NonNullCount,
				IsJoinable:        column.IsJoinable,
				JoinabilityReason: column.JoinabilityReason,
			})
		}

		exported = append(exported, models.OntologyExportTable{
			SchemaName: table.SchemaName,
			TableName:  table.TableName,
			RowCount:   table.RowCount,
			Columns:    columns,
		})
	}

	return exported
}

func buildExportRelationships(
	relationships []*models.SchemaRelationship,
	resolver schemaRefResolver,
) []models.OntologyExportRelationship {
	exported := make([]models.OntologyExportRelationship, 0)
	for _, rel := range relationships {
		if rel == nil {
			continue
		}

		sourceRef, sourceOK := resolver.columnByID[rel.SourceColumnID]
		targetRef, targetOK := resolver.columnByID[rel.TargetColumnID]
		if !sourceOK || !targetOK {
			continue
		}

		exported = append(exported, models.OntologyExportRelationship{
			Source:           sourceRef,
			Target:           targetRef,
			RelationshipType: rel.RelationshipType,
			Cardinality:      rel.Cardinality,
			Confidence:       rel.Confidence,
			InferenceMethod:  rel.InferenceMethod,
			IsValidated:      rel.IsValidated,
			Validation:       rel.ValidationResults,
			IsApproved:       rel.IsApproved,
		})
	}

	sort.Slice(exported, func(i, j int) bool {
		if cmp := compareColumnRefs(exported[i].Source, exported[j].Source); cmp != 0 {
			return cmp < 0
		}
		return compareColumnRefs(exported[i].Target, exported[j].Target) < 0
	})

	return exported
}

func buildExportTableMetadata(
	metadata []*models.TableMetadata,
	resolver schemaRefResolver,
) []models.OntologyExportTableMetadata {
	exported := make([]models.OntologyExportTableMetadata, 0)
	for _, item := range metadata {
		if item == nil {
			continue
		}
		tableRef, ok := resolver.tableByID[item.SchemaTableID]
		if !ok {
			continue
		}

		exported = append(exported, models.OntologyExportTableMetadata{
			Table:                tableRef,
			TableType:            item.TableType,
			Description:          item.Description,
			UsageNotes:           item.UsageNotes,
			IsEphemeral:          item.IsEphemeral,
			PreferredAlternative: resolver.tableRefByName(ptrString(item.PreferredAlternative)),
			Confidence:           item.Confidence,
			Features:             item.Features,
			Source:               item.Source,
			LastEditSource:       item.LastEditSource,
		})
	}

	sort.Slice(exported, func(i, j int) bool {
		return compareTableRefs(exported[i].Table, exported[j].Table) < 0
	})

	return exported
}

func buildExportColumnMetadata(
	metadata []*models.ColumnMetadata,
	resolver schemaRefResolver,
) []models.OntologyExportColumnMetadata {
	exported := make([]models.OntologyExportColumnMetadata, 0)
	for _, item := range metadata {
		if item == nil {
			continue
		}
		columnRef, ok := resolver.columnByID[item.SchemaColumnID]
		if !ok {
			continue
		}

		exported = append(exported, models.OntologyExportColumnMetadata{
			Column:                columnRef,
			ClassificationPath:    item.ClassificationPath,
			Purpose:               item.Purpose,
			SemanticType:          item.SemanticType,
			Role:                  item.Role,
			Description:           item.Description,
			Confidence:            item.Confidence,
			Features:              item.Features,
			NeedsEnumAnalysis:     item.NeedsEnumAnalysis,
			NeedsFKResolution:     item.NeedsFKResolution,
			NeedsCrossColumnCheck: item.NeedsCrossColumnCheck,
			NeedsClarification:    item.NeedsClarification,
			ClarificationQuestion: item.ClarificationQuestion,
			IsSensitive:           item.IsSensitive,
			Source:                item.Source,
			LastEditSource:        item.LastEditSource,
		})
	}

	sort.Slice(exported, func(i, j int) bool {
		return compareColumnRefs(exported[i].Column, exported[j].Column) < 0
	})

	return exported
}

func buildExportQuestions(
	questions []*models.OntologyQuestion,
	resolver schemaRefResolver,
) []models.OntologyExportQuestion {
	exported := make([]models.OntologyExportQuestion, 0, len(questions))
	for _, question := range questions {
		if question == nil || question.Status == models.QuestionStatusDeleted {
			continue
		}

		var affects *models.OntologyExportQuestionAffects
		if question.Affects != nil {
			tableRefs := make([]models.OntologyExportTableRef, 0, len(question.Affects.Tables))
			for _, tableName := range question.Affects.Tables {
				if ref := resolver.tableRefByName(tableName); ref != nil {
					tableRefs = append(tableRefs, *ref)
				}
			}
			sort.Slice(tableRefs, func(i, j int) bool {
				return compareTableRefs(tableRefs[i], tableRefs[j]) < 0
			})

			columnRefs := make([]models.OntologyExportColumnRef, 0, len(question.Affects.Columns))
			for _, value := range question.Affects.Columns {
				if ref := resolver.columnRefByValue(value); ref != nil {
					columnRefs = append(columnRefs, *ref)
				}
			}
			sort.Slice(columnRefs, func(i, j int) bool {
				return compareColumnRefs(columnRefs[i], columnRefs[j]) < 0
			})

			if len(tableRefs) > 0 || len(columnRefs) > 0 {
				affects = &models.OntologyExportQuestionAffects{
					Tables:  tableRefs,
					Columns: columnRefs,
				}
			}
		}

		exported = append(exported, models.OntologyExportQuestion{
			Text:            question.Text,
			Priority:        question.Priority,
			IsRequired:      question.IsRequired,
			Category:        question.Category,
			Reasoning:       question.Reasoning,
			Affects:         affects,
			DetectedPattern: question.DetectedPattern,
			Status:          question.Status,
			StatusReason:    question.StatusReason,
			Answer:          question.Answer,
		})
	}

	sort.Slice(exported, func(i, j int) bool {
		if exported[i].Priority != exported[j].Priority {
			return exported[i].Priority < exported[j].Priority
		}
		return exported[i].Text < exported[j].Text
	})

	return exported
}

func buildExportKnowledge(facts []*models.KnowledgeFact) []models.OntologyExportKnowledgeFact {
	exported := make([]models.OntologyExportKnowledgeFact, 0, len(facts))
	for _, fact := range facts {
		if fact == nil {
			continue
		}
		exported = append(exported, models.OntologyExportKnowledgeFact{
			FactType:       fact.FactType,
			Value:          fact.Value,
			Context:        fact.Context,
			Source:         fact.Source,
			LastEditSource: fact.LastEditSource,
		})
	}

	sort.Slice(exported, func(i, j int) bool {
		if exported[i].FactType != exported[j].FactType {
			return exported[i].FactType < exported[j].FactType
		}
		return exported[i].Value < exported[j].Value
	})

	return exported
}

func buildExportGlossaryTerms(terms []*models.BusinessGlossaryTerm) []models.OntologyExportGlossaryTerm {
	exported := make([]models.OntologyExportGlossaryTerm, 0, len(terms))
	for _, term := range terms {
		if term == nil {
			continue
		}

		aliases := append([]string(nil), term.Aliases...)
		sort.Strings(aliases)

		outputColumns := append([]models.OutputColumn(nil), term.OutputColumns...)
		sort.Slice(outputColumns, func(i, j int) bool {
			return outputColumns[i].Name < outputColumns[j].Name
		})

		exported = append(exported, models.OntologyExportGlossaryTerm{
			Term:             term.Term,
			Definition:       term.Definition,
			DefiningSQL:      term.DefiningSQL,
			BaseTable:        term.BaseTable,
			OutputColumns:    outputColumns,
			Aliases:          aliases,
			EnrichmentStatus: term.EnrichmentStatus,
			EnrichmentError:  term.EnrichmentError,
			NeedsReview:      term.NeedsReview,
			ReviewReason:     term.ReviewReason,
			Source:           term.Source,
			LastEditSource:   term.LastEditSource,
		})
	}

	sort.Slice(exported, func(i, j int) bool {
		return exported[i].Term < exported[j].Term
	})

	return exported
}

func buildApprovedQueryExports(queries []*models.Query) []models.OntologyExportApprovedQuery {
	approved := make([]*models.Query, 0, len(queries))
	for _, query := range queries {
		if query == nil || query.Status != "approved" {
			continue
		}
		approved = append(approved, query)
	}

	sort.Slice(approved, func(i, j int) bool {
		if approved[i].NaturalLanguagePrompt != approved[j].NaturalLanguagePrompt {
			return approved[i].NaturalLanguagePrompt < approved[j].NaturalLanguagePrompt
		}
		if approved[i].SQLQuery != approved[j].SQLQuery {
			return approved[i].SQLQuery < approved[j].SQLQuery
		}
		return approved[i].ID.String() < approved[j].ID.String()
	})

	exported := make([]models.OntologyExportApprovedQuery, 0, len(approved))
	for index, query := range approved {
		key := fmt.Sprintf("query_%03d", index+1)

		parameters := append([]models.QueryParameter(nil), query.Parameters...)
		sort.Slice(parameters, func(i, j int) bool {
			return parameters[i].Name < parameters[j].Name
		})

		outputColumns := append([]models.OutputColumn(nil), query.OutputColumns...)
		sort.Slice(outputColumns, func(i, j int) bool {
			return outputColumns[i].Name < outputColumns[j].Name
		})

		tags := append([]string(nil), query.Tags...)
		sort.Strings(tags)

		exported = append(exported, models.OntologyExportApprovedQuery{
			Key:                   key,
			DatasourceKey:         ontologyExportDatasourceKey,
			NaturalLanguagePrompt: query.NaturalLanguagePrompt,
			AdditionalContext:     query.AdditionalContext,
			SQL:                   query.SQLQuery,
			Dialect:               query.Dialect,
			Enabled:               query.IsEnabled,
			Parameters:            parameters,
			OutputColumns:         outputColumns,
			Constraints:           query.Constraints,
			Tags:                  tags,
			AllowsModification:    query.AllowsModification,
		})
	}

	return exported
}

func buildRequiredApps(queries []models.OntologyExportApprovedQuery) []string {
	apps := []string{models.AppIDOntologyForge}
	if len(queries) > 0 {
		apps = append(apps, models.AppIDAIDataLiaison)
	}
	return apps
}

func sanitizeDatasourceConfig(config map[string]any) map[string]any {
	if len(config) == 0 {
		return map[string]any{}
	}

	sanitized, ok := sanitizeValue(config).(map[string]any)
	if !ok || sanitized == nil {
		return map[string]any{}
	}
	return sanitized
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		sanitized := make(map[string]any)
		for key, nested := range typed {
			if isSensitiveDatasourceKey(key) {
				continue
			}
			sanitizedNested := sanitizeValue(nested)
			if shouldOmitSanitizedValue(sanitizedNested) {
				continue
			}
			sanitized[key] = sanitizedNested
		}
		return sanitized
	case []any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			sanitizedItem := sanitizeValue(item)
			if shouldOmitSanitizedValue(sanitizedItem) {
				continue
			}
			items = append(items, sanitizedItem)
		}
		return items
	default:
		return value
	}
}

func shouldOmitSanitizedValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case map[string]any:
		return len(typed) == 0
	case []any:
		return false
	default:
		return false
	}
}

func isSensitiveDatasourceKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")

	switch normalized {
	case "user", "username", "password", "pass", "passwd", "pwd",
		"url", "uri", "dsn", "connection_string", "connectionstring",
		"client_secret", "private_key", "service_account_json":
		return true
	}

	sensitiveFragments := []string{
		"password",
		"secret",
		"token",
		"credential",
		"api_key",
		"apikey",
		"private_key",
		"client_secret",
		"connection_string",
	}
	for _, fragment := range sensitiveFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}

	return false
}

func slugifyFilename(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = nonFilenameChars.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

func compareTableRefs(left, right models.OntologyExportTableRef) int {
	if left.SchemaName != right.SchemaName {
		if left.SchemaName < right.SchemaName {
			return -1
		}
		return 1
	}
	if left.TableName != right.TableName {
		if left.TableName < right.TableName {
			return -1
		}
		return 1
	}
	return 0
}

func compareColumnRefs(left, right models.OntologyExportColumnRef) int {
	if cmp := compareTableRefs(left.Table, right.Table); cmp != 0 {
		return cmp
	}
	if left.ColumnName < right.ColumnName {
		return -1
	}
	if left.ColumnName > right.ColumnName {
		return 1
	}
	return 0
}

func (r schemaRefResolver) tableRefByName(tableName string) *models.OntologyExportTableRef {
	if tableName == "" {
		return nil
	}
	refs := r.tableByName[tableName]
	if len(refs) == 1 {
		ref := refs[0]
		return &ref
	}
	ref := models.OntologyExportTableRef{TableName: tableName}
	return &ref
}

func (r schemaRefResolver) columnRefByValue(value string) *models.OntologyExportColumnRef {
	tableName, columnName, found := strings.Cut(value, ".")
	if !found {
		return nil
	}
	refs := r.columnByValue[tableName+"."+columnName]
	if len(refs) == 1 {
		ref := refs[0]
		return &ref
	}
	ref := models.OntologyExportColumnRef{
		Table: models.OntologyExportTableRef{
			TableName: tableName,
		},
		ColumnName: columnName,
	}
	return &ref
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
