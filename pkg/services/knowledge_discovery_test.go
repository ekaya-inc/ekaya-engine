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

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
)

func TestKnowledgeDiscovery_ScanCodeComments_GoFiles(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger, nil)

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

	// Check that we found the expected facts for the complete fee structure
	// Note: DurationPerTik contains "billing" in its comment, so it's categorized as business_rule
	var foundTikDuration, foundPlatformFee, foundTikrShare, foundCalculateFees bool
	for _, fact := range facts {
		// DurationPerTik has "billing" in comment, so it's business_rule
		if strings.Contains(fact.Fact, "DurationPerTik") || strings.Contains(fact.Fact, "6 seconds") {
			foundTikDuration = true
		}
		if fact.FactType == "business_rule" && (strings.Contains(fact.Fact, "PlatformFeeRate") || strings.Contains(fact.Fact, "4.5%")) {
			foundPlatformFee = true
		}
		// Fee structure: Tikr share (30%)
		if fact.FactType == "business_rule" && (strings.Contains(fact.Fact, "TikrShareRate") || strings.Contains(fact.Fact, "30%")) {
			foundTikrShare = true
		}
		// CalculateFees documents the complete fee breakdown including host earnings calculation
		if fact.FactType == "business_rule" && strings.Contains(fact.Fact, "CalculateFees") {
			foundCalculateFees = true
		}
	}

	assert.True(t, foundTikDuration, "Should find tik duration fact")
	assert.True(t, foundPlatformFee, "Should find platform fee business rule (4.5%)")
	assert.True(t, foundTikrShare, "Should find Tikr share business rule (30%)")
	assert.True(t, foundCalculateFees, "Should find CalculateFees business rule documenting fee structure")
}

func TestKnowledgeDiscovery_ScanCodeComments_TypeScriptFiles(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger, nil)

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
	kd := NewKnowledgeDiscovery(logger, nil)

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
	kd := NewKnowledgeDiscovery(logger, nil)

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
	kd := NewKnowledgeDiscovery(logger, nil)

	tmpDir := t.TempDir()

	// Run the scanner on empty directory
	facts, err := kd.ScanCodeComments(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Len(t, facts, 0, "Should return empty slice for empty directory")
}

func TestKnowledgeDiscovery_CategorizeComment(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger, nil)

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
	kd := NewKnowledgeDiscovery(logger, nil)

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
	kd := NewKnowledgeDiscovery(logger, nil)

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
	kd := NewKnowledgeDiscovery(logger, nil)

	// Test with a path in current directory
	result := kd.getRelativePath("./test/file.go")
	assert.Equal(t, "test/file.go", result)

	// Test with absolute path outside current directory
	// This should return the original path if rel would start with ..
	result = kd.getRelativePath("/some/random/path.go")
	// The result depends on the current working directory
	assert.NotEmpty(t, result)
}

// Tests for ScanDocumentation

func TestKnowledgeDiscovery_ScanDocumentation_NoLLMClient(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger, nil) // No LLM client

	tmpDir := t.TempDir()

	// Create a markdown file
	mdPath := filepath.Join(tmpDir, "README.md")
	err := os.WriteFile(mdPath, []byte("# Test\nSome content"), 0600)
	require.NoError(t, err)

	// Should return nil without error when no LLM client is configured
	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Nil(t, facts, "Should return nil when no LLM client is configured")
}

func TestKnowledgeDiscovery_ScanDocumentation_EmptyDirectory(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := createKnowledgeMockLLMClient()
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	// Should return nil for empty directory
	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Nil(t, facts, "Should return nil for empty directory")
	assert.Equal(t, 0, mockLLM.GenerateResponseCalls, "Should not call LLM for empty directory")
}

