package services

import (
	"fmt"
	"strings"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// PromotionThreshold is the minimum score required for a table to be promoted to entity status.
// Tables scoring below this threshold are demoted (filtered from default context views).
const PromotionThreshold = 50

// Point values for promotion criteria
const (
	pointsHubMajor       = 30 // 5+ inbound references
	pointsHubMinor       = 20 // 3+ inbound references
	pointsMultipleRoles  = 25 // 2+ distinct roles reference this table
	pointsRelatedTables  = 20 // multiple tables share this concept
	pointsBusinessAlias  = 15 // has business aliases from LLM
	pointsOutboundMinor  = 10 // 3+ outbound relationships
)

// PromotionResult contains the score and reasons for a table's promotion evaluation.
type PromotionResult struct {
	Score   int      `json:"score"`
	Reasons []string `json:"reasons"`
}

// PromotionInput contains the data needed to evaluate a table for entity promotion.
// This allows flexibility in what data is provided (full entities vs just aliases).
type PromotionInput struct {
	// TableName is the name of the table being evaluated (required)
	TableName string
	// SchemaName is the schema containing the table (required)
	SchemaName string
	// AllTables is the list of all tables in the schema (for related tables check)
	AllTables []*models.SchemaTable
	// Relationships are all entity relationships in the ontology
	Relationships []*models.EntityRelationship
	// Aliases are the aliases for this entity (for business alias check)
	Aliases []*models.OntologyEntityAlias
}

// PromotionScore evaluates if a table warrants entity status.
// Returns a score 0-100, where >= PromotionThreshold means promote to entity.
// The function is stateless and operates on the provided inputs.
//
// Scoring criteria (in order of evaluation):
//   - Hub in relationship graph: 30 points for 5+ inbound refs, 20 for 3+
//   - Multiple roles reference table: 25 points for 2+ distinct roles
//   - Multiple tables share concept: 20 points
//   - Has business aliases from LLM: 15 points
//   - Outbound relationships: 10 points for 3+
//
// Note: Scoring should happen AFTER relationship detection in the DAG workflow.
func PromotionScore(input PromotionInput) PromotionResult {
	score := 0
	var reasons []string

	// Criterion 1: Hub in relationship graph (30 or 20 points)
	// Count inbound relationships (how many other tables reference this one)
	inboundCount := countInboundRelationships(input.TableName, input.SchemaName, input.Relationships)
	if inboundCount >= 5 {
		score += pointsHubMajor
		reasons = append(reasons, fmt.Sprintf("hub with %d inbound references", inboundCount))
	} else if inboundCount >= 3 {
		score += pointsHubMinor
		reasons = append(reasons, fmt.Sprintf("%d inbound references", inboundCount))
	}

	// Criterion 2: Multiple roles reference this table (25 points)
	// e.g., host_id and visitor_id both reference users
	roleRefs := findRoleBasedReferences(input.TableName, input.SchemaName, input.Relationships)
	if len(roleRefs) >= 2 {
		score += pointsMultipleRoles
		reasons = append(reasons, fmt.Sprintf("%d distinct roles (%s)", len(roleRefs), strings.Join(roleRefs, ", ")))
	}

	// Criterion 3: Multiple tables share this concept (20 points)
	// Use core concept extraction to find related tables
	relatedTables := findRelatedTables(input.TableName, input.AllTables)
	if len(relatedTables) > 1 {
		score += pointsRelatedTables
		reasons = append(reasons, fmt.Sprintf("aggregates %d tables", len(relatedTables)))
	}

	// Criterion 4: Has business aliases from LLM (15 points)
	// Aliases indicate the LLM identified meaningful alternative names
	aliasCount := countBusinessAliases(input.Aliases)
	if aliasCount > 0 {
		score += pointsBusinessAlias
		reasons = append(reasons, fmt.Sprintf("%d business aliases", aliasCount))
	}

	// Criterion 5: Outbound relationships (10 points)
	// Tables that connect to many other entities are worth naming
	outboundCount := countOutboundRelationships(input.TableName, input.SchemaName, input.Relationships)
	if outboundCount >= 3 {
		score += pointsOutboundMinor
		reasons = append(reasons, fmt.Sprintf("%d outbound relationships", outboundCount))
	}

	return PromotionResult{Score: score, Reasons: reasons}
}

// countInboundRelationships counts how many relationships have this table as the target.
// Inbound relationships indicate that other tables reference this table via FK.
func countInboundRelationships(tableName, schemaName string, relationships []*models.EntityRelationship) int {
	count := 0
	for _, rel := range relationships {
		// Match target table (case-insensitive for robustness)
		if strings.EqualFold(rel.TargetColumnTable, tableName) &&
			strings.EqualFold(rel.TargetColumnSchema, schemaName) {
			count++
		}
	}
	return count
}

// countOutboundRelationships counts how many relationships have this table as the source.
// Outbound relationships indicate that this table references other tables via FK.
func countOutboundRelationships(tableName, schemaName string, relationships []*models.EntityRelationship) int {
	count := 0
	for _, rel := range relationships {
		// Match source table (case-insensitive for robustness)
		if strings.EqualFold(rel.SourceColumnTable, tableName) &&
			strings.EqualFold(rel.SourceColumnSchema, schemaName) {
			count++
		}
	}
	return count
}

// findRoleBasedReferences finds distinct roles that reference this table.
// Returns a list of role names (from Association field) for relationships targeting this table.
// Role-based references indicate semantic complexity (e.g., host_id and visitor_id both → users).
func findRoleBasedReferences(tableName, schemaName string, relationships []*models.EntityRelationship) []string {
	roleSet := make(map[string]struct{})

	for _, rel := range relationships {
		// Check if this table is the target of the relationship
		if strings.EqualFold(rel.TargetColumnTable, tableName) &&
			strings.EqualFold(rel.TargetColumnSchema, schemaName) {
			// Use Association if available, otherwise derive from source column name
			var role string
			if rel.Association != nil && *rel.Association != "" {
				role = *rel.Association
			} else {
				// Derive role from source column name (e.g., "host_id" → "host")
				role = deriveRoleFromColumn(rel.SourceColumnName)
			}
			if role != "" {
				roleSet[strings.ToLower(role)] = struct{}{}
			}
		}
	}

	// Convert set to slice
	roles := make([]string, 0, len(roleSet))
	for role := range roleSet {
		roles = append(roles, role)
	}
	return roles
}

// deriveRoleFromColumn extracts a role name from a column name.
// Common patterns: user_id → user, host_id → host, created_by_id → created_by
func deriveRoleFromColumn(columnName string) string {
	name := strings.ToLower(columnName)

	// Remove common FK suffixes
	suffixes := []string{"_id", "_uuid", "_fk"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			name = strings.TrimSuffix(name, suffix)
			break
		}
	}

	// Skip generic column names that don't indicate a role
	genericNames := map[string]bool{
		"id":         true,
		"uuid":       true,
		"key":        true,
		"ref":        true,
		"reference":  true,
		"fk":         true,
		"foreign":    true,
	}

	if genericNames[name] {
		return ""
	}

	return name
}

// findRelatedTables finds tables that share the same core concept as the given table.
// Uses the same extractCoreConcept logic as entity discovery.
// Returns all tables (including the input table) that share the concept.
func findRelatedTables(tableName string, allTables []*models.SchemaTable) []*models.SchemaTable {
	targetConcept := extractCoreConcept(tableName)

	var related []*models.SchemaTable
	for _, t := range allTables {
		if extractCoreConcept(t.TableName) == targetConcept {
			related = append(related, t)
		}
	}
	return related
}

// countBusinessAliases counts meaningful business aliases (excluding table grouping aliases).
// Aliases from LLM discovery indicate the table has business significance.
func countBusinessAliases(aliases []*models.OntologyEntityAlias) int {
	count := 0
	for _, alias := range aliases {
		// Skip aliases that came from table grouping (these are just test table names)
		if alias.Source != nil && *alias.Source == "table_grouping" {
			continue
		}
		count++
	}
	return count
}
