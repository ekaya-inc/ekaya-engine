package etl

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/xuri/excelize/v2"
)

// ParseXLSX parses an XLSX file, returning data for each sheet.
func ParseXLSX(r io.ReaderAt, size int64) ([]models.SheetData, error) {
	f, err := excelize.OpenReader(io.NewSectionReader(r, 0, size))
	if err != nil {
		return nil, fmt.Errorf("xlsx open error: %w", err)
	}
	defer f.Close()

	return parseExcelFile(f)
}

// ParseXLSXFromBytes parses XLSX from a byte slice.
func ParseXLSXFromBytes(data []byte) ([]models.SheetData, error) {
	r := newBytesReaderAt(data)
	return ParseXLSX(r, int64(len(data)))
}

func parseExcelFile(f *excelize.File) ([]models.SheetData, error) {
	sheetList := f.GetSheetList()
	var results []models.SheetData

	for _, sheetName := range sheetList {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			return nil, fmt.Errorf("failed to read sheet %q: %w", sheetName, err)
		}

		// Skip empty sheets
		rows = skipEmptyRows(rows)
		if len(rows) < 1 {
			continue
		}

		headers := rows[0]

		// Skip sheets where header row is completely empty
		if allEmpty(headers) {
			continue
		}

		var dataRows [][]string
		if len(rows) > 1 {
			dataRows = normalizeRows(rows[1:], len(headers))
		}

		// Convert date serial numbers in cells
		convertDateSerials(f, sheetName, headers, dataRows)

		results = append(results, models.SheetData{
			SheetName: sheetName,
			Headers:   headers,
			Rows:      dataRows,
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("xlsx file has no sheets with data")
	}

	return results, nil
}

// skipEmptyRows removes leading and trailing empty rows.
func skipEmptyRows(rows [][]string) [][]string {
	// Skip leading empty rows
	start := 0
	for start < len(rows) && allEmpty(rows[start]) {
		start++
	}

	// Skip trailing empty rows
	end := len(rows)
	for end > start && allEmpty(rows[end-1]) {
		end--
	}

	if start >= end {
		return nil
	}
	return rows[start:end]
}

func allEmpty(row []string) bool {
	for _, v := range row {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// normalizeRows pads or trims each row to match the header count.
func normalizeRows(rows [][]string, colCount int) [][]string {
	result := make([][]string, 0, len(rows))
	for _, row := range rows {
		if allEmpty(row) {
			continue
		}
		normalized := make([]string, colCount)
		for i := 0; i < colCount && i < len(row); i++ {
			normalized[i] = strings.TrimSpace(row[i])
		}
		result = append(result, normalized)
	}
	return result
}

// convertDateSerials detects and converts Excel date serial numbers to ISO date strings.
func convertDateSerials(f *excelize.File, sheet string, headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}

	// Check each column to see if it has date-formatted cells
	for colIdx := range headers {
		// Check the first non-empty data row for this column's cell style
		for rowIdx, row := range rows {
			if colIdx >= len(row) || row[colIdx] == "" {
				continue
			}

			cellName, err := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2) // +2: 1-based + header row
			if err != nil {
				break
			}

			styleIdx, err := f.GetCellStyle(sheet, cellName)
			if err != nil || styleIdx == 0 {
				break
			}

			style, err := f.GetStyle(styleIdx)
			if err != nil || style == nil {
				break
			}

			// Check if the number format looks like a date
			if isDateFormat(style.NumFmt) {
				// Convert all values in this column
				for ri, r := range rows {
					if colIdx >= len(r) || r[colIdx] == "" {
						continue
					}
					if converted, ok := excelSerialToDate(r[colIdx]); ok {
						rows[ri][colIdx] = converted
					}
				}
				break
			}
			break
		}
	}
}

func isDateFormat(numFmt int) bool {
	// Excel built-in date format codes
	return (numFmt >= 14 && numFmt <= 22) || (numFmt >= 27 && numFmt <= 36) || (numFmt >= 45 && numFmt <= 47)
}

// excelSerialToDate converts an Excel serial date number to ISO 8601 date string.
func excelSerialToDate(s string) (string, bool) {
	var serial float64
	_, err := fmt.Sscanf(s, "%f", &serial)
	if err != nil || serial < 1 || serial > 2958465 { // Max Excel date
		return "", false
	}

	// Excel epoch: January 0, 1900 (yes, zero — it's a known Excel bug)
	// Date serial 1 = January 1, 1900
	// Account for the Lotus 1-2-3 bug: Excel thinks 1900 was a leap year
	days := int(math.Floor(serial))
	if days > 59 { // After Feb 28, 1900
		days-- // Adjust for the non-existent Feb 29, 1900
	}

	epoch := time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC)
	date := epoch.AddDate(0, 0, days)
	return date.Format("2006-01-02"), true
}

// bytesReaderAt wraps a byte slice to implement io.ReaderAt.
type bytesReaderAt struct {
	data []byte
}

func newBytesReaderAt(data []byte) *bytesReaderAt {
	return &bytesReaderAt{data: data}
}

func (b *bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(b.data)) {
		return 0, io.EOF
	}
	n = copy(p, b.data[off:])
	if n < len(p) {
		err = io.EOF
	}
	return
}
