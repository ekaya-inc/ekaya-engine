package services

import (
	"fmt"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"go.uber.org/zap"
)

// TableGraph represents a graph of tables connected by foreign key relationships.
type TableGraph struct {
	// Adjacency list: table -> list of tables it's connected to
	edges map[string][]string
	// All unique tables in the graph
	tables map[string]bool
}

// NewTableGraph creates a new empty table graph.
func NewTableGraph() *TableGraph {
	return &TableGraph{
		edges:  make(map[string][]string),
		tables: make(map[string]bool),
	}
}

// AddForeignKey adds a foreign key relationship to the graph.
// This creates an undirected edge between source and target tables.
func (g *TableGraph) AddForeignKey(fk datasource.ForeignKeyMetadata) {
	sourceTable := fmt.Sprintf("%s.%s", fk.SourceSchema, fk.SourceTable)
	targetTable := fmt.Sprintf("%s.%s", fk.TargetSchema, fk.TargetTable)

	// Add tables to the set
	g.tables[sourceTable] = true
	g.tables[targetTable] = true

	// Add undirected edges (both directions)
	g.edges[sourceTable] = append(g.edges[sourceTable], targetTable)
	g.edges[targetTable] = append(g.edges[targetTable], sourceTable)
}

// AddTable adds a table to the graph without any edges.
// Used to track tables that have no foreign key relationships.
func (g *TableGraph) AddTable(schema, table string) {
	fullName := fmt.Sprintf("%s.%s", schema, table)
	g.tables[fullName] = true
}

// ConnectedComponent represents a group of tables connected by foreign keys.
type ConnectedComponent struct {
	Tables []string
	Size   int
}

// FindConnectedComponents identifies all connected components in the graph using DFS.
// Returns a list of components sorted by size (largest first) and a list of island tables.
func (g *TableGraph) FindConnectedComponents(logger *zap.Logger) ([]ConnectedComponent, []string) {
	visited := make(map[string]bool)
	var components []ConnectedComponent

	// Run DFS from each unvisited table
	for table := range g.tables {
		if !visited[table] {
			component := g.dfs(table, visited)
			components = append(components, ConnectedComponent{
				Tables: component,
				Size:   len(component),
			})
		}
	}

	// Separate out island tables (components with size 1)
	var nonIslands []ConnectedComponent
	var islands []string

	for _, comp := range components {
		if comp.Size == 1 {
			islands = append(islands, comp.Tables[0])
		} else {
			nonIslands = append(nonIslands, comp)
		}
	}

	// Sort non-island components by size (largest first)
	// Simple bubble sort since we expect few components
	for i := 0; i < len(nonIslands); i++ {
		for j := i + 1; j < len(nonIslands); j++ {
			if nonIslands[j].Size > nonIslands[i].Size {
				nonIslands[i], nonIslands[j] = nonIslands[j], nonIslands[i]
			}
		}
	}

	return nonIslands, islands
}

// dfs performs depth-first search starting from a table.
// Returns all tables in the connected component.
func (g *TableGraph) dfs(start string, visited map[string]bool) []string {
	var component []string
	stack := []string{start}

	for len(stack) > 0 {
		// Pop from stack
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if visited[current] {
			continue
		}

		visited[current] = true
		component = append(component, current)

		// Add all neighbors to stack
		for _, neighbor := range g.edges[current] {
			if !visited[neighbor] {
				stack = append(stack, neighbor)
			}
		}
	}

	return component
}

// LogConnectivity logs the connectivity analysis results in a human-readable format.
func LogConnectivity(
	fkCount int,
	components []ConnectedComponent,
	islands []string,
	logger *zap.Logger,
) {
	logger.Info("Graph connectivity analysis:")
	logger.Info(fmt.Sprintf("  Foreign keys: %d relationships", fkCount))

	if len(components) > 0 {
		logger.Info("")
		for i, comp := range components {
			// Show first 5 tables, then "..."
			tableList := comp.Tables
			preview := tableList
			suffix := ""
			if len(tableList) > 5 {
				preview = tableList[:5]
				suffix = fmt.Sprintf(", ... (%d more)", len(tableList)-5)
			}

			logger.Info(fmt.Sprintf("  Component %d (%d tables): %v%s",
				i+1, comp.Size, preview, suffix))
		}
	}

	if len(islands) > 0 {
		// Show first 5 islands, then count
		preview := islands
		suffix := ""
		if len(islands) > 5 {
			preview = islands[:5]
			suffix = fmt.Sprintf(", ... (%d more)", len(islands)-5)
		}

		logger.Info(fmt.Sprintf("  Island tables (%d): %v%s", len(islands), preview, suffix))
	}

	logger.Info("")
	logger.Info(fmt.Sprintf("Summary: %d connected components, %d island tables need bridging",
		len(components), len(islands)))
}
