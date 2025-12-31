package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"go.uber.org/zap"
)

func TestTableGraph_AddForeignKey(t *testing.T) {
	g := NewTableGraph()

	fk := datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetSchema: "public",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	g.AddForeignKey(fk)

	// Verify both tables are in the graph
	if !g.tables["public.orders"] {
		t.Error("Expected public.orders to be in graph")
	}
	if !g.tables["public.users"] {
		t.Error("Expected public.users to be in graph")
	}

	// Verify bidirectional edges exist
	if len(g.edges["public.orders"]) != 1 || g.edges["public.orders"][0] != "public.users" {
		t.Error("Expected edge from public.orders to public.users")
	}
	if len(g.edges["public.users"]) != 1 || g.edges["public.users"][0] != "public.orders" {
		t.Error("Expected edge from public.users to public.orders")
	}
}

func TestTableGraph_AddTable(t *testing.T) {
	g := NewTableGraph()

	g.AddTable("public", "standalone")

	// Verify table is in the graph
	if !g.tables["public.standalone"] {
		t.Error("Expected public.standalone to be in graph")
	}

	// Verify no edges
	if len(g.edges["public.standalone"]) != 0 {
		t.Error("Expected no edges for standalone table")
	}
}

func TestTableGraph_FindConnectedComponents_SingleComponent(t *testing.T) {
	g := NewTableGraph()
	logger := zap.NewNop()

	// Create a chain: users -> orders -> order_items
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetSchema: "public",
		TargetTable:  "users",
		TargetColumn: "id",
	})
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "order_items",
		SourceColumn: "order_id",
		TargetSchema: "public",
		TargetTable:  "orders",
		TargetColumn: "id",
	})

	components, islands := g.FindConnectedComponents(logger)

	// Should have 1 component with 3 tables
	if len(components) != 1 {
		t.Errorf("Expected 1 component, got %d", len(components))
	}
	if components[0].Size != 3 {
		t.Errorf("Expected component size 3, got %d", components[0].Size)
	}

	// Should have no islands
	if len(islands) != 0 {
		t.Errorf("Expected no islands, got %d", len(islands))
	}
}

func TestTableGraph_FindConnectedComponents_MultipleComponents(t *testing.T) {
	g := NewTableGraph()
	logger := zap.NewNop()

	// Component 1: users -> orders
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetSchema: "public",
		TargetTable:  "users",
		TargetColumn: "id",
	})

	// Component 2: products -> categories
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "products",
		SourceColumn: "category_id",
		TargetSchema: "public",
		TargetTable:  "categories",
		TargetColumn: "id",
	})

	components, islands := g.FindConnectedComponents(logger)

	// Should have 2 components
	if len(components) != 2 {
		t.Errorf("Expected 2 components, got %d", len(components))
	}

	// Each component should have 2 tables
	for i, comp := range components {
		if comp.Size != 2 {
			t.Errorf("Expected component %d size 2, got %d", i, comp.Size)
		}
	}

	// Should have no islands
	if len(islands) != 0 {
		t.Errorf("Expected no islands, got %d", len(islands))
	}
}

func TestTableGraph_FindConnectedComponents_WithIslands(t *testing.T) {
	g := NewTableGraph()
	logger := zap.NewNop()

	// Connected: users -> orders
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetSchema: "public",
		TargetTable:  "users",
		TargetColumn: "id",
	})

	// Islands: standalone tables
	g.AddTable("public", "audit_logs")
	g.AddTable("public", "config")
	g.AddTable("public", "sessions")

	components, islands := g.FindConnectedComponents(logger)

	// Should have 1 connected component
	if len(components) != 1 {
		t.Errorf("Expected 1 component, got %d", len(components))
	}
	if components[0].Size != 2 {
		t.Errorf("Expected component size 2, got %d", components[0].Size)
	}

	// Should have 3 islands
	if len(islands) != 3 {
		t.Errorf("Expected 3 islands, got %d", len(islands))
	}

	// Verify island names
	islandSet := make(map[string]bool)
	for _, island := range islands {
		islandSet[island] = true
	}
	expectedIslands := []string{"public.audit_logs", "public.config", "public.sessions"}
	for _, expected := range expectedIslands {
		if !islandSet[expected] {
			t.Errorf("Expected island %s not found", expected)
		}
	}
}

func TestTableGraph_FindConnectedComponents_ComplexGraph(t *testing.T) {
	g := NewTableGraph()
	logger := zap.NewNop()

	// Large component: users -> orders -> order_items -> products -> categories
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetSchema: "public",
		TargetTable:  "users",
		TargetColumn: "id",
	})
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "order_items",
		SourceColumn: "order_id",
		TargetSchema: "public",
		TargetTable:  "orders",
		TargetColumn: "id",
	})
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "order_items",
		SourceColumn: "product_id",
		TargetSchema: "public",
		TargetTable:  "products",
		TargetColumn: "id",
	})
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "products",
		SourceColumn: "category_id",
		TargetSchema: "public",
		TargetTable:  "categories",
		TargetColumn: "id",
	})

	// Small component: audit_logs -> audit_users
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "audit_logs",
		SourceColumn: "user_id",
		TargetSchema: "public",
		TargetTable:  "audit_users",
		TargetColumn: "id",
	})

	// Islands
	g.AddTable("public", "config")
	g.AddTable("public", "sessions")

	components, islands := g.FindConnectedComponents(logger)

	// Should have 2 components
	if len(components) != 2 {
		t.Errorf("Expected 2 components, got %d", len(components))
	}

	// First component should be larger (sorted by size)
	if components[0].Size != 5 {
		t.Errorf("Expected first component size 5, got %d", components[0].Size)
	}
	if components[1].Size != 2 {
		t.Errorf("Expected second component size 2, got %d", components[1].Size)
	}

	// Should have 2 islands
	if len(islands) != 2 {
		t.Errorf("Expected 2 islands, got %d", len(islands))
	}
}

