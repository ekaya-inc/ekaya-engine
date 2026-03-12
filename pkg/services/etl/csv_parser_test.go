package etl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCSV_CommaSeparated(t *testing.T) {
	data := []byte("name,age,city\nAlice,30,NYC\nBob,25,LA\n")

	result, err := ParseCSV(data)
	require.NoError(t, err)

	assert.Equal(t, []string{"name", "age", "city"}, result.Headers)
	assert.Len(t, result.Rows, 2)
	assert.Equal(t, []string{"Alice", "30", "NYC"}, result.Rows[0])
	assert.Equal(t, ',', result.Delimiter)
}

func TestParseCSV_TabSeparated(t *testing.T) {
	data := []byte("name\tage\tcity\nAlice\t30\tNYC\nBob\t25\tLA\n")

	result, err := ParseCSV(data)
	require.NoError(t, err)

	assert.Equal(t, []string{"name", "age", "city"}, result.Headers)
	assert.Len(t, result.Rows, 2)
	assert.Equal(t, '\t', result.Delimiter)
}

func TestParseCSV_SemicolonSeparated(t *testing.T) {
	data := []byte("name;age;city\nAlice;30;NYC\nBob;25;LA\n")

	result, err := ParseCSV(data)
	require.NoError(t, err)

	assert.Equal(t, []string{"name", "age", "city"}, result.Headers)
	assert.Len(t, result.Rows, 2)
	assert.Equal(t, ';', result.Delimiter)
}

func TestParseCSV_PipeSeparated(t *testing.T) {
	data := []byte("name|age|city\nAlice|30|NYC\nBob|25|LA\n")

	result, err := ParseCSV(data)
	require.NoError(t, err)

	assert.Equal(t, []string{"name", "age", "city"}, result.Headers)
	assert.Equal(t, '|', result.Delimiter)
}

func TestParseCSV_EmptyFile(t *testing.T) {
	_, err := ParseCSV([]byte(""))
	assert.Error(t, err)
}

func TestParseCSV_HeaderOnly(t *testing.T) {
	data := []byte("name,age,city\n")

	result, err := ParseCSV(data)
	require.NoError(t, err)

	assert.Equal(t, []string{"name", "age", "city"}, result.Headers)
	assert.Empty(t, result.Rows)
}

func TestParseCSV_QuotedFields(t *testing.T) {
	data := []byte(`name,description,price
"Widget","A nice, shiny widget",10.99
"Gadget","A ""cool"" gadget",24.99
`)

	result, err := ParseCSV(data)
	require.NoError(t, err)

	assert.Len(t, result.Rows, 2)
	assert.Equal(t, "A nice, shiny widget", result.Rows[0][1])
	assert.Equal(t, `A "cool" gadget`, result.Rows[1][1])
}

func TestDetectDelimiter_PreferMoreColumns(t *testing.T) {
	// Both comma and semicolon produce consistent counts,
	// but semicolon produces more columns
	data := []byte("a;b;c;d\n1;2;3;4\n5;6;7;8\n")
	assert.Equal(t, ';', detectDelimiter(data))
}

func TestDetectDelimiter_FallbackToComma(t *testing.T) {
	// Single column, no delimiter detected
	data := []byte("hello\nworld\n")
	assert.Equal(t, ',', detectDelimiter(data))
}
