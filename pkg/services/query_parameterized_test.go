package services

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/audit"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// TestValidateRequiredParameters tests parameter validation logic.
func TestValidateRequiredParameters(t *testing.T) {
	svc := &queryService{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name          string
		paramDefs     []models.QueryParameter
		supplied      map[string]any
		expectedError string
	}{
		{
			name: "all required params provided",
			paramDefs: []models.QueryParameter{
				{Name: "customer_id", Type: "uuid", Required: true},
				{Name: "limit", Type: "integer", Required: false, Default: 100},
			},
			supplied:      map[string]any{"customer_id": "550e8400-e29b-41d4-a716-446655440000"},
			expectedError: "",
		},
		{
			name: "required param missing",
			paramDefs: []models.QueryParameter{
				{Name: "customer_id", Type: "uuid", Required: true},
			},
			supplied:      map[string]any{},
			expectedError: "required parameter 'customer_id' is missing",
		},
		{
			name: "required param nil but has default",
			paramDefs: []models.QueryParameter{
				{Name: "customer_id", Type: "uuid", Required: true, Default: "default-uuid"},
			},
			supplied:      map[string]any{"customer_id": nil},
			expectedError: "",
		},
		{
			name: "optional param missing is ok",
			paramDefs: []models.QueryParameter{
				{Name: "limit", Type: "integer", Required: false, Default: 100},
			},
			supplied:      map[string]any{},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.validateRequiredParameters(tt.paramDefs, tt.supplied)
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

// TestCoerceParameterTypes tests type coercion logic.
func TestCoerceParameterTypes(t *testing.T) {
	svc := &queryService{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name          string
		paramDefs     []models.QueryParameter
		supplied      map[string]any
		expected      map[string]any
		expectedError string
	}{
		{
			name: "string type coercion",
			paramDefs: []models.QueryParameter{
				{Name: "name", Type: "string"},
			},
			supplied: map[string]any{"name": "Alice"},
			expected: map[string]any{"name": "Alice"},
		},
		{
			name: "integer from float64",
			paramDefs: []models.QueryParameter{
				{Name: "count", Type: "integer"},
			},
			supplied: map[string]any{"count": float64(42)},
			expected: map[string]any{"count": int64(42)},
		},
		{
			name: "integer from string",
			paramDefs: []models.QueryParameter{
				{Name: "count", Type: "integer"},
			},
			supplied: map[string]any{"count": "123"},
			expected: map[string]any{"count": int64(123)},
		},
		{
			name: "decimal from int",
			paramDefs: []models.QueryParameter{
				{Name: "price", Type: "decimal"},
			},
			supplied: map[string]any{"price": 99},
			expected: map[string]any{"price": float64(99)},
		},
		{
			name: "boolean from string",
			paramDefs: []models.QueryParameter{
				{Name: "active", Type: "boolean"},
			},
			supplied: map[string]any{"active": "true"},
			expected: map[string]any{"active": true},
		},
		{
			name: "uuid validation",
			paramDefs: []models.QueryParameter{
				{Name: "id", Type: "uuid"},
			},
			supplied: map[string]any{"id": "550e8400-e29b-41d4-a716-446655440000"},
			expected: map[string]any{"id": "550e8400-e29b-41d4-a716-446655440000"},
		},
		{
			name: "invalid uuid",
			paramDefs: []models.QueryParameter{
				{Name: "id", Type: "uuid"},
			},
			supplied:      map[string]any{"id": "not-a-uuid"},
			expectedError: "invalid UUID format",
		},
		{
			name: "date validation",
			paramDefs: []models.QueryParameter{
				{Name: "start_date", Type: "date"},
			},
			supplied: map[string]any{"start_date": "2024-01-15"},
			expected: map[string]any{"start_date": "2024-01-15"},
		},
		{
			name: "invalid date",
			paramDefs: []models.QueryParameter{
				{Name: "start_date", Type: "date"},
			},
			supplied:      map[string]any{"start_date": "not-a-date"},
			expectedError: "invalid date format",
		},
		{
			name: "timestamp validation",
			paramDefs: []models.QueryParameter{
				{Name: "created_at", Type: "timestamp"},
			},
			supplied: map[string]any{"created_at": "2024-01-15T10:30:00Z"},
			expected: map[string]any{"created_at": "2024-01-15T10:30:00Z"},
		},
		{
			name: "string array",
			paramDefs: []models.QueryParameter{
				{Name: "tags", Type: "string[]"},
			},
			supplied: map[string]any{"tags": []interface{}{"foo", "bar"}},
			expected: map[string]any{"tags": []string{"foo", "bar"}},
		},
		{
			name: "integer array",
			paramDefs: []models.QueryParameter{
				{Name: "ids", Type: "integer[]"},
			},
			supplied: map[string]any{"ids": []interface{}{float64(1), float64(2), float64(3)}},
			expected: map[string]any{"ids": []int64{1, 2, 3}},
		},
		{
			name: "unknown parameter",
			paramDefs: []models.QueryParameter{
				{Name: "known", Type: "string"},
			},
			supplied:      map[string]any{"unknown": "value"},
			expectedError: "unknown parameter 'unknown'",
		},
		{
			name: "nil values skipped",
			paramDefs: []models.QueryParameter{
				{Name: "optional", Type: "string"},
			},
			supplied: map[string]any{"optional": nil},
			expected: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := svc.coerceParameterTypes(tt.paramDefs, tt.supplied)
			if tt.expectedError == "" {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

// TestValidateParameterizedQuery tests SQL template validation.
func TestValidateParameterizedQuery(t *testing.T) {
	svc := &queryService{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name          string
		sqlQuery      string
		params        []models.QueryParameter
		expectedError string
	}{
		{
			name:     "all params defined",
			sqlQuery: "SELECT * FROM users WHERE id = {{user_id}} AND active = {{is_active}}",
			params: []models.QueryParameter{
				{Name: "user_id", Type: "uuid"},
				{Name: "is_active", Type: "boolean"},
			},
			expectedError: "",
		},
		{
			name:     "param used but not defined",
			sqlQuery: "SELECT * FROM users WHERE id = {{user_id}}",
			params: []models.QueryParameter{
				{Name: "other_param", Type: "string"},
			},
			expectedError: "parameter {{user_id}} used in SQL but not defined",
		},
		{
			name:          "no params in SQL",
			sqlQuery:      "SELECT * FROM users",
			params:        []models.QueryParameter{},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ValidateParameterizedQuery(tt.sqlQuery, tt.params)
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

// TestGetClientIPFromContext tests the context IP extraction helper.
func TestGetClientIPFromContext(t *testing.T) {
	ctx := context.Background()
	ip := getClientIPFromContext(ctx)
	// For now, this returns empty string as it's a placeholder
	assert.Equal(t, "", ip)
}

// TestSecurityAuditorIntegration tests that the auditor is called on injection attempts.
func TestSecurityAuditorIntegration(t *testing.T) {
	// This test would require a mock auditor or integration test setup
	// For now, we'll just verify the auditor is created properly
	logger := zap.NewNop()
	auditor := audit.NewSecurityAuditor(logger)
	assert.NotNil(t, auditor)
}

// TestExecuteWithParameters_InjectionDetection would be an integration test
// that creates a full service and tests injection detection, but that requires
// database setup and is beyond the scope of unit tests.

// TestValidate_ParameterDetection tests that Validate returns early with a custom message
// when {{param}} placeholders are detected in the SQL.
func TestValidate_ParameterDetection(t *testing.T) {
	tests := []struct {
		name            string
		sqlQuery        string
		expectSkipDB    bool // true = should skip DB validation and return early
		expectedMessage string
	}{
		{
			name:            "single parameter - skips DB validation",
			sqlQuery:        "SELECT * FROM users WHERE status = {{status}}",
			expectSkipDB:    true,
			expectedMessage: "Parameters detected - full validation on Test Query",
		},
		{
			name:            "multiple parameters - skips DB validation",
			sqlQuery:        "SELECT * FROM users WHERE status = {{status}} LIMIT {{limit}} OFFSET {{offset}}",
			expectSkipDB:    true,
			expectedMessage: "Parameters detected - full validation on Test Query",
		},
		{
			name:            "parameter in WHERE clause - skips DB validation",
			sqlQuery:        "SELECT * FROM bookings WHERE host_id = {{host_id}} AND started_at > {{start_date}}",
			expectSkipDB:    true,
			expectedMessage: "Parameters detected - full validation on Test Query",
		},
		{
			name:            "no parameters - would need DB validation",
			sqlQuery:        "SELECT * FROM users WHERE status = 'active'",
			expectSkipDB:    false,
			expectedMessage: "", // Would be "SQL is valid" but requires DB
		},
		{
			name:            "no parameters with limit - would need DB validation",
			sqlQuery:        "SELECT * FROM users LIMIT 10",
			expectSkipDB:    false,
			expectedMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For tests that expect parameter detection (early return), we can test
			// without mocking the datasource service. For tests without parameters,
			// we'd need integration tests with a real DB.
			if tt.expectSkipDB {
				// Create minimal service - only needs logger for this code path
				// The parameter check happens before any DB calls
				svc := &queryService{
					logger: zap.NewNop(),
				}

				// Create context with mock user claims
				ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{
					RegisteredClaims: jwt.RegisteredClaims{
						Subject: "test-user-id",
					},
				})

				// Call Validate - datasourceID and projectID don't matter for this test
				// because we return early when parameters are detected
				result, err := svc.Validate(ctx, uuid.New(), uuid.New(), tt.sqlQuery)

				require.NoError(t, err)
				require.NotNil(t, result)
				assert.True(t, result.Valid)
				assert.Equal(t, tt.expectedMessage, result.Message)
			}
			// For non-parameter cases, skip - they require DB integration tests
		})
	}
}

// TestValidate_EmptySQL tests that Validate returns an error for empty SQL.
func TestValidate_EmptySQL(t *testing.T) {
	svc := &queryService{
		logger: zap.NewNop(),
	}

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "test-user-id",
		},
	})

	tests := []struct {
		name     string
		sqlQuery string
	}{
		{name: "empty string", sqlQuery: ""},
		{name: "whitespace only", sqlQuery: "   "},
		{name: "newlines only", sqlQuery: "\n\n"},
		{name: "tabs only", sqlQuery: "\t\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := svc.Validate(ctx, uuid.New(), uuid.New(), tt.sqlQuery)
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), "SQL query is required")
		})
	}
}

// TestValidate_NoUserContext tests that Validate returns error when user ID is missing.
func TestValidate_NoUserContext(t *testing.T) {
	svc := &queryService{
		logger: zap.NewNop(),
	}

	// Context without claims
	ctx := context.Background()

	result, err := svc.Validate(ctx, uuid.New(), uuid.New(), "SELECT 1")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "user ID not found in context")
}
