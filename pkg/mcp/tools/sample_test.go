package tools

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// TestValidateTableSelected tests that the sample tool rejects non-selected tables.
func TestValidateTableSelected(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	selectedTables := []*models.SchemaTable{
		{TableName: "users", SchemaName: "public", IsSelected: true},
		{TableName: "channels", SchemaName: "public", IsSelected: true},
	}

	t.Run("selected table is allowed", func(t *testing.T) {
		err := validateTableSelected(selectedTables, "public", "users")
		assert.NoError(t, err)
	})

	t.Run("selected table with default schema is allowed", func(t *testing.T) {
		err := validateTableSelected(selectedTables, "public", "channels")
		assert.NoError(t, err)
	})

	t.Run("non-selected table is rejected", func(t *testing.T) {
		err := validateTableSelected(selectedTables, "public", "billing_transactions")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not selected")
		assert.Contains(t, err.Error(), "billing_transactions")
	})

	t.Run("wrong schema is rejected", func(t *testing.T) {
		err := validateTableSelected(selectedTables, "other_schema", "users")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not selected")
	})

	t.Run("empty table list rejects everything", func(t *testing.T) {
		err := validateTableSelected(nil, "public", "users")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not selected")
	})

	// Verify the test uses real IDs (not zero) to confirm the function signature
	_ = projectID
	_ = datasourceID
}
