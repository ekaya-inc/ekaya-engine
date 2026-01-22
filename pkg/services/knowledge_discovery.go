package services

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
)

// CodeFact represents a fact discovered from code analysis.
type CodeFact struct {
	FactType string // "terminology", "business_rule", "enumeration", "convention"
	Fact     string // The fact content
	Context  string // Where it was found (file:line)
}

// KnowledgeFact represents a fact discovered from documentation analysis via LLM.
// Used as JSON output structure from ScanDocumentation.
type KnowledgeFact struct {
	FactType string `json:"fact_type"` // "terminology", "business_rule", "user_role", "convention"
	Fact     string `json:"fact"`      // The fact content
	Context  string `json:"context"`   // Source file where fact was found
}

// KnowledgeDiscovery provides code and documentation analysis to extract domain knowledge.
type KnowledgeDiscovery struct {
	llmClient llm.LLMClient
	logger    *zap.Logger
}

// NewKnowledgeDiscovery creates a new KnowledgeDiscovery service.
// The llmClient is optional and only needed for ScanDocumentation (LLM-based extraction).
func NewKnowledgeDiscovery(logger *zap.Logger, llmClient llm.LLMClient) *KnowledgeDiscovery {
	return &KnowledgeDiscovery{
		llmClient: llmClient,
		logger:    logger.Named("knowledge_discovery"),
	}
}

// ScanCodeComments scans Go and TypeScript files in the given repository path
// to extract const blocks with comments and functions with business rule comments.
// Returns a slice of discovered facts.
func (kd *KnowledgeDiscovery) ScanCodeComments(ctx context.Context, repoPath string) ([]CodeFact, error) {
	var facts []CodeFact

	// Find Go files
	goFiles, err := kd.findFiles(repoPath, "*.go")
	if err != nil {
		return nil, err
	}

	// Find TypeScript files
	tsFiles, err := kd.findFiles(repoPath, "*.ts")
	if err != nil {
		return nil, err
	}

	// Parse Go files
	for _, file := range goFiles {
		select {
		case <-ctx.Done():
			return facts, ctx.Err()
		default:
		}

		goFacts, err := kd.parseGoFile(file)
		if err != nil {
			kd.logger.Error("Failed to parse Go file",
				zap.String("file", file),
				zap.Error(err))
			continue
		}
		facts = append(facts, goFacts...)
	}

	// Parse TypeScript files
	for _, file := range tsFiles {
		select {
		case <-ctx.Done():
			return facts, ctx.Err()
		default:
		}

		tsFacts, err := kd.parseTypeScriptFile(file)
		if err != nil {
			kd.logger.Error("Failed to parse TypeScript file",
				zap.String("file", file),
				zap.Error(err))
			continue
		}
		facts = append(facts, tsFacts...)
	}

	kd.logger.Info("Code scanning complete",
		zap.Int("go_files", len(goFiles)),
		zap.Int("ts_files", len(tsFiles)),
		zap.Int("facts_discovered", len(facts)))

	return facts, nil
}

// findFiles finds all files matching the pattern recursively.
func (kd *KnowledgeDiscovery) findFiles(root, pattern string) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}

		// Skip hidden directories and common non-source directories
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file matches pattern
		matched, err := filepath.Match(pattern, info.Name())
		if err != nil {
			return err
		}
		if matched {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// parseGoFile parses a Go file and extracts facts from const blocks and comments.
func (kd *KnowledgeDiscovery) parseGoFile(filePath string) ([]CodeFact, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var facts []CodeFact
	relPath := kd.getRelativePath(filePath)

	// Process const declarations
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}

		// Check for doc comment on the const block
		var blockComment string
		if genDecl.Doc != nil {
			blockComment = genDecl.Doc.Text()
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			// Get the line number
			pos := fset.Position(valueSpec.Pos())

			// Get the comment for this specific const
			var comment string
			if valueSpec.Doc != nil {
				comment = valueSpec.Doc.Text()
			} else if valueSpec.Comment != nil {
				comment = valueSpec.Comment.Text()
			} else if blockComment != "" {
				comment = blockComment
			}

			// Only include consts with meaningful comments
			if comment == "" {
				continue
			}

			// Extract fact based on comment analysis
			fact := kd.extractFactFromGoConst(valueSpec, comment, relPath, pos.Line)
			if fact != nil {
				facts = append(facts, *fact)
			}
		}
	}

	// Process function declarations with business rule comments
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Doc == nil {
			continue
		}

		pos := fset.Position(funcDecl.Pos())
		fact := kd.extractFactFromGoFunc(funcDecl, relPath, pos.Line)
		if fact != nil {
			facts = append(facts, *fact)
		}
	}

	return facts, nil
}

