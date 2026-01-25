package services

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// DeterministicQuestionGenerator generates questions from data patterns without LLM.
// It detects data quality issues and ambiguous enum values that require human clarification.
type DeterministicQuestionGenerator struct {
	projectID  uuid.UUID
	ontologyID uuid.UUID
	workflowID *uuid.UUID
}

// NewDeterministicQuestionGenerator creates a new deterministic question generator.
func NewDeterministicQuestionGenerator(projectID, ontologyID uuid.UUID, workflowID *uuid.UUID) *DeterministicQuestionGenerator {
	return &DeterministicQuestionGenerator{
		projectID:  projectID,
		ontologyID: ontologyID,
		workflowID: workflowID,
	}
}

// GenerateFromSchema analyzes schema tables and generates questions based on data patterns.
// Returns questions for:
// - High NULL rate columns (>80%) that don't match known optional patterns
// - Cryptic enum values (single letters, numbers) that need human interpretation
func (g *DeterministicQuestionGenerator) GenerateFromSchema(tables []*models.SchemaTable) []*models.OntologyQuestion {
	var questions []*models.OntologyQuestion

	for _, table := range tables {
		if !table.IsSelected {
			continue
		}
		for _, col := range table.Columns {
			if !col.IsSelected {
				continue
			}

			// Check for high NULL rate
			if q := g.checkHighNullRate(table.TableName, &col, table.RowCount); q != nil {
				questions = append(questions, q)
			}

			// Check for cryptic enum values
			if q := g.checkCrypticEnumValues(table.TableName, &col); q != nil {
				questions = append(questions, q)
			}
		}
	}

	return questions
}

// highNullRateThreshold is the NULL rate above which we generate questions.
const highNullRateThreshold = 0.80

// checkHighNullRate generates a question if a column has >80% NULL values
// and doesn't match known optional column patterns.
func (g *DeterministicQuestionGenerator) checkHighNullRate(tableName string, col *models.SchemaColumn, tableRowCount *int64) *models.OntologyQuestion {
	// Need both null count and row count to calculate rate
	if col.NullCount == nil || tableRowCount == nil || *tableRowCount == 0 {
		return nil
	}

	nullRate := float64(*col.NullCount) / float64(*tableRowCount)
	if nullRate <= highNullRateThreshold {
		return nil
	}

	// Skip known optional column patterns
	if isKnownOptionalColumn(col.ColumnName) {
		return nil
	}

	return &models.OntologyQuestion{
		ProjectID:       g.projectID,
		OntologyID:      g.ontologyID,
		WorkflowID:      g.workflowID,
		Text:            fmt.Sprintf("Column %s.%s has %.0f%% NULL values - is this expected?", tableName, col.ColumnName, nullRate*100),
		Priority:        3, // nice-to-have
		IsRequired:      false,
		Category:        models.QuestionCategoryDataQuality,
		DetectedPattern: "high_null_rate",
		Reasoning:       fmt.Sprintf("Detected %.0f%% NULL rate which may indicate data quality issues or an optional field.", nullRate*100),
		Affects: &models.QuestionAffects{
			Tables:  []string{tableName},
			Columns: []string{fmt.Sprintf("%s.%s", tableName, col.ColumnName)},
		},
		Status: models.QuestionStatusPending,
	}
}

// knownOptionalColumnPatterns are column name patterns that are commonly optional.
// We don't generate NULL rate questions for these.
var knownOptionalColumnPatterns = []string{
	// Soft delete and lifecycle timestamps
	"deleted_at", "deleted_on", "deleted_date",
	"archived_at", "archived_on", "archived_date",
	"canceled_at", "cancelled_at", "canceled_on", "cancelled_on",
	"completed_at", "completed_on", "completed_date",
	"expired_at", "expired_on", "expiry_date", "expires_at",
	"ended_at", "ended_on", "end_date",
	"closed_at", "closed_on", "closed_date",
	"suspended_at", "suspended_on",
	"terminated_at", "terminated_on",
	"revoked_at", "revoked_on",
	"deactivated_at", "deactivated_on",
	"last_login_at", "last_login",
	"last_seen_at", "last_seen",
	"last_active_at", "last_active",
	"verified_at", "verified_on", "email_verified_at",
	"confirmed_at", "confirmed_on",
	"approved_at", "approved_on",

	// Optional relationship references
	"parent_id", "parent_uuid",
	"manager_id", "supervisor_id",
	"referrer_id", "referred_by",
	"assigned_to", "assigned_to_id",
	"reviewed_by", "reviewed_by_id",
	"approved_by", "approved_by_id",

	// Optional descriptive fields
	"description", "notes", "comment", "comments", "memo", "remarks",
	"middle_name", "suffix", "title", "nickname",
	"secondary_email", "alt_email", "alternate_email",
	"secondary_phone", "alt_phone", "mobile", "fax",
	"address_line_2", "address2", "apt", "suite", "unit",
	"company", "organization", "employer",
	"website", "url", "homepage",
	"bio", "about", "summary",
	"avatar", "avatar_url", "profile_image", "photo", "picture",

	// Optional tracking fields
	"updated_by", "modified_by", "changed_by",
	"source", "source_id", "origin", "referral_source",
	"campaign", "campaign_id", "utm_source", "utm_medium",
	"legacy_id", "external_id", "old_id",
	"metadata", "extra", "custom_fields", "attributes", "properties", "data",
	"tags", "labels", "categories",

	// Optional payment/billing
	"discount", "discount_amount", "discount_percent",
	"coupon", "coupon_code", "promo_code",
	"refund_amount", "refunded_at",
	"tax", "tax_amount", "tax_rate",
	"shipping", "shipping_cost", "shipping_address",
	"billing_address", "billing_address_id",
}

