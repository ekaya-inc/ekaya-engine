package mssql

import (
	"fmt"
	"strings"
)

// parseSchemaTable parses a table name that may include schema.
// SQL Server format: [schema].[table] or schema.table
// Returns (schema, table). Defaults to "dbo" schema if not specified.
func parseSchemaTable(tableName string) (string, string) {
	// Remove brackets if present
	cleaned := strings.ReplaceAll(tableName, "[", "")
	cleaned = strings.ReplaceAll(cleaned, "]", "")

	parts := strings.Split(cleaned, ".")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}

	// Default schema is "dbo" in SQL Server
	return "dbo", cleaned
}

// escapeStringLiteral escapes a string for use in SQL Server string literals.
// In SQL Server, single quotes are escaped by doubling them.
func escapeStringLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// quoteName returns a SQL Server QUOTENAME expression for safe identifier handling.
// This is the equivalent of using QUOTENAME() function in SQL Server.
// For use in dynamic SQL, we build the QUOTENAME() call as a string.
func quoteName(identifier string) string {
	// QUOTENAME in SQL Server uses square brackets and escapes ] as ]]
	escaped := strings.ReplaceAll(identifier, "]", "]]")
	return fmt.Sprintf("[%s]", escaped)
}

// quoteNameForSQL returns a QUOTENAME() function call for use in SQL statements.
// This is safer than building the quote manually as it uses SQL Server's built-in function.
func quoteNameForSQL(identifier string) string {
	// Escape single quotes for the string literal inside QUOTENAME()
	escaped := escapeStringLiteral(identifier)
	return fmt.Sprintf("QUOTENAME(N'%s')", escaped)
}

// buildFullyQualifiedName builds a fully qualified table name: [schema].[table]
func buildFullyQualifiedName(schema, table string) string {
	return fmt.Sprintf("%s.%s", quoteName(schema), quoteName(table))
}

// mapSQLServerType maps SQL Server type names to standard type names.
// This provides a consistent interface across different database adapters.
func mapSQLServerType(sqlServerType string) string {
	sqlServerType = strings.ToUpper(sqlServerType)

	switch sqlServerType {
	// Integer types
	case "TINYINT":
		return "TINYINT"
	case "SMALLINT":
		return "SMALLINT"
	case "INT":
		return "INTEGER"
	case "BIGINT":
		return "BIGINT"

	// Decimal types
	case "DECIMAL", "NUMERIC":
		return "NUMERIC"
	case "MONEY", "SMALLMONEY":
		return "MONEY"
	case "FLOAT":
		return "DOUBLE PRECISION"
	case "REAL":
		return "REAL"

	// String types
	case "CHAR", "NCHAR":
		return "CHAR"
	case "VARCHAR", "NVARCHAR":
		return "VARCHAR"
	case "TEXT", "NTEXT":
		return "TEXT"

	// Binary types
	case "BINARY", "VARBINARY":
		return "BYTEA"
	case "IMAGE":
		return "BLOB"

	// Date/Time types
	case "DATE":
		return "DATE"
	case "TIME":
		return "TIME"
	case "DATETIME", "DATETIME2", "SMALLDATETIME":
		return "TIMESTAMP"
	case "DATETIMEOFFSET":
		return "TIMESTAMP WITH TIME ZONE"

	// Boolean
	case "BIT":
		return "BOOLEAN"

	// UUID/GUID
	case "UNIQUEIDENTIFIER":
		return "UUID"

	// JSON (SQL Server 2016+)
	case "JSON":
		return "JSON"

	// XML
	case "XML":
		return "XML"

	// Other types - return as-is
	default:
		return sqlServerType
	}
}

// isNumericType returns true if the type is a numeric type in SQL Server.
func isNumericType(sqlType string) bool {
	sqlType = strings.ToUpper(sqlType)
	numericTypes := []string{
		"TINYINT", "SMALLINT", "INT", "BIGINT",
		"DECIMAL", "NUMERIC", "MONEY", "SMALLMONEY",
		"FLOAT", "REAL",
	}

	for _, t := range numericTypes {
		if sqlType == t {
			return true
		}
	}
	return false
}

// isStringType returns true if the type is a string type in SQL Server.
func isStringType(sqlType string) bool {
	sqlType = strings.ToUpper(sqlType)
	stringTypes := []string{
		"CHAR", "NCHAR", "VARCHAR", "NVARCHAR",
		"TEXT", "NTEXT",
	}

	for _, t := range stringTypes {
		if sqlType == t {
			return true
		}
	}
	return false
}

// isDateTimeType returns true if the type is a date/time type in SQL Server.
func isDateTimeType(sqlType string) bool {
	sqlType = strings.ToUpper(sqlType)
	dateTimeTypes := []string{
		"DATE", "TIME", "DATETIME", "DATETIME2",
		"SMALLDATETIME", "DATETIMEOFFSET",
	}

	for _, t := range dateTimeTypes {
		if sqlType == t {
			return true
		}
	}
	return false
}