// extractFactFromGoConst extracts a CodeFact from a Go const declaration with comment.
func (kd *KnowledgeDiscovery) extractFactFromGoConst(spec *ast.ValueSpec, comment, filePath string, line int) *CodeFact {
	comment = strings.TrimSpace(comment)

	// Determine fact type based on comment content
	factType := kd.categorizeComment(comment)
	if factType == "" {
		return nil
	}

	// Build the fact string
	constName := ""
	if len(spec.Names) > 0 {
		constName = spec.Names[0].Name
	}

	var constValue string
	if len(spec.Values) > 0 {
		constValue = kd.astExprToString(spec.Values[0])
	}

	// Create a meaningful fact
	var factText string
	if constValue != "" {
		factText = strings.TrimSuffix(comment, "\n")
		if !strings.Contains(strings.ToLower(factText), constName) {
			factText = constName + " = " + constValue + ": " + factText
		}
	} else {
		factText = strings.TrimSuffix(comment, "\n")
	}

	return &CodeFact{
		FactType: factType,
		Fact:     factText,
		Context:  filePath + ":" + itoa(line),
	}
}

// extractFactFromGoFunc extracts a CodeFact from a Go function with business rule comments.
func (kd *KnowledgeDiscovery) extractFactFromGoFunc(funcDecl *ast.FuncDecl, filePath string, line int) *CodeFact {
	comment := strings.TrimSpace(funcDecl.Doc.Text())

	// Only extract if comment suggests a business rule
	if !kd.isBusinessRuleComment(comment) {
		return nil
	}

	funcName := funcDecl.Name.Name

	return &CodeFact{
		FactType: "business_rule",
		Fact:     funcName + ": " + comment,
		Context:  filePath + ":" + itoa(line),
	}
}

// parseTypeScriptFile parses a TypeScript file and extracts facts from const declarations and comments.
func (kd *KnowledgeDiscovery) parseTypeScriptFile(filePath string) ([]CodeFact, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	relPath := kd.getRelativePath(filePath)
	return kd.extractTypeScriptFacts(string(content), relPath)
}

