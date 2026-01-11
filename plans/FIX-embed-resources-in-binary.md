# FIX: Embed Resources in Binary

## Problem

ekaya-engine requires external directories at runtime:
- `./migrations/*.sql` - Database migrations
- `./ui/dist/*` - Frontend static files

This means the binary cannot be distributed standalone. Running from a different directory fails because paths are relative.

**Example failure:**
```
cd /some/other/project
/path/to/ekaya-engine  # Hangs on migrations - can't find ./migrations/
```

## Solution

Use Go's `//go:embed` directive to embed resources at compile time.

### 1. Embed Migrations

Create `migrations/embed.go`:
```go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

Update `pkg/database/migrations.go`:
```go
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
```

Remove unused import in `pkg/database/migrations.go`:
```diff
- _ "github.com/golang-migrate/migrate/v4/source/file"
```

### 2. Embed UI Static Files

Create `ui/embed.go` (NOT in `ui/dist/` since that's gitignored):
```go
package ui

import "embed"

//go:embed dist
var DistFS embed.FS
```

**Note:** Using `//go:embed dist` (without `/*`) embeds the entire directory tree including subdirectories. The paths will be `dist/index.html`, `dist/assets/...`, etc.

### 3. Update main.go

Update `runMigrations` function call (line 622):
```diff
 func runMigrations(stdDB *sql.DB, logger *zap.Logger) error {
-	return database.RunMigrations(stdDB, "./migrations", logger)
+	return database.RunMigrations(stdDB, logger)
 }
```

Update UI serving (lines 489-516):
```go
import (
	"io/fs"
	// ... existing imports ...
	"github.com/ekaya-inc/ekaya-engine/ui"
)

// Replace the current UI serving block with:

// Serve static UI files from embedded filesystem with SPA routing
uiFS, err := fs.Sub(ui.DistFS, "dist")
if err != nil {
	logger.Fatal("Failed to create UI filesystem", zap.Error(err))
}
fileServer := http.FileServer(http.FS(uiFS))

// Read index.html once at startup for SPA fallback
indexHTML, err := fs.ReadFile(uiFS, "index.html")
if err != nil {
	logger.Fatal("Failed to read index.html from embedded filesystem", zap.Error(err))
}

// Handle SPA routing - serve index.html for non-API routes when file doesn't exist
mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	// Don't serve index.html for API routes
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	// Check if the file exists in embedded filesystem
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	if _, err := fs.Stat(uiFS, path); err == nil {
		// File exists, serve it
		fileServer.ServeHTTP(w, r)
		return
	}

	// File doesn't exist, serve index.html for SPA routing
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
})
```

### 4. Update Dockerfile

The final stage no longer needs to copy UI files separately since they're embedded:

```diff
 # Copy binary from builder
 COPY --from=builder /app/ekaya-engine /usr/local/bin/ekaya-engine

-# Copy built UI files
-COPY --from=ui-builder /app/ui/dist /app/ui/dist
-
 # Switch to non-root user
 USER ekaya
```

The migrations directory is also no longer needed in the final image.

### 5. Build Order Requirement

The build process must ensure `ui/dist` exists before `go build`. The current Makefile already handles this correctly:
- `make run`, `make build`, `make dev-server` all run `npm run build` before `go build`
- Dockerfile builds UI first, then copies to Go builder stage before `go build`

No Makefile changes required.

## Implementation Status

### Task 1: Embed Migrations - [x] COMPLETE

**Files Modified:**
- ✅ `migrations/embed.go` - Created with `//go:embed *.sql` directive
- ✅ `pkg/database/migrations.go` - Updated to use `iofs.New(migrations.FS, ".")` instead of file:// source
- ✅ `main.go` - Removed `migrationsPath` parameter from `runMigrations()` call
- ✅ `pkg/testhelpers/containers.go` - Removed runtime.Caller() path resolution, uses embedded FS

**Implementation Notes:**
- The `RunMigrations` function signature changed from `(db, path, logger)` to `(db, logger)`
- Tests no longer need to compute relative paths to migrations directory
- The migration driver setup uses `iofs.New()` from `github.com/golang-migrate/migrate/v4/source/iofs`
- Removed unused import: `_ "github.com/golang-migrate/migrate/v4/source/file"`

**Testing:**
- Integration tests pass (verified via `make test-short`)
- Migrations run successfully from embedded filesystem
- No path resolution issues in test helpers

### Task 2: Embed UI Static Files - [ ] TODO

**Files to Modify:**
1. `ui/embed.go` - **NEW**
2. `main.go` - Update UI serving to use embedded FS
3. `Dockerfile` - Remove UI files COPY in final stage (optional optimization)

## Expected Outcome

After this fix:
```bash
# Binary is fully self-contained
./bin/ekaya-engine

# Can run from anywhere with just config
cd /any/directory
/path/to/ekaya-engine  # Works! Uses embedded migrations + UI
```

Only `config.yaml` (optional) and environment variables are needed at runtime.

## Testing

1. Build: `make build`
2. Move binary to temp location: `cp bin/ekaya-engine /tmp/`
3. Run from different directory: `cd /tmp && ./ekaya-engine`
4. Verify:
   - Migrations run successfully
   - UI loads at http://localhost:3443
   - SPA routing works (navigate to /projects, refresh page)