// isKnownOptionalColumn checks if a column name matches known optional patterns.
func isKnownOptionalColumn(columnName string) bool {
	lower := strings.ToLower(columnName)

	// Exact matches
	if slices.Contains(knownOptionalColumnPatterns, lower) {
		return true
	}

	// Suffix patterns (e.g., _notes, _description)
	optionalSuffixes := []string{
		"_notes", "_note", "_comment", "_comments", "_memo", "_remarks",
		"_description", "_desc",
		"_url", "_link",
		"_at", "_on", // Timestamp suffixes often nullable
	}
	for _, suffix := range optionalSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}

	// Prefix patterns
	optionalPrefixes := []string{
		"old_", "legacy_", "deprecated_",
		"alt_", "alternate_", "secondary_",
		"custom_", "extra_", "meta_",
	}
	for _, prefix := range optionalPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	return false
}

// checkCrypticEnumValues generates a question if sample values appear to be
// cryptic codes (single letters, numbers, abbreviations) that need interpretation.
func (g *DeterministicQuestionGenerator) checkCrypticEnumValues(tableName string, col *models.SchemaColumn) *models.OntologyQuestion {
	// Only check columns with sample values (low cardinality enum-like columns)
	if len(col.SampleValues) == 0 {
		return nil
	}

	// Skip columns with too many distinct values (not really an enum)
	if col.DistinctCount != nil && *col.DistinctCount > 20 {
		return nil
	}

	// Skip boolean-like columns
	if len(col.SampleValues) == 2 && isBooleanLike(col.SampleValues) {
		return nil
	}

	// Check if values are cryptic
	crypticValues := filterCrypticValues(col.SampleValues)
	if len(crypticValues) == 0 {
		return nil
	}

	// Format values for the question
	formattedValues := formatValuesList(crypticValues)

	return &models.OntologyQuestion{
		ProjectID:       g.projectID,
		OntologyID:      g.ontologyID,
		WorkflowID:      g.workflowID,
		Text:            fmt.Sprintf("What do the values %s represent in %s.%s?", formattedValues, tableName, col.ColumnName),
		Priority:        1, // critical - enum meanings are important for query generation
		IsRequired:      true,
		Category:        models.QuestionCategoryEnumeration,
		DetectedPattern: models.PatternEnumColumn,
		Reasoning:       fmt.Sprintf("Detected cryptic enum values that may require domain knowledge to interpret: %s", formattedValues),
		Affects: &models.QuestionAffects{
			Tables:  []string{tableName},
			Columns: []string{fmt.Sprintf("%s.%s", tableName, col.ColumnName)},
		},
		Status: models.QuestionStatusPending,
	}
}

// isBooleanLike checks if the values appear to be boolean representations.
func isBooleanLike(values []string) bool {
	if len(values) != 2 {
		return false
	}

	boolPairs := [][]string{
		{"true", "false"},
		{"t", "f"},
		{"yes", "no"},
		{"y", "n"},
		{"1", "0"},
		{"on", "off"},
		{"active", "inactive"},
		{"enabled", "disabled"},
	}

	lower0 := strings.ToLower(values[0])
	lower1 := strings.ToLower(values[1])

	for _, pair := range boolPairs {
		if (lower0 == pair[0] && lower1 == pair[1]) || (lower0 == pair[1] && lower1 == pair[0]) {
			return true
		}
	}

	return false
}

// filterCrypticValues returns values that appear to be cryptic codes.
func filterCrypticValues(values []string) []string {
	var cryptic []string

	for _, v := range values {
		if isCrypticValue(v) {
			cryptic = append(cryptic, v)
		}
	}

	// Only return if majority of values are cryptic
	// (prevents flagging mixed columns with a few abbreviations)
	if len(cryptic) >= len(values)/2 || len(cryptic) >= 3 {
		return cryptic
	}

	return nil
}

// singleLetterPattern matches single uppercase or lowercase letters.
var singleLetterPattern = regexp.MustCompile(`^[A-Za-z]$`)

// numericCodePattern matches short numeric codes (1-3 digits).
var numericCodePattern = regexp.MustCompile(`^[0-9]{1,3}$`)

// abbreviationPattern matches uppercase abbreviations (2-4 letters).
var abbreviationPattern = regexp.MustCompile(`^[A-Z]{2,4}$`)

// isCrypticValue checks if a value appears to be a cryptic code.
func isCrypticValue(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}

	// Single letters (A, B, C, etc.)
	if singleLetterPattern.MatchString(v) {
		return true
	}

	// Short numeric codes (1, 2, 3, 01, 99, etc.)
	if numericCodePattern.MatchString(v) {
		return true
	}

	// Uppercase abbreviations (OK, NY, USA, etc.) but only if 2-3 letters
	// 4-letter abbreviations might be readable words
	if abbreviationPattern.MatchString(v) && len(v) <= 3 {
		return true
	}

	// Mixed letter/number codes (A1, 2B, X99, etc.)
	if len(v) <= 3 && isMixedAlphanumeric(v) {
		return true
	}

	return false
}

// isMixedAlphanumeric checks if a string contains both letters and numbers.
func isMixedAlphanumeric(s string) bool {
	hasLetter := false
	hasDigit := false

	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}

	return hasLetter && hasDigit
}

// formatValuesList formats a list of values for display in a question.
func formatValuesList(values []string) string {
	if len(values) == 0 {
		return ""
	}

	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("'%s'", v)
	}

	if len(quoted) <= 5 {
		return strings.Join(quoted, ", ")
	}

	// Truncate long lists
	return strings.Join(quoted[:5], ", ") + fmt.Sprintf(" (and %d more)", len(quoted)-5)
}