func TestKnowledgeDiscovery_ScanDocumentation_FindsREADME(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := createKnowledgeMockLLMClientWithResponse(`[{"fact_type": "terminology", "fact": "Test term", "context": "Found in intro"}]`)
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	// Create README.md in root
	readmePath := filepath.Join(tmpDir, "README.md")
	err := os.WriteFile(readmePath, []byte("# Project\nThis is a test project."), 0600)
	require.NoError(t, err)

	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.Equal(t, "terminology", facts[0].FactType)
	assert.Equal(t, "Test term", facts[0].Fact)
	assert.Equal(t, 1, mockLLM.GenerateResponseCalls)
}

func TestKnowledgeDiscovery_ScanDocumentation_FindsDocsDirectory(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := createKnowledgeMockLLMClientWithResponse(`[{"fact_type": "business_rule", "fact": "Platform fee is 5%", "context": "billing section"}]`)
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	// Create docs directory with markdown
	docsDir := filepath.Join(tmpDir, "docs")
	err := os.MkdirAll(docsDir, 0755)
	require.NoError(t, err)

	docPath := filepath.Join(docsDir, "billing.md")
	err = os.WriteFile(docPath, []byte("# Billing\nPlatform fee is 5%."), 0600)
	require.NoError(t, err)

	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.Equal(t, "business_rule", facts[0].FactType)
	assert.Contains(t, facts[0].Context, "billing.md")
}

func TestKnowledgeDiscovery_ScanDocumentation_SkipsNodeModules(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := createKnowledgeMockLLMClientWithResponse(`[]`)
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	// Create markdown in node_modules (should be skipped)
	nodeModulesDir := filepath.Join(tmpDir, "node_modules", "some-package")
	err := os.MkdirAll(nodeModulesDir, 0755)
	require.NoError(t, err)

	skipPath := filepath.Join(nodeModulesDir, "README.md")
	err = os.WriteFile(skipPath, []byte("# Package\nThis should be skipped."), 0600)
	require.NoError(t, err)

	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Nil(t, facts, "Should not process files in node_modules")
	assert.Equal(t, 0, mockLLM.GenerateResponseCalls, "Should not call LLM for node_modules files")
}

func TestKnowledgeDiscovery_ScanDocumentation_SkipsNonRootMD(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := createKnowledgeMockLLMClientWithResponse(`[]`)
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	// Create markdown in src/ (not root or docs/) - should be skipped unless it's README
	srcDir := filepath.Join(tmpDir, "src")
	err := os.MkdirAll(srcDir, 0755)
	require.NoError(t, err)

	mdPath := filepath.Join(srcDir, "notes.md")
	err = os.WriteFile(mdPath, []byte("# Notes\nSome developer notes."), 0600)
	require.NoError(t, err)

	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Nil(t, facts, "Should not process .md files outside root or docs/")
	assert.Equal(t, 0, mockLLM.GenerateResponseCalls)
}

func TestKnowledgeDiscovery_ScanDocumentation_FindsREADMEAnywhere(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := createKnowledgeMockLLMClientWithResponse(`[{"fact_type": "user_role", "fact": "Admin users have elevated privileges", "context": "roles section"}]`)
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	// Create README in a subdirectory (should still be found)
	srcDir := filepath.Join(tmpDir, "src", "api")
	err := os.MkdirAll(srcDir, 0755)
	require.NoError(t, err)

	readmePath := filepath.Join(srcDir, "README.md")
	err = os.WriteFile(readmePath, []byte("# API\nAdmin users have elevated privileges."), 0600)
	require.NoError(t, err)

	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.Equal(t, "user_role", facts[0].FactType)
}

