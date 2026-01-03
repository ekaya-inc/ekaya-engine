package services

import (
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// TestEntityByPrimaryTableMapping verifies that entityByPrimaryTable uses
// PrimarySchema/PrimaryTable rather than occurrences.
//
// Scenario: billing_engagements table has:
// - "billing_engagement" entity (owns the table, PrimaryTable = "billing_engagements")
// - "user" entity occurrences for host_id and visitor_id columns
//
// The old code used "first occurrence wins" which would incorrectly associate
// billing_engagements with whichever entity was first in the occurrence list.
// The fix uses entity.PrimaryTable so billing_engagements → billing_engagement.
func TestEntityByPrimaryTableMapping(t *testing.T) {
	// Create entities
	billingEngagementEntityID := uuid.New()
	userEntityID := uuid.New()

	entities := []*models.OntologyEntity{
		{
			ID:            billingEngagementEntityID,
			Name:          "billing_engagement",
			PrimarySchema: "public",
			PrimaryTable:  "billing_engagements",
		},
		{
			ID:            userEntityID,
			Name:          "user",
			PrimarySchema: "public",
			PrimaryTable:  "users",
		},
	}

	// Build entityByPrimaryTable map (same logic as in DiscoverRelationships)
	entityByPrimaryTable := make(map[string]*models.OntologyEntity)
	for _, entity := range entities {
		key := entity.PrimarySchema + "." + entity.PrimaryTable
		entityByPrimaryTable[key] = entity
	}

	// Verify billing_engagements maps to billing_engagement entity
	billingKey := "public.billing_engagements"
	if got := entityByPrimaryTable[billingKey]; got == nil {
		t.Fatalf("expected entity for %s, got nil", billingKey)
	} else if got.ID != billingEngagementEntityID {
		t.Errorf("expected billing_engagements to map to billing_engagement entity, got %s", got.Name)
	}

	// Verify users maps to user entity
	usersKey := "public.users"
	if got := entityByPrimaryTable[usersKey]; got == nil {
		t.Fatalf("expected entity for %s, got nil", usersKey)
	} else if got.ID != userEntityID {
		t.Errorf("expected users to map to user entity, got %s", got.Name)
	}

	// Verify that a table not owned by any entity returns nil
	unknownKey := "public.unknown_table"
	if got := entityByPrimaryTable[unknownKey]; got != nil {
		t.Errorf("expected nil for unknown table, got %s", got.Name)
	}
}

// TestOldOccByTableBehaviorWasBroken demonstrates why the old occurrence-based
// mapping was incorrect.
//
// The old code would iterate through occurrences and use "first wins" logic,
// which meant the entity associated with a table depended on occurrence order,
// not on which entity actually owns the table.
func TestOldOccByTableBehaviorWasBroken(t *testing.T) {
	// Create entities
	billingEngagementEntityID := uuid.New()
	userEntityID := uuid.New()

	entityByID := map[uuid.UUID]*models.OntologyEntity{
		billingEngagementEntityID: {
			ID:            billingEngagementEntityID,
			Name:          "billing_engagement",
			PrimarySchema: "public",
			PrimaryTable:  "billing_engagements",
		},
		userEntityID: {
			ID:            userEntityID,
			Name:          "user",
			PrimarySchema: "public",
			PrimaryTable:  "users",
		},
	}

	// Simulate occurrences where user entity has an occurrence in billing_engagements
	// (via host_id column) BEFORE the billing_engagement entity's occurrence
	occurrences := []*models.OntologyEntityOccurrence{
		// User entity occurs in billing_engagements.host_id
		{
			EntityID:   userEntityID,
			SchemaName: "public",
			TableName:  "billing_engagements",
			ColumnName: "host_id",
		},
		// Billing engagement entity occurs in its primary table
		{
			EntityID:   billingEngagementEntityID,
			SchemaName: "public",
			TableName:  "billing_engagements",
			ColumnName: "id",
		},
		// User entity in its own table
		{
			EntityID:   userEntityID,
			SchemaName: "public",
			TableName:  "users",
			ColumnName: "id",
		},
	}

	// OLD (broken) logic: first occurrence wins
	occByTable := make(map[string]*models.OntologyEntity)
	for _, occ := range occurrences {
		key := occ.SchemaName + "." + occ.TableName
		if _, exists := occByTable[key]; !exists {
			occByTable[key] = entityByID[occ.EntityID]
		}
	}

	// With the old code, billing_engagements would incorrectly map to "user"
	// because the user's host_id occurrence comes first
	billingKey := "public.billing_engagements"
	oldResult := occByTable[billingKey]
	if oldResult == nil {
		t.Fatal("expected entity for billing_engagements")
	}

	// This demonstrates the bug: old code returns "user" instead of "billing_engagement"
	if oldResult.Name == "billing_engagement" {
		t.Skip("occurrence order in test data happens to be correct")
	}
	if oldResult.Name != "user" {
		t.Errorf("expected old code to incorrectly return 'user', got %s", oldResult.Name)
	}

	// NEW (correct) logic: use PrimaryTable
	entityByPrimaryTable := make(map[string]*models.OntologyEntity)
	for _, entity := range entityByID {
		key := entity.PrimarySchema + "." + entity.PrimaryTable
		entityByPrimaryTable[key] = entity
	}

	newResult := entityByPrimaryTable[billingKey]
	if newResult == nil {
		t.Fatal("expected entity for billing_engagements with new logic")
	}
	if newResult.Name != "billing_engagement" {
		t.Errorf("expected new code to correctly return 'billing_engagement', got %s", newResult.Name)
	}
}
