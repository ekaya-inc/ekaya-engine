package services

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

const projectOverviewFactType = "project_overview"

type projectKnowledgePromptFactsKey struct{}

var promptKnowledgeTypeOrder = map[string]int{
	models.FactTypeFiscalYear:   0,
	models.FactTypeBusinessRule: 1,
	models.FactTypeTerminology:  2,
	models.FactTypeEnumeration:  3,
	models.FactTypeConvention:   4,
	models.FactTypeRelationship: 5,
}

func withProjectKnowledgeFactsForPrompt(ctx context.Context, facts []*models.KnowledgeFact) context.Context {
	if len(facts) == 0 {
		return context.WithValue(ctx, projectKnowledgePromptFactsKey{}, []*models.KnowledgeFact{})
	}
	return context.WithValue(ctx, projectKnowledgePromptFactsKey{}, facts)
}

func withLoadedProjectKnowledgeFactsForPrompt(
	ctx context.Context,
	projectID uuid.UUID,
	logger *zap.Logger,
) context.Context {
	if _, ok := ctx.Value(projectKnowledgePromptFactsKey{}).([]*models.KnowledgeFact); ok {
		return ctx
	}
	if _, ok := database.GetTenantScope(ctx); !ok {
		return ctx
	}

	facts, err := repositories.NewKnowledgeRepository().GetByProject(ctx, projectID)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to load project knowledge for prompt context",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
		}
		return ctx
	}

	return withProjectKnowledgeFactsForPrompt(ctx, facts)
}

func buildRelevantProjectKnowledgeSection(
	ctx context.Context,
	projectID uuid.UUID,
	logger *zap.Logger,
) string {
	if facts, ok := ctx.Value(projectKnowledgePromptFactsKey{}).([]*models.KnowledgeFact); ok {
		return formatProjectKnowledgeForPrompt(facts)
	}
	if _, ok := database.GetTenantScope(ctx); !ok {
		return ""
	}

	facts, err := repositories.NewKnowledgeRepository().GetByProject(ctx, projectID)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to load project knowledge for prompt",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
		}
		return ""
	}

	return formatProjectKnowledgeForPrompt(facts)
}

func prependProjectKnowledgeToPrompt(prompt, knowledgeSection string) string {
	if knowledgeSection == "" {
		return prompt
	}
	return knowledgeSection + "\n\n" + prompt
}

func formatProjectKnowledgeForPrompt(facts []*models.KnowledgeFact) string {
	filtered := filterProjectKnowledgeFactsForPrompt(facts)
	if len(filtered) == 0 {
		return ""
	}

	sort.Slice(filtered, func(i, j int) bool {
		left := projectKnowledgeTypeSortOrder(filtered[i].FactType)
		right := projectKnowledgeTypeSortOrder(filtered[j].FactType)
		if left != right {
			return left < right
		}
		if strings.EqualFold(filtered[i].FactType, filtered[j].FactType) {
			return strings.ToLower(filtered[i].Value) < strings.ToLower(filtered[j].Value)
		}
		return strings.ToLower(filtered[i].FactType) < strings.ToLower(filtered[j].FactType)
	})

	var sb strings.Builder
	sb.WriteString("## Relevant Project Knowledge\n\n")

	currentType := ""
	for _, fact := range filtered {
		factType := strings.ToLower(strings.TrimSpace(fact.FactType))
		if factType != currentType {
			if currentType != "" {
				sb.WriteString("\n")
			}
			sb.WriteString("### ")
			sb.WriteString(capitalizeWords(strings.ReplaceAll(factType, "_", " ")))
			sb.WriteString("\n")
			currentType = factType
		}

		sb.WriteString("- ")
		sb.WriteString(strings.TrimSpace(fact.Value))
		if strings.TrimSpace(fact.Context) != "" {
			sb.WriteString(" (")
			sb.WriteString(strings.TrimSpace(fact.Context))
			sb.WriteString(")")
		}
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}

func filterProjectKnowledgeFactsForPrompt(facts []*models.KnowledgeFact) []*models.KnowledgeFact {
	filtered := make([]*models.KnowledgeFact, 0, len(facts))
	seen := make(map[string]struct{}, len(facts))

	for _, fact := range facts {
		if fact == nil {
			continue
		}
		factType := strings.ToLower(strings.TrimSpace(fact.FactType))
		value := strings.TrimSpace(fact.Value)
		if factType == "" || value == "" || factType == projectOverviewFactType {
			continue
		}

		contextText := strings.TrimSpace(fact.Context)
		key := factType + "\x00" + normalizeProjectKnowledgePromptText(value) + "\x00" + normalizeProjectKnowledgePromptText(contextText)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		filtered = append(filtered, &models.KnowledgeFact{
			FactType: factType,
			Value:    value,
			Context:  contextText,
		})
	}

	return filtered
}

func normalizeProjectKnowledgePromptText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func projectKnowledgeTypeSortOrder(factType string) int {
	if order, ok := promptKnowledgeTypeOrder[strings.ToLower(strings.TrimSpace(factType))]; ok {
		return order
	}
	return len(promptKnowledgeTypeOrder) + 1
}
