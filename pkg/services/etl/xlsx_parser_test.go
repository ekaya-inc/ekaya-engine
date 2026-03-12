package etl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestParseXLSX_BasicData(t *testing.T) {
	// Create a test XLSX file in memory
	f := excelize.NewFile()
	defer f.Close()

	// Write headers and data
	f.SetCellValue("Sheet1", "A1", "Name")
	f.SetCellValue("Sheet1", "B1", "Age")
	f.SetCellValue("Sheet1", "C1", "City")
	f.SetCellValue("Sheet1", "A2", "Alice")
	f.SetCellValue("Sheet1", "B2", 30)
	f.SetCellValue("Sheet1", "C2", "NYC")
	f.SetCellValue("Sheet1", "A3", "Bob")
	f.SetCellValue("Sheet1", "B3", 25)
	f.SetCellValue("Sheet1", "C3", "LA")

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)

	sheets, err := ParseXLSXFromBytes(buf.Bytes())
	require.NoError(t, err)
	require.Len(t, sheets, 1)

	assert.Equal(t, "Sheet1", sheets[0].SheetName)
	assert.Equal(t, []string{"Name", "Age", "City"}, sheets[0].Headers)
	assert.Len(t, sheets[0].Rows, 2)
	assert.Equal(t, "Alice", sheets[0].Rows[0][0])
	assert.Equal(t, "30", sheets[0].Rows[0][1])
}

func TestParseXLSX_MultipleSheets(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	// Sheet1
	f.SetCellValue("Sheet1", "A1", "Name")
	f.SetCellValue("Sheet1", "A2", "Alice")

	// Sheet2
	_, err := f.NewSheet("Sheet2")
	require.NoError(t, err)
	f.SetCellValue("Sheet2", "A1", "Product")
	f.SetCellValue("Sheet2", "A2", "Widget")

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)

	sheets, err := ParseXLSXFromBytes(buf.Bytes())
	require.NoError(t, err)
	require.Len(t, sheets, 2)

	assert.Equal(t, "Sheet1", sheets[0].SheetName)
	assert.Equal(t, "Sheet2", sheets[1].SheetName)
}

func TestParseXLSX_EmptySheet(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	// Sheet1 has data
	f.SetCellValue("Sheet1", "A1", "Name")
	f.SetCellValue("Sheet1", "A2", "Alice")

	// Sheet2 is empty
	_, err := f.NewSheet("Sheet2")
	require.NoError(t, err)

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)

	sheets, err := ParseXLSXFromBytes(buf.Bytes())
	require.NoError(t, err)
	// Empty sheet should be skipped
	require.Len(t, sheets, 1)
	assert.Equal(t, "Sheet1", sheets[0].SheetName)
}

func TestParseXLSX_VariableRowLengths(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	f.SetCellValue("Sheet1", "A1", "Col1")
	f.SetCellValue("Sheet1", "B1", "Col2")
	f.SetCellValue("Sheet1", "C1", "Col3")
	f.SetCellValue("Sheet1", "A2", "a")
	// Row 2 only has 1 column — should be padded

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)

	sheets, err := ParseXLSXFromBytes(buf.Bytes())
	require.NoError(t, err)
	require.Len(t, sheets, 1)
	require.Len(t, sheets[0].Rows, 1)
	// Row should be padded to 3 columns
	assert.Len(t, sheets[0].Rows[0], 3)
	assert.Equal(t, "a", sheets[0].Rows[0][0])
	assert.Equal(t, "", sheets[0].Rows[0][1])
	assert.Equal(t, "", sheets[0].Rows[0][2])
}

func TestSanitizeTableName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"sales_report.csv", "sales_report"},
		{"My Data File.xlsx", "my_data_file"},
		{"123-orders.tsv", "_123_orders"},
		{".csv", "imported_data"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeTableName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