// extractTypeScriptFacts extracts facts from TypeScript source code using regex patterns.
// This is a simple parser for const declarations with JSDoc or inline comments.
func (kd *KnowledgeDiscovery) extractTypeScriptFacts(content, filePath string) ([]CodeFact, error) {
	var facts []CodeFact
	lines := strings.Split(content, "\n")

	constPattern := regexp.MustCompile(`(?:export\s+)?const\s+(\w+)\s*(?::\s*[\w<>[\]|&\s]+)?\s*=\s*(.+?)(?:;|$)`)
	singleCommentPattern := regexp.MustCompile(`^\s*//\s*(.+)$`)

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Check for JSDoc comment start (multi-line)
		if strings.Contains(line, "/**") {
			// Collect the entire JSDoc block
			var jsdocLines []string
			startLine := i
			for j := i; j < len(lines); j++ {
				jsdocLines = append(jsdocLines, lines[j])
				if strings.Contains(lines[j], "*/") {
					i = j
					break
				}
			}

			// Parse the JSDoc content
			jsdocContent := strings.Join(jsdocLines, "\n")
			comment := kd.cleanJSDocComment(jsdocContent)

			// Look for const on subsequent lines
			for j := i + 1; j < len(lines) && j < i+5; j++ {
				if constMatch := constPattern.FindStringSubmatch(lines[j]); constMatch != nil {
					fact := kd.extractFactFromTSConst(constMatch[1], constMatch[2], comment, filePath, j+1)
					if fact != nil {
						facts = append(facts, *fact)
					}
					i = j
					break
				}
				// Stop if we hit a non-comment, non-empty line that's not a const
				trimmed := strings.TrimSpace(lines[j])
				if trimmed != "" && !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "*") {
					break
				}
			}
			_ = startLine // silence unused warning
			continue
		}

		// Check for single-line comment followed by const
		if commentMatch := singleCommentPattern.FindStringSubmatch(line); commentMatch != nil {
			if i+1 < len(lines) {
				if constMatch := constPattern.FindStringSubmatch(lines[i+1]); constMatch != nil {
					fact := kd.extractFactFromTSConst(constMatch[1], constMatch[2], commentMatch[1], filePath, i+2)
					if fact != nil {
						facts = append(facts, *fact)
					}
					i++
				}
			}
			continue
		}

		// Check for inline comment on const line
		if constMatch := constPattern.FindStringSubmatch(line); constMatch != nil {
			if inlineIdx := strings.Index(line, "//"); inlineIdx > 0 {
				comment := strings.TrimSpace(line[inlineIdx+2:])
				fact := kd.extractFactFromTSConst(constMatch[1], constMatch[2], comment, filePath, i+1)
				if fact != nil {
					facts = append(facts, *fact)
				}
			}
		}
	}

	return facts, nil
}

// extractFactFromTSConst extracts a CodeFact from a TypeScript const declaration.
func (kd *KnowledgeDiscovery) extractFactFromTSConst(name, value, comment, filePath string, line int) *CodeFact {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return nil
	}

	factType := kd.categorizeComment(comment)
	if factType == "" {
		return nil
	}

	value = strings.TrimSpace(value)

	var factText string
	if !strings.Contains(strings.ToLower(comment), strings.ToLower(name)) {
		factText = name + " = " + value + ": " + comment
	} else {
		factText = comment
	}

	return &CodeFact{
		FactType: factType,
		Fact:     factText,
		Context:  filePath + ":" + itoa(line),
	}
}

// cleanJSDocComment cleans up a JSDoc comment by removing /** */, * prefixes and @tags.
func (kd *KnowledgeDiscovery) cleanJSDocComment(comment string) string {
	// Remove the /** and */ markers
	comment = strings.TrimPrefix(comment, "/**")
	comment = strings.TrimSuffix(comment, "*/")

	lines := strings.Split(comment, "\n")
	var cleaned []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Remove leading * that's common in JSDoc
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)

		// Skip @tags (like @param, @returns, @example)
		if strings.HasPrefix(line, "@") {
			continue
		}

		if line != "" {
			cleaned = append(cleaned, line)
		}
	}

	return strings.Join(cleaned, " ")
}

// categorizeComment determines the fact type based on comment content.
// Returns empty string if the comment doesn't appear to contain domain knowledge.
func (kd *KnowledgeDiscovery) categorizeComment(comment string) string {
	lower := strings.ToLower(comment)

	// Business rules - calculations, percentages, thresholds
	businessRulePatterns := []string{
		"fee", "rate", "percent", "share", "commission", "margin",
		"threshold", "limit", "minimum", "maximum", "calculated",
		"formula", "rule", "policy", "billing", "pricing",
	}
	for _, pattern := range businessRulePatterns {
		if strings.Contains(lower, pattern) {
			return "business_rule"
		}
	}

	// Terminology - definitions, units, domain terms
	terminologyPatterns := []string{
		"represents", "defines", "is a", "refers to", "means",
		"unit", "duration", "time", "period", "interval",
	}
	for _, pattern := range terminologyPatterns {
		if strings.Contains(lower, pattern) {
			return "terminology"
		}
	}

	// Enumerations - status values, type codes
	enumerationPatterns := []string{
		"status", "state", "type", "kind", "category",
		"active", "inactive", "pending", "completed",
	}
	for _, pattern := range enumerationPatterns {
		if strings.Contains(lower, pattern) {
			return "enumeration"
		}
	}

	// Conventions - currency, formatting, naming
	conventionPatterns := []string{
		"currency", "cents", "dollars", "format", "convention",
		"stored as", "stored in", "timezone", "utc",
	}
	for _, pattern := range conventionPatterns {
		if strings.Contains(lower, pattern) {
			return "convention"
		}
	}

	return ""
}

