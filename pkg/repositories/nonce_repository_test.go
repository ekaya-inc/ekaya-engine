//go:build integration

package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

type nonceRepositoryTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	repo      NonceRepository
	projectID uuid.UUID
}

func setupNonceRepositoryTest(t *testing.T) *nonceRepositoryTestContext {
	t.Helper()

	tc := &nonceRepositoryTestContext{
		t:         t,
		engineDB:  testhelpers.GetEngineDB(t),
		repo:      NewNonceRepository(),
		projectID: uuid.MustParse("00000000-0000-0000-0000-000000000033"),
	}
	tc.ensureTestProject()
	tc.cleanup()
	return tc
}

func (tc *nonceRepositoryTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Nonce Repository Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}
}

func (tc *nonceRepositoryTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create cleanup scope: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_nonces WHERE project_id = $1`, tc.projectID)
}

func (tc *nonceRepositoryTestContext) createTenantContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}

	return database.SetTenantScope(ctx, scope), func() {
		scope.Close()
	}
}

func (tc *nonceRepositoryTestContext) createGlobalContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create global scope: %v", err)
	}

	return database.SetTenantScope(ctx, scope), func() {
		scope.Close()
	}
}

func TestNonceRepository_CreateAndValidateAndConsume_Success(t *testing.T) {
	tc := setupNonceRepositoryTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTenantContext()
	defer cleanup()

	err := tc.repo.Create(ctx, "nonce-success", "install", tc.projectID, "app-1", time.Now().Add(15*time.Minute))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	valid, err := tc.repo.ValidateAndConsume(ctx, "nonce-success", "install", tc.projectID, "app-1")
	if err != nil {
		t.Fatalf("ValidateAndConsume failed: %v", err)
	}
	if !valid {
		t.Fatal("expected nonce to validate successfully")
	}

	valid, err = tc.repo.ValidateAndConsume(ctx, "nonce-success", "install", tc.projectID, "app-1")
	if err != nil {
		t.Fatalf("second ValidateAndConsume failed: %v", err)
	}
	if valid {
		t.Fatal("expected consumed nonce to be invalid on second validation")
	}
}

func TestNonceRepository_ValidateAndConsume_ExpiredNonceReturnsFalseAndDeletesRow(t *testing.T) {
	tc := setupNonceRepositoryTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTenantContext()
	defer cleanup()

	err := tc.repo.Create(ctx, "nonce-expired", "activate", tc.projectID, "app-1", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	valid, err := tc.repo.ValidateAndConsume(ctx, "nonce-expired", "activate", tc.projectID, "app-1")
	if err != nil {
		t.Fatalf("ValidateAndConsume failed: %v", err)
	}
	if valid {
		t.Fatal("expected expired nonce to be invalid")
	}

	globalCtx, globalCleanup := tc.createGlobalContext()
	defer globalCleanup()

	scope, ok := database.GetTenantScope(globalCtx)
	if !ok {
		t.Fatal("expected global tenant scope in context")
	}

	var count int
	err = scope.Conn.QueryRow(globalCtx, `SELECT COUNT(*) FROM engine_nonces WHERE nonce = $1`, "nonce-expired").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query nonce row count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected expired nonce row to be deleted, got %d rows", count)
	}
}

func TestNonceRepository_DeleteExpired_RemovesOnlyExpiredRows(t *testing.T) {
	tc := setupNonceRepositoryTest(t)
	defer tc.cleanup()

	tenantCtx, tenantCleanup := tc.createTenantContext()
	defer tenantCleanup()

	if err := tc.repo.Create(tenantCtx, "nonce-old", "install", tc.projectID, "app-1", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("failed to create expired nonce: %v", err)
	}
	if err := tc.repo.Create(tenantCtx, "nonce-fresh", "install", tc.projectID, "app-1", time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("failed to create fresh nonce: %v", err)
	}

	globalCtx, globalCleanup := tc.createGlobalContext()
	defer globalCleanup()

	deleted, err := tc.repo.DeleteExpired(globalCtx)
	if err != nil {
		t.Fatalf("DeleteExpired failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted nonce, got %d", deleted)
	}

	scope, ok := database.GetTenantScope(globalCtx)
	if !ok {
		t.Fatal("expected global tenant scope in context")
	}

	var count int
	err = scope.Conn.QueryRow(globalCtx, `SELECT COUNT(*) FROM engine_nonces WHERE project_id = $1`, tc.projectID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query remaining nonce rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 remaining nonce row, got %d", count)
	}
}
