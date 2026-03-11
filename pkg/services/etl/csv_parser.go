package etl

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ParseCSV parses a CSV/TSV file, auto-detecting the delimiter.
func ParseCSV(data []byte) (*models.ParseResult, error) {
	delimiter := detectDelimiter(data)

	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // Allow variable column counts

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv parse error: %w", err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("csv file is empty")
	}

	headers := records[0]
	var rows [][]string
	if len(records) > 1 {
		rows = records[1:]
	}

	return &models.ParseResult{
		Headers:   headers,
		Rows:      rows,
		Delimiter: delimiter,
	}, nil
}

// ParseCSVReader parses from an io.Reader for streaming large files.
func ParseCSVReader(r io.Reader, delimiter rune) (*models.ParseResult, error) {
	reader := csv.NewReader(r)
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv parse error: %w", err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("csv file is empty")
	}

	return &models.ParseResult{
		Headers:   records[0],
		Rows:      records[1:],
		Delimiter: delimiter,
	}, nil
}

// detectDelimiter tries comma, tab, semicolon, pipe on the first 5 rows
// and picks the one that produces the most consistent column count.
func detectDelimiter(data []byte) rune {
	candidates := []rune{',', '\t', ';', '|'}
	bestDelim := ','
	bestScore := -1

	// Use first 5 lines for detection
	lines := firstNLines(data, 5)
	if len(lines) < 2 {
		return bestDelim
	}

	for _, delim := range candidates {
		score := scoreDelimiter(lines, delim)
		if score > bestScore {
			bestScore = score
			bestDelim = delim
		}
	}

	return bestDelim
}

func firstNLines(data []byte, n int) []string {
	scanner := strings.NewReader(string(data))
	reader := csv.NewReader(scanner)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	var lines []string
	for i := 0; i < n; i++ {
		// Read raw lines instead
		break
	}

	// Simple line splitting for delimiter detection
	raw := string(data)
	for _, line := range strings.SplitN(raw, "\n", n+1) {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			lines = append(lines, line)
		}
		if len(lines) >= n {
			break
		}
	}
	return lines
}

// scoreDelimiter returns a score for how good a delimiter choice is.
// Higher score = more consistent column counts across rows with multiple columns.
func scoreDelimiter(lines []string, delim rune) int {
	delimStr := string(delim)
	counts := make([]int, len(lines))
	for i, line := range lines {
		counts[i] = strings.Count(line, delimStr) + 1
	}

	if len(counts) == 0 {
		return 0
	}

	// Must produce at least 2 columns in the first row
	if counts[0] < 2 {
		return 0
	}

	// Score: consistency (how many rows have the same count as the header)
	headerCount := counts[0]
	score := headerCount * 10 // Prefer more columns
	for i := 1; i < len(counts); i++ {
		if counts[i] == headerCount {
			score += 100 // Consistency bonus
		}
	}

	return score
}
