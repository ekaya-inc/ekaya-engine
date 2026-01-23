package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectSQLType(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected SQLStatementType
	}{
		// SELECT statements
		{
			name:     "simple SELECT",
			sql:      "SELECT * FROM users",
			expected: SQLTypeSelect,
		},
		{
			name:     "SELECT with lowercase",
			sql:      "select id, name from users",
			expected: SQLTypeSelect,
		},
		{
			name:     "SELECT with leading whitespace",
			sql:      "   SELECT * FROM users",
			expected: SQLTypeSelect,
		},

		// WITH (CTE) statements
		{
			name:     "simple CTE with SELECT",
			sql:      "WITH cte AS (SELECT * FROM users) SELECT * FROM cte",
			expected: SQLTypeSelect,
		},
		{
			name:     "nested CTE",
			sql:      "WITH a AS (SELECT 1), b AS (SELECT 2) SELECT * FROM a, b",
			expected: SQLTypeSelect,
		},
		{
			name:     "data-modifying CTE with DELETE",
			sql:      "WITH deleted AS (DELETE FROM users WHERE id = 1 RETURNING *) SELECT * FROM deleted",
			expected: SQLTypeUnknown,
		},
		{
			name:     "data-modifying CTE with INSERT",
			sql:      "WITH inserted AS (INSERT INTO users (name) VALUES ('test') RETURNING *) SELECT * FROM inserted",
			expected: SQLTypeUnknown,
		},
		{
			name:     "data-modifying CTE with UPDATE",
			sql:      "WITH updated AS (UPDATE users SET name = 'new' RETURNING *) SELECT * FROM updated",
			expected: SQLTypeUnknown,
		},

		// INSERT statements
		{
			name:     "simple INSERT",
			sql:      "INSERT INTO users (name) VALUES ('test')",
			expected: SQLTypeInsert,
		},
		{
			name:     "INSERT with RETURNING",
			sql:      "INSERT INTO users (name) VALUES ('test') RETURNING id, name",
			expected: SQLTypeInsert,
		},
		{
			name:     "INSERT ... SELECT",
			sql:      "INSERT INTO users (name) SELECT name FROM temp_users",
			expected: SQLTypeInsert,
		},

		// UPDATE statements
		{
			name:     "simple UPDATE",
			sql:      "UPDATE users SET name = 'new' WHERE id = 1",
			expected: SQLTypeUpdate,
		},
		{
			name:     "UPDATE with RETURNING",
			sql:      "UPDATE users SET name = 'new' WHERE id = 1 RETURNING *",
			expected: SQLTypeUpdate,
		},

		// DELETE statements
		{
			name:     "simple DELETE",
			sql:      "DELETE FROM users WHERE id = 1",
			expected: SQLTypeDelete,
		},
		{
			name:     "DELETE with RETURNING",
			sql:      "DELETE FROM users WHERE id = 1 RETURNING id",
			expected: SQLTypeDelete,
		},

		// CALL statements (stored procedures)
		{
			name:     "simple CALL",
			sql:      "CALL process_orders()",
			expected: SQLTypeCall,
		},
		{
			name:     "CALL with parameters",
			sql:      "CALL update_user_status($1, $2)",
			expected: SQLTypeCall,
		},

		// DDL statements (blocked)
		{
			name:     "CREATE TABLE",
			sql:      "CREATE TABLE users (id INT)",
			expected: SQLTypeDDL,
		},
		{
			name:     "ALTER TABLE",
			sql:      "ALTER TABLE users ADD COLUMN email VARCHAR(255)",
			expected: SQLTypeDDL,
		},
		{
			name:     "DROP TABLE",
			sql:      "DROP TABLE users",
			expected: SQLTypeDDL,
		},
		{
			name:     "TRUNCATE TABLE",
			sql:      "TRUNCATE TABLE users",
			expected: SQLTypeDDL,
		},

		// Transaction control (blocked)
		{
			name:     "BEGIN transaction",
			sql:      "BEGIN",
			expected: SQLTypeUnknown,
		},
		{
			name:     "COMMIT",
			sql:      "COMMIT",
			expected: SQLTypeUnknown,
		},
		{
			name:     "ROLLBACK",
			sql:      "ROLLBACK",
			expected: SQLTypeUnknown,
		},
		{
			name:     "SAVEPOINT",
			sql:      "SAVEPOINT my_savepoint",
			expected: SQLTypeUnknown,
		},

		// Unknown/unsupported statements
		{
			name:     "EXPLAIN",
			sql:      "EXPLAIN SELECT * FROM users",
			expected: SQLTypeUnknown,
		},
		{
			name:     "GRANT",
			sql:      "GRANT SELECT ON users TO app_user",
			expected: SQLTypeUnknown,
		},
		{
			name:     "empty string",
			sql:      "",
			expected: SQLTypeUnknown,
		},
		{
			name:     "whitespace only",
			sql:      "   \t\n",
			expected: SQLTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectSQLType(tt.sql)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsModifyingStatement(t *testing.T) {
	tests := []struct {
		name       string
		sqlType    SQLStatementType
		isModifying bool
	}{
		{"SELECT", SQLTypeSelect, false},
		{"INSERT", SQLTypeInsert, true},
		{"UPDATE", SQLTypeUpdate, true},
		{"DELETE", SQLTypeDelete, true},
		{"CALL", SQLTypeCall, true},
		{"DDL", SQLTypeDDL, false},
		{"UNKNOWN", SQLTypeUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsModifyingStatement(tt.sqlType)
			assert.Equal(t, tt.isModifying, result)
		})
	}
}

func TestValidateSQLType(t *testing.T) {
	tests := []struct {
		name               string
		sql                string
		allowsModification bool
		expectedType       SQLStatementType
		expectError        bool
		errorContains      string
	}{
		// Valid SELECT without flag
		{
			name:               "SELECT without flag",
			sql:                "SELECT * FROM users",
			allowsModification: false,
			expectedType:       SQLTypeSelect,
			expectError:        false,
		},
		// SELECT with flag (allowed, but flag will be auto-corrected elsewhere)
		{
			name:               "SELECT with flag",
			sql:                "SELECT * FROM users",
			allowsModification: true,
			expectedType:       SQLTypeSelect,
			expectError:        false,
		},
		// INSERT requires flag
		{
			name:               "INSERT without flag",
			sql:                "INSERT INTO users (name) VALUES ('test')",
			allowsModification: false,
			expectedType:       SQLTypeInsert,
			expectError:        true,
			errorContains:      "modifies data",
		},
		{
			name:               "INSERT with flag",
			sql:                "INSERT INTO users (name) VALUES ('test')",
			allowsModification: true,
			expectedType:       SQLTypeInsert,
			expectError:        false,
		},
		// UPDATE requires flag
		{
			name:               "UPDATE without flag",
			sql:                "UPDATE users SET name = 'new'",
			allowsModification: false,
			expectedType:       SQLTypeUpdate,
			expectError:        true,
			errorContains:      "modifies data",
		},
		{
			name:               "UPDATE with flag",
			sql:                "UPDATE users SET name = 'new'",
			allowsModification: true,
			expectedType:       SQLTypeUpdate,
			expectError:        false,
		},
		// DELETE requires flag
		{
			name:               "DELETE without flag",
			sql:                "DELETE FROM users WHERE id = 1",
			allowsModification: false,
			expectedType:       SQLTypeDelete,
			expectError:        true,
			errorContains:      "modifies data",
		},
		{
			name:               "DELETE with flag",
			sql:                "DELETE FROM users WHERE id = 1",
			allowsModification: true,
			expectedType:       SQLTypeDelete,
			expectError:        false,
		},
		// CALL requires flag
		{
			name:               "CALL without flag",
			sql:                "CALL process_orders()",
			allowsModification: false,
			expectedType:       SQLTypeCall,
			expectError:        true,
			errorContains:      "modifies data",
		},
		{
			name:               "CALL with flag",
			sql:                "CALL process_orders()",
			allowsModification: true,
			expectedType:       SQLTypeCall,
			expectError:        false,
		},
		// DDL is always blocked
		{
			name:               "DDL without flag",
			sql:                "CREATE TABLE test (id INT)",
			allowsModification: false,
			expectedType:       SQLTypeDDL,
			expectError:        true,
			errorContains:      "DDL statements",
		},
		{
			name:               "DDL with flag",
			sql:                "DROP TABLE test",
			allowsModification: true,
			expectedType:       SQLTypeDDL,
			expectError:        true,
			errorContains:      "DDL statements",
		},
		// Unknown is blocked
		{
			name:               "Unknown statement",
			sql:                "EXPLAIN SELECT * FROM users",
			allowsModification: false,
			expectedType:       SQLTypeUnknown,
			expectError:        true,
			errorContains:      "unrecognized SQL statement",
		},
		// Data-modifying CTE is blocked
		{
			name:               "Data-modifying CTE",
			sql:                "WITH deleted AS (DELETE FROM users RETURNING *) SELECT * FROM deleted",
			allowsModification: true,
			expectedType:       SQLTypeUnknown,
			expectError:        true,
			errorContains:      "unrecognized SQL statement",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlType, err := ValidateSQLType(tt.sql, tt.allowsModification)
			assert.Equal(t, tt.expectedType, sqlType)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				// Verify it's a SQLTypeError
				var sqlErr *SQLTypeError
				assert.ErrorAs(t, err, &sqlErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestShouldAutoCorrectAllowsModification(t *testing.T) {
	tests := []struct {
		name               string
		sqlType            SQLStatementType
		allowsModification bool
		shouldCorrect      bool
	}{
		// SELECT with flag should be corrected
		{
			name:               "SELECT with flag",
			sqlType:            SQLTypeSelect,
			allowsModification: true,
			shouldCorrect:      true,
		},
		// SELECT without flag - no correction needed
		{
			name:               "SELECT without flag",
			sqlType:            SQLTypeSelect,
			allowsModification: false,
			shouldCorrect:      false,
		},
		// Modifying statements should not be corrected
		{
			name:               "INSERT with flag",
			sqlType:            SQLTypeInsert,
			allowsModification: true,
			shouldCorrect:      false,
		},
		{
			name:               "UPDATE with flag",
			sqlType:            SQLTypeUpdate,
			allowsModification: true,
			shouldCorrect:      false,
		},
		{
			name:               "DELETE with flag",
			sqlType:            SQLTypeDelete,
			allowsModification: true,
			shouldCorrect:      false,
		},
		{
			name:               "CALL with flag",
			sqlType:            SQLTypeCall,
			allowsModification: true,
			shouldCorrect:      false,
		},
		// DDL and Unknown - technically won't reach here due to validation error
		{
			name:               "DDL with flag",
			sqlType:            SQLTypeDDL,
			allowsModification: true,
			shouldCorrect:      true, // non-modifying, so would correct
		},
		{
			name:               "Unknown with flag",
			sqlType:            SQLTypeUnknown,
			allowsModification: true,
			shouldCorrect:      true, // non-modifying, so would correct
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldAutoCorrectAllowsModification(tt.sqlType, tt.allowsModification)
			assert.Equal(t, tt.shouldCorrect, result)
		})
	}
}

func TestContainsModifyingCTE(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected bool
	}{
		{
			name:     "simple SELECT CTE",
			sql:      "WITH cte AS (SELECT * FROM users) SELECT * FROM cte",
			expected: false,
		},
		{
			name:     "DELETE in CTE",
			sql:      "WITH deleted AS (DELETE FROM users WHERE id = 1 RETURNING *) SELECT * FROM deleted",
			expected: true,
		},
		{
			name:     "INSERT in CTE",
			sql:      "WITH inserted AS (INSERT INTO users (name) VALUES ('test') RETURNING *) SELECT * FROM inserted",
			expected: true,
		},
		{
			name:     "UPDATE in CTE",
			sql:      "WITH updated AS (UPDATE users SET name = 'new' RETURNING *) SELECT * FROM updated",
			expected: true,
		},
		{
			name:     "lowercase DELETE in CTE",
			sql:      "with deleted as (delete from users where id = 1 returning *) select * from deleted",
			expected: true,
		},
		{
			name:     "CTE with word containing DELETE",
			sql:      "WITH cte AS (SELECT deleteable FROM users) SELECT * FROM cte",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsModifyingCTE(tt.sql)
			assert.Equal(t, tt.expected, result)
		})
	}
}
