package database

import (
	"database/sql"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/migrations"
)

// RunMigrations executes pending database migrations from the embedded filesystem.
// It is idempotent and safe to call multiple times - only pending migrations will be executed.
func RunMigrations(db *sql.DB, logger *zap.Logger) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", driver)
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
