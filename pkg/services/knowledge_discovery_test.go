package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestKnowledgeDiscovery_ScanCodeComments_GoFiles(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger)

	// Create a temporary directory with Go files
	tmpDir := t.TempDir()

	// Create a Go file with const blocks and comments
	goContent := `package billing

// DurationPerTik represents the billing unit - 6 seconds of engagement time
const DurationPerTik = 6

const (
	// PlatformFeeRate is the fee charged by the platform (4.5%)
	PlatformFeeRate = 0.045

	// TikrShareRate is the percentage taken by Tikr (30%)
	TikrShareRate = 0.30
)

// MinCaptureAmount is the minimum amount that can be captured (in cents)
const MinCaptureAmount = 100 // Minimum capture amount is $1.00

// CalculateFees computes the fee breakdown for a transaction.
// The platform takes 4.5%, then Tikr takes 30% of the remainder.
func CalculateFees(amount int) (platformFee, tikrShare, hostEarnings int) {
	return 0, 0, 0
}
`
	goPath := filepath.Join(tmpDir, "billing_helpers.go")
	err := os.WriteFile(goPath, []byte(goContent), 0600)
	require.NoError(t, err)

	// Run the scanner
	facts, err := kd.ScanCodeComments(context.Background(), tmpDir)
	require.NoError(t, err)

	// Verify we extracted facts
	assert.GreaterOrEqual(t, len(facts), 3, "Should extract at least 3 facts")

	// Check that we found the expected facts
	// Note: DurationPerTik contains "billing" in its comment, so it's categorized as business_rule
	var foundTikDuration, foundPlatformFee, foundCalculateFees bool
	for _, fact := range facts {
		// DurationPerTik has "billing" in comment, so it's business_rule
		if strings.Contains(fact.Fact, "DurationPerTik") || strings.Contains(fact.Fact, "6 seconds") {
			foundTikDuration = true
		}
		if fact.FactType == "business_rule" && (strings.Contains(fact.Fact, "PlatformFeeRate") || strings.Contains(fact.Fact, "4.5%")) {
			foundPlatformFee = true
		}
		if fact.FactType == "business_rule" && strings.Contains(fact.Fact, "CalculateFees") {
			foundCalculateFees = true
		}
	}

	assert.True(t, foundTikDuration, "Should find tik duration fact")
	assert.True(t, foundPlatformFee, "Should find platform fee business rule")
	assert.True(t, foundCalculateFees, "Should find CalculateFees business rule")
}

func TestKnowledgeDiscovery_ScanCodeComments_TypeScriptFiles(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger)

	// Create a temporary directory with TypeScript files
	tmpDir := t.TempDir()

	// Create a TypeScript file with const declarations and comments
	tsContent := `// Duration per tik in seconds - billing unit
export const DURATION_PER_TIK = 6;

/**
 * Platform fee rate is 4.5% of the transaction amount.
 * This is charged before the Tikr share is calculated.
 */
export const PLATFORM_FEE_RATE = 0.045;

// Tikr share rate - 30% of amount after platform fees
const TIKR_SHARE_RATE = 0.30;

// Minimum capture amount in cents
const MIN_CAPTURE_AMOUNT = 100; // Minimum capture is $1.00 (currency convention)
`
	tsPath := filepath.Join(tmpDir, "billing.ts")
	err := os.WriteFile(tsPath, []byte(tsContent), 0600)
	require.NoError(t, err)

	// Run the scanner
	facts, err := kd.ScanCodeComments(context.Background(), tmpDir)
	require.NoError(t, err)

	// Verify we extracted facts
	assert.GreaterOrEqual(t, len(facts), 2, "Should extract at least 2 facts")

	// Check fact types
	factTypes := make(map[string]int)
	for _, fact := range facts {
		factTypes[fact.FactType]++
		// Verify context contains file and line info
		assert.Contains(t, fact.Context, "billing.ts:")
	}

	// Should find terminology and business rules
	assert.Greater(t, factTypes["terminology"]+factTypes["business_rule"]+factTypes["convention"], 0,
		"Should find at least one terminology, business_rule, or convention fact")
}

func TestKnowledgeDiscovery_ScanCodeComments_SkipsNodeModules(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger)

	tmpDir := t.TempDir()

	// Create a file in node_modules that should be skipped
	nodeModulesDir := filepath.Join(tmpDir, "node_modules", "some-package")
	err := os.MkdirAll(nodeModulesDir, 0755)
	require.NoError(t, err)

	skipContent := `// This should be skipped - business rule that should not be found
const PLATFORM_FEE_RATE = 0.99;
`
	skipPath := filepath.Join(nodeModulesDir, "config.ts")
	err = os.WriteFile(skipPath, []byte(skipContent), 0600)
	require.NoError(t, err)

	// Create a file in the src directory that should be scanned
	srcDir := filepath.Join(tmpDir, "src")
	err = os.MkdirAll(srcDir, 0755)
	require.NoError(t, err)

	scanContent := `// Platform fee rate - business rule
const FEE_RATE = 0.045;
`
	scanPath := filepath.Join(srcDir, "billing.ts")
	err = os.WriteFile(scanPath, []byte(scanContent), 0600)
	require.NoError(t, err)

	// Run the scanner
	facts, err := kd.ScanCodeComments(context.Background(), tmpDir)
	require.NoError(t, err)

	// Verify we only found facts from src, not node_modules
	for _, fact := range facts {
		assert.NotContains(t, fact.Context, "node_modules", "Should not include facts from node_modules")
	}

	// Should find at least one fact from src
	assert.GreaterOrEqual(t, len(facts), 1, "Should find facts from src directory")
}

