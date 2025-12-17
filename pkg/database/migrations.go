package database

import (
	"database/sql"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"go.uber.org/zap"
)

// RunMigrations executes pending database migrations from the specified directory.
// It is idempotent and safe to call multiple times - only pending migrations will be executed.
func RunMigrations(db *sql.DB, migrationsPath string, logger *zap.Logger) error {
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
			logger.Warn("Failed to close migration source", zap.Error(srcErr))
		}
		if dbErr != nil {
			logger.Warn("Failed to close migration database", zap.Error(dbErr))
		}
	}()

	err = m.Up()
	if err == migrate.ErrNoChange {
		logger.Info("No migrations to apply (database up-to-date)")
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	newVersion, _, _ := m.Version()
	logger.Info("Applied migrations successfully", zap.Uint("version", newVersion))
	return nil
}