func TestKnowledgeDiscovery_ScanDocumentation_MultipleFacts(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := createKnowledgeMockLLMClientWithResponse(`[
		{"fact_type": "terminology", "fact": "A tik is 6 seconds of engagement", "context": "glossary"},
		{"fact_type": "business_rule", "fact": "Platform takes 4.5% fee", "context": "billing"},
		{"fact_type": "user_role", "fact": "Host is a content creator", "context": "user types"},
		{"fact_type": "convention", "fact": "All amounts stored in cents", "context": "data format"}
	]`)
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	readmePath := filepath.Join(tmpDir, "README.md")
	err := os.WriteFile(readmePath, []byte("# Project\nRich documentation with many facts."), 0600)
	require.NoError(t, err)

	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Len(t, facts, 4)

	// Verify all fact types are present
	factTypes := make(map[string]bool)
	for _, fact := range facts {
		factTypes[fact.FactType] = true
	}
	assert.True(t, factTypes["terminology"])
	assert.True(t, factTypes["business_rule"])
	assert.True(t, factTypes["user_role"])
	assert.True(t, factTypes["convention"])
}

func TestKnowledgeDiscovery_ScanDocumentation_ContextCancellation(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := createKnowledgeMockLLMClientWithResponse(`[]`)
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	readmePath := filepath.Join(tmpDir, "README.md")
	err := os.WriteFile(readmePath, []byte("# Test"), 0600)
	require.NoError(t, err)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = kd.ScanDocumentation(ctx, tmpDir)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestKnowledgeDiscovery_ScanDocumentation_LLMResponseWithMarkdown(t *testing.T) {
	logger := zap.NewNop()
	// LLM response wrapped in markdown code blocks
	mockLLM := createKnowledgeMockLLMClientWithResponse("```json\n[{\"fact_type\": \"terminology\", \"fact\": \"A tik is 6 seconds\", \"context\": \"glossary\"}]\n```")
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	readmePath := filepath.Join(tmpDir, "README.md")
	err := os.WriteFile(readmePath, []byte("# Test"), 0600)
	require.NoError(t, err)

	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.Equal(t, "A tik is 6 seconds", facts[0].Fact)
}

func TestKnowledgeDiscovery_ScanDocumentation_LLMError(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := createKnowledgeMockLLMClientWithError(assert.AnError)
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	readmePath := filepath.Join(tmpDir, "README.md")
	err := os.WriteFile(readmePath, []byte("# Test"), 0600)
	require.NoError(t, err)

	// Should not return error (continues to next file), but no facts extracted
	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Nil(t, facts)
}

func TestKnowledgeDiscovery_ScanDocumentation_InvalidJSONResponse(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := createKnowledgeMockLLMClientWithResponse("This is not valid JSON")
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	readmePath := filepath.Join(tmpDir, "README.md")
	err := os.WriteFile(readmePath, []byte("# Test"), 0600)
	require.NoError(t, err)

	// Should not return error (logs warning), but no facts extracted
	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Nil(t, facts)
}

func TestKnowledgeDiscovery_ScanDocumentation_EmptyContextFilled(t *testing.T) {
	logger := zap.NewNop()
	// Response with empty context - should be filled with source path
	mockLLM := createKnowledgeMockLLMClientWithResponse(`[{"fact_type": "terminology", "fact": "Test fact", "context": ""}]`)
	kd := NewKnowledgeDiscovery(logger, mockLLM)

	tmpDir := t.TempDir()

	readmePath := filepath.Join(tmpDir, "README.md")
	err := os.WriteFile(readmePath, []byte("# Test"), 0600)
	require.NoError(t, err)

	facts, err := kd.ScanDocumentation(context.Background(), tmpDir)
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.Contains(t, facts[0].Context, "README.md", "Empty context should be filled with source path")
}

// TestKnowledgeDiscovery_ScanCodeComments_FeeStructure verifies that the complete Tikr fee structure
// is captured from code comments: platform fees (4.5%), Tikr share (30%), and the resulting
// host earnings (~66.35% of total transaction).
func TestKnowledgeDiscovery_ScanCodeComments_FeeStructure(t *testing.T) {
	logger := zap.NewNop()
	kd := NewKnowledgeDiscovery(logger, nil)

	tmpDir := t.TempDir()

	// Create a Go file with billing calculations that documents the fee structure
	goContent := `package billing

// PlatformFeeRate is the platform's fee percentage (4.5%)
// This fee covers payment processing costs.
const PlatformFeeRate = 0.045

// TikrShareRate is the percentage taken by Tikr (30% of amount after platform fees)
// Host receives the remainder (~66.35% of total).
const TikrShareRate = 0.30

// HostEarningsRate is approximately 66.35% of total transaction
// Calculation: (1 - 0.045) * (1 - 0.30) = 0.9550 * 0.70 = 0.6685 ≈ 66.85%
// Note: Actual calculation may vary slightly due to rounding.
const HostEarningsRate = 0.6685

// CalculateEarnings computes the fee breakdown for a billing transaction.
// Fee structure: Platform fees (4.5%), Tikr share (30%), Host earnings (~66.35%)
func CalculateEarnings(totalAmount int) (platformFees, tikrShare, hostEarnings int) {
	platformFees = totalAmount * 45 / 1000      // 4.5%
	afterFees := totalAmount - platformFees
	tikrShare = afterFees * 30 / 100            // 30% of remainder
	hostEarnings = afterFees - tikrShare        // ~66.35% of total
	return
}
`
	goPath := filepath.Join(tmpDir, "billing_helpers.go")
	err := os.WriteFile(goPath, []byte(goContent), 0600)
	require.NoError(t, err)

	// Run the scanner
	facts, err := kd.ScanCodeComments(context.Background(), tmpDir)
	require.NoError(t, err)

	// Verify we extracted the complete fee structure
	var foundPlatformFee, foundTikrShare, foundHostEarnings, foundCalculateEarnings bool
	for _, fact := range facts {
		// Platform fee rate (4.5%)
		if fact.FactType == "business_rule" && (strings.Contains(fact.Fact, "PlatformFeeRate") || strings.Contains(fact.Fact, "4.5%")) {
			foundPlatformFee = true
		}
		// Tikr share rate (30%)
		if fact.FactType == "business_rule" && (strings.Contains(fact.Fact, "TikrShareRate") || strings.Contains(fact.Fact, "30%")) {
			foundTikrShare = true
		}
		// Host earnings rate (~66.35%)
		if fact.FactType == "business_rule" && (strings.Contains(fact.Fact, "HostEarningsRate") || strings.Contains(fact.Fact, "66")) {
			foundHostEarnings = true
		}
		// CalculateEarnings function documenting fee structure
		if fact.FactType == "business_rule" && strings.Contains(fact.Fact, "CalculateEarnings") {
			foundCalculateEarnings = true
		}
	}

	assert.True(t, foundPlatformFee, "Should find platform fee rate (4.5%)")
	assert.True(t, foundTikrShare, "Should find Tikr share rate (30%)")
	assert.True(t, foundHostEarnings, "Should find host earnings rate (~66.35%)")
	assert.True(t, foundCalculateEarnings, "Should find CalculateEarnings function documenting fee structure")
}

// Helper functions for mock LLM clients

func createKnowledgeMockLLMClient() *knowledgeMockLLMClient {
	return &knowledgeMockLLMClient{}
}

func createKnowledgeMockLLMClientWithResponse(response string) *knowledgeMockLLMClient {
	return &knowledgeMockLLMClient{response: response}
}

func createKnowledgeMockLLMClientWithError(err error) *knowledgeMockLLMClient {
	return &knowledgeMockLLMClient{err: err}
}

type knowledgeMockLLMClient struct {
	response              string
	err                   error
	GenerateResponseCalls int
}

func (m *knowledgeMockLLMClient) GenerateResponse(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
	m.GenerateResponseCalls++
	if m.err != nil {
		return nil, m.err
	}
	return &llm.GenerateResponseResult{
		Content: m.response,
	}, nil
}

func (m *knowledgeMockLLMClient) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	return nil, nil
}

func (m *knowledgeMockLLMClient) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	return nil, nil
}

func (m *knowledgeMockLLMClient) GetModel() string {
	return "mock-model"
}

func (m *knowledgeMockLLMClient) GetEndpoint() string {
	return "http://mock"
}