// isBusinessRuleComment checks if a function comment describes a business rule.
func (kd *KnowledgeDiscovery) isBusinessRuleComment(comment string) bool {
	lower := strings.ToLower(comment)

	businessRuleIndicators := []string{
		"calculates", "computes", "determines", "validates",
		"fee", "rate", "price", "amount", "total",
		"billing", "payment", "charge", "revenue",
		"rule", "policy", "logic", "business",
	}

	for _, indicator := range businessRuleIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}

// astExprToString converts an AST expression to a string representation.
func (kd *KnowledgeDiscovery) astExprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Value
	case *ast.Ident:
		return e.Name
	case *ast.BinaryExpr:
		return kd.astExprToString(e.X) + " " + e.Op.String() + " " + kd.astExprToString(e.Y)
	case *ast.UnaryExpr:
		return e.Op.String() + kd.astExprToString(e.X)
	case *ast.ParenExpr:
		return "(" + kd.astExprToString(e.X) + ")"
	default:
		return ""
	}
}

// getRelativePath returns the path relative to the repo root if possible.
func (kd *KnowledgeDiscovery) getRelativePath(fullPath string) string {
	// Try to get relative path from current directory
	if rel, err := filepath.Rel(".", fullPath); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return fullPath
}

// ScanDocumentation scans markdown documentation files in the repository and uses
// LLM to extract domain-specific facts: business terminology, business rules,
// user roles, and conventions.
// It searches for: *.md, docs/**/*.md, README*
func (kd *KnowledgeDiscovery) ScanDocumentation(ctx context.Context, repoPath string) ([]KnowledgeFact, error) {
	if kd.llmClient == nil {
		return nil, nil // No LLM client configured, return empty
	}

	// Find documentation files
	files, err := kd.findDocumentationFiles(repoPath)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		kd.logger.Info("No documentation files found", zap.String("repo_path", repoPath))
		return nil, nil
	}

	kd.logger.Info("Found documentation files",
		zap.String("repo_path", repoPath),
		zap.Int("file_count", len(files)))

	var allFacts []KnowledgeFact

	for _, file := range files {
		select {
		case <-ctx.Done():
			return allFacts, ctx.Err()
		default:
		}

		content, err := os.ReadFile(file)
		if err != nil {
			kd.logger.Error("Failed to read documentation file",
				zap.String("file", file),
				zap.Error(err))
			continue
		}

		// Skip empty files
		if len(content) == 0 {
			continue
		}

		// Extract facts via LLM
		facts, err := kd.extractFactsFromDocumentation(ctx, string(content), kd.getRelativePath(file))
		if err != nil {
			kd.logger.Error("Failed to extract facts from documentation",
				zap.String("file", file),
				zap.Error(err))
			continue
		}

		allFacts = append(allFacts, facts...)
	}

	kd.logger.Info("Documentation scanning complete",
		zap.Int("files_scanned", len(files)),
		zap.Int("facts_discovered", len(allFacts)))

	return allFacts, nil
}

