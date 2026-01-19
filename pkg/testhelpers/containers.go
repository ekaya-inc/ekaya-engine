package testhelpers

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver for database/sql (migrations)
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
)

// EngineTestImage is the custom PostgreSQL image with pre-loaded test schema.
const EngineTestImage = "ghcr.io/ekaya-inc/ekaya-engine-test-image:latest"

// TestDB holds a shared test database container and connection pool.
type TestDB struct {
	Container testcontainers.Container
	Pool      *pgxpool.Pool
	ConnStr   string
}

var (
	sharedTestDB     *TestDB
	sharedTestDBOnce sync.Once
	sharedTestDBErr  error
)

// GetTestDB returns a shared PostgreSQL container for integration tests.
// The container is created once and reused across all tests in the run.
// Uses the ekaya-engine-test-image with pre-loaded test schema.
func GetTestDB(t *testing.T) *TestDB {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode (requires Docker)")
	}

	sharedTestDBOnce.Do(func() {
		sharedTestDB, sharedTestDBErr = setupTestDB()
	})

	if sharedTestDBErr != nil {
		t.Fatalf("Failed to setup test database: %v", sharedTestDBErr)
	}

	return sharedTestDB
}

func setupTestDB() (*TestDB, error) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        EngineTestImage,
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "test_data",
			"POSTGRES_USER":     "ekaya",
			"POSTGRES_PASSWORD": "test_password",
		},
		WaitingFor: wait.ForLog("EKAYA_TEST_DB_READY").
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start test container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		return nil, fmt.Errorf("failed to get container port: %w", err)
	}

	connStr := fmt.Sprintf("postgres://ekaya:test_password@%s:%s/test_data?sslmode=disable",
		host, port.Port())

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection with retry
	for i := 0; i < 10; i++ {
		if err := pool.Ping(ctx); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	return &TestDB{
		Container: container,
		Pool:      pool,
		ConnStr:   connStr,
	}, nil
}

// EngineDB holds the engine database connection with migrations applied.
// Use this for testing handlers, services, and repositories against a real database.
type EngineDB struct {
	DB      *database.DB
	ConnStr string
}

var (
	sharedEngineDB     *EngineDB
	sharedEngineDBOnce sync.Once
	sharedEngineDBErr  error
)

// GetEngineDB returns a shared engine database for integration tests.
// The database has migrations applied and is reused across all tests.
// Uses the ekaya_engine_test database from the test container.
func GetEngineDB(t *testing.T) *EngineDB {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode (requires Docker)")
	}

	// Ensure test container is running first
	testDB := GetTestDB(t)

	sharedEngineDBOnce.Do(func() {
		sharedEngineDB, sharedEngineDBErr = setupEngineDB(testDB)
	})

	if sharedEngineDBErr != nil {
		t.Fatalf("Failed to setup engine database: %v", sharedEngineDBErr)
	}

	return sharedEngineDB
}

func setupEngineDB(testDB *TestDB) (*EngineDB, error) {
	ctx := context.Background()

	// Get container host and port from the existing TestDB
	host, err := testDB.Container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := testDB.Container.MappedPort(ctx, "5432")
	if err != nil {
		return nil, fmt.Errorf("failed to get container port: %w", err)
	}

	// Connect to ekaya_engine_test database
	connStr := fmt.Sprintf("postgres://ekaya:test_password@%s:%s/ekaya_engine_test?sslmode=disable",
		host, port.Port())

	db, err := database.NewConnection(ctx, &database.Config{
		URL:            connStr,
		MaxConnections: 5,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to engine database: %w", err)
	}

	// Run migrations using database/sql (required by golang-migrate)
	sqlDB, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open sql connection: %w", err)
	}
	defer sqlDB.Close()

	if err := database.RunMigrations(sqlDB, zap.NewNop()); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &EngineDB{
		DB:      db,
		ConnStr: connStr,
	}, nil
}
