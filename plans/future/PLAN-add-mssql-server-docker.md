# PLAN: Add MSSQL Server Docker Support for Integration Testing

**Goal:** Enable MSSQL integration tests to run both locally (Mac/PC) and in CI/CD, in parallel with existing Postgres tests.

## Background

The MSSQL adapter has integration tests (`pkg/adapters/datasource/mssql/schema_test.go`) that currently require manual setup of environment variables and an external MSSQL server. We want these tests to run automatically like Postgres tests do.

### Current State

- **Postgres tests**: Use `testcontainers-go` with custom image `ghcr.io/ekaya-inc/ekaya-engine-test-image:latest`
- **MSSQL tests**: Require `MSSQL_HOST`, `MSSQL_USER`, `MSSQL_PASSWORD`, `MSSQL_DATABASE` env vars
- **Build tags**: MSSQL tests use `//go:build mssql || all_adapters`
- **CI/CD**: Runs unit tests only (`-short`), integration tests run locally via `make check`

### Target State

- MSSQL tests run automatically via `testcontainers-go` (same pattern as Postgres)
- Works on Mac (M1/M2 via Rosetta emulation), Windows, and Linux x64
- CI/CD can optionally run MSSQL integration tests
- No license cost (SQL Server Developer Edition)

## Technical Approach

### Container Image

Use official Microsoft SQL Server Developer Edition:
```
mcr.microsoft.com/mssql/server:2022-latest
```

Environment variables:
- `MSSQL_PID=Developer` - Free Developer Edition
- `ACCEPT_EULA=Y` - Required EULA acceptance
- `SA_PASSWORD=YourStrong!Passw0rd` - SA password (must meet complexity requirements)

### Architecture Considerations

| Platform | Native Support | Solution |
|----------|---------------|----------|
| Linux x64 | Yes | Direct container |
| Mac Intel | Yes | Direct container |
| Mac ARM (M1/M2) | No | Rosetta emulation in Docker Desktop |
| Windows x64 | Yes | Direct container |

**Note:** Mac ARM users need Docker Desktop with "Use Rosetta for x86_64/amd64 emulation" enabled in Settings → General.

## Implementation Steps

### Step 1: Add MSSQL Container Helper

**File:** `pkg/testhelpers/mssql_containers.go`

```go
//go:build mssql || all_adapters

package testhelpers

import (
    "context"
    "database/sql"
    "fmt"
    "sync"
    "testing"
    "time"

    _ "github.com/microsoft/go-mssqldb"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
)

const MSSQLTestImage = "mcr.microsoft.com/mssql/server:2022-latest"

type MSSQLTestDB struct {
    Container testcontainers.Container
    DB        *sql.DB
    ConnStr   string
    Host      string
    Port      string
}

var (
    sharedMSSQLDB     *MSSQLTestDB
    sharedMSSQLDBOnce sync.Once
    sharedMSSQLDBErr  error
)

func GetMSSQLTestDB(t *testing.T) *MSSQLTestDB {
    t.Helper()

    if testing.Short() {
        t.Skip("Skipping MSSQL integration test in short mode (requires Docker)")
    }

    sharedMSSQLDBOnce.Do(func() {
        sharedMSSQLDB, sharedMSSQLDBErr = setupMSSQLTestDB()
    })

    if sharedMSSQLDBErr != nil {
        t.Fatalf("Failed to setup MSSQL test database: %v", sharedMSSQLDBErr)
    }

    return sharedMSSQLDB
}

func setupMSSQLTestDB() (*MSSQLTestDB, error) {
    ctx := context.Background()

    req := testcontainers.ContainerRequest{
        Image:        MSSQLTestImage,
        ExposedPorts: []string{"1433/tcp"},
        Env: map[string]string{
            "MSSQL_PID":      "Developer",
            "ACCEPT_EULA":    "Y",
            "SA_PASSWORD":    "Test@Password123!",
        },
        WaitingFor: wait.ForSQL("1433/tcp", "sqlserver",
            func(host string, port nat.Port) string {
                return fmt.Sprintf("sqlserver://sa:Test@Password123!@%s:%s?database=master", host, port.Port())
            }).WithStartupTimeout(120 * time.Second), // MSSQL takes longer to start
    }

    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to start MSSQL container: %w", err)
    }

    host, err := container.Host(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to get container host: %w", err)
    }

    port, err := container.MappedPort(ctx, "1433")
    if err != nil {
        return nil, fmt.Errorf("failed to get container port: %w", err)
    }

    connStr := fmt.Sprintf("sqlserver://sa:Test@Password123!@%s:%s?database=master",
        host, port.Port())

    db, err := sql.Open("sqlserver", connStr)
    if err != nil {
        return nil, fmt.Errorf("failed to open connection: %w", err)
    }

    // Verify connection with retry (MSSQL needs time after reporting ready)
    for i := 0; i < 30; i++ {
        if err := db.PingContext(ctx); err == nil {
            break
        }
        time.Sleep(2 * time.Second)
    }

    return &MSSQLTestDB{
        Container: container,
        DB:        db,
        ConnStr:   connStr,
        Host:      host,
        Port:      port.Port(),
    }, nil
}
```

