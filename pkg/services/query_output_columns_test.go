package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// TestOutputColumnsModel tests that OutputColumn struct is correctly defined.
func TestOutputColumnsModel(t *testing.T) {
	tests := []struct {
		name string
		oc   models.OutputColumn
	}{
		{
			name: "basic output column",
			oc: models.OutputColumn{
				Name:        "customer_name",
				Type:        "string",
				Description: "Full name of the customer",
			},
		},
		{
			name: "integer output column",
			oc: models.OutputColumn{
				Name:        "total_orders",
				Type:        "integer",
				Description: "Total number of orders placed by the customer",
			},
		},
		{
			name: "decimal output column",
			oc: models.OutputColumn{
				Name:        "revenue",
				Type:        "decimal",
				Description: "Total revenue from customer orders",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify fields are accessible
			assert.NotEmpty(t, tt.oc.Name)
			assert.NotEmpty(t, tt.oc.Type)
			assert.NotEmpty(t, tt.oc.Description)
		})
	}
}

// TestCreateQueryRequestWithOutputColumns tests that CreateQueryRequest includes OutputColumns.
func TestCreateQueryRequestWithOutputColumns(t *testing.T) {
	outputCols := []models.OutputColumn{
		{
			Name:        "customer_name",
			Type:        "string",
			Description: "Full name of the customer",
		},
		{
			Name:        "total_revenue",
			Type:        "decimal",
			Description: "Total revenue from customer",
		},
	}

	req := CreateQueryRequest{
		NaturalLanguagePrompt: "Get customer revenue",
		SQLQuery:              "SELECT name, SUM(amount) FROM orders GROUP BY name",
		IsEnabled:             true,
		OutputColumns:         outputCols,
	}

	assert.Equal(t, 2, len(req.OutputColumns))
	assert.Equal(t, "customer_name", req.OutputColumns[0].Name)
	assert.Equal(t, "string", req.OutputColumns[0].Type)
	assert.Equal(t, "total_revenue", req.OutputColumns[1].Name)
	assert.Equal(t, "decimal", req.OutputColumns[1].Type)
}

// TestUpdateQueryRequestWithOutputColumns tests that UpdateQueryRequest includes OutputColumns.
func TestUpdateQueryRequestWithOutputColumns(t *testing.T) {
	outputCols := []models.OutputColumn{
		{
			Name:        "order_id",
			Type:        "uuid",
			Description: "Unique order identifier",
		},
	}

	req := UpdateQueryRequest{
		OutputColumns: &outputCols,
	}

	assert.NotNil(t, req.OutputColumns)
	assert.Equal(t, 1, len(*req.OutputColumns))
	assert.Equal(t, "order_id", (*req.OutputColumns)[0].Name)
}

// TestQueryModelWithOutputColumns tests that Query model includes OutputColumns.
func TestQueryModelWithOutputColumns(t *testing.T) {
	query := models.Query{
		NaturalLanguagePrompt: "Get customer revenue",
		SQLQuery:              "SELECT name, SUM(amount) FROM orders GROUP BY name",
		OutputColumns: []models.OutputColumn{
			{
				Name:        "customer_name",
				Type:        "string",
				Description: "Full name of the customer",
			},
			{
				Name:        "total_revenue",
				Type:        "decimal",
				Description: "Total revenue from customer",
			},
		},
	}

	assert.Equal(t, 2, len(query.OutputColumns))
	assert.Equal(t, "customer_name", query.OutputColumns[0].Name)
	assert.Equal(t, "total_revenue", query.OutputColumns[1].Name)
}

// TestQueryWithEmptyOutputColumns tests that OutputColumns can be empty.
func TestQueryWithEmptyOutputColumns(t *testing.T) {
	query := models.Query{
		NaturalLanguagePrompt: "Get customer revenue",
		SQLQuery:              "SELECT name, SUM(amount) FROM orders GROUP BY name",
		OutputColumns:         []models.OutputColumn{},
	}

	assert.Equal(t, 0, len(query.OutputColumns))
	assert.NotNil(t, query.OutputColumns)
}
