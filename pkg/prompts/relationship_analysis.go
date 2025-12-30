package prompts

import (
	"fmt"
	"strings"
)

// TableContext provides full schema context for a table.
type TableContext struct {
	Name     string
	RowCount *int64
	PKColumn string
	Columns  []ColumnContext
}

// ColumnContext provides column details for LLM analysis.
type ColumnContext struct {
	Name               string
	DataType           string
	IsNullable         bool
	NullPercent        float64
	IsPrimaryKey       bool
	IsForeignKey       bool
	ForeignKeyTarget   string // "table.column" if known FK
	LooksLikeForeignKey bool  // Naming pattern suggests FK
}

// CandidateContext provides candidate details for LLM analysis.
type CandidateContext struct {
	ID               string
	SourceTable      string
	SourceColumn     string
	SourceColumnType string
	TargetTable      string
	TargetColumn     string
	TargetColumnType string
	DetectionMethod  string
	ValueMatchRate   *float64
	Cardinality      *string
	JoinMatchRate    *float64
	OrphanRate       *float64
	TargetCoverage   *float64
	SourceRowCount   *int64
	TargetRowCount   *int64
}

// BuildRelationshipAnalysisPrompt creates the prompt for LLM relationship analysis.
// It includes schema context, candidate details, join metrics interpretation guide,
// and JSON response format for decisions and new_relationships.
func BuildRelationshipAnalysisPrompt(tables []TableContext, candidates []CandidateContext) string {
	var prompt strings.Builder

	prompt.WriteString("# Database Relationship Analysis\n\n")
	prompt.WriteString("Analyze the following relationship candidates and determine which are true foreign key relationships.\n\n")

	// Schema context
	prompt.WriteString("## Database Schema\n\n")
	for _, table := range tables {
		prompt.WriteString(fmt.Sprintf("### %s\n", table.Name))
		if table.RowCount != nil {
			prompt.WriteString(fmt.Sprintf("Row count: %d\n", *table.RowCount))
		}
		if table.PKColumn != "" {
			prompt.WriteString(fmt.Sprintf("Primary Key: %s\n", table.PKColumn))
		}
		prompt.WriteString("Columns:\n")
		for _, col := range table.Columns {
			flags := ""
			if col.IsPrimaryKey {
				flags += " [PK]"
			}
			if col.IsForeignKey {
				flags += fmt.Sprintf(" [FK→%s]", col.ForeignKeyTarget)
			}
			if col.LooksLikeForeignKey {
				flags += " [looks like FK]"
			}
			nullInfo := ""
			if col.IsNullable {
				nullInfo = fmt.Sprintf(" (nullable, %.1f%% null)", col.NullPercent)
			}
			prompt.WriteString(fmt.Sprintf("- %s (%s)%s%s\n", col.Name, col.DataType, flags, nullInfo))
		}
		prompt.WriteString("\n")
	}

	// Candidates to analyze
	prompt.WriteString("## Relationship Candidates\n\n")
	prompt.WriteString("For each candidate, you have the following information:\n\n")
	for i, c := range candidates {
		prompt.WriteString(fmt.Sprintf("### Candidate %d: %s.%s → %s.%s\n",
			i+1, c.SourceTable, c.SourceColumn, c.TargetTable, c.TargetColumn))
		prompt.WriteString(fmt.Sprintf("- **ID**: %s\n", c.ID))
		prompt.WriteString(fmt.Sprintf("- **Detection method**: %s\n", c.DetectionMethod))
		prompt.WriteString(fmt.Sprintf("- **Column types**: %s → %s\n", c.SourceColumnType, c.TargetColumnType))

		if c.ValueMatchRate != nil {
			prompt.WriteString(fmt.Sprintf("- **Sample value match rate**: %.1f%%\n", *c.ValueMatchRate*100))
		}

		if c.Cardinality != nil {
			prompt.WriteString(fmt.Sprintf("- **Cardinality**: %s\n", *c.Cardinality))
		}

		if c.JoinMatchRate != nil {
			prompt.WriteString(fmt.Sprintf("- **Join match rate**: %.1f%% (%d source rows)\n",
				*c.JoinMatchRate*100, valueOrZero(c.SourceRowCount)))
		}

		if c.OrphanRate != nil {
			prompt.WriteString(fmt.Sprintf("- **Orphan rate**: %.1f%% (source rows with no match)\n",
				*c.OrphanRate*100))
		}

		if c.TargetCoverage != nil {
			prompt.WriteString(fmt.Sprintf("- **Target coverage**: %.1f%% (of %d target rows)\n",
				*c.TargetCoverage*100, valueOrZero(c.TargetRowCount)))
		}

		prompt.WriteString("\n")
	}

	// Analysis guidelines
	prompt.WriteString("## Analysis Guidelines\n\n")
	prompt.WriteString("**Strong signals for CONFIRM**:\n")
	prompt.WriteString("- Cardinality is N:1 (many source rows → one target row, typical FK pattern)\n")
	prompt.WriteString("- Column naming follows FK convention (e.g., user_id → users.id)\n")
	prompt.WriteString("- High join match rate (>70%) and low orphan rate (<30%)\n")
	prompt.WriteString("- Target column is a primary key\n")
	prompt.WriteString("- Type match is exact (uuid→uuid, int→int, text→text)\n\n")

	prompt.WriteString("**Strong signals for REJECT**:\n")
	prompt.WriteString("- High orphan rate (>50%) suggests weak or coincidental relationship\n")
	prompt.WriteString("- Cardinality is N:M without clear junction table semantics\n")
	prompt.WriteString("- Column names suggest different domains (e.g., order_total → user_id)\n")
	prompt.WriteString("- Neither column is a key (both are regular data columns)\n\n")

	prompt.WriteString("**Mark as NEEDS_REVIEW when**:\n")
	prompt.WriteString("- Uncertain about business meaning (e.g., ambiguous column names)\n")
	prompt.WriteString("- Moderate orphan rate (30-50%) - could be data quality issue or optional FK\n")
	prompt.WriteString("- Cardinality unexpected for naming pattern\n\n")

	prompt.WriteString("## Interpretation Guide\n")
	prompt.WriteString("- **Orphan Rate**: Percentage of source rows that don't match any target row\n")
	prompt.WriteString("  - 0-10%: Normal for optional FKs or data in transition\n")
	prompt.WriteString("  - 10-30%: Warning sign, but may still be valid FK with data quality issues\n")
	prompt.WriteString("  - >50%: Likely not a real FK relationship\n")
	prompt.WriteString("- **Orphan Rate vs Null Rate**: Orphans are non-null values that don't match; different from NULL FKs\n")
	prompt.WriteString("- **Target Coverage**: Low coverage means few target rows are actually referenced (may indicate unused/stale data)\n\n")

	prompt.WriteString("## Output Format\n\n")
	prompt.WriteString("Respond in JSON with:\n")
	prompt.WriteString("- `decisions`: Array of decisions for each candidate\n")
	prompt.WriteString("  - `candidate_id`: The candidate ID from above\n")
	prompt.WriteString("  - `action`: One of \"confirm\", \"reject\", \"needs_review\"\n")
	prompt.WriteString("  - `confidence`: 0.0-1.0 (how confident you are in this decision)\n")
	prompt.WriteString("  - `reasoning`: Brief explanation (1-2 sentences)\n")
	prompt.WriteString("- `new_relationships`: Array of additional relationships you infer from schema (may be empty)\n")
	prompt.WriteString("  - `source_table`, `source_column`, `target_table`, `target_column`\n")
	prompt.WriteString("  - `confidence`: 0.0-1.0\n")
	prompt.WriteString("  - `reasoning`: Why you think this is a relationship\n\n")

	prompt.WriteString("Example:\n")
	prompt.WriteString("```json\n")
	prompt.WriteString(`{
  "decisions": [
    {
      "candidate_id": "abc-123",
      "action": "confirm",
      "confidence": 0.95,
      "reasoning": "Clear FK pattern: orders.user_id → users.id. N:1 cardinality with 98% match rate confirms strong relationship."
    }
  ],
  "new_relationships": [
    {
      "source_table": "order_items",
      "source_column": "product_id",
      "target_table": "products",
      "target_column": "id",
      "confidence": 0.85,
      "reasoning": "Standard FK naming pattern not detected by value matching. Column name clearly indicates product reference."
    }
  ]
}
`)
	prompt.WriteString("```\n\n")

	prompt.WriteString("Return ONLY the JSON, no additional text.\n")

	return prompt.String()
}

// BuildRelationshipAnalysisSystemMessage returns the system message for the LLM.
func BuildRelationshipAnalysisSystemMessage() string {
	return `You are a database relationship analysis expert. Your task is to review detected relationship candidates and determine which are true foreign key relationships.`
}

func valueOrZero(ptr *int64) int64 {
	if ptr != nil {
		return *ptr
	}
	return 0
}
