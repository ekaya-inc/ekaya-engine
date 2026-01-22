// cleanup-test-data removes test-like glossary terms from the database.
//
// Test patterns matched (case-insensitive):
// - ^test (starts with "test")
// - test$ (ends with "test")
// - ^uitest (UI test prefix)
// - ^debug (debug prefix)
// - ^todo (todo prefix)
// - ^fixme (fixme prefix)
// - ^dummy (dummy prefix)
// - ^sample (sample prefix)
// - ^example (example prefix)
// - \d{4}$ (ends with 4 digits, e.g., "Term2026")
//
// Usage: go run ./scripts/cleanup-test-data <project-id>
//
// Database connection: Uses standard PG* environment variables
//
// Flags:
//
//	-dry-run   Show what would be deleted without actually deleting (default: true)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// testTermPatterns defines regex patterns to identify test glossary terms.
// These patterns are used with PostgreSQL's ~* (case-insensitive regex) operator.
//
// IMPORTANT: Keep in sync with testTermPatterns in pkg/services/glossary_service.go
var testTermPatterns = []string{
	`^test`,    // Starts with "test"
	`test$`,    // Ends with "test"
	`^uitest`,  // UI test prefix
	`^debug`,   // Debug prefix
	`^todo`,    // Todo prefix
	`^fixme`,   // Fixme prefix
	`^dummy`,   // Dummy prefix
	`^sample`,  // Sample prefix
	`^example`, // Example prefix
	`\d{4}$`,   // Ends with 4 digits (year-like suffix)
}

func main() {
	dryRun := flag.Bool("dry-run", true, "Show what would be deleted without actually deleting")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [-dry-run=false] <project-id>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		fmt.Fprintf(os.Stderr, "  -dry-run  Show what would be deleted without deleting (default: true)\n")
		os.Exit(1)
	}

	projectID, err := uuid.Parse(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid project ID: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, buildConnString())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	// Set RLS context for project
	if _, err := conn.Exec(ctx, "SELECT set_config('app.current_project_id', $1, false)", projectID.String()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set RLS context: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		fmt.Println("DRY RUN - no changes will be made")
		fmt.Println("Run with -dry-run=false to actually delete terms")
		fmt.Println()
	}

	totalDeleted := 0
	for _, pattern := range testTermPatterns {
		count, err := cleanupTestGlossaryTerms(ctx, conn, projectID, pattern, *dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error cleaning pattern %q: %v\n", pattern, err)
			os.Exit(1)
		}
		totalDeleted += count
	}

	if *dryRun {
		fmt.Printf("\nTotal terms that would be deleted: %d\n", totalDeleted)
	} else {
		fmt.Printf("\nTotal terms deleted: %d\n", totalDeleted)
	}
}

// cleanupTestGlossaryTerms deletes glossary terms matching the given regex pattern.
// If dryRun is true, it only shows what would be deleted without making changes.
func cleanupTestGlossaryTerms(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID, pattern string, dryRun bool) (int, error) {
	if dryRun {
		// Show what would be deleted
		rows, err := conn.Query(ctx, `
			SELECT term, definition, source
			FROM engine_business_glossary
			WHERE project_id = $1
			  AND term ~* $2
		`, projectID, pattern)
		if err != nil {
			return 0, fmt.Errorf("query failed: %w", err)
		}
		defer rows.Close()

		var count int
		for rows.Next() {
			var term, definition, source string
			if err := rows.Scan(&term, &definition, &source); err != nil {
				return 0, fmt.Errorf("scan failed: %w", err)
			}
			count++
			fmt.Printf("  [%s] %q - %s (source: %s)\n", pattern, term, truncate(definition, 60), source)
		}
		if err := rows.Err(); err != nil {
			return 0, fmt.Errorf("rows iteration failed: %w", err)
		}

		if count == 0 {
			fmt.Printf("  [%s] No matching terms\n", pattern)
		}
		return count, nil
	}

	// Actually delete
	result, err := conn.Exec(ctx, `
		DELETE FROM engine_business_glossary
		WHERE project_id = $1
		  AND term ~* $2
	`, projectID, pattern)
	if err != nil {
		return 0, fmt.Errorf("delete failed: %w", err)
	}

	count := int(result.RowsAffected())
	fmt.Printf("Deleted %d terms matching pattern: %s\n", count, pattern)
	return count, nil
}

func buildConnString() string {
	host := getEnvOrDefault("PGHOST", "localhost")
	port := getEnvOrDefault("PGPORT", "5432")
	user := getEnvOrDefault("PGUSER", "postgres")
	password := os.Getenv("PGPASSWORD")
	dbname := getEnvOrDefault("PGDATABASE", "ekaya_engine")

	connStr := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		host, port, user, dbname)
	if password != "" {
		connStr += fmt.Sprintf(" password=%s", password)
	}
	return connStr
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// truncate shortens a string to maxLen characters, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