func TestTableGraph_FindConnectedComponents_Cycle(t *testing.T) {
	g := NewTableGraph()
	logger := zap.NewNop()

	// Create a cycle: A -> B -> C -> A
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "table_a",
		SourceColumn: "b_id",
		TargetSchema: "public",
		TargetTable:  "table_b",
		TargetColumn: "id",
	})
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "table_b",
		SourceColumn: "c_id",
		TargetSchema: "public",
		TargetTable:  "table_c",
		TargetColumn: "id",
	})
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "public",
		SourceTable:  "table_c",
		SourceColumn: "a_id",
		TargetSchema: "public",
		TargetTable:  "table_a",
		TargetColumn: "id",
	})

	components, islands := g.FindConnectedComponents(logger)

	// Should have 1 component with 3 tables (cycle is still connected)
	if len(components) != 1 {
		t.Errorf("Expected 1 component, got %d", len(components))
	}
	if components[0].Size != 3 {
		t.Errorf("Expected component size 3, got %d", components[0].Size)
	}
	if len(islands) != 0 {
		t.Errorf("Expected no islands, got %d", len(islands))
	}
}

func TestTableGraph_FindConnectedComponents_EmptyGraph(t *testing.T) {
	g := NewTableGraph()
	logger := zap.NewNop()

	components, islands := g.FindConnectedComponents(logger)

	// Empty graph should have no components or islands
	if len(components) != 0 {
		t.Errorf("Expected 0 components, got %d", len(components))
	}
	if len(islands) != 0 {
		t.Errorf("Expected 0 islands, got %d", len(islands))
	}
}

func TestTableGraph_FindConnectedComponents_OnlyIslands(t *testing.T) {
	g := NewTableGraph()
	logger := zap.NewNop()

	// Add only standalone tables
	g.AddTable("public", "table1")
	g.AddTable("public", "table2")
	g.AddTable("public", "table3")

	components, islands := g.FindConnectedComponents(logger)

	// Should have no components (all are islands)
	if len(components) != 0 {
		t.Errorf("Expected 0 components, got %d", len(components))
	}

	// Should have 3 islands
	if len(islands) != 3 {
		t.Errorf("Expected 3 islands, got %d", len(islands))
	}
}

func TestTableGraph_FindConnectedComponents_MultipleSchemas(t *testing.T) {
	g := NewTableGraph()
	logger := zap.NewNop()

	// Component spanning schemas: public.users -> private.orders
	g.AddForeignKey(datasource.ForeignKeyMetadata{
		SourceSchema: "private",
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetSchema: "public",
		TargetTable:  "users",
		TargetColumn: "id",
	})

	// Island in different schema
	g.AddTable("archive", "old_data")

	components, islands := g.FindConnectedComponents(logger)

	// Should have 1 component with 2 tables from different schemas
	if len(components) != 1 {
		t.Errorf("Expected 1 component, got %d", len(components))
	}
	if components[0].Size != 2 {
		t.Errorf("Expected component size 2, got %d", components[0].Size)
	}

	// Should have 1 island
	if len(islands) != 1 {
		t.Errorf("Expected 1 island, got %d", len(islands))
	}
	if islands[0] != "archive.old_data" {
		t.Errorf("Expected island archive.old_data, got %s", islands[0])
	}
}

func TestLogConnectivity(t *testing.T) {
	// This is primarily a logging function, so we just verify it doesn't panic
	logger := zap.NewNop()

	components := []ConnectedComponent{
		{Tables: []string{"public.users", "public.orders", "public.order_items"}, Size: 3},
		{Tables: []string{"public.audit_logs", "public.audit_users"}, Size: 2},
	}
	islands := []string{"public.config", "public.sessions"}

	// Should not panic
	LogConnectivity(14, components, islands, logger)
}

func TestLogConnectivity_ManyTables(t *testing.T) {
	// Test with more than 5 tables to verify preview logic
	logger := zap.NewNop()

	tables := []string{
		"public.t1", "public.t2", "public.t3",
		"public.t4", "public.t5", "public.t6",
		"public.t7", "public.t8",
	}
	components := []ConnectedComponent{
		{Tables: tables, Size: len(tables)},
	}

	manyIslands := []string{
		"public.i1", "public.i2", "public.i3",
		"public.i4", "public.i5", "public.i6",
		"public.i7",
	}

	// Should not panic and should handle preview logic
	LogConnectivity(20, components, manyIslands, logger)
}
