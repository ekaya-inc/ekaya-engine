package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadKnowledgeSeedFile_YAML(t *testing.T) {
	// Create a temporary YAML file
	content := `
terminology:
  - fact: "A tik represents 6 seconds of engagement time"
    context: "Billing unit - from billing_helpers.go:413"
  - fact: "Host is a content creator who receives payments"
    context: "User role"

business_rules:
  - fact: "Platform fees are 4.5% of total amount"
    context: "billing_helpers.go:373"

conventions:
  - fact: "All monetary amounts are stored in cents (USD)"
    context: "Currency convention"
`
	tmpDir := t.TempDir()
	seedPath := filepath.Join(tmpDir, "knowledge.yaml")
	err := os.WriteFile(seedPath, []byte(content), 0600)
	require.NoError(t, err)

	// Load the seed file
	seedFile, err := loadKnowledgeSeedFile(seedPath)
	require.NoError(t, err)

	// Verify counts
	assert.Len(t, seedFile.Terminology, 2)
	assert.Len(t, seedFile.BusinessRules, 1)
	assert.Len(t, seedFile.Conventions, 1)
	assert.Len(t, seedFile.Enumerations, 0)

	// Verify AllFacts
	allFacts := seedFile.AllFacts()
	assert.Len(t, allFacts, 4)

	// Verify fact types
	var factTypes []string
	for _, f := range allFacts {
		factTypes = append(factTypes, f.FactType)
	}
	assert.Contains(t, factTypes, "terminology")
	assert.Contains(t, factTypes, "business_rule")
	assert.Contains(t, factTypes, "convention")
}

func TestLoadKnowledgeSeedFile_JSON(t *testing.T) {
	content := `{
		"terminology": [
			{"fact": "Test fact", "context": "Test context"}
		],
		"business_rules": [
			{"fact": "Business rule", "context": "Rule context"}
		]
	}`
	tmpDir := t.TempDir()
	seedPath := filepath.Join(tmpDir, "knowledge.json")
	err := os.WriteFile(seedPath, []byte(content), 0600)
	require.NoError(t, err)

	seedFile, err := loadKnowledgeSeedFile(seedPath)
	require.NoError(t, err)

	assert.Len(t, seedFile.Terminology, 1)
	assert.Len(t, seedFile.BusinessRules, 1)
	assert.Equal(t, "Test fact", seedFile.Terminology[0].Fact)
	assert.Equal(t, "Business rule", seedFile.BusinessRules[0].Fact)
}

func TestLoadKnowledgeSeedFile_FileNotFound(t *testing.T) {
	_, err := loadKnowledgeSeedFile("/nonexistent/path/knowledge.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read seed file")
}

func TestLoadKnowledgeSeedFile_InvalidYAML(t *testing.T) {
	content := `
terminology:
  - fact: "Test
    context: missing closing quote
`
	tmpDir := t.TempDir()
	seedPath := filepath.Join(tmpDir, "invalid.yaml")
	err := os.WriteFile(seedPath, []byte(content), 0600)
	require.NoError(t, err)

	_, err = loadKnowledgeSeedFile(seedPath)
	assert.Error(t, err)
}

func TestKnowledgeSeedFile_AllFacts_Empty(t *testing.T) {
	seedFile := &KnowledgeSeedFile{}
	allFacts := seedFile.AllFacts()
	assert.Len(t, allFacts, 0)
}

func TestKnowledgeSeedFile_AllFacts_AllCategories(t *testing.T) {
	seedFile := &KnowledgeSeedFile{
		Terminology:   []KnowledgeSeedFact{{Fact: "term1", Context: "ctx1"}},
		BusinessRules: []KnowledgeSeedFact{{Fact: "rule1", Context: "ctx2"}},
		Enumerations:  []KnowledgeSeedFact{{Fact: "enum1", Context: "ctx3"}},
		Conventions:   []KnowledgeSeedFact{{Fact: "conv1", Context: "ctx4"}},
	}

	allFacts := seedFile.AllFacts()
	assert.Len(t, allFacts, 4)

	// Verify all types are represented
	typeCount := make(map[string]int)
	for _, f := range allFacts {
		typeCount[f.FactType]++
	}
	assert.Equal(t, 1, typeCount["terminology"])
	assert.Equal(t, 1, typeCount["business_rule"])
	assert.Equal(t, 1, typeCount["enumeration"])
	assert.Equal(t, 1, typeCount["convention"])
}