func TestKnowledgeDiscovery_ScanCodeComments_ContextCancellation(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger)

	tmpDir := t.TempDir()

	// Create a Go file
	goContent := `package test
// Test duration
const Duration = 6
`
	goPath := filepath.Join(tmpDir, "test.go")
	err := os.WriteFile(goPath, []byte(goContent), 0600)
	require.NoError(t, err)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Run the scanner with cancelled context
	_, err = kd.ScanCodeComments(ctx, tmpDir)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestKnowledgeDiscovery_ScanCodeComments_EmptyDirectory(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger)

	tmpDir := t.TempDir()

	// Run the scanner on empty directory
	facts, err := kd.ScanCodeComments(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Len(t, facts, 0, "Should return empty slice for empty directory")
}

func TestKnowledgeDiscovery_CategorizeComment(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger)

	tests := []struct {
		comment  string
		expected string
	}{
		// Business rules (checked first, takes priority)
		{"Platform fee rate is 4.5%", "business_rule"},
		{"Commission rate for sellers", "business_rule"},
		{"Minimum threshold for capture", "business_rule"},
		{"Pricing tier calculation", "business_rule"},
		{"Defines the billing unit", "business_rule"}, // "billing" matches business_rule

		// Terminology (when no business_rule keywords)
		{"Represents 6 seconds of engagement", "terminology"},
		{"Duration of a single session", "terminology"},
		{"This refers to the user role", "terminology"},

		// Enumerations
		{"Status code for users", "enumeration"},
		{"Pending state constant", "enumeration"},
		{"Transaction type identifier", "enumeration"},

		// Conventions
		{"Amount stored in cents", "convention"},
		{"Data stored in UTC format", "convention"}, // "stored" matches convention
		{"Currency convention for USD", "convention"},

		// No match
		{"This is just a regular comment", ""},
		{"TODO: fix this later", ""},
		{"Returns the value", ""},
	}

	for _, tt := range tests {
		t.Run(tt.comment, func(t *testing.T) {
			result := kd.categorizeComment(tt.comment)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKnowledgeDiscovery_ExtractTypeScriptFacts_JSDocComments(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger)

	content := `/**
 * Platform fee rate is 4.5% of the transaction.
 * @example 0.045
 */
export const PLATFORM_FEE_RATE = 0.045;

/**
 * Tikr's share rate - 30% of amount after platform fees.
 */
const TIKR_SHARE_RATE = 0.30;
`

	facts, err := kd.extractTypeScriptFacts(content, "billing.ts")
	require.NoError(t, err)

	// Should find at least one business rule fact
	assert.GreaterOrEqual(t, len(facts), 1, "Should extract facts from JSDoc comments")

	// Check that @example was stripped
	for _, fact := range facts {
		assert.NotContains(t, fact.Fact, "@example", "Should strip @tags from JSDoc comments")
	}
}

func TestKnowledgeDiscovery_ParseGoFile_InlineComments(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger)

	tmpDir := t.TempDir()

	// Create a Go file with inline comments
	goContent := `package config

const (
	MaxRetries = 3  // Maximum retry limit for failed operations
	Timeout = 30    // Timeout in seconds
)
`
	goPath := filepath.Join(tmpDir, "config.go")
	err := os.WriteFile(goPath, []byte(goContent), 0600)
	require.NoError(t, err)

	facts, err := kd.parseGoFile(goPath)
	require.NoError(t, err)

	// Should find the limit fact (which matches "limit" pattern)
	var foundLimit bool
	for _, fact := range facts {
		if strings.Contains(fact.Fact, "MaxRetries") || strings.Contains(fact.Fact, "limit") {
			foundLimit = true
		}
	}
	assert.True(t, foundLimit, "Should extract fact from inline comment with 'limit'")
}

func TestKnowledgeDiscovery_GetRelativePath(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger)

	// Test with a path in current directory
	result := kd.getRelativePath("./test/file.go")
	assert.Equal(t, "test/file.go", result)

	// Test with absolute path outside current directory
	// This should return the original path if rel would start with ..
	result = kd.getRelativePath("/some/random/path.go")
	// The result depends on the current working directory
	assert.NotEmpty(t, result)
}
