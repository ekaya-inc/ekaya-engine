package database

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations executes pending database migrations from the specified directory.
// It is idempotent and safe to call multiple times - only pending migrations will be executed.
func RunMigrations(db *sql.DB, migrationsPath string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migration instance: %w", err)
	}

	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Printf("WARNING: Failed to close migration source: %v", srcErr)
		}
		if dbErr != nil {
			log.Printf("WARNING: Failed to close migration database: %v", dbErr)
		}
	}()

	err = m.Up()
	if err == migrate.ErrNoChange {
		log.Printf("No migrations to apply (database up-to-date)")
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	newVersion, _, _ := m.Version()
	log.Printf("Applied migrations successfully (now at version %d)", newVersion)
	return nil
}
