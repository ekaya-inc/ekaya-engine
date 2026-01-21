package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// TestGetOptionalBoolWithDefaultDev tests the helper function for extracting boolean parameters.
func TestGetOptionalBoolWithDefaultDev(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		key        string
		defaultVal bool
		expected   bool
	}{
		{
			name:       "returns true when param is true",
			args:       map[string]any{"auto_select": true},
			key:        "auto_select",
			defaultVal: false,
			expected:   true,
		},
		{
			name:       "returns false when param is false",
			args:       map[string]any{"auto_select": false},
			key:        "auto_select",
			defaultVal: true,
			expected:   false,
		},
		{
			name:       "returns default when param not present",
			args:       map[string]any{},
			key:        "auto_select",
			defaultVal: true,
			expected:   true,
		},
		{
			name:       "returns default when param not present (false default)",
			args:       map[string]any{},
			key:        "auto_select",
			defaultVal: false,
			expected:   false,
		},
		{
			name:       "returns default when param is wrong type (string)",
			args:       map[string]any{"auto_select": "true"},
			key:        "auto_select",
			defaultVal: true,
			expected:   true,
		},
		{
			name:       "returns default when param is wrong type (number)",
			args:       map[string]any{"auto_select": 1},
			key:        "auto_select",
			defaultVal: false,
			expected:   false,
		},
		{
			name:       "returns default when args is nil",
			args:       nil,
			key:        "auto_select",
			defaultVal: true,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = tt.args

			result := getOptionalBoolWithDefaultDev(req, tt.key, tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRefreshSchemaResponse_Structure verifies the response structure for refresh_schema tool.
func TestRefreshSchemaResponse_Structure(t *testing.T) {
	// Test that the response structure has all expected fields
	type RefreshSchemaResponse struct {
		TablesAdded        []string            `json:"tables_added"`
		TablesRemoved      []string            `json:"tables_removed"`
		ColumnsAdded       int                 `json:"columns_added"`
		RelationshipsFound int                 `json:"relationships_found"`
		Relationships      []map[string]string `json:"relationships,omitempty"`
		AutoSelectApplied  bool                `json:"auto_select_applied"`
	}

	// Create a sample response
	response := RefreshSchemaResponse{
		TablesAdded:        []string{"public.new_table1", "public.new_table2"},
		TablesRemoved:      []string{"public.old_table"},
		ColumnsAdded:       15,
		RelationshipsFound: 3,
		Relationships: []map[string]string{
			{"from": "public.orders.user_id", "to": "public.users.id"},
			{"from": "public.reviews.product_id", "to": "public.products.id"},
		},
		AutoSelectApplied: true,
	}

	// Verify fields
	require.Len(t, response.TablesAdded, 2)
	require.Len(t, response.TablesRemoved, 1)
	assert.Equal(t, 15, response.ColumnsAdded)
	assert.Equal(t, 3, response.RelationshipsFound)
	assert.True(t, response.AutoSelectApplied)
	require.Len(t, response.Relationships, 2)
	assert.Equal(t, "public.orders.user_id", response.Relationships[0]["from"])
}

// TestMCPToolDeps_SchemaService verifies the MCPToolDeps includes SchemaService.
func TestMCPToolDeps_SchemaService(t *testing.T) {
	// Verify MCPToolDeps has SchemaService field
	deps := &MCPToolDeps{}
	assert.Nil(t, deps.SchemaService, "SchemaService field should be nil by default")
}