### Step 2: Update MSSQL Tests to Use Container

**File:** `pkg/adapters/datasource/mssql/schema_test.go`

Update `setupSchemaDiscovererTest` to use the testcontainer instead of environment variables:

```go
func setupSchemaDiscovererTest(t *testing.T) *schemaTestContext {
    t.Helper()

    // Use testcontainer helper
    mssqlDB := testhelpers.GetMSSQLTestDB(t)

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

    logger := zaptest.NewLogger(t)
    cfg := &Config{
        Host:       mssqlDB.Host,
        Port:       1433, // Parse from mssqlDB.Port
        Database:   "master",
        AuthMethod: "sql",
        Username:   "sa",
        Password:   "Test@Password123!",
        Encrypt:    false,
    }

    discoverer, err := NewSchemaDiscoverer(ctx, cfg, nil, uuid.Nil, uuid.Nil, "", logger)
    require.NoError(t, err, "failed to create schema discoverer")

    return &schemaTestContext{
        discoverer: discoverer,
        cleanup: func() {
            cancel()
            discoverer.Close()
        },
    }
}
```

### Step 3: Add Makefile Targets

**File:** `Makefile`

```makefile
# MSSQL test targets
test-mssql: ## Run MSSQL integration tests (requires Docker)
	@echo "$(YELLOW)Running MSSQL integration tests...$(NC)"
	@go test -tags="mssql" ./pkg/adapters/datasource/mssql/... -timeout 5m
	@echo "$(GREEN)✓ MSSQL tests passed$(NC)"

test-all-adapters: ## Run all adapter integration tests (Postgres + MSSQL)
	@echo "$(YELLOW)Running all adapter integration tests...$(NC)"
	@go test -tags="all_adapters" ./... -timeout 10m
	@echo "$(GREEN)✓ All adapter tests passed$(NC)"
```

### Step 4: Update CI/CD (Optional)

**File:** `.github/workflows/pr-checks.yml`

If we want MSSQL tests in CI, add a separate job:

```yaml
  mssql-tests:
    name: MSSQL Integration Tests
    runs-on: ubuntu-latest
    # Only run on main branch or when explicitly requested
    if: github.ref == 'refs/heads/main' || contains(github.event.pull_request.labels.*.name, 'test-mssql')

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Run MSSQL tests
        run: go test -tags="mssql" ./pkg/adapters/datasource/mssql/... -timeout 5m
```

### Step 5: Documentation

**File:** `CLAUDE.md`

Add section on running MSSQL tests:

```markdown
### MSSQL Integration Tests

MSSQL tests require Docker. On Mac ARM (M1/M2), enable Rosetta emulation:
- Docker Desktop → Settings → General → "Use Rosetta for x86_64/amd64 emulation"

```bash
# Run MSSQL tests only
make test-mssql

# Run all adapter tests (Postgres + MSSQL)
make test-all-adapters
```
```

## Optional: Custom MSSQL Test Image

If we need pre-loaded test data (like the Postgres test image), create:

**Directory:** `test/docker/mssql-test-db/`

```dockerfile
FROM mcr.microsoft.com/mssql/server:2022-latest

ENV MSSQL_PID=Developer
ENV ACCEPT_EULA=Y
ENV SA_PASSWORD=Test@Password123!

# Copy initialization scripts
COPY init-db.sql /docker-entrypoint-initdb.d/

# Custom entrypoint that runs init scripts after MSSQL starts
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
```

This would allow pre-loading test schemas and data, similar to the Postgres test image.

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `pkg/testhelpers/mssql_containers.go` | Create | MSSQL testcontainer helper |
| `pkg/adapters/datasource/mssql/schema_test.go` | Modify | Use testcontainer instead of env vars |
| `Makefile` | Modify | Add `test-mssql` and `test-all-adapters` targets |
| `CLAUDE.md` | Modify | Document MSSQL test requirements |
| `.github/workflows/pr-checks.yml` | Modify (optional) | Add MSSQL test job |

## Success Criteria

- [ ] `make test-mssql` runs MSSQL tests with auto-started container
- [ ] Tests pass on Mac Intel
- [ ] Tests pass on Mac ARM (M1/M2) with Rosetta enabled
- [ ] Tests pass on Linux x64 (CI/CD runner)
- [ ] MSSQL container starts in parallel with Postgres (no conflicts)
- [ ] Tests skip gracefully when Docker unavailable (`-short` mode)
- [ ] No license required (Developer Edition)

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| MSSQL startup is slow (~30-60s) | Use `sync.Once` to share container across tests |
| Mac ARM emulation overhead | Performance not critical for CI; documented requirement |
| Large image size (~1.5GB) | First pull is slow; subsequent runs use cache |
| SA password complexity | Use password that meets requirements |

## Notes

- SQL Server Developer Edition is free for non-production use (CI/CD, testing, development)
- The `testcontainers-go` library handles container lifecycle automatically
- Tests will skip gracefully if Docker is unavailable or in `-short` mode
- MSSQL and Postgres containers run independently (different ports, different `sync.Once`)