// findDocumentationFiles finds markdown documentation files in the repository.
// Patterns: *.md in root, docs/**/*.md, README* anywhere.
func (kd *KnowledgeDiscovery) findDocumentationFiles(root string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)

	// Walk the directory tree
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}

		// Skip hidden directories and common non-documentation directories
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}

		fileName := info.Name()
		relPath, _ := filepath.Rel(root, path)

		// Match README* files anywhere
		if strings.HasPrefix(strings.ToUpper(fileName), "README") {
			if !seen[path] {
				files = append(files, path)
				seen[path] = true
			}
			return nil
		}

		// Match *.md files
		if strings.HasSuffix(strings.ToLower(fileName), ".md") {
			// Include if in root directory or in docs/ directory
			inRoot := !strings.Contains(relPath, string(filepath.Separator))
			inDocs := strings.HasPrefix(relPath, "docs"+string(filepath.Separator)) ||
				strings.HasPrefix(relPath, "doc"+string(filepath.Separator))

			if inRoot || inDocs {
				if !seen[path] {
					files = append(files, path)
					seen[path] = true
				}
			}
		}

		return nil
	})

	return files, err
}

// extractFactsFromDocumentation uses LLM to extract domain facts from documentation content.
func (kd *KnowledgeDiscovery) extractFactsFromDocumentation(ctx context.Context, content, sourcePath string) ([]KnowledgeFact, error) {
	// Truncate very long documents to avoid token limits
	const maxContentLength = 15000
	if len(content) > maxContentLength {
		content = content[:maxContentLength] + "\n\n[Content truncated...]"
	}

	prompt := kd.buildDocumentationExtractionPrompt(content, sourcePath)

	systemMessage := `You are a domain knowledge extraction specialist. Your task is to analyze documentation and extract domain-specific facts that would help someone understand the business domain.

Focus on:
1. Business terminology - unique terms, abbreviations, domain-specific concepts
2. Business rules - calculations, thresholds, percentages, policies
3. User roles - different user types and their meanings in the system
4. Conventions - currency handling, time zones, naming patterns

Return ONLY a valid JSON array. Do not include any explanation or markdown formatting.`

	result, err := kd.llmClient.GenerateResponse(ctx, prompt, systemMessage, 0.0, false)
	if err != nil {
		return nil, err
	}

	return kd.parseDocumentationFacts(result.Content, sourcePath)
}

// buildDocumentationExtractionPrompt creates the prompt for LLM-based fact extraction.
func (kd *KnowledgeDiscovery) buildDocumentationExtractionPrompt(content, sourcePath string) string {
	return `Analyze the following documentation and extract domain-specific facts.

Extract facts in these categories:
- terminology: Domain-specific terms, abbreviations, or concepts unique to this business
- business_rule: Calculations, percentages, thresholds, or policies
- user_role: Different user types and what they represent in the system
- convention: Currency handling, date formats, naming patterns, storage conventions

Return a JSON array of objects with these fields:
- fact_type: One of "terminology", "business_rule", "user_role", "convention"
- fact: A clear, concise statement of the fact
- context: Brief note about where in the document this was found or derived from

Only extract facts that are specific to this domain. Skip generic information.
If no domain-specific facts are found, return an empty array: []

Source file: ` + sourcePath + `

Documentation content:
` + content + `

Return ONLY a valid JSON array:`
}

// parseDocumentationFacts parses the LLM response into KnowledgeFact structs.
func (kd *KnowledgeDiscovery) parseDocumentationFacts(llmResponse, sourcePath string) ([]KnowledgeFact, error) {
	// Extract JSON from response (LLM might wrap it in markdown code blocks)
	jsonStr := extractJSON(llmResponse)

	var facts []KnowledgeFact
	if err := json.Unmarshal([]byte(jsonStr), &facts); err != nil {
		kd.logger.Error("Failed to parse LLM response as JSON",
			zap.String("source", sourcePath),
			zap.Error(err),
			zap.String("response_preview", truncateString(llmResponse, 200)))
		return nil, err
	}

	// Ensure all facts have the source context if not provided
	for i := range facts {
		if facts[i].Context == "" {
			facts[i].Context = sourcePath
		} else if !strings.Contains(facts[i].Context, sourcePath) {
			facts[i].Context = sourcePath + ": " + facts[i].Context
		}
	}

	return facts, nil
}

// itoa converts an int to a string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
