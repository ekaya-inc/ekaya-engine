package services

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// TestCreateBidirectionalRelationship verifies that creating a relationship
// results in both forward and reverse rows being created.
func TestCreateBidirectionalRelationship(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Create a test relationship
	rel := &models.EntityRelationship{
		OntologyID:         ontologyID,
		SourceEntityID:     orderEntityID,
		TargetEntityID:     userEntityID,
		SourceColumnSchema: "public",
		SourceColumnTable:  "orders",
		SourceColumnName:   "user_id",
		TargetColumnSchema: "public",
		TargetColumnTable:  "users",
		TargetColumnName:   "id",
		DetectionMethod:    models.DetectionMethodForeignKey,
		Confidence:         1.0,
		Status:             models.RelationshipStatusConfirmed,
	}

	// Call createBidirectionalRelationship
	svc := service.(*deterministicRelationshipService)
	err := svc.createBidirectionalRelationship(context.Background(), rel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: 2 relationships created (forward and reverse)
	if len(mocks.relationshipRepo.created) != 2 {
		t.Fatalf("expected 2 relationships (forward and reverse), got %d", len(mocks.relationshipRepo.created))
	}

	// Verify forward relationship
	forward := mocks.relationshipRepo.created[0]
	if forward.SourceEntityID != orderEntityID || forward.TargetEntityID != userEntityID {
		t.Errorf("forward relationship incorrect: source=%v target=%v", forward.SourceEntityID, forward.TargetEntityID)
	}
	if forward.SourceColumnName != "user_id" || forward.TargetColumnName != "id" {
		t.Errorf("forward relationship columns incorrect: source=%s target=%s", forward.SourceColumnName, forward.TargetColumnName)
	}

	// Verify reverse relationship
	reverse := mocks.relationshipRepo.created[1]
	if reverse.SourceEntityID != userEntityID || reverse.TargetEntityID != orderEntityID {
		t.Errorf("reverse relationship incorrect: source=%v target=%v", reverse.SourceEntityID, reverse.TargetEntityID)
	}
	if reverse.SourceColumnName != "id" || reverse.TargetColumnName != "user_id" {
		t.Errorf("reverse relationship columns incorrect: source=%s target=%s", reverse.SourceColumnName, reverse.TargetColumnName)
	}

	// Verify reverse has no description (will be set during enrichment)
	if reverse.Description != nil {
		t.Errorf("expected reverse description to be nil, got %v", *reverse.Description)
	}
}

// TestCreateBidirectionalRelationship_ColumnIDs verifies that column IDs are properly
// swapped when creating the reverse relationship.
func TestCreateBidirectionalRelationship_ColumnIDs(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()
	sourceColumnID := uuid.New()
	targetColumnID := uuid.New()

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Create a test relationship with column IDs
	rel := &models.EntityRelationship{
		OntologyID:         ontologyID,
		SourceEntityID:     orderEntityID,
		TargetEntityID:     userEntityID,
		SourceColumnSchema: "public",
		SourceColumnTable:  "orders",
		SourceColumnName:   "user_id",
		SourceColumnID:     &sourceColumnID,
		TargetColumnSchema: "public",
		TargetColumnTable:  "users",
		TargetColumnName:   "id",
		TargetColumnID:     &targetColumnID,
		DetectionMethod:    models.DetectionMethodForeignKey,
		Confidence:         1.0,
		Status:             models.RelationshipStatusConfirmed,
	}

	// Call createBidirectionalRelationship
	svc := service.(*deterministicRelationshipService)
	err := svc.createBidirectionalRelationship(context.Background(), rel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: 2 relationships created (forward and reverse)
	if len(mocks.relationshipRepo.created) != 2 {
		t.Fatalf("expected 2 relationships (forward and reverse), got %d", len(mocks.relationshipRepo.created))
	}

	// Verify forward relationship column IDs
	forward := mocks.relationshipRepo.created[0]
	if forward.SourceColumnID == nil || *forward.SourceColumnID != sourceColumnID {
		t.Errorf("forward relationship source column ID incorrect: got %v, want %v", forward.SourceColumnID, sourceColumnID)
	}
	if forward.TargetColumnID == nil || *forward.TargetColumnID != targetColumnID {
		t.Errorf("forward relationship target column ID incorrect: got %v, want %v", forward.TargetColumnID, targetColumnID)
	}

	// Verify reverse relationship column IDs are swapped
	reverse := mocks.relationshipRepo.created[1]
	if reverse.SourceColumnID == nil || *reverse.SourceColumnID != targetColumnID {
		t.Errorf("reverse relationship source column ID incorrect: got %v, want %v (should be target from forward)", reverse.SourceColumnID, targetColumnID)
	}
	if reverse.TargetColumnID == nil || *reverse.TargetColumnID != sourceColumnID {
		t.Errorf("reverse relationship target column ID incorrect: got %v, want %v (should be source from forward)", reverse.TargetColumnID, sourceColumnID)
	}
}

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

// TestPKMatch_NilStatsWithJoinableTrue verifies that columns with IsJoinable=true
// but DistinctCount=nil proceed to join validation instead of being skipped.
// This fixes BUG-3: columns like visitor_id, host_id with NULL stats but is_joinable=true
// should create relationships when join validation succeeds.
func TestPKMatch_NilStatsWithJoinableTrue(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()

	distinctCount := int64(100)
	isJoinableTrue := true

	// Create mocks with user entity
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Add order entity
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

	// Track if AnalyzeJoin is called for the nil-stats column
	analyzeJoinCalled := false
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		if sourceColumn == "buyer_id" {
			analyzeJoinCalled = true
		}
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil // Valid FK: all rows match
	}

	// Schema: orders.buyer_id has IsJoinable=true but no DistinctCount
	// With the fix, it should proceed to join validation
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: ordersTableID,
					ColumnName:    "id",
					DataType:      "int8", // Different type prevents bidirectional matching
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
				{
					SchemaTableID: ordersTableID,
					ColumnName:    "buyer_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue, // Marked as joinable
					DistinctCount: nil,             // But NO stats (BUG-3 scenario)
				},
			},
		},
	}

	// Create service
	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Execute PK match discovery
	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: AnalyzeJoin was called for the nil-stats column
	if !analyzeJoinCalled {
		t.Error("expected AnalyzeJoin to be called for buyer_id (nil stats with is_joinable=true)")
	}

	// Verify: relationship was created since join validation passed
	if result.InferredRelationships == 0 {
		t.Error("expected relationship to be created for nil-stats column with valid join")
	}
}

// TestPKMatch_WorksWithoutRowCount verifies that columns with DistinctCount
// but missing RowCount can still pass the cardinality filter.
func TestPKMatch_WorksWithoutRowCount(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	distinctCount := int64(50) // Meets absolute threshold (>= 20)
	isJoinableTrue := true

	// Create mocks with user entity
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Mock discoverer returns 0 orphans (100% match)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			OrphanCount: 0, // All source rows match = valid relationship
		}, nil
	}

	// Schema: orders.buyer (uuid) has DistinctCount but users table has NO RowCount
	// Uses different type for orders.id (int8) to prevent bidirectional matching
	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	usersPKID := uuid.New()
	buyerColID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   nil, // NO row count - ratio check should be skipped
			Columns: []models.SchemaColumn{
				{
					ID:            usersPKID,
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid", // PK is uuid
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount, // Has distinct count >= 20
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: ordersTableID,
					ColumnName:    "id",
					DataType:      "int8", // Different type - prevents bidirectional matching
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount, // Orders PK
				},
				{
					ID:            buyerColID,
					SchemaTableID: ordersTableID,
					ColumnName:    "buyer", // FK column
					DataType:      "uuid",  // Matches users.id type
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount, // Has stats
				},
			},
		},
	}

	// Create service
	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Execute PK match discovery
	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify relationships were created
	// Missing RowCount doesn't block when DistinctCount >= 20
	// PKMatchDiscovery now writes to SchemaRelationship (unidirectional)
	// With high cardinality columns on both tables, both directions may match
	if result.InferredRelationships < 1 {
		t.Errorf("expected at least 1 inferred relationship, got %d", result.InferredRelationships)
	}

	// Verify the key relationship orders.buyer -> users.id exists
	foundBuyerToUser := false
	for _, rel := range mocks.schemaRepo.upsertedRelationships {
		if rel.SourceTableID == ordersTableID && rel.TargetTableID == usersTableID &&
			rel.SourceColumnID == buyerColID && rel.TargetColumnID == usersPKID {
			foundBuyerToUser = true
			if rel.InferenceMethod == nil || *rel.InferenceMethod != models.InferenceMethodPKMatch {
				t.Errorf("expected inference method pk_match, got %v", rel.InferenceMethod)
			}
		}
	}
	if !foundBuyerToUser {
		t.Errorf("expected relationship orders.buyer -> users.id to be discovered")
	}
}

// NOTE: TestIsPKMatchExcludedName has been removed.
// Column filtering now uses stored ColumnFeatures.Purpose instead of name-based patterns.
// See PLAN-extracting-column-features.md for details.

// TestPKMatch_RequiresJoinableFlag verifies that columns with IsJoinable=false
// or IsJoinable=nil are skipped and do not create relationships.
func TestPKMatch_RequiresJoinableFlag(t *testing.T) {
	t.Run("IsJoinable_nil_skipped", func(t *testing.T) {
		// Test verifies that a column with IsJoinable=nil is NOT used as an FK candidate.
		//
		// Challenge: Columns with DistinctCount >= 20 become BOTH candidates AND entityRefColumns.
		// To isolate the IsJoinable filter, we use a low DistinctCount (<20) on the FK column.
		// This ensures the FK column isn't used as an entityRefColumn, while still testing
		// that IsJoinable filtering happens before the DistinctCount filter in candidates.
		//
		// Note: This test documents current behavior where IsJoinable check comes before
		// DistinctCount check in candidate filtering (lines 300-315 of the service).

		projectID := uuid.New()
		datasourceID := uuid.New()
		ontologyID := uuid.New()
		userEntityID := uuid.New()
		orderEntityID := uuid.New()

		highDistinct := int64(100)
		lowDistinct := int64(15) // Below threshold of 20
		isJoinableTrue := true

		mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

		mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
			ID:            orderEntityID,
			OntologyID:    ontologyID,
			Name:          "order",
			PrimarySchema: "public",
			PrimaryTable:  "orders",
		})

		// Track if AnalyzeJoin was called
		mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
			t.Errorf("AnalyzeJoin unexpectedly called: %s.%s.%s -> %s.%s.%s",
				sourceSchema, sourceTable, sourceColumn,
				targetSchema, targetTable, targetColumn)
			return &datasource.JoinAnalysis{OrphanCount: 0}, nil
		}

		usersTableID := uuid.New()
		ordersTableID := uuid.New()

		mocks.schemaRepo.tables = []*models.SchemaTable{
			{
				ID:         usersTableID,
				SchemaName: "public",
				TableName:  "users",
				RowCount:   &highDistinct,
				Columns: []models.SchemaColumn{
					{
						SchemaTableID: usersTableID,
						ColumnName:    "id",
						DataType:      "uuid",
						IsPrimaryKey:  true,
						IsJoinable:    &isJoinableTrue,
						DistinctCount: &highDistinct,
					},
				},
			},
			{
				ID:         ordersTableID,
				SchemaName: "public",
				TableName:  "orders",
				RowCount:   &highDistinct,
				Columns: []models.SchemaColumn{
					{
						SchemaTableID: ordersTableID,
						ColumnName:    "id",
						DataType:      "uuid",
						IsPrimaryKey:  true,
						IsJoinable:    &isJoinableTrue,
						DistinctCount: &highDistinct,
					},
					{
						SchemaTableID: ordersTableID,
						ColumnName:    "buyer", // FK column
						DataType:      "uuid",
						IsPrimaryKey:  false,
						IsJoinable:    nil,          // NO joinability - should be skipped
						DistinctCount: &lowDistinct, // Low distinct to avoid being entityRefColumn
					},
				},
			},
		}

		service := NewDeterministicRelationshipService(
			mocks.datasourceService,
			mocks.projectService,
			mocks.adapterFactory,
			mocks.ontologyRepo,
			mocks.entityRepo,
			mocks.relationshipRepo,
			mocks.schemaRepo,
			zap.NewNop(),
		)

		result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify: NO relationships created
		// Note: The column would fail both IsJoinable check AND DistinctCount check.
		// This test ensures that at minimum, non-joinable columns don't form relationships.
		if result.InferredRelationships != 0 {
			t.Errorf("expected 0 inferred relationships (IsJoinable=nil should be skipped), got %d", result.InferredRelationships)
		}

		if len(mocks.relationshipRepo.created) != 0 {
			t.Errorf("expected 0 relationships to be created, got %d", len(mocks.relationshipRepo.created))
		}
	})

	t.Run("IsJoinable_false_skipped", func(t *testing.T) {
		// Test verifies that a column with IsJoinable=false is NOT used as an FK candidate.
		// Uses low DistinctCount on FK column to prevent it from being an entityRefColumn.

		projectID := uuid.New()
		datasourceID := uuid.New()
		ontologyID := uuid.New()
		userEntityID := uuid.New()
		orderEntityID := uuid.New()

		highDistinct := int64(100)
		lowDistinct := int64(15)
		isJoinableFalse := false
		isJoinableTrue := true

		mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

		mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
			ID:            orderEntityID,
			OntologyID:    ontologyID,
			Name:          "order",
			PrimarySchema: "public",
			PrimaryTable:  "orders",
		})

		// Mock discoverer would return 0 orphans if called - but should NOT be called
		mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
			t.Errorf("AnalyzeJoin unexpectedly called: %s.%s.%s -> %s.%s.%s",
				sourceSchema, sourceTable, sourceColumn,
				targetSchema, targetTable, targetColumn)
			return &datasource.JoinAnalysis{OrphanCount: 0}, nil
		}

		usersTableID := uuid.New()
		ordersTableID := uuid.New()

		mocks.schemaRepo.tables = []*models.SchemaTable{
			{
				ID:         usersTableID,
				SchemaName: "public",
				TableName:  "users",
				RowCount:   &highDistinct,
				Columns: []models.SchemaColumn{
					{
						SchemaTableID: usersTableID,
						ColumnName:    "id",
						DataType:      "uuid",
						IsPrimaryKey:  true,
						IsJoinable:    &isJoinableTrue,
						DistinctCount: &highDistinct,
					},
				},
			},
			{
				ID:         ordersTableID,
				SchemaName: "public",
				TableName:  "orders",
				RowCount:   &highDistinct,
				Columns: []models.SchemaColumn{
					{
						SchemaTableID: ordersTableID,
						ColumnName:    "id",
						DataType:      "uuid",
						IsPrimaryKey:  true,
						IsJoinable:    &isJoinableTrue,
						DistinctCount: &highDistinct,
					},
					{
						SchemaTableID: ordersTableID,
						ColumnName:    "buyer", // FK column
						DataType:      "uuid",
						IsPrimaryKey:  false,
						IsJoinable:    &isJoinableFalse, // NOT joinable - should be skipped
						DistinctCount: &lowDistinct,     // Low distinct to avoid being entityRefColumn
					},
				},
			},
		}

		service := NewDeterministicRelationshipService(
			mocks.datasourceService,
			mocks.projectService,
			mocks.adapterFactory,
			mocks.ontologyRepo,
			mocks.entityRepo,
			mocks.relationshipRepo,
			mocks.schemaRepo,
			zap.NewNop(),
		)

		result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify: NO relationships created because account_id has IsJoinable=false
		if result.InferredRelationships != 0 {
			t.Errorf("expected 0 inferred relationships (IsJoinable=false should be skipped), got %d", result.InferredRelationships)
		}

		if len(mocks.relationshipRepo.created) != 0 {
			t.Errorf("expected 0 relationships to be created, got %d", len(mocks.relationshipRepo.created))
		}
	})

	t.Run("IsJoinable_true_passes", func(t *testing.T) {
		// Test verifies that a column with IsJoinable=true IS used as an FK candidate.
		// Uses different types for users.id (uuid) and orders.id (int8) to prevent bidirectional matching.
		// The FK column orders.buyer (uuid) should match users.id (uuid) creating exactly one relationship.

		projectID := uuid.New()
		datasourceID := uuid.New()
		ontologyID := uuid.New()
		userEntityID := uuid.New()
		orderEntityID := uuid.New()

		highDistinct := int64(100)
		isJoinableTrue := true

		mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

		mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
			ID:            orderEntityID,
			OntologyID:    ontologyID,
			Name:          "order",
			PrimarySchema: "public",
			PrimaryTable:  "orders",
		})

		// Mock discoverer returns 0 orphans (100% match) - relationships should be created
		mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
			return &datasource.JoinAnalysis{
				OrphanCount: 0,
			}, nil
		}

		usersTableID := uuid.New()
		ordersTableID := uuid.New()

		mocks.schemaRepo.tables = []*models.SchemaTable{
			{
				ID:         usersTableID,
				SchemaName: "public",
				TableName:  "users",
				RowCount:   &highDistinct,
				Columns: []models.SchemaColumn{
					{
						SchemaTableID: usersTableID,
						ColumnName:    "id",
						DataType:      "uuid", // PK is uuid
						IsPrimaryKey:  true,
						IsJoinable:    &isJoinableTrue,
						DistinctCount: &highDistinct,
					},
				},
			},
			{
				ID:         ordersTableID,
				SchemaName: "public",
				TableName:  "orders",
				RowCount:   &highDistinct,
				Columns: []models.SchemaColumn{
					{
						SchemaTableID: ordersTableID,
						ColumnName:    "id",
						DataType:      "int8", // Different type - prevents bidirectional matching
						IsPrimaryKey:  true,
						IsJoinable:    &isJoinableTrue,
						DistinctCount: &highDistinct,
					},
					{
						SchemaTableID: ordersTableID,
						ColumnName:    "buyer", // FK column
						DataType:      "uuid",  // Matches users.id type
						IsPrimaryKey:  false,
						IsJoinable:    &isJoinableTrue, // Joinable - should pass
						DistinctCount: &highDistinct,
					},
				},
			},
		}

		service := NewDeterministicRelationshipService(
			mocks.datasourceService,
			mocks.projectService,
			mocks.adapterFactory,
			mocks.ontologyRepo,
			mocks.entityRepo,
			mocks.relationshipRepo,
			mocks.schemaRepo,
			zap.NewNop(),
		)

		result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify relationships were discovered
		// PKMatchDiscovery now writes unidirectional SchemaRelationships
		// With high cardinality columns, both directions may be detected
		if result.InferredRelationships < 1 {
			t.Errorf("expected at least 1 inferred relationship (IsJoinable=true should pass), got %d", result.InferredRelationships)
		}

		if len(mocks.schemaRepo.upsertedRelationships) < 1 {
			t.Fatalf("expected at least 1 SchemaRelationship to be created, got %d", len(mocks.schemaRepo.upsertedRelationships))
		}

		// Verify at least one relationship was created (orders.buyer -> users.id or reverse)
		foundRelationship := false
		for _, rel := range mocks.schemaRepo.upsertedRelationships {
			if (rel.SourceTableID == ordersTableID && rel.TargetTableID == usersTableID) ||
				(rel.SourceTableID == usersTableID && rel.TargetTableID == ordersTableID) {
				foundRelationship = true
				break
			}
		}
		if !foundRelationship {
			t.Error("expected relationship between orders and users tables")
		}
	})

	t.Run("IsJoinable_nil_allowed_for_id_columns", func(t *testing.T) {
		// Test verifies that _id columns with IsJoinable=nil ARE included as FK candidates.
		// This is the new behavior: for text UUID columns, IsJoinable may be nil initially
		// before stats collection, so _id columns should be included for validation.
		//
		// Non-_id columns (like "buyer") with IsJoinable=nil are still skipped.

		projectID := uuid.New()
		datasourceID := uuid.New()
		ontologyID := uuid.New()
		userEntityID := uuid.New()
		orderEntityID := uuid.New()

		highDistinct := int64(100)
		isJoinableTrue := true
		rowCount := int64(100)

		mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

		mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
			ID:            orderEntityID,
			OntologyID:    ontologyID,
			Name:          "order",
			PrimarySchema: "public",
			PrimaryTable:  "orders",
		})

		// Track AnalyzeJoin calls - we expect it to be called for user_id
		var joinAnalysisCalls int
		mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
			joinAnalysisCalls++
			return &datasource.JoinAnalysis{OrphanCount: 0}, nil
		}

		usersTableID := uuid.New()
		ordersTableID := uuid.New()

		mocks.schemaRepo.tables = []*models.SchemaTable{
			{
				ID:         usersTableID,
				SchemaName: "public",
				TableName:  "users",
				RowCount:   &rowCount,
				Columns: []models.SchemaColumn{
					{
						SchemaTableID: usersTableID,
						ColumnName:    "id",
						DataType:      "uuid",
						IsPrimaryKey:  true,
						IsJoinable:    &isJoinableTrue,
						DistinctCount: &highDistinct,
					},
				},
			},
			{
				ID:         ordersTableID,
				SchemaName: "public",
				TableName:  "orders",
				RowCount:   &rowCount,
				Columns: []models.SchemaColumn{
					{
						SchemaTableID: ordersTableID,
						ColumnName:    "id",
						DataType:      "int8", // Different type to prevent bidirectional matching
						IsPrimaryKey:  true,
						IsJoinable:    &isJoinableTrue,
						DistinctCount: &highDistinct,
					},
					{
						SchemaTableID: ordersTableID,
						ColumnName:    "user_id", // _id column - should be included even with nil IsJoinable
						DataType:      "uuid",
						IsPrimaryKey:  false,
						IsJoinable:    nil,           // nil joinability - but should be included for _id columns
						DistinctCount: &highDistinct, // Sufficient distinct count
					},
				},
			},
		}

		service := NewDeterministicRelationshipService(
			mocks.datasourceService,
			mocks.projectService,
			mocks.adapterFactory,
			mocks.ontologyRepo,
			mocks.entityRepo,
			mocks.relationshipRepo,
			mocks.schemaRepo,
			zap.NewNop(),
		)

		result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify: AnalyzeJoin WAS called because user_id (with _id suffix) is included
		if joinAnalysisCalls == 0 {
			t.Errorf("expected AnalyzeJoin to be called for user_id column with nil IsJoinable, but it wasn't")
		}

		// Verify: relationships were created
		if result.InferredRelationships == 0 {
			t.Errorf("expected at least 1 inferred relationship (user_id with nil IsJoinable should be included), got %d", result.InferredRelationships)
		}
	})
}

// mockTestServices holds all mock dependencies for testing
type mockTestServices struct {
	datasourceService *mockTestDatasourceService
	projectService    *mockTestProjectService
	adapterFactory    *mockTestAdapterFactory
	discoverer        *mockTestSchemaDiscoverer
	ontologyRepo      *mockTestOntologyRepo
	entityRepo        *mockTestEntityRepo
	relationshipRepo  *mockTestRelationshipRepo
	schemaRepo        *mockTestSchemaRepo
}

// setupMocks creates all mock dependencies with sensible defaults
func setupMocks(projectID, ontologyID, datasourceID, entityID uuid.UUID) *mockTestServices {
	discoverer := &mockTestSchemaDiscoverer{}

	return &mockTestServices{
		datasourceService: &mockTestDatasourceService{
			datasource: &models.Datasource{
				ID:             datasourceID,
				DatasourceType: "postgres",
				Config:         map[string]any{},
			},
		},
		projectService: &mockTestProjectService{
			ontologySettings: &OntologySettings{UseLegacyPatternMatching: true},
		},
		adapterFactory: &mockTestAdapterFactory{
			discoverer: discoverer,
		},
		discoverer: discoverer,
		ontologyRepo: &mockTestOntologyRepo{
			ontology: &models.TieredOntology{
				ID:        ontologyID,
				ProjectID: projectID,
			},
		},
		entityRepo: &mockTestEntityRepo{
			entities: []*models.OntologyEntity{
				{
					ID:            entityID,
					OntologyID:    ontologyID,
					Name:          "user",
					PrimarySchema: "public",
					PrimaryTable:  "users",
				},
			},
		},
		relationshipRepo: &mockTestRelationshipRepo{
			created: []*models.EntityRelationship{},
		},
		schemaRepo: &mockTestSchemaRepo{
			tables: []*models.SchemaTable{},
		},
	}
}

// Mock implementations for deterministic relationship service tests

type mockTestDatasourceService struct {
	datasource *models.Datasource
}

func (m *mockTestDatasourceService) GetByID(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

func (m *mockTestDatasourceService) Get(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

func (m *mockTestDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.DatasourceWithStatus, error) {
	return nil, nil
}

func (m *mockTestDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType, provider string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockTestDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType, provider string, config map[string]any) error {
	return nil
}

func (m *mockTestDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockTestDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any, datasourceID uuid.UUID) error {
	return nil
}

func (m *mockTestDatasourceService) SetDefault(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

type mockTestProjectService struct {
	ontologySettings *OntologySettings
}

func (m *mockTestProjectService) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockTestProjectService) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockTestProjectService) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockTestProjectService) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockTestProjectService) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockTestProjectService) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func (m *mockTestProjectService) SetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID, datasourceID uuid.UUID) error {
	return nil
}

func (m *mockTestProjectService) SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string) {
}

func (m *mockTestProjectService) GetAuthServerURL(ctx context.Context, projectID uuid.UUID) (string, error) {
	return "", nil
}

func (m *mockTestProjectService) UpdateAuthServerURL(ctx context.Context, projectID uuid.UUID, authServerURL string) error {
	return nil
}

func (m *mockTestProjectService) GetAutoApproveSettings(ctx context.Context, projectID uuid.UUID) (*AutoApproveSettings, error) {
	return nil, nil
}

func (m *mockTestProjectService) SetAutoApproveSettings(ctx context.Context, projectID uuid.UUID, settings *AutoApproveSettings) error {
	return nil
}

func (m *mockTestProjectService) GetOntologySettings(ctx context.Context, projectID uuid.UUID) (*OntologySettings, error) {
	if m.ontologySettings != nil {
		return m.ontologySettings, nil
	}
	return &OntologySettings{UseLegacyPatternMatching: true}, nil
}

func (m *mockTestProjectService) SetOntologySettings(ctx context.Context, projectID uuid.UUID, settings *OntologySettings) error {
	return nil
}

type mockTestAdapterFactory struct {
	discoverer *mockTestSchemaDiscoverer
}

func (m *mockTestAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, nil
}

func (m *mockTestAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	return m.discoverer, nil
}

func (m *mockTestAdapterFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, nil
}

func (m *mockTestAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

type mockTestSchemaDiscoverer struct {
	joinAnalysisFunc func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error)
}

func (m *mockTestSchemaDiscoverer) DiscoverTables(ctx context.Context) ([]datasource.TableMetadata, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) SupportsForeignKeys() bool {
	return false
}

func (m *mockTestSchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) CheckValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string, sampleLimit int) (*datasource.ValueOverlapResult, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	if m.joinAnalysisFunc != nil {
		return m.joinAnalysisFunc(ctx, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn)
	}
	// Default: no matches
	return &datasource.JoinAnalysis{
		OrphanCount: 100, // All rows orphaned = no relationship
	}, nil
}

func (m *mockTestSchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) GetEnumValueDistribution(ctx context.Context, schemaName, tableName, columnName string, completionTimestampCol string, limit int) (*datasource.EnumDistributionResult, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) Close() error {
	return nil
}

type mockTestOntologyRepo struct {
	ontology *models.TieredOntology
}

func (m *mockTestOntologyRepo) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}

func (m *mockTestOntologyRepo) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	return m.ontology, nil
}

func (m *mockTestOntologyRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.TieredOntology, error) {
	return m.ontology, nil
}

func (m *mockTestOntologyRepo) Update(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}

func (m *mockTestOntologyRepo) List(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]*models.TieredOntology, error) {
	return nil, nil
}

type mockTestEntityRepo struct {
	entities []*models.OntologyEntity
}

func (m *mockTestEntityRepo) Create(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockTestEntityRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return m.entities, nil
}

func (m *mockTestEntityRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) Update(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockTestEntityRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}

func (m *mockTestEntityRepo) GetAliases(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}

func (m *mockTestEntityRepo) GetKeyColumns(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) CountOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockTestEntityRepo) GetOccurrenceTablesByEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]string, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetPromotedByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}

func (m *mockTestEntityRepo) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

type mockTestRelationshipRepo struct {
	created  []*models.EntityRelationship
	existing []*models.EntityRelationship // Pre-existing relationships (for GetByOntology)
}

func (m *mockTestRelationshipRepo) Create(ctx context.Context, relationship *models.EntityRelationship) error {
	m.created = append(m.created, relationship)
	return nil
}

func (m *mockTestRelationshipRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	return m.created, nil
}

func (m *mockTestRelationshipRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockTestRelationshipRepo) Update(ctx context.Context, relationship *models.EntityRelationship) error {
	return nil
}

func (m *mockTestRelationshipRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

type mockTestSchemaRepo struct {
	tables                []*models.SchemaTable
	columns               []*models.SchemaColumn
	relationships         []*models.SchemaRelationship
	upsertedRelationships []*models.SchemaRelationship // Tracks relationships upserted via UpsertRelationship
	upsertedMetrics       []*models.DiscoveryMetrics   // Tracks metrics passed to UpsertRelationshipWithMetrics
	upsertError           error                        // If set, UpsertRelationship/UpsertRelationshipWithMetrics will return this error
}

func (m *mockTestSchemaRepo) GetTables(ctx context.Context, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return m.tables, nil
}

func (m *mockTestSchemaRepo) GetColumns(ctx context.Context, datasourceID uuid.UUID, schemaName, tableName string) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	return m.tables, nil
}

func (m *mockTestSchemaRepo) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	// Flatten columns from tables
	var result []*models.SchemaColumn
	for _, table := range m.tables {
		for i := range table.Columns {
			result = append(result, &table.Columns[i])
		}
	}
	return result, nil
}

func (m *mockTestSchemaRepo) GetColumnsWithFeaturesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string, selectedOnly bool) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockTestSchemaRepo) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	return nil
}

// Additional stub methods to satisfy interfaces

func (m *mockTestOntologyRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockTestOntologyRepo) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockTestOntologyRepo) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return nil
}

func (m *mockTestOntologyRepo) UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error {
	return nil
}

func (m *mockTestOntologyRepo) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	return nil
}

func (m *mockTestOntologyRepo) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	return nil
}

func (m *mockTestEntityRepo) DeleteAlias(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) DeleteInferenceEntitiesByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}

func (m *mockTestEntityRepo) MarkInferenceEntitiesStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) ClearStaleFlag(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) GetStaleEntities(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) TransferAliasesToEntity(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockTestEntityRepo) TransferKeyColumnsToEntity(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockTestRelationshipRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockTestRelationshipRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	// Return existing + created relationships for deduplication tests
	all := make([]*models.EntityRelationship, 0, len(m.existing)+len(m.created))
	all = append(all, m.existing...)
	all = append(all, m.created...)
	return all, nil
}

func (m *mockTestRelationshipRepo) GetByOntologyGroupedByTarget(ctx context.Context, ontologyID uuid.UUID) (map[uuid.UUID][]*models.EntityRelationship, error) {
	return make(map[uuid.UUID][]*models.EntityRelationship), nil
}

func (m *mockTestSchemaRepo) GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}

// More missing stub methods

func (m *mockTestDatasourceService) Delete(ctx context.Context, datasourceID uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockTestRelationshipRepo) GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockTestRelationshipRepo) UpdateDescription(ctx context.Context, id uuid.UUID, description string) error {
	return nil
}

func (m *mockTestRelationshipRepo) UpdateDescriptionAndAssociation(ctx context.Context, id uuid.UUID, description string, association string) error {
	return nil
}

func (m *mockTestRelationshipRepo) GetByTargetEntity(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockTestRelationshipRepo) GetByEntityPair(ctx context.Context, ontologyID uuid.UUID, fromEntityID uuid.UUID, toEntityID uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockTestRelationshipRepo) Upsert(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (m *mockTestRelationshipRepo) MarkInferenceRelationshipsStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockTestRelationshipRepo) ClearStaleFlag(ctx context.Context, relationshipID uuid.UUID) error {
	return nil
}

func (m *mockTestRelationshipRepo) GetStaleRelationships(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockTestRelationshipRepo) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}

func (m *mockTestRelationshipRepo) UpdateSourceEntityID(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockTestRelationshipRepo) UpdateTargetEntityID(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockTestSchemaRepo) GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error {
	return nil
}

func (m *mockTestSchemaRepo) GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return m.relationships, nil
}

func (m *mockTestSchemaRepo) GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error {
	if m.upsertError != nil {
		return m.upsertError
	}
	// Track upserted relationships
	m.upsertedRelationships = append(m.upsertedRelationships, rel)
	return nil
}

func (m *mockTestSchemaRepo) UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error {
	return nil
}

func (m *mockTestSchemaRepo) SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}

func (m *mockTestSchemaRepo) SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockTestSchemaRepo) GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error {
	if m.upsertError != nil {
		return m.upsertError
	}
	// Track upserted relationships (same as UpsertRelationship for test verification)
	m.upsertedRelationships = append(m.upsertedRelationships, rel)
	// Track metrics
	m.upsertedMetrics = append(m.upsertedMetrics, metrics)
	return nil
}

func (m *mockTestSchemaRepo) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	return nil
}

func (m *mockTestSchemaRepo) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return nil
}

func (m *mockTestSchemaRepo) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID, selectedOnly bool) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error {
	return nil
}

func (m *mockTestSchemaRepo) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64, sampleValues []string) error {
	return nil
}

func (m *mockTestSchemaRepo) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return nil
}

func (m *mockTestSchemaRepo) UpdateColumnFeatures(ctx context.Context, projectID, columnID uuid.UUID, features *models.ColumnFeatures) error {
	return nil
}

func (m *mockTestSchemaRepo) UpsertTable(ctx context.Context, table *models.SchemaTable) error {
	return nil
}

func (m *mockTestSchemaRepo) SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []repositories.TableKey) (int64, error) {
	return 0, nil
}

func (m *mockTestSchemaRepo) SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error) {
	return 0, nil
}

func (m *mockTestSchemaRepo) SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}
func (m *mockTestSchemaRepo) ClearColumnFeaturesByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockTestSchemaRepo) GetRelationshipsByMethod(ctx context.Context, projectID, datasourceID uuid.UUID, method string) ([]*models.SchemaRelationship, error) {
	return nil, nil
}

// TestPKMatch_SmallIntegerValues tests that columns with all small integer values (1-10)
// are rejected unless the target table is also small (lookup table scenario)
func TestPKMatch_SmallIntegerValues(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	accountEntityID := uuid.New()
	reviewEntityID := uuid.New()

	distinctCount := int64(100)
	smallDistinctCount := int64(5)
	rowCount := int64(1000)
	isJoinableTrue := true

	// Create mocks with account and review entities
	mocks := setupMocks(projectID, ontologyID, datasourceID, accountEntityID)
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            reviewEntityID,
		OntologyID:    ontologyID,
		Name:          "review",
		PrimarySchema: "public",
		PrimaryTable:  "reviews",
	})

	// Mock discoverer: if called, returns 0 orphans (all small values exist in accounts.id)
	maxSourceValue := int64(5) // Rating is 1-5
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			OrphanCount:    0,
			MaxSourceValue: &maxSourceValue,
		}, nil
	}

	accountsTableID := uuid.New()
	reviewsTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         accountsTableID,
			SchemaName: "public",
			TableName:  "accounts",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: accountsTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
		{
			ID:         reviewsTableID,
			SchemaName: "public",
			TableName:  "reviews",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: reviewsTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
				{
					SchemaTableID: reviewsTableID,
					ColumnName:    "rating", // 1-5 rating - should be rejected
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &smallDistinctCount, // Only 5 distinct values
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: NO relationships created because rating has max value of 5
	if result.InferredRelationships != 0 {
		t.Errorf("expected 0 inferred relationships (small integer column should be rejected), got %d", result.InferredRelationships)
	}

	if len(mocks.relationshipRepo.created) != 0 {
		t.Errorf("expected no relationships in repo, got %d", len(mocks.relationshipRepo.created))
	}
}

// TestPKMatch_SmallIntegerValues_LookupTable tests that small integer values ARE allowed
// when the target table is also small (lookup table scenario)
func TestPKMatch_SmallIntegerValues_LookupTable(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	statusEntityID := uuid.New()
	orderEntityID := uuid.New()

	candidateDistinctCount := int64(25) // 25 distinct in candidate (PK) column - passes pre-join filter (>= 20)
	refDistinctCount := int64(15)       // 15 distinct in ref (FK) column - passes cardinality check (15/100 = 15% > 1%)
	refTableRowCount := int64(100)      // Smaller FK table so cardinality ratio is acceptable
	isJoinableTrue := true

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, statusEntityID)
	mocks.entityRepo.entities[0] = &models.OntologyEntity{
		ID:            statusEntityID,
		OntologyID:    ontologyID,
		Name:          "status",
		PrimarySchema: "public",
		PrimaryTable:  "order_statuses",
	}
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

	// Mock discoverer: returns 0 orphans, max FK value is 20 (all FK values reference valid PKs)
	// This tests that small-ish FK values (20) are allowed when PK table is also small-ish (25 distinct)
	maxSourceValue := int64(20)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			OrphanCount:    0,
			MaxSourceValue: &maxSourceValue,
		}, nil
	}

	statusTableID := uuid.New()
	ordersTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         statusTableID,
			SchemaName: "public",
			TableName:  "order_statuses",
			RowCount:   &candidateDistinctCount, // 60 rows in lookup table
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: statusTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinctCount, // 60 distinct values - passes pre-join filter (>= 20)
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &refTableRowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: ordersTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &refTableRowCount,
				},
				{
					SchemaTableID: ordersTableID,
					ColumnName:    "status_id", // FK to lookup table
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &refDistinctCount, // 15 distinct values (passes cardinality check: 15/100 = 15% > 1%)
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: relationship IS created because target table is also small (lookup table)
	// PKMatchDiscovery now writes unidirectional SchemaRelationships
	if result.InferredRelationships < 1 {
		t.Errorf("expected at least 1 inferred relationship (small FK to small lookup table is valid), got %d", result.InferredRelationships)
	}

	if len(mocks.schemaRepo.upsertedRelationships) < 1 {
		t.Fatalf("expected at least 1 SchemaRelationship to be created, got %d", len(mocks.schemaRepo.upsertedRelationships))
	}

	// Verify at least one relationship between orders and order_statuses was created
	foundRelationship := false
	for _, rel := range mocks.schemaRepo.upsertedRelationships {
		if (rel.SourceTableID == ordersTableID && rel.TargetTableID == statusTableID) ||
			(rel.SourceTableID == statusTableID && rel.TargetTableID == ordersTableID) {
			foundRelationship = true
			break
		}
	}
	if !foundRelationship {
		t.Error("expected relationship between orders and order_statuses tables")
	}
}

// TestPKMatch_LowCardinality_Excluded tests that columns with DistinctCount < 20
// are excluded from being FK candidates (absolute threshold check)
func TestPKMatch_LowCardinality_Excluded(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()

	distinctCount := int64(100)
	lowDistinctCount := int64(5) // Below threshold of 20
	rowCount := int64(1000)
	isJoinableTrue := true

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

	// Mock discoverer should NOT be called because column is filtered by cardinality
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		t.Errorf("AnalyzeJoin should not be called (column has DistinctCount < 20): %s.%s.%s -> %s.%s.%s",
			sourceSchema, sourceTable, sourceColumn,
			targetSchema, targetTable, targetColumn)
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: ordersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
				{
					SchemaTableID: ordersTableID,
					ColumnName:    "status_code", // Low cardinality column
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &lowDistinctCount, // Only 5 distinct values - should be excluded
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: NO relationships created because status_code has DistinctCount < 20
	if result.InferredRelationships != 0 {
		t.Errorf("expected 0 inferred relationships (DistinctCount < 20 should be excluded), got %d", result.InferredRelationships)
	}

	if len(mocks.relationshipRepo.created) != 0 {
		t.Errorf("expected 0 relationships to be created, got %d", len(mocks.relationshipRepo.created))
	}
}

// TestPKMatch_CountColumns_NeverJoined tests that columns with PurposeMeasure feature
// are excluded from being FK candidates (they should not try to join TO entity PKs)
func TestPKMatch_CountColumns_NeverJoined(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	accountEntityID := uuid.New()

	highDistinct := int64(100)
	candidateDistinct := int64(30) // Above cardinality threshold (>= 20) BUT below ratio threshold
	rowCount := int64(10000)       // High row count means ratio will be 30/10000 = 0.003 (< 0.05)
	isJoinableTrue := true

	// Create mocks with user entity
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            accountEntityID,
		OntologyID:    ontologyID,
		Name:          "account",
		PrimarySchema: "public",
		PrimaryTable:  "accounts",
	})

	// Mock discoverer should NOT be called because count columns are filtered by purpose (PurposeMeasure)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		t.Errorf("AnalyzeJoin should not be called (measure columns should be excluded by purpose): %s.%s.%s -> %s.%s.%s",
			sourceSchema, sourceTable, sourceColumn,
			targetSchema, targetTable, targetColumn)
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	usersTableID := uuid.New()
	accountsTableID := uuid.New()

	// Schema: users.id is a PK (entityRefColumn), accounts has count columns
	// Count columns should NOT be FK candidates - they are excluded by PURPOSE filter
	// (PurposeMeasure) before the cardinality ratio check would exclude them
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &highDistinct,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
			},
		},
		{
			ID:         accountsTableID,
			SchemaName: "public",
			TableName:  "accounts",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: accountsTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
				{
					SchemaTableID: accountsTableID,
					ColumnName:    "num_users", // Count column classified as measure
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct, // Has sufficient absolute distinct count (>= 20)
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeMeasure,
						},
					},
				},
				{
					SchemaTableID: accountsTableID,
					ColumnName:    "user_count", // Count column classified as measure
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeMeasure,
						},
					},
				},
				{
					SchemaTableID: accountsTableID,
					ColumnName:    "total_items", // Count column classified as measure
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeMeasure,
						},
					},
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: NO relationships created because count columns are excluded by purpose (PurposeMeasure)
	// Even though they have sufficient DistinctCount, the purpose filter should prevent them
	// from being FK candidates
	if result.InferredRelationships != 0 {
		t.Errorf("expected 0 inferred relationships (count columns should be excluded), got %d", result.InferredRelationships)
	}

	if len(mocks.relationshipRepo.created) != 0 {
		t.Errorf("expected 0 relationships to be created, got %d", len(mocks.relationshipRepo.created))
	}
}

// TestPKMatch_RatingColumns_NeverJoined tests that columns with PurposeMeasure feature
// are excluded from being FK candidates (they should not try to join TO entity PKs)
func TestPKMatch_RatingColumns_NeverJoined(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	reviewEntityID := uuid.New()

	highDistinct := int64(100)
	candidateDistinct := int64(30) // Above cardinality threshold (>= 20) BUT below ratio threshold
	rowCount := int64(10000)       // High row count means ratio will be 30/10000 = 0.003 (< 0.05)
	isJoinableTrue := true

	// Create mocks with user entity
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            reviewEntityID,
		OntologyID:    ontologyID,
		Name:          "review",
		PrimarySchema: "public",
		PrimaryTable:  "reviews",
	})

	// Mock discoverer should NOT be called because rating/score/level columns are filtered by purpose (PurposeMeasure)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		t.Errorf("AnalyzeJoin should not be called (measure columns should be excluded by purpose): %s.%s.%s -> %s.%s.%s",
			sourceSchema, sourceTable, sourceColumn,
			targetSchema, targetTable, targetColumn)
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	usersTableID := uuid.New()
	reviewsTableID := uuid.New()

	// Schema: users.id is a PK (entityRefColumn), reviews has rating/score/level columns
	// Rating/score/level columns should NOT be FK candidates - they are excluded by PURPOSE filter
	// (PurposeMeasure) before the cardinality ratio check would exclude them
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &highDistinct,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
			},
		},
		{
			ID:         reviewsTableID,
			SchemaName: "public",
			TableName:  "reviews",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: reviewsTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
				{
					SchemaTableID: reviewsTableID,
					ColumnName:    "rating", // Rating column classified as measure
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct, // Has sufficient absolute distinct count (>= 20)
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeMeasure,
						},
					},
				},
				{
					SchemaTableID: reviewsTableID,
					ColumnName:    "mod_level", // Level column classified as measure
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeMeasure,
						},
					},
				},
				{
					SchemaTableID: reviewsTableID,
					ColumnName:    "score", // Score column classified as measure
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeMeasure,
						},
					},
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: NO relationships created because rating/score/level columns are excluded by purpose (PurposeMeasure)
	// Even though they have sufficient DistinctCount, the purpose filter should prevent them
	// from being FK candidates
	if result.InferredRelationships != 0 {
		t.Errorf("expected 0 inferred relationships (rating/score/level columns should be excluded), got %d", result.InferredRelationships)
	}

	if len(mocks.relationshipRepo.created) != 0 {
		t.Errorf("expected 0 relationships to be created, got %d", len(mocks.relationshipRepo.created))
	}
}

// TestPKMatch_NoGarbageRelationships is the golden end-to-end test that verifies
// the complete fix prevents garbage relationships in real-ish scenarios.
// This test uses a realistic schema with accounts.num_users (a COUNT column)
// and payout_accounts.id (a PK), verifying that no relationship is inferred
// between them even though the values might overlap.
func TestPKMatch_NoGarbageRelationships(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	accountEntityID := uuid.New()
	payoutAccountEntityID := uuid.New()

	highDistinct := int64(1000)
	lowDistinct := int64(3) // num_users only has values 1, 2, 3
	rowCount := int64(5000)
	isJoinableTrue := true
	isJoinableFalse := false // num_users should be marked NOT joinable

	// Create mocks with both entities
	mocks := setupMocks(projectID, ontologyID, datasourceID, accountEntityID)
	mocks.entityRepo.entities[0].Name = "account"
	mocks.entityRepo.entities[0].PrimaryTable = "accounts"
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            payoutAccountEntityID,
		OntologyID:    ontologyID,
		Name:          "payout_account",
		PrimarySchema: "public",
		PrimaryTable:  "payout_accounts",
	})

	// Mock discoverer: if called, it would return 0 orphans (small integers exist in any PK sequence)
	// But this should NEVER be called because num_users should be filtered out
	maxSourceValue := int64(3)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		t.Errorf("AnalyzeJoin should not be called (num_users should be filtered out): %s.%s.%s -> %s.%s.%s",
			sourceSchema, sourceTable, sourceColumn,
			targetSchema, targetTable, targetColumn)
		return &datasource.JoinAnalysis{
			OrphanCount:    0, // Would match if we got here
			MaxSourceValue: &maxSourceValue,
		}, nil
	}

	accountsTableID := uuid.New()
	payoutAccountsTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         accountsTableID,
			SchemaName: "public",
			TableName:  "accounts",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: accountsTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
				{
					SchemaTableID: accountsTableID,
					ColumnName:    "num_users", // COUNT column - should NOT be a FK candidate
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableFalse, // Should be marked as NOT joinable
					DistinctCount: &lowDistinct,     // Only 3 distinct values
				},
			},
		},
		{
			ID:         payoutAccountsTableID,
			SchemaName: "public",
			TableName:  "payout_accounts",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: payoutAccountsTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: NO relationships created
	// This is the key assertion: accounts.num_users should NEVER create a relationship
	// with payout_accounts.id, even though their values might overlap
	if result.InferredRelationships != 0 {
		t.Errorf("GOLDEN TEST FAILED: expected 0 inferred relationships (num_users should not match payout_accounts.id), got %d", result.InferredRelationships)
	}

	if len(mocks.relationshipRepo.created) != 0 {
		t.Errorf("GOLDEN TEST FAILED: expected 0 relationships to be created, got %d relationships", len(mocks.relationshipRepo.created))
		for i, rel := range mocks.relationshipRepo.created {
			t.Logf("  Relationship %d: %s.%s -> %s.%s", i+1, rel.SourceColumnTable, rel.SourceColumnName, rel.TargetColumnTable, rel.TargetColumnName)
		}
	}
}

// TestFKDiscovery_ManualRelationshipType tests that schema relationships with
// relationship_type="manual" are skipped by Phase 2 (only FK relationships are processed).
// Manual relationships retain their original type and are not updated during FK discovery.
func TestFKDiscovery_ManualRelationshipType(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	usersTableID := uuid.New()
	channelsTableID := uuid.New()
	userIDColumnID := uuid.New()
	ownerIDColumnID := uuid.New()

	// Setup tables with columns (entities not required since FKDiscovery no longer depends on them)
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.SchemaColumn{
				{
					ID:            userIDColumnID,
					SchemaTableID: usersTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
				},
			},
		},
		{
			ID:         channelsTableID,
			SchemaName: "public",
			TableName:  "channels",
			Columns: []models.SchemaColumn{
				{
					ID:            ownerIDColumnID,
					SchemaTableID: channelsTableID,
					ColumnName:    "owner_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
				},
			},
		},
	}

	// Add manual schema relationship (no inference_method set, but has RelationshipType=manual)
	// Phase 2 only processes FK relationships (inference_method='foreign_key'), so manual ones are skipped
	manualMethod := models.RelationshipTypeManual
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    usersTableID,
			SourceColumnID:   userIDColumnID,
			TargetTableID:    channelsTableID,
			TargetColumnID:   ownerIDColumnID,
			RelationshipType: models.RelationshipTypeManual, // Manual relationship
			InferenceMethod:  &manualMethod,                 // Manual inference method
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: 0 FK relationships discovered (manual relationships are skipped by Phase 2)
	if result.FKRelationships != 0 {
		t.Errorf("expected 0 FK relationships (manual should be skipped), got %d", result.FKRelationships)
	}

	// Verify: no SchemaRelationships were upserted (manual relationships are not processed)
	if len(mocks.schemaRepo.upsertedRelationships) != 0 {
		t.Fatalf("expected 0 SchemaRelationships to be upserted, got %d", len(mocks.schemaRepo.upsertedRelationships))
	}
}

// TestFKDiscovery_ForeignKeyRelationshipType tests that schema relationships with
// inference_method="foreign_key" are processed by Phase 2 and updated with cardinality.
// These relationships are created during schema import and updated during FK discovery.
func TestFKDiscovery_ForeignKeyRelationshipType(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	// Create mocks (entities not required since FKDiscovery no longer depends on them)
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	userIDColumnID := uuid.New()
	buyerIDColumnID := uuid.New()

	// Setup tables with columns
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.SchemaColumn{
				{
					ID:            userIDColumnID,
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			Columns: []models.SchemaColumn{
				{
					ID:            buyerIDColumnID,
					SchemaTableID: ordersTableID,
					ColumnName:    "buyer_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
				},
			},
		},
	}

	// Add FK schema relationship with inference_method='foreign_key'
	fkMethod := models.InferenceMethodForeignKey
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    ordersTableID,
			SourceColumnID:   buyerIDColumnID,
			TargetTableID:    usersTableID,
			TargetColumnID:   userIDColumnID,
			RelationshipType: models.RelationshipTypeFK, // FK from DDL
			InferenceMethod:  &fkMethod,
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: 1 FK relationship processed
	if result.FKRelationships != 1 {
		t.Errorf("expected 1 FK relationship, got %d", result.FKRelationships)
	}

	// Verify: 1 SchemaRelationship upserted (unidirectional, not bidirectional)
	if len(mocks.schemaRepo.upsertedRelationships) != 1 {
		t.Fatalf("expected 1 SchemaRelationship to be upserted, got %d", len(mocks.schemaRepo.upsertedRelationships))
	}

	// Verify: upserted relationship has IsValidated=true and cardinality computed
	updatedRel := mocks.schemaRepo.upsertedRelationships[0]
	if !updatedRel.IsValidated {
		t.Errorf("expected IsValidated=true, got false")
	}
	if updatedRel.Cardinality == "" {
		t.Errorf("expected Cardinality to be set, got empty string")
	}
}

// TestFKDiscovery_SelfReferentialRelationship tests that self-referential FK constraints
// (e.g., employee.manager_id → employee.id) are processed correctly.
// These represent hierarchies/trees and are intentional design patterns.
func TestFKDiscovery_SelfReferentialRelationship(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	employeeEntityID := uuid.New()

	// Create mocks (entities not required since FKDiscovery no longer depends on them)
	mocks := setupMocks(projectID, ontologyID, datasourceID, employeeEntityID)

	employeesTableID := uuid.New()
	idColumnID := uuid.New()
	managerIDColumnID := uuid.New()

	// Setup employees table with self-referential FK
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         employeesTableID,
			SchemaName: "public",
			TableName:  "employees",
			Columns: []models.SchemaColumn{
				{
					ID:            idColumnID,
					SchemaTableID: employeesTableID,
					ColumnName:    "id",
					DataType:      "integer",
					IsPrimaryKey:  true,
				},
				{
					ID:            managerIDColumnID,
					SchemaTableID: employeesTableID,
					ColumnName:    "manager_id",
					DataType:      "integer",
					IsPrimaryKey:  false,
				},
			},
		},
	}

	// Add self-referential FK schema relationship (manager_id → id in same table)
	fkMethod := models.InferenceMethodForeignKey
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    employeesTableID,
			SourceColumnID:   managerIDColumnID,
			TargetTableID:    employeesTableID, // Same table
			TargetColumnID:   idColumnID,       // References own PK
			RelationshipType: models.RelationshipTypeFK,
			InferenceMethod:  &fkMethod,
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: 1 FK relationship processed
	if result.FKRelationships != 1 {
		t.Errorf("expected 1 FK relationship, got %d", result.FKRelationships)
	}

	// Verify: 1 SchemaRelationship upserted (unidirectional)
	if len(mocks.schemaRepo.upsertedRelationships) != 1 {
		t.Fatalf("expected 1 SchemaRelationship to be upserted, got %d", len(mocks.schemaRepo.upsertedRelationships))
	}

	// Verify: self-referential relationship is correctly stored
	updatedRel := mocks.schemaRepo.upsertedRelationships[0]
	if updatedRel.SourceTableID != employeesTableID {
		t.Errorf("expected source table to be employees, got %v", updatedRel.SourceTableID)
	}
	if updatedRel.TargetTableID != employeesTableID {
		t.Errorf("expected target table to be employees (self-reference), got %v", updatedRel.TargetTableID)
	}
	if updatedRel.SourceColumnID != managerIDColumnID {
		t.Errorf("expected source column to be manager_id, got %v", updatedRel.SourceColumnID)
	}
	if updatedRel.TargetColumnID != idColumnID {
		t.Errorf("expected target column to be id, got %v", updatedRel.TargetColumnID)
	}
	if !updatedRel.IsValidated {
		t.Errorf("expected IsValidated=true, got false")
	}
}

// TestPKMatch_LowCardinalityRatio tests that columns with very low cardinality
// relative to row count are rejected (status/type columns)
func TestPKMatch_LowCardinalityRatio(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	accountEntityID := uuid.New()
	userEntityID := uuid.New()

	distinctCount := int64(100)
	lowDistinctCount := int64(5) // Only 5 distinct values
	rowCount := int64(10000)     // 10,000 rows -> ratio = 0.0005 (0.05%)
	isJoinableTrue := true

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, accountEntityID)
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            userEntityID,
		OntologyID:    ontologyID,
		Name:          "user",
		PrimarySchema: "public",
		PrimaryTable:  "users",
	})

	// Mock discoverer: if called, returns 0 orphans
	maxSourceValue := int64(5)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			OrphanCount:    0,
			MaxSourceValue: &maxSourceValue,
		}, nil
	}

	accountsTableID := uuid.New()
	usersTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         accountsTableID,
			SchemaName: "public",
			TableName:  "accounts",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: accountsTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &rowCount, // 10,000 rows
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &rowCount,
				},
				{
					SchemaTableID: usersTableID,
					ColumnName:    "mod_level", // Only 5 values in 10k rows = 0.05% unique
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &lowDistinctCount, // Very low cardinality
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: NO relationships created due to low cardinality ratio (< 1%)
	if result.InferredRelationships != 0 {
		t.Errorf("expected 0 inferred relationships (low cardinality ratio should be rejected), got %d", result.InferredRelationships)
	}

	if len(mocks.relationshipRepo.created) != 0 {
		t.Errorf("expected no relationships in repo, got %d", len(mocks.relationshipRepo.created))
	}
}

// TestFKDiscovery_LegitimateFK_EmailNotDiscovered verifies that:
// 1. Actual DB FK constraints are discovered (account_authentications.account_id → accounts.account_id)
// 2. email→email is NOT discovered (FK discovery only follows FK constraints, not column name patterns)
//
// This is the end-to-end validation test for BUG-4 fixes.
func TestFKDiscovery_LegitimateFK_EmailNotDiscovered(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	accountEntityID := uuid.New()

	// Create mocks (entities not required since FKDiscovery no longer depends on them)
	mocks := setupMocks(projectID, ontologyID, datasourceID, accountEntityID)

	accountsTableID := uuid.New()
	accountAuthTableID := uuid.New()
	accountIDColumnID := uuid.New()
	accountEmailColumnID := uuid.New()
	authAccountIDColumnID := uuid.New()
	authEmailColumnID := uuid.New()

	rowCount := int64(1000)
	authRowCount := int64(500)
	distinctCount := int64(1000)
	authDistinctCount := int64(500)
	isJoinableTrue := true

	// Setup tables: accounts and account_authentications
	// accounts has: account_id (PK), email
	// account_authentications has: account_id (FK → accounts.account_id), email
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         accountsTableID,
			SchemaName: "public",
			TableName:  "accounts",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            accountIDColumnID,
					SchemaTableID: accountsTableID,
					ColumnName:    "account_id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
				{
					ID:            accountEmailColumnID,
					SchemaTableID: accountsTableID,
					ColumnName:    "email",
					DataType:      "varchar",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
		{
			ID:         accountAuthTableID,
			SchemaName: "public",
			TableName:  "account_authentications",
			RowCount:   &authRowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            authAccountIDColumnID,
					SchemaTableID: accountAuthTableID,
					ColumnName:    "account_id",
					DataType:      "uuid",
					IsPrimaryKey:  false, // FK, not PK
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &authDistinctCount,
				},
				{
					ID:            authEmailColumnID,
					SchemaTableID: accountAuthTableID,
					ColumnName:    "email",
					DataType:      "varchar",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &authDistinctCount,
				},
			},
		},
	}

	// Add actual FK schema relationship: account_authentications.account_id → accounts.account_id
	// This simulates what the schema discovery would find from a FOREIGN KEY constraint
	fkMethod := models.InferenceMethodForeignKey
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    accountAuthTableID,    // FK holder
			SourceColumnID:   authAccountIDColumnID, // FK column
			TargetTableID:    accountsTableID,       // PK holder
			TargetColumnID:   accountIDColumnID,     // PK column
			RelationshipType: models.RelationshipTypeFK,
			InferenceMethod:  &fkMethod,
		},
	}

	// Suppress unused variable warnings for email column IDs used in test setup
	_ = accountEmailColumnID
	_ = authEmailColumnID

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Run FK discovery
	result, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: 1 FK relationship processed
	if result.FKRelationships != 1 {
		t.Errorf("expected 1 FK relationship, got %d", result.FKRelationships)
	}

	// Verify: 1 SchemaRelationship upserted (unidirectional)
	if len(mocks.schemaRepo.upsertedRelationships) != 1 {
		t.Fatalf("expected 1 SchemaRelationship to be upserted, got %d", len(mocks.schemaRepo.upsertedRelationships))
	}

	// Verify: upserted relationship is account_authentications.account_id → accounts.account_id
	updatedRel := mocks.schemaRepo.upsertedRelationships[0]
	if updatedRel.SourceTableID != accountAuthTableID {
		t.Errorf("expected source table 'account_authentications', got table ID %v", updatedRel.SourceTableID)
	}
	if updatedRel.TargetTableID != accountsTableID {
		t.Errorf("expected target table 'accounts', got table ID %v", updatedRel.TargetTableID)
	}
	if updatedRel.SourceColumnID != authAccountIDColumnID {
		t.Errorf("expected source column 'account_id', got column ID %v", updatedRel.SourceColumnID)
	}
	if updatedRel.TargetColumnID != accountIDColumnID {
		t.Errorf("expected target column 'account_id', got column ID %v", updatedRel.TargetColumnID)
	}
	if !updatedRel.IsValidated {
		t.Errorf("expected IsValidated=true, got false")
	}
}

// TestPKMatch_EmailColumnsExcluded verifies that email columns are excluded from
// PK match discovery even if they have high cardinality and could technically join.
// This tests the attribute column exclusion logic for inferred relationships.
func TestPKMatch_EmailColumnsExcluded(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	accountEntityID := uuid.New()

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, accountEntityID)

	// Replace default user entity with account entity
	mocks.entityRepo.entities = []*models.OntologyEntity{
		{
			ID:            accountEntityID,
			OntologyID:    ontologyID,
			Name:          "account",
			PrimarySchema: "public",
			PrimaryTable:  "accounts",
		},
	}

	accountsTableID := uuid.New()
	accountAuthTableID := uuid.New()

	// Track email column IDs to verify they're excluded from relationships
	accountsEmailColID := uuid.New()
	authEmailColID := uuid.New()

	rowCount := int64(1000)
	authRowCount := int64(500)
	idDistinctCount := int64(1000)    // High cardinality for id columns
	authIdDistinctCount := int64(500) // High cardinality for id columns
	emailDistinctCount := int64(10)   // Low cardinality so email isn't a target column
	isJoinableTrue := true
	isJoinableFalse := false

	// Setup tables with email columns that have overlapping values
	// Email columns are NOT joinable (IsJoinable=false) and have Purpose=text from ColumnFeatureExtraction
	// They should be excluded from FK candidate consideration
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         accountsTableID,
			SchemaName: "public",
			TableName:  "accounts",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: accountsTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &idDistinctCount,
				},
				{
					ID:            accountsEmailColID,
					SchemaTableID: accountsTableID,
					ColumnName:    "email",
					DataType:      "varchar",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableFalse,    // Email columns are not joinable
					DistinctCount: &emailDistinctCount, // Low cardinality - won't be target column
					// Email columns are classified as "text" by ColumnFeatureExtraction
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeText,
						},
					},
				},
			},
		},
		{
			ID:         accountAuthTableID,
			SchemaName: "public",
			TableName:  "account_authentications",
			RowCount:   &authRowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: accountAuthTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &authIdDistinctCount,
				},
				{
					ID:            authEmailColID,
					SchemaTableID: accountAuthTableID,
					ColumnName:    "email",
					DataType:      "varchar",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableFalse,    // Email columns are not joinable
					DistinctCount: &emailDistinctCount, // Low cardinality - won't be target column
					// Email columns are classified as "text" by ColumnFeatureExtraction
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeText,
						},
					},
				},
			},
		},
	}

	// Mock join analysis to return perfect overlap for email columns
	// This simulates the case where emails DO match (same users in both tables)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		// Return 0 orphans - perfect match
		return &datasource.JoinAnalysis{
			OrphanCount: 0,
		}, nil
	}

	// No FK relationships (we're testing PK match discovery)
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Run PK match discovery
	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: NO email column relationships in the schema repo
	// Email columns should be excluded because:
	// 1. They have Purpose=text (not identifier), so not considered FK candidates
	// 2. They have low cardinality, so not considered target columns
	// 3. They have IsJoinable=false
	for _, rel := range mocks.schemaRepo.upsertedRelationships {
		if rel.SourceColumnID == accountsEmailColID || rel.SourceColumnID == authEmailColID ||
			rel.TargetColumnID == accountsEmailColID || rel.TargetColumnID == authEmailColID {
			t.Errorf("email column relationship should NOT be discovered: source=%v, target=%v",
				rel.SourceColumnID, rel.TargetColumnID)
		}
	}

	// Verify no relationships were discovered at all - this is expected since:
	// - The only high-cardinality columns are PKs (id columns)
	// - The id columns are not FK candidates (they're PKs, not FKs)
	// - Email columns are excluded from both candidates and targets
	if result.InferredRelationships != 0 {
		t.Errorf("expected 0 inferred relationships, got %d", result.InferredRelationships)
	}
}

// TestPKMatch_ReversedDirectionRejected verifies that when PK match discovery
// considers a relationship, the zero-orphan requirement rejects the wrong direction.
//
// Scenario: accounts (100 rows) and account_password_resets (60 rows, 50 distinct account_ids)
// - accounts.account_id → account_password_resets.account_id has 50 orphans (accounts without resets)
// - account_password_resets.account_id → accounts.account_id has 0 orphans (all resets have valid accounts)
//
// The zero-orphan requirement ensures only the correct FK direction is discovered.
// Note: PK match creates bidirectional relationships for navigation, so the "reverse" in the
// created relationships is NOT the same as "wrong direction" - it's the navigation reverse.
// TestPKMatch_LowCardinalityRatio_IDColumn tests that _id columns bypass the cardinality
// ratio check, allowing FK discovery even with low cardinality relative to row count.
// This fixes BUG-9 where visitor_id (500 unique visitors in 100,000 rows = 0.5%) was
// incorrectly filtered out by the 5% cardinality ratio threshold.
func TestPKMatch_LowCardinalityRatio_IDColumn(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	billingEngagementEntityID := uuid.New()

	userDistinct := int64(500)       // 500 unique users
	billingRowCount := int64(100000) // 100k billing engagements
	billingDistinct := int64(100000) // all unique
	visitorDistinct := int64(500)    // only 500 unique visitors = 0.5% ratio (below old 5% threshold)
	userRowCount := int64(500)       // same as distinct for users
	isJoinableTrue := true

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Add billing_engagement entity
	mocks.entityRepo.entities = []*models.OntologyEntity{
		{
			ID:            userEntityID,
			OntologyID:    ontologyID,
			Name:          "user",
			PrimarySchema: "public",
			PrimaryTable:  "users",
		},
		{
			ID:            billingEngagementEntityID,
			OntologyID:    ontologyID,
			Name:          "billing_engagement",
			PrimarySchema: "public",
			PrimaryTable:  "billing_engagements",
		},
	}

	// Mock discoverer returns 0 orphans (all visitor_ids have matching user)
	joinAnalysisCalled := false
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		// Track that join analysis was called for visitor_id
		if sourceColumn == "visitor_id" {
			joinAnalysisCalled = true
		}
		return &datasource.JoinAnalysis{
			OrphanCount: 0, // All references valid
		}, nil
	}

	usersTableID := uuid.New()
	billingTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &userRowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "user_id",
					DataType:      "text", // text UUID
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &userDistinct,
				},
			},
		},
		{
			ID:         billingTableID,
			SchemaName: "public",
			TableName:  "billing_engagements",
			RowCount:   &billingRowCount, // 100k rows
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: billingTableID,
					ColumnName:    "engagement_id",
					DataType:      "text",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &billingDistinct,
				},
				{
					SchemaTableID: billingTableID,
					ColumnName:    "visitor_id", // FK to users.user_id
					DataType:      "text",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &visitorDistinct, // 500/100k = 0.5% (below 5% threshold)
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: visitor_id should have passed the cardinality filter because it has PurposeIdentifier
	if !joinAnalysisCalled {
		t.Error("expected join analysis to be called for visitor_id column (should bypass cardinality ratio check for identifier columns)")
	}

	// Verify: relationship created (visitor_id → user_id)
	if result.InferredRelationships < 1 {
		t.Errorf("expected at least 1 inferred relationship (visitor_id → user_id), got %d", result.InferredRelationships)
	}

	// Verify: at least one relationship exists for billing_engagements.visitor_id → users.user_id
	var foundVisitorRelationship bool
	for _, rel := range mocks.schemaRepo.upsertedRelationships {
		// SchemaRelationship uses table IDs, so check for billing_engagements -> users
		if rel.SourceTableID == billingTableID && rel.TargetTableID == usersTableID {
			foundVisitorRelationship = true
			break
		}
	}
	if !foundVisitorRelationship {
		t.Error("expected relationship billing_engagements.visitor_id → users.user_id to be discovered")
	}
}

// TestPKMatch_BillingEngagement_MultiSoftFK_Discovery tests that billing_engagements
// with multiple soft FK columns (visitor_id, host_id, session_id, offer_id) discovers
// 4+ relationships. This is the verification test for BUG-9 fix.
//
// Scenario: billing_engagements table has:
// - visitor_id → users.user_id (role: visitor)
// - host_id → users.user_id (role: host)
// - session_id → sessions.session_id
// - offer_id → offers.offer_id
//
// All FKs use text UUIDs (soft FKs) with low cardinality ratios that would have been
// filtered out by the old 5% threshold.
func TestPKMatch_BillingEngagement_MultiSoftFK_Discovery(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()

	// Entity IDs
	userEntityID := uuid.New()
	billingEngagementEntityID := uuid.New()
	sessionEntityID := uuid.New()
	offerEntityID := uuid.New()

	// Stats - simulate low cardinality scenarios that old threshold would filter
	billingRowCount := int64(100000) // 100k billing engagements
	userRowCount := int64(1000)      // 1k users
	sessionRowCount := int64(5000)   // 5k sessions
	offerRowCount := int64(200)      // 200 offers
	billingDistinct := int64(100000) // all unique
	userDistinct := int64(1000)      // all unique
	sessionDistinct := int64(5000)   // all unique
	offerDistinct := int64(200)      // all unique
	visitorDistinct := int64(800)    // 800/100k = 0.8% (below old 5% threshold)
	hostDistinct := int64(500)       // 500/100k = 0.5% (below old 5% threshold)
	sessionFKDistinct := int64(3000) // 3000/100k = 3% (below old 5% threshold)
	offerFKDistinct := int64(150)    // 150/100k = 0.15% (below old 5% threshold)
	isJoinableTrue := true

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Setup entities
	mocks.entityRepo.entities = []*models.OntologyEntity{
		{
			ID:            userEntityID,
			OntologyID:    ontologyID,
			Name:          "user",
			PrimarySchema: "public",
			PrimaryTable:  "users",
		},
		{
			ID:            billingEngagementEntityID,
			OntologyID:    ontologyID,
			Name:          "billing_engagement",
			PrimarySchema: "public",
			PrimaryTable:  "billing_engagements",
		},
		{
			ID:            sessionEntityID,
			OntologyID:    ontologyID,
			Name:          "session",
			PrimarySchema: "public",
			PrimaryTable:  "sessions",
		},
		{
			ID:            offerEntityID,
			OntologyID:    ontologyID,
			Name:          "offer",
			PrimarySchema: "public",
			PrimaryTable:  "offers",
		},
	}

	// Track which columns had join analysis called
	joinAnalysisCalls := make(map[string]bool)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		key := fmt.Sprintf("%s.%s→%s.%s", sourceTable, sourceColumn, targetTable, targetColumn)
		joinAnalysisCalls[key] = true
		return &datasource.JoinAnalysis{
			OrphanCount: 0, // All references valid
		}, nil
	}

	// Table IDs
	usersTableID := uuid.New()
	billingTableID := uuid.New()
	sessionsTableID := uuid.New()
	offersTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &userRowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "user_id",
					DataType:      "text", // text UUID
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &userDistinct,
				},
			},
		},
		{
			ID:         sessionsTableID,
			SchemaName: "public",
			TableName:  "sessions",
			RowCount:   &sessionRowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: sessionsTableID,
					ColumnName:    "session_id",
					DataType:      "text", // text UUID
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &sessionDistinct,
				},
			},
		},
		{
			ID:         offersTableID,
			SchemaName: "public",
			TableName:  "offers",
			RowCount:   &offerRowCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: offersTableID,
					ColumnName:    "offer_id",
					DataType:      "text", // text UUID
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &offerDistinct,
				},
			},
		},
		{
			ID:         billingTableID,
			SchemaName: "public",
			TableName:  "billing_engagements",
			RowCount:   &billingRowCount, // 100k rows
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: billingTableID,
					ColumnName:    "engagement_id",
					DataType:      "text",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &billingDistinct,
				},
				{
					SchemaTableID: billingTableID,
					ColumnName:    "visitor_id", // FK to users.user_id (role: visitor)
					DataType:      "text",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &visitorDistinct, // 0.8% cardinality
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
				{
					SchemaTableID: billingTableID,
					ColumnName:    "host_id", // FK to users.user_id (role: host)
					DataType:      "text",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &hostDistinct, // 0.5% cardinality
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
				{
					SchemaTableID: billingTableID,
					ColumnName:    "session_id", // FK to sessions.session_id
					DataType:      "text",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &sessionFKDistinct, // 3% cardinality
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
				{
					SchemaTableID: billingTableID,
					ColumnName:    "offer_id", // FK to offers.offer_id
					DataType:      "text",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &offerFKDistinct, // 0.15% cardinality
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: All 4 FK columns should have had join analysis called
	expectedJoinCalls := []string{
		"billing_engagements.visitor_id",
		"billing_engagements.host_id",
		"billing_engagements.session_id",
		"billing_engagements.offer_id",
	}
	for _, col := range expectedJoinCalls {
		found := false
		for key := range joinAnalysisCalls {
			if strings.HasPrefix(key, col+"→") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected join analysis to be called for %s (should bypass cardinality ratio check for identifier columns)", col)
		}
	}

	// Verify: At least 4 inferred relationships
	if result.InferredRelationships < 4 {
		t.Errorf("expected at least 4 inferred relationships (billing_engagements → users, sessions, offers), got %d", result.InferredRelationships)
	}

	// Verify: Specific relationships exist
	// PKMatchDiscovery now writes to SchemaRelationship (unidirectional)
	// Map table IDs to table names for verification
	type expectedRel struct {
		sourceTableID uuid.UUID
		targetTableID uuid.UUID
		description   string
	}
	expectedRels := []expectedRel{
		{billingTableID, usersTableID, "billing_engagements.visitor_id → users.user_id"},
		{billingTableID, usersTableID, "billing_engagements.host_id → users.user_id"},
		{billingTableID, sessionsTableID, "billing_engagements.session_id → sessions.session_id"},
		{billingTableID, offersTableID, "billing_engagements.offer_id → offers.offer_id"},
	}

	// Count how many expected relationships were found
	// Note: visitor_id and host_id both go to users, so we expect 4 unique source->target pairs
	// but since there are 2 FKs to users, we need to find at least 4 SchemaRelationships total
	foundTargets := make(map[uuid.UUID]int) // targetTableID -> count of relationships to it
	for _, rel := range mocks.schemaRepo.upsertedRelationships {
		if rel.SourceTableID == billingTableID {
			foundTargets[rel.TargetTableID]++
		}
	}

	// Verify we found relationships to all expected targets
	expectedTargets := map[uuid.UUID]string{
		usersTableID:    "users (expecting 2: visitor_id, host_id)",
		sessionsTableID: "sessions",
		offersTableID:   "offers",
	}
	for targetID, desc := range expectedTargets {
		if foundTargets[targetID] == 0 {
			t.Errorf("expected relationship billing_engagements → %s to be discovered", desc)
		}
	}
	// Should have at least 2 relationships to users (visitor_id and host_id)
	if foundTargets[usersTableID] < 2 {
		t.Errorf("expected 2 relationships to users (visitor_id, host_id), got %d", foundTargets[usersTableID])
	}

	// Log summary for debugging
	t.Logf("Total SchemaRelationships created: %d", len(mocks.schemaRepo.upsertedRelationships))
	for i, rel := range mocks.schemaRepo.upsertedRelationships {
		t.Logf("  %d: tableID=%v → tableID=%v (method=%v)",
			i+1, rel.SourceTableID, rel.TargetTableID, rel.InferenceMethod)
	}
	_ = expectedRels // Silence unused variable warning for documentation
}

// NOTE: TestIsLikelyFKColumn has been removed.
// FK column identification now uses stored ColumnFeatures.Purpose instead of name-based patterns.
// See PLAN-extracting-column-features.md for details.

// TestPKMatch_NonIDColumn_StillFilteredByCardinality verifies that non-_id columns
// still get filtered by the cardinality ratio check.
func TestPKMatch_NonIDColumn_StillFilteredByCardinality(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	accountEntityID := uuid.New()
	userEntityID := uuid.New()

	distinctCount := int64(100)
	lowDistinctCount := int64(30) // 30 distinct values
	rowCount := int64(10000)      // 10,000 rows -> ratio = 0.003 (0.3%) < 5%
	isJoinableTrue := true

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, accountEntityID)
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            userEntityID,
		OntologyID:    ontologyID,
		Name:          "user",
		PrimarySchema: "public",
		PrimaryTable:  "users",
	})

	// Mock discoverer should NOT be called for "buyer" column (filtered by cardinality ratio)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		if sourceColumn == "buyer" {
			t.Errorf("AnalyzeJoin should not be called for 'buyer' column (non-_id column should be filtered by cardinality ratio)")
		}
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	accountsTableID := uuid.New()
	usersTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         accountsTableID,
			SchemaName: "public",
			TableName:  "accounts",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: accountsTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &rowCount, // 10,000 rows
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "bigint",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &rowCount,
				},
				{
					SchemaTableID: usersTableID,
					ColumnName:    "buyer", // Non-_id column with low cardinality ratio
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &lowDistinctCount, // 30/10000 = 0.3% < 5%
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: NO relationships for the "buyer" column due to low cardinality ratio
	// (non-_id columns still get filtered by the 5% threshold)
	for _, rel := range mocks.relationshipRepo.created {
		if rel.SourceColumnName == "buyer" || rel.TargetColumnName == "buyer" {
			t.Errorf("unexpected relationship involving 'buyer' column: %s.%s → %s.%s",
				rel.SourceColumnTable, rel.SourceColumnName,
				rel.TargetColumnTable, rel.TargetColumnName)
		}
	}

	// The result may include other relationships (like id columns), but buyer should not appear
	_ = result
}

func TestPKMatch_ReversedDirectionRejected(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	accountEntityID := uuid.New()
	passwordResetEntityID := uuid.New()

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, accountEntityID)

	// Add password_reset entity
	mocks.entityRepo.entities = []*models.OntologyEntity{
		{
			ID:            accountEntityID,
			OntologyID:    ontologyID,
			Name:          "account",
			PrimarySchema: "public",
			PrimaryTable:  "accounts",
		},
		{
			ID:            passwordResetEntityID,
			OntologyID:    ontologyID,
			Name:          "password_reset",
			PrimarySchema: "public",
			PrimaryTable:  "account_password_resets",
		},
	}

	accountsTableID := uuid.New()
	passwordResetTableID := uuid.New()

	// accounts has 100 distinct accounts
	// password_resets only has 50 distinct accounts (only 50 accounts have reset passwords)
	accountRowCount := int64(100)
	resetRowCount := int64(60) // 60 reset records for 50 distinct accounts
	accountDistinct := int64(100)
	resetDistinct := int64(50) // Only 50 accounts have password resets
	isJoinableTrue := true

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         accountsTableID,
			SchemaName: "public",
			TableName:  "accounts",
			RowCount:   &accountRowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: accountsTableID,
					ColumnName:    "account_id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &accountDistinct,
				},
			},
		},
		{
			ID:         passwordResetTableID,
			SchemaName: "public",
			TableName:  "account_password_resets",
			RowCount:   &resetRowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: passwordResetTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &resetRowCount,
				},
				{
					ID:            uuid.New(),
					SchemaTableID: passwordResetTableID,
					ColumnName:    "account_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &resetDistinct,
				},
			},
		},
	}

	// Track which join analyses were performed to verify both directions were tested
	joinAnalysisCalls := make(map[string]int)

	// Mock join analysis
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		key := fmt.Sprintf("%s.%s.%s→%s.%s.%s", sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn)
		joinAnalysisCalls[key]++

		// accounts.account_id → password_resets (wrong direction) has 50 orphans
		if sourceTable == "accounts" && sourceColumn == "account_id" &&
			targetTable == "account_password_resets" && targetColumn == "account_id" {
			return &datasource.JoinAnalysis{
				OrphanCount: 50, // 50 accounts have no password reset - would cause orphans
			}, nil
		}

		// password_resets.account_id → accounts.account_id (correct direction) has 0 orphans
		if sourceTable == "account_password_resets" && sourceColumn == "account_id" &&
			targetTable == "accounts" && targetColumn == "account_id" {
			return &datasource.JoinAnalysis{
				OrphanCount: 0, // All password resets have valid account references
			}, nil
		}

		// Default: high orphan count (no match)
		return &datasource.JoinAnalysis{OrphanCount: 100}, nil
	}

	// No FK relationships (testing inference only)
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Run PK match discovery
	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: 1 inferred relationship (correct direction only)
	// PKMatchDiscovery now writes unidirectional SchemaRelationships
	if result.InferredRelationships != 1 {
		t.Errorf("expected 1 inferred relationship (correct direction), got %d", result.InferredRelationships)
	}

	// Verify: 1 SchemaRelationship in repo (unidirectional)
	if len(mocks.schemaRepo.upsertedRelationships) != 1 {
		t.Errorf("expected 1 SchemaRelationship, got %d", len(mocks.schemaRepo.upsertedRelationships))
	}

	// Verify: The relationship should be the correct FK direction
	// (account_password_resets.account_id → accounts.account_id)
	if len(mocks.schemaRepo.upsertedRelationships) >= 1 {
		rel := mocks.schemaRepo.upsertedRelationships[0]
		if rel.SourceTableID != passwordResetTableID || rel.TargetTableID != accountsTableID {
			t.Errorf("expected direction account_password_resets→accounts, got tableIDs %v→%v",
				rel.SourceTableID, rel.TargetTableID)
		}
		if rel.InferenceMethod == nil || *rel.InferenceMethod != models.InferenceMethodPKMatch {
			t.Errorf("expected InferenceMethod=%q, got %v", models.InferenceMethodPKMatch, rel.InferenceMethod)
		}
	}
}

// ============================================================================
// areTypesCompatibleForFK Tests
// ============================================================================

func TestAreTypesCompatibleForFK_ExactMatch(t *testing.T) {
	tests := []struct {
		source, target string
		want           bool
	}{
		{"uuid", "uuid", true},
		{"text", "text", true},
		{"integer", "integer", true},
		{"bigint", "bigint", true},
	}

	for _, tt := range tests {
		got := areTypesCompatibleForFK(tt.source, tt.target)
		if got != tt.want {
			t.Errorf("areTypesCompatibleForFK(%q, %q) = %v, want %v", tt.source, tt.target, got, tt.want)
		}
	}
}

func TestAreTypesCompatibleForFK_UUIDCompatibility(t *testing.T) {
	// UUID types should be compatible with each other (text ↔ uuid ↔ varchar ↔ character varying)
	uuidTypes := []string{"uuid", "text", "varchar", "character varying"}

	for _, source := range uuidTypes {
		for _, target := range uuidTypes {
			got := areTypesCompatibleForFK(source, target)
			if !got {
				t.Errorf("areTypesCompatibleForFK(%q, %q) = false, want true (UUID compatibility)", source, target)
			}
		}
	}
}

func TestAreTypesCompatibleForFK_IntegerCompatibility(t *testing.T) {
	// Integer types should be compatible with each other
	intTypes := []string{"int", "int2", "int4", "int8", "integer", "bigint", "smallint", "serial", "bigserial"}

	for _, source := range intTypes {
		for _, target := range intTypes {
			got := areTypesCompatibleForFK(source, target)
			if !got {
				t.Errorf("areTypesCompatibleForFK(%q, %q) = false, want true (integer compatibility)", source, target)
			}
		}
	}
}

func TestAreTypesCompatibleForFK_IncompatibleTypes(t *testing.T) {
	tests := []struct {
		source, target string
	}{
		{"integer", "uuid"},
		{"integer", "text"},
		{"bigint", "varchar"},
		{"uuid", "integer"},
		{"text", "bigint"},
	}

	for _, tt := range tests {
		got := areTypesCompatibleForFK(tt.source, tt.target)
		if got {
			t.Errorf("areTypesCompatibleForFK(%q, %q) = true, want false (incompatible types)", tt.source, tt.target)
		}
	}
}

func TestAreTypesCompatibleForFK_NormalizesLengthSpecifiers(t *testing.T) {
	tests := []struct {
		source, target string
		want           bool
	}{
		{"varchar(255)", "varchar(100)", true},   // Same type, different lengths
		{"varchar(36)", "uuid", true},            // varchar(36) is UUID compatible
		{"character varying(255)", "text", true}, // character varying is UUID compatible
		{"int(11)", "integer", true},             // MySQL-style int with display width
		{"INT(11)", "INTEGER", true},             // Case insensitive
	}

	for _, tt := range tests {
		got := areTypesCompatibleForFK(tt.source, tt.target)
		if got != tt.want {
			t.Errorf("areTypesCompatibleForFK(%q, %q) = %v, want %v", tt.source, tt.target, got, tt.want)
		}
	}
}

func TestAreTypesCompatibleForFK_CaseInsensitive(t *testing.T) {
	tests := []struct {
		source, target string
		want           bool
	}{
		{"UUID", "uuid", true},
		{"Text", "TEXT", true},
		{"INTEGER", "integer", true},
		{"VARCHAR", "text", true},
	}

	for _, tt := range tests {
		got := areTypesCompatibleForFK(tt.source, tt.target)
		if got != tt.want {
			t.Errorf("areTypesCompatibleForFK(%q, %q) = %v, want %v", tt.source, tt.target, got, tt.want)
		}
	}
}

// ============================================================================
// Data-Based FK Detection Tests (UseLegacyPatternMatching=false)
// ============================================================================

// TestPKMatch_DataBased_ExcludedNamesNotFiltered verifies that when
// UseLegacyPatternMatching=false, columns with "excluded" name patterns
// (like num_users, rating, score) are still considered as FK candidates.
// This is the new behavior where only data (joinability, cardinality) matters.
func TestPKMatch_DataBased_ExcludedNamesNotFiltered(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	productEntityID := uuid.New()
	categoryEntityID := uuid.New()

	highDistinct := int64(100)
	isJoinableTrue := true

	// Create mocks with legacy pattern matching DISABLED
	mocks := setupMocks(projectID, ontologyID, datasourceID, productEntityID)
	mocks.projectService.ontologySettings = &OntologySettings{UseLegacyPatternMatching: false}

	mocks.entityRepo.entities = []*models.OntologyEntity{
		{
			ID:            productEntityID,
			OntologyID:    ontologyID,
			Name:          "product",
			PrimarySchema: "public",
			PrimaryTable:  "products",
		},
		{
			ID:            categoryEntityID,
			OntologyID:    ontologyID,
			Name:          "category",
			PrimarySchema: "public",
			PrimaryTable:  "categories",
		},
	}

	// Track join analysis calls
	joinAnalysisCalls := make(map[string]bool)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		key := fmt.Sprintf("%s.%s→%s.%s", sourceTable, sourceColumn, targetTable, targetColumn)
		joinAnalysisCalls[key] = true
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil // 0 orphans = valid FK
	}

	productsTableID := uuid.New()
	categoriesTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         productsTableID,
			SchemaName: "public",
			TableName:  "products",
			RowCount:   &highDistinct,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: productsTableID,
					ColumnName:    "id",
					DataType:      "integer",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
				// "num_products" would be excluded by legacy pattern matching but should
				// be included when data-based detection is enabled
				{
					SchemaTableID: productsTableID,
					ColumnName:    "num_products",
					DataType:      "integer",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct, // High distinct = good FK candidate
				},
			},
		},
		{
			ID:         categoriesTableID,
			SchemaName: "public",
			TableName:  "categories",
			RowCount:   &highDistinct,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: categoriesTableID,
					ColumnName:    "id",
					DataType:      "integer",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	_, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With data-based detection, "num_products" should have been evaluated
	// (not filtered out by name pattern)
	numProductsEvaluated := false
	for key := range joinAnalysisCalls {
		if strings.Contains(key, "num_products") {
			numProductsEvaluated = true
			break
		}
	}
	if !numProductsEvaluated {
		t.Error("expected 'num_products' to be evaluated as FK candidate when UseLegacyPatternMatching=false")
	}
}

// TestPKMatch_DataBased_RequiresExplicitJoinability verifies that when
// UseLegacyPatternMatching=false, ALL columns (even those with _id suffix)
// must have explicit IsJoinable=true to be considered as SOURCE candidates.
//
// Note: This test verifies SOURCE candidate filtering via join analysis calls.
// Columns with nil IsJoinable can still appear in relationships as TARGETS
// (via entityRefColumns which don't check IsJoinable) and then as SOURCE
// in the reverse relationship created for bidirectional navigation.
//
// The key constraint tested: join analysis should NOT be triggered with
// a nil-IsJoinable column as the source.
func TestPKMatch_DataBased_RequiresExplicitJoinability(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()

	highDistinct := int64(100)
	isJoinableTrue := true

	// Create mocks with legacy pattern matching DISABLED
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)
	mocks.projectService.ontologySettings = &OntologySettings{UseLegacyPatternMatching: false}

	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

	// Track which columns are used as SOURCE in join analysis
	// In data-based mode, user_id_nil should NOT be used as source in join analysis
	// (This is the key constraint - nil IsJoinable columns are not candidates)
	joinAnalysisSourceColumns := make(map[string]bool)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		joinAnalysisSourceColumns[sourceColumn] = true
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &highDistinct,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &highDistinct,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: ordersTableID,
					ColumnName:    "id",
					DataType:      "int8",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
				// This _id column has nil IsJoinable - should NOT be used as SOURCE
				// in join analysis calls (not added to candidates list)
				{
					SchemaTableID: ordersTableID,
					ColumnName:    "user_id_nil",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    nil, // nil = no joinability info = NOT a candidate
					DistinctCount: &highDistinct,
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	_, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify user_id_nil was NOT used as SOURCE in any join analysis call
	// This is the key constraint: nil IsJoinable columns should not be candidates
	if joinAnalysisSourceColumns["user_id_nil"] {
		t.Error("user_id_nil should NOT be used as source in join analysis (nil IsJoinable in data-based mode)")
	}
}

// TestPKMatch_DataBased_CardinalityRatioAlwaysApplied verifies that when
// UseLegacyPatternMatching=false, the cardinality ratio check applies to ALL columns,
// including those with _id suffix.
func TestPKMatch_DataBased_CardinalityRatioAlwaysApplied(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	eventEntityID := uuid.New()

	highDistinct := int64(100)
	lowDistinct := int64(50)     // Low distinct
	highRowCount := int64(10000) // 50/10000 = 0.5% < 5% threshold
	isJoinableTrue := true

	// Create mocks with legacy pattern matching DISABLED
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)
	mocks.projectService.ontologySettings = &OntologySettings{UseLegacyPatternMatching: false}

	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            eventEntityID,
		OntologyID:    ontologyID,
		Name:          "event",
		PrimarySchema: "public",
		PrimaryTable:  "events",
	})

	// Join analysis should NOT be called for columns with low cardinality ratio
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		if sourceColumn == "user_id" && sourceTable == "events" {
			t.Errorf("AnalyzeJoin should NOT be called for events.user_id (low cardinality ratio in non-legacy mode)")
		}
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	usersTableID := uuid.New()
	eventsTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &highDistinct,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
			},
		},
		{
			ID:         eventsTableID,
			SchemaName: "public",
			TableName:  "events",
			RowCount:   &highRowCount, // 10,000 rows
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: eventsTableID,
					ColumnName:    "id",
					DataType:      "int8",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highRowCount,
				},
				// This _id column has low cardinality ratio (0.5% < 5%)
				// In legacy mode, _id columns skip this check
				// In data-based mode, this column would be filtered out
				{
					SchemaTableID: eventsTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &lowDistinct, // 50/10000 = 0.5% < 5%
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify no relationships involving events.user_id (filtered by cardinality ratio)
	for _, rel := range mocks.relationshipRepo.created {
		if rel.SourceColumnTable == "events" && rel.SourceColumnName == "user_id" {
			t.Errorf("unexpected relationship from events.user_id: data-based mode should filter by cardinality ratio")
		}
	}
	_ = result
}

// TestPKMatch_IdentifierColumnsExemptFromCardinalityFilter verifies that columns
// with Purpose=identifier (from stored ColumnFeatures) are exempt from the
// cardinality ratio check, allowing them to be evaluated as FK candidates.
func TestPKMatch_IdentifierColumnsExemptFromCardinalityFilter(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	eventEntityID := uuid.New()

	highDistinct := int64(100)
	lowDistinct := int64(50)     // Low distinct
	highRowCount := int64(10000) // 50/10000 = 0.5% < 5% threshold
	isJoinableTrue := true

	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            eventEntityID,
		OntologyID:    ontologyID,
		Name:          "event",
		PrimarySchema: "public",
		PrimaryTable:  "events",
	})

	// Identifier columns with stored features should be evaluated even with low cardinality ratio
	joinAnalysisCalled := false
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		if sourceColumn == "user_id" && sourceTable == "events" {
			joinAnalysisCalled = true
		}
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	usersTableID := uuid.New()
	eventsTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &highDistinct,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highDistinct,
				},
			},
		},
		{
			ID:         eventsTableID,
			SchemaName: "public",
			TableName:  "events",
			RowCount:   &highRowCount, // 10,000 rows
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: eventsTableID,
					ColumnName:    "id",
					DataType:      "int8",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &highRowCount,
				},
				// This column has low cardinality ratio (0.5% < 5%)
				// But has Purpose=identifier in its features, so it skips cardinality check
				{
					SchemaTableID: eventsTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &lowDistinct, // 50/10000 = 0.5% < 5%
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	_, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Identifier columns (via stored features) should be evaluated (exempt from cardinality ratio check)
	if !joinAnalysisCalled {
		t.Error("expected events.user_id to be evaluated (identifier columns exempt from cardinality ratio check)")
	}
}

// TestFKDiscovery_Cardinality verifies that FK relationships compute cardinality
// from actual data (via AnalyzeJoin) and store it in SchemaRelationship.
func TestFKDiscovery_Cardinality(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	// Create mocks (entities not required since FKDiscovery no longer depends on them)
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Configure AnalyzeJoin to return N:1 cardinality pattern
	// (100 orders with 100 unique user_ids, joining to 10 distinct users)
	// sourceRatio = 100/100 = 1.0 (unique on source side)
	// targetRatio = 100/10 = 10.0 (multiple source rows per target)
	// This gives N:1 cardinality
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			JoinCount:     100, // 100 matching rows
			SourceMatched: 100, // 100 distinct FK values
			TargetMatched: 10,  // 10 distinct PK values
			OrphanCount:   0,   // All FK values have matching PKs
		}, nil
	}

	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	userIDColumnID := uuid.New()
	orderUserIDColumnID := uuid.New()

	// Setup tables with columns
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.SchemaColumn{
				{
					ID:            userIDColumnID,
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			Columns: []models.SchemaColumn{
				{
					ID:            orderUserIDColumnID,
					SchemaTableID: ordersTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
				},
			},
		},
	}

	// Add FK relationship: orders.user_id → users.id with inference_method='foreign_key'
	fkMethod := models.InferenceMethodForeignKey
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    ordersTableID,
			SourceColumnID:   orderUserIDColumnID,
			TargetTableID:    usersTableID,
			TargetColumnID:   userIDColumnID,
			RelationshipType: models.RelationshipTypeFK,
			InferenceMethod:  &fkMethod,
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: 1 FK relationship processed
	if result.FKRelationships != 1 {
		t.Errorf("expected 1 FK relationship, got %d", result.FKRelationships)
	}

	// Verify: 1 SchemaRelationship upserted (unidirectional)
	if len(mocks.schemaRepo.upsertedRelationships) != 1 {
		t.Fatalf("expected 1 SchemaRelationship to be upserted, got %d", len(mocks.schemaRepo.upsertedRelationships))
	}

	// Verify: relationship has cardinality N:1 (many orders belong to one user)
	updatedRel := mocks.schemaRepo.upsertedRelationships[0]
	if updatedRel.Cardinality != models.CardinalityNTo1 {
		t.Errorf("expected cardinality=%q, got %q", models.CardinalityNTo1, updatedRel.Cardinality)
	}
	if !updatedRel.IsValidated {
		t.Errorf("expected IsValidated=true, got false")
	}
}

// TestFKDiscovery_Cardinality_1to1 verifies that 1:1 cardinality is correctly
// detected from data when both sides have unique values.
func TestFKDiscovery_Cardinality_1to1(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	// Create mocks (entities not required since FKDiscovery no longer depends on them)
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Configure AnalyzeJoin to return 1:1 cardinality pattern
	// (50 profiles with 50 unique user_ids, joining to 50 distinct users)
	// sourceRatio = 50/50 = 1.0 (unique on source side)
	// targetRatio = 50/50 = 1.0 (unique on target side)
	// This gives 1:1 cardinality
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			JoinCount:     50, // 50 matching rows
			SourceMatched: 50, // 50 distinct FK values
			TargetMatched: 50, // 50 distinct PK values (1:1)
			OrphanCount:   0,  // All FK values have matching PKs
		}, nil
	}

	usersTableID := uuid.New()
	profilesTableID := uuid.New()
	userIDColumnID := uuid.New()
	profileUserIDColumnID := uuid.New()

	// Setup tables with columns
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.SchemaColumn{
				{
					ID:            userIDColumnID,
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
				},
			},
		},
		{
			ID:         profilesTableID,
			SchemaName: "public",
			TableName:  "profiles",
			Columns: []models.SchemaColumn{
				{
					ID:            profileUserIDColumnID,
					SchemaTableID: profilesTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
				},
			},
		},
	}

	// Add FK relationship: profiles.user_id → users.id with inference_method='foreign_key'
	fkMethod := models.InferenceMethodForeignKey
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    profilesTableID,
			SourceColumnID:   profileUserIDColumnID,
			TargetTableID:    usersTableID,
			TargetColumnID:   userIDColumnID,
			RelationshipType: models.RelationshipTypeFK,
			InferenceMethod:  &fkMethod,
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.FKRelationships != 1 {
		t.Errorf("expected 1 FK relationship, got %d", result.FKRelationships)
	}

	if len(mocks.schemaRepo.upsertedRelationships) != 1 {
		t.Fatalf("expected 1 SchemaRelationship to be upserted, got %d", len(mocks.schemaRepo.upsertedRelationships))
	}

	// Verify: relationship has cardinality 1:1 (unique on both sides)
	updatedRel := mocks.schemaRepo.upsertedRelationships[0]
	if updatedRel.Cardinality != models.Cardinality1To1 {
		t.Errorf("expected cardinality=%q, got %q", models.Cardinality1To1, updatedRel.Cardinality)
	}
}

// TestFKDiscovery_Cardinality_FallbackOnError verifies that when AnalyzeJoin fails,
// the FK relationship still gets updated with the default N:1 cardinality.
func TestFKDiscovery_Cardinality_FallbackOnError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	// Create mocks (entities not required since FKDiscovery no longer depends on them)
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Configure AnalyzeJoin to return an error (e.g., table doesn't exist, permission denied)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return nil, fmt.Errorf("permission denied on table")
	}

	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	userIDColumnID := uuid.New()
	orderUserIDColumnID := uuid.New()

	// Setup tables with columns
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.SchemaColumn{
				{
					ID:            userIDColumnID,
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			Columns: []models.SchemaColumn{
				{
					ID:            orderUserIDColumnID,
					SchemaTableID: ordersTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
				},
			},
		},
	}

	// Add FK relationship: orders.user_id → users.id with inference_method='foreign_key'
	fkMethod := models.InferenceMethodForeignKey
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    ordersTableID,
			SourceColumnID:   orderUserIDColumnID,
			TargetTableID:    usersTableID,
			TargetColumnID:   userIDColumnID,
			RelationshipType: models.RelationshipTypeFK,
			InferenceMethod:  &fkMethod,
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Should still succeed even though AnalyzeJoin failed
	result, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.FKRelationships != 1 {
		t.Errorf("expected 1 FK relationship, got %d", result.FKRelationships)
	}

	if len(mocks.schemaRepo.upsertedRelationships) != 1 {
		t.Fatalf("expected 1 SchemaRelationship to be upserted, got %d", len(mocks.schemaRepo.upsertedRelationships))
	}

	// Verify: relationship falls back to N:1 when AnalyzeJoin fails
	updatedRel := mocks.schemaRepo.upsertedRelationships[0]
	if updatedRel.Cardinality != models.CardinalityNTo1 {
		t.Errorf("expected cardinality=%q (fallback), got %q", models.CardinalityNTo1, updatedRel.Cardinality)
	}
}

// TestCreateBidirectionalRelationship_Cardinality verifies that cardinality is
// correctly swapped when creating bidirectional relationships.
func TestCreateBidirectionalRelationship_Cardinality(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	testCases := []struct {
		name             string
		inputCardinality string
		expectedForward  string
		expectedReverse  string
	}{
		{
			name:             "N:1 becomes 1:N for reverse",
			inputCardinality: models.CardinalityNTo1,
			expectedForward:  models.CardinalityNTo1,
			expectedReverse:  models.Cardinality1ToN,
		},
		{
			name:             "1:N becomes N:1 for reverse",
			inputCardinality: models.Cardinality1ToN,
			expectedForward:  models.Cardinality1ToN,
			expectedReverse:  models.CardinalityNTo1,
		},
		{
			name:             "1:1 stays 1:1",
			inputCardinality: models.Cardinality1To1,
			expectedForward:  models.Cardinality1To1,
			expectedReverse:  models.Cardinality1To1,
		},
		{
			name:             "N:M stays N:M",
			inputCardinality: models.CardinalityNToM,
			expectedForward:  models.CardinalityNToM,
			expectedReverse:  models.CardinalityNToM,
		},
		{
			name:             "unknown stays unknown",
			inputCardinality: models.CardinalityUnknown,
			expectedForward:  models.CardinalityUnknown,
			expectedReverse:  models.CardinalityUnknown,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear previous created relationships
			mocks.relationshipRepo.created = nil

			// Create a test relationship with specified cardinality
			rel := &models.EntityRelationship{
				OntologyID:         ontologyID,
				SourceEntityID:     orderEntityID,
				TargetEntityID:     userEntityID,
				SourceColumnSchema: "public",
				SourceColumnTable:  "orders",
				SourceColumnName:   "user_id",
				TargetColumnSchema: "public",
				TargetColumnTable:  "users",
				TargetColumnName:   "id",
				DetectionMethod:    models.DetectionMethodForeignKey,
				Confidence:         1.0,
				Status:             models.RelationshipStatusConfirmed,
				Cardinality:        tc.inputCardinality,
			}

			// Call createBidirectionalRelationship
			svc := service.(*deterministicRelationshipService)
			err := svc.createBidirectionalRelationship(context.Background(), rel)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify: 2 relationships created (forward and reverse)
			if len(mocks.relationshipRepo.created) != 2 {
				t.Fatalf("expected 2 relationships (forward and reverse), got %d", len(mocks.relationshipRepo.created))
			}

			// Verify forward relationship cardinality
			forward := mocks.relationshipRepo.created[0]
			if forward.Cardinality != tc.expectedForward {
				t.Errorf("forward cardinality: expected %q, got %q", tc.expectedForward, forward.Cardinality)
			}

			// Verify reverse relationship cardinality
			reverse := mocks.relationshipRepo.created[1]
			if reverse.Cardinality != tc.expectedReverse {
				t.Errorf("reverse cardinality: expected %q, got %q", tc.expectedReverse, reverse.Cardinality)
			}
		})
	}
}

// TestPKMatch_InfersCardinalityFromJoinAnalysis verifies that PK-Match relationships
// correctly infer cardinality from join analysis data.
func TestPKMatch_InfersCardinalityFromJoinAnalysis(t *testing.T) {
	testCases := []struct {
		name                string
		joinCount           int64
		sourceMatched       int64
		targetMatched       int64
		expectedCardinality string
	}{
		{
			name:                "N:1 - many orders per user",
			joinCount:           1000,
			sourceMatched:       1000, // Each order matches exactly one user
			targetMatched:       100,  // 100 users, each matched by ~10 orders
			expectedCardinality: models.CardinalityNTo1,
		},
		{
			name:                "1:1 - one-to-one relationship",
			joinCount:           100,
			sourceMatched:       100,
			targetMatched:       100,
			expectedCardinality: models.Cardinality1To1,
		},
		{
			name:                "1:N - one source matches many targets",
			joinCount:           1000,
			sourceMatched:       100,  // 100 sources, each matched by ~10 targets
			targetMatched:       1000, // Each target matches exactly one source
			expectedCardinality: models.Cardinality1ToN,
		},
		{
			name:                "N:M - many-to-many",
			joinCount:           5000,
			sourceMatched:       500,
			targetMatched:       500,
			expectedCardinality: models.CardinalityNToM,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			projectID := uuid.New()
			datasourceID := uuid.New()
			ontologyID := uuid.New()
			userEntityID := uuid.New()
			orderEntityID := uuid.New()

			mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

			mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
				ID:            orderEntityID,
				OntologyID:    ontologyID,
				Name:          "order",
				PrimarySchema: "public",
				PrimaryTable:  "orders",
			})

			// Configure join analysis to return test-specific values
			mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
				return &datasource.JoinAnalysis{
					OrphanCount:   0, // Valid FK: no orphans
					JoinCount:     tc.joinCount,
					SourceMatched: tc.sourceMatched,
					TargetMatched: tc.targetMatched,
				}, nil
			}

			usersTableID := uuid.New()
			ordersTableID := uuid.New()
			distinctCount := int64(1000)
			rowCount := int64(10000)
			isJoinable := true

			mocks.schemaRepo.tables = []*models.SchemaTable{
				{
					ID:         usersTableID,
					SchemaName: "public",
					TableName:  "users",
					RowCount:   &rowCount,
					Columns: []models.SchemaColumn{
						{
							ID:            uuid.New(),
							SchemaTableID: usersTableID,
							ColumnName:    "id",
							DataType:      "uuid",
							IsPrimaryKey:  true,
							IsJoinable:    &isJoinable,
							DistinctCount: &distinctCount,
						},
					},
				},
				{
					ID:         ordersTableID,
					SchemaName: "public",
					TableName:  "orders",
					RowCount:   &rowCount,
					Columns: []models.SchemaColumn{
						{
							ID:            uuid.New(),
							SchemaTableID: ordersTableID,
							ColumnName:    "id",
							DataType:      "int8", // Different type to prevent bidirectional matching
							IsPrimaryKey:  true,
							IsJoinable:    &isJoinable,
							DistinctCount: &distinctCount,
						},
						{
							ID:            uuid.New(),
							SchemaTableID: ordersTableID,
							ColumnName:    "user_id",
							DataType:      "uuid", // Matches users.id type
							IsPrimaryKey:  false,
							IsJoinable:    &isJoinable,
							DistinctCount: &distinctCount,
						},
					},
				},
			}

			service := NewDeterministicRelationshipService(
				mocks.datasourceService,
				mocks.projectService,
				mocks.adapterFactory,
				mocks.ontologyRepo,
				mocks.entityRepo,
				mocks.relationshipRepo,
				mocks.schemaRepo,
				zap.NewNop(),
			)

			_, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify relationships were created
			// PKMatchDiscovery now writes unidirectional SchemaRelationships
			if len(mocks.schemaRepo.upsertedRelationships) < 1 {
				t.Fatalf("expected at least 1 SchemaRelationship, got %d", len(mocks.schemaRepo.upsertedRelationships))
			}

			// Find the relationship (order -> user)
			var forwardRel *models.SchemaRelationship
			for _, rel := range mocks.schemaRepo.upsertedRelationships {
				if rel.SourceTableID == ordersTableID && rel.TargetTableID == usersTableID {
					forwardRel = rel
					break
				}
			}

			if forwardRel == nil {
				t.Fatal("relationship (orders -> users) not found")
			}

			// Verify cardinality
			if forwardRel.Cardinality != tc.expectedCardinality {
				t.Errorf("forward cardinality: expected %q, got %q", tc.expectedCardinality, forwardRel.Cardinality)
			}

			// Note: PKMatchDiscovery now creates unidirectional SchemaRelationships
			// No reverse relationship is created - bidirectional navigation is handled at query time
		})
	}
}

// TestFKDiscovery_FromColumnFeatures tests that SchemaRelationship records are created from
// columns where ColumnFeatureExtraction Phase 4 has already resolved FK targets.
func TestFKDiscovery_FromColumnFeatures(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	// Create mocks (entities not required since FKDiscovery no longer depends on them)
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	userIDColumnID := uuid.New()
	buyerIDColumnID := uuid.New()

	// Setup tables with columns
	// The buyer_id column has ColumnFeatures with FKTargetTable already resolved
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.SchemaColumn{
				{
					ID:            userIDColumnID,
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			Columns: []models.SchemaColumn{
				{
					ID:            buyerIDColumnID,
					SchemaTableID: ordersTableID,
					ColumnName:    "buyer_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					// ColumnFeatures stored in Metadata with FK target already resolved
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose":       "identifier",
							"semantic_type": "foreign_key",
							"role":          "foreign_key",
							"identifier_features": map[string]any{
								"identifier_type":  "foreign_key",
								"fk_target_table":  "users",
								"fk_target_column": "id",
								"fk_confidence":    0.95,
							},
						},
					},
				},
			},
		},
	}

	// No schema-level FK relationships - only ColumnFeatures
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{}

	// Setup join analysis to return valid join (no orphans)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		// Simulate a valid FK relationship: all source values exist in target
		return &datasource.JoinAnalysis{
			JoinCount:     100,
			SourceMatched: 100,
			TargetMatched: 50,
			OrphanCount:   0, // No orphans = valid FK
		}, nil
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: 1 FK relationship discovered from ColumnFeatures
	if result.FKRelationships != 1 {
		t.Errorf("expected 1 FK relationship, got %d", result.FKRelationships)
	}

	// Verify: 1 SchemaRelationship upserted (unidirectional)
	if len(mocks.schemaRepo.upsertedRelationships) != 1 {
		t.Fatalf("expected 1 SchemaRelationship to be upserted, got %d", len(mocks.schemaRepo.upsertedRelationships))
	}

	// Verify the upserted relationship
	rel := mocks.schemaRepo.upsertedRelationships[0]

	// Verify: inference_method="column_features"
	if rel.InferenceMethod == nil || *rel.InferenceMethod != models.InferenceMethodColumnFeatures {
		var method string
		if rel.InferenceMethod != nil {
			method = *rel.InferenceMethod
		}
		t.Errorf("expected InferenceMethod=%q, got %q", models.InferenceMethodColumnFeatures, method)
	}

	// Verify: confidence matches the ColumnFeatures FK confidence
	if rel.Confidence != 0.95 {
		t.Errorf("expected confidence=0.95, got %f", rel.Confidence)
	}

	// Verify: source/target table/column IDs
	if rel.SourceTableID != ordersTableID {
		t.Errorf("expected source table ID=%v, got %v", ordersTableID, rel.SourceTableID)
	}
	if rel.SourceColumnID != buyerIDColumnID {
		t.Errorf("expected source column ID=%v, got %v", buyerIDColumnID, rel.SourceColumnID)
	}
	if rel.TargetTableID != usersTableID {
		t.Errorf("expected target table ID=%v, got %v", usersTableID, rel.TargetTableID)
	}
	if rel.TargetColumnID != userIDColumnID {
		t.Errorf("expected target column ID=%v, got %v", userIDColumnID, rel.TargetColumnID)
	}
}

// TestFKDiscovery_ColumnFeaturesAndSchemaFK_Deduplication tests that when both
// ColumnFeatures and schema FK constraints point to the same relationship,
// both are processed (via upsert semantics, the DB would deduplicate them).
func TestFKDiscovery_ColumnFeaturesAndSchemaFK_Deduplication(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	// Create mocks (entities not required since FKDiscovery no longer depends on them)
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	userIDColumnID := uuid.New()
	buyerIDColumnID := uuid.New()

	// Setup tables with columns - buyer_id has ColumnFeatures with FK resolved
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.SchemaColumn{
				{
					ID:            userIDColumnID,
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			Columns: []models.SchemaColumn{
				{
					ID:            buyerIDColumnID,
					SchemaTableID: ordersTableID,
					ColumnName:    "buyer_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose":       "identifier",
							"semantic_type": "foreign_key",
							"role":          "foreign_key",
							"identifier_features": map[string]any{
								"identifier_type":  "foreign_key",
								"fk_target_table":  "users",
								"fk_target_column": "id",
								"fk_confidence":    0.85, // Lower confidence from data overlap
							},
						},
					},
				},
			},
		},
	}

	// ALSO have a schema-level FK constraint for the same relationship
	fkMethod := models.InferenceMethodForeignKey
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    ordersTableID,
			SourceColumnID:   buyerIDColumnID,
			TargetTableID:    usersTableID,
			TargetColumnID:   userIDColumnID,
			RelationshipType: models.RelationshipTypeFK,
			InferenceMethod:  &fkMethod,
		},
	}

	// Setup join analysis
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			JoinCount:     100,
			SourceMatched: 100,
			TargetMatched: 50,
			OrphanCount:   0,
		}, nil
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Result count shows both sources found the relationship
	// (ColumnFeatures: 1, SchemaFK: 1, total: 2)
	// The DB upsert would deduplicate by source_column_id + target_column_id
	if result.FKRelationships != 2 {
		t.Errorf("expected 2 FK relationships reported (from both sources), got %d", result.FKRelationships)
	}

	// The mock doesn't implement upsert deduplication, so we see 2 upserts
	// (1 from ColumnFeatures Phase 1, 1 from Schema FK Phase 2)
	// In production, the DB upsert would deduplicate these.
	if len(mocks.schemaRepo.upsertedRelationships) != 2 {
		t.Fatalf("expected 2 SchemaRelationships upserted (from both sources), got %d", len(mocks.schemaRepo.upsertedRelationships))
	}

	// Verify we have one with column_features and one with foreign_key inference method
	foundColumnFeatures := false
	foundForeignKey := false
	for _, rel := range mocks.schemaRepo.upsertedRelationships {
		if rel.InferenceMethod != nil && *rel.InferenceMethod == models.InferenceMethodColumnFeatures {
			foundColumnFeatures = true
		}
		if rel.InferenceMethod != nil && *rel.InferenceMethod == models.InferenceMethodForeignKey {
			foundForeignKey = true
		}
	}

	if !foundColumnFeatures {
		t.Error("expected at least one relationship with inference_method=column_features")
	}
	if !foundForeignKey {
		t.Error("expected at least one relationship with inference_method=foreign_key")
	}
}

// TestFKDiscovery_ColumnFeatures_NoTargetTable tests that columns with ColumnFeatures
// but unresolvable target tables are skipped gracefully.
func TestFKDiscovery_ColumnFeatures_NoTargetTable(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	orderEntityID := uuid.New()

	// Create mocks with just the order entity (no user entity/table)
	mocks := setupMocks(projectID, ontologyID, datasourceID, orderEntityID)
	mocks.entityRepo.entities = []*models.OntologyEntity{
		{
			ID:            orderEntityID,
			OntologyID:    ontologyID,
			Name:          "order",
			PrimarySchema: "public",
			PrimaryTable:  "orders",
		},
	}

	ordersTableID := uuid.New()
	buyerIDColumnID := uuid.New()

	// Setup tables - only orders table, no users table
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			Columns: []models.SchemaColumn{
				{
					ID:            buyerIDColumnID,
					SchemaTableID: ordersTableID,
					ColumnName:    "buyer_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					// ColumnFeatures points to non-existent "users" table
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose":       "identifier",
							"semantic_type": "foreign_key",
							"role":          "foreign_key",
							"identifier_features": map[string]any{
								"identifier_type":  "foreign_key",
								"fk_target_table":  "users", // Table doesn't exist
								"fk_target_column": "id",
								"fk_confidence":    0.95,
							},
						},
					},
				},
			},
		},
	}

	mocks.schemaRepo.relationships = []*models.SchemaRelationship{}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No relationships should be created since target table doesn't exist
	if result.FKRelationships != 0 {
		t.Errorf("expected 0 FK relationships (target table missing), got %d", result.FKRelationships)
	}

	if len(mocks.relationshipRepo.created) != 0 {
		t.Errorf("expected 0 relationships created, got %d", len(mocks.relationshipRepo.created))
	}
}

// TestPKMatch_SkipsHighConfidenceFK verifies that PKMatchDiscovery skips columns
// with FKConfidence > 0.8 from ColumnFeatureExtraction Phase 4, avoiding redundant SQL analysis.
func TestPKMatch_SkipsHighConfidenceFK(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()

	distinctCount := int64(100)
	isJoinableTrue := true

	// Create mocks with user entity
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Add order entity
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

	// Track if AnalyzeJoin is called for the high-confidence column
	analyzeJoinCalled := false
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		if sourceColumn == "user_id" {
			analyzeJoinCalled = true
		}
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	// Schema: orders.user_id has high FKConfidence (0.95) with resolved FK target
	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	userIDColumnID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: ordersTableID,
					ColumnName:    "id",
					DataType:      "int8",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
				{
					ID:            userIDColumnID,
					SchemaTableID: ordersTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
					// High confidence FK from Phase 4 - should be SKIPPED
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
							"role":    models.RoleForeignKey,
							"identifier_features": map[string]any{
								"fk_confidence":   0.95, // > 0.8 threshold
								"fk_target_table": "users",
							},
						},
					},
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Execute PK match discovery
	_, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: AnalyzeJoin was NOT called for user_id (skipped due to high confidence)
	if analyzeJoinCalled {
		t.Error("expected AnalyzeJoin NOT to be called for user_id (high FK confidence should skip)")
	}
}

// TestPKMatch_SkipsColumnsWithExistingRelationships verifies that PKMatchDiscovery
// skips columns that already have relationships from FKDiscovery.
func TestPKMatch_SkipsColumnsWithExistingRelationships(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()

	distinctCount := int64(100)
	isJoinableTrue := true

	// Create mocks with user entity
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Add order entity
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

	// Track if AnalyzeJoin is called
	analyzeJoinCalled := false
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		if sourceColumn == "user_id" {
			analyzeJoinCalled = true
		}
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	// Schema with user_id column
	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	userIDColumnID := uuid.New()
	targetColumnID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					ID:            targetColumnID,
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: ordersTableID,
					ColumnName:    "id",
					DataType:      "int8",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
				{
					ID:            userIDColumnID,
					SchemaTableID: ordersTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
			},
		},
	}

	// Pre-existing SchemaRelationship from FKDiscovery for user_id column
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:             uuid.New(),
			ProjectID:      projectID,
			SourceTableID:  ordersTableID,
			SourceColumnID: userIDColumnID,
			TargetTableID:  usersTableID,
			TargetColumnID: targetColumnID,
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Execute PK match discovery
	_, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: AnalyzeJoin was NOT called for user_id (skipped due to existing relationship)
	if analyzeJoinCalled {
		t.Error("expected AnalyzeJoin NOT to be called for user_id (existing relationship should skip)")
	}
}

// TestPKMatch_PrioritizesForeignKeyRole verifies that columns with Role=foreign_key
// are processed before regular candidates.
func TestPKMatch_PrioritizesForeignKeyRole(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()

	distinctCount := int64(100)
	isJoinableTrue := true

	// Create mocks with user entity
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Add order entity
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

	// Track the order in which columns are analyzed
	var analyzedColumns []string
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		analyzedColumns = append(analyzedColumns, sourceColumn)
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	// Schema with two FK candidate columns - one with Role=foreign_key, one without
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: ordersTableID,
					ColumnName:    "id",
					DataType:      "int8",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
				// Regular candidate (no Role set) - should be processed SECOND
				{
					ID:            uuid.New(),
					SchemaTableID: ordersTableID,
					ColumnName:    "other_user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
							// No "role" set - this is a regular candidate
						},
					},
				},
				// Priority candidate with Role=foreign_key - should be processed FIRST
				{
					ID:            uuid.New(),
					SchemaTableID: ordersTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
							"role":    models.RoleForeignKey, // Priority candidate
						},
					},
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Execute PK match discovery
	_, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: user_id (with Role=foreign_key) was analyzed BEFORE other_user_id
	if len(analyzedColumns) < 2 {
		t.Fatalf("expected at least 2 columns analyzed, got %d: %v", len(analyzedColumns), analyzedColumns)
	}

	// Find positions of each column in the analyzed order
	userIDPos := -1
	otherUserIDPos := -1
	for i, col := range analyzedColumns {
		if col == "user_id" && userIDPos == -1 {
			userIDPos = i
		}
		if col == "other_user_id" && otherUserIDPos == -1 {
			otherUserIDPos = i
		}
	}

	if userIDPos == -1 || otherUserIDPos == -1 {
		t.Fatalf("expected both columns to be analyzed, got: %v", analyzedColumns)
	}

	if userIDPos > otherUserIDPos {
		t.Errorf("expected user_id (Role=foreign_key) to be analyzed before other_user_id, but order was: %v", analyzedColumns)
	}
}

// TestPKMatch_RejectsBidirectionalOrphans verifies that PKMatchDiscovery rejects
// relationships where >50% of target values don't exist in source (reverse orphans).
// This catches false positives like identity_provider → jobs.id where:
// - identity_provider has 3 values {1,2,3}
// - jobs.id has 83 values {1-83}
// - Source→target: all 3 exist → 0 orphans (would PASS old check)
// - Target→source: 80 values don't exist → 96% reverse orphans (REJECT)
func TestPKMatch_RejectsBidirectionalOrphans(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	distinctCountSmall := int64(3)
	distinctCountLarge := int64(83)
	rowCountSmall := int64(100)
	rowCountLarge := int64(1000)
	isJoinableTrue := true

	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Configure AnalyzeJoin to simulate the false positive scenario:
	// - 0 orphans (all source values exist in target)
	// - High reverse orphans (>50% of target values don't exist in source)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		// Simulate identity_provider → jobs.id pattern:
		// Source has 3 values, all exist in target → 0 orphans
		// Target has 83 values, only 3 exist in source → 80 reverse orphans
		return &datasource.JoinAnalysis{
			JoinCount:          3,
			SourceMatched:      3,
			TargetMatched:      3,
			OrphanCount:        0,  // All source values exist in target
			ReverseOrphanCount: 80, // 80 of 83 target values don't exist in source
		}, nil
	}

	// Schema with a small lookup-like column and a large table PK
	smallTableID := uuid.New()
	largeTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         smallTableID,
			SchemaName: "public",
			TableName:  "small_lookup",
			RowCount:   &rowCountSmall,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: smallTableID,
					ColumnName:    "provider_id",
					DataType:      "int4",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCountSmall,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
			},
		},
		{
			ID:         largeTableID,
			SchemaName: "public",
			TableName:  "large_table",
			RowCount:   &rowCountLarge,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: largeTableID,
					ColumnName:    "id",
					DataType:      "int4",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCountLarge,
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Execute PK match discovery
	_, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: No relationship was created due to high reverse orphan rate (80/83 = 96% > 50%)
	if len(mocks.schemaRepo.upsertedRelationships) > 0 {
		t.Errorf("expected relationship to be REJECTED due to high reverse orphan rate (96%%), but %d were created",
			len(mocks.schemaRepo.upsertedRelationships))
	}
}

// TestPKMatch_AcceptsLowReverseOrphans verifies that PKMatchDiscovery accepts
// relationships with low reverse orphan rates (<=50%).
func TestPKMatch_AcceptsLowReverseOrphans(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	distinctCount := int64(100)
	rowCount := int64(1000)
	isJoinableTrue := true

	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Configure AnalyzeJoin with low reverse orphan rate (<50%)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		// Simulate a valid FK: 80 source values match 80 target values
		// Only 20% of target values are reverse orphans (below 50% threshold)
		return &datasource.JoinAnalysis{
			JoinCount:          80,
			SourceMatched:      80,
			TargetMatched:      80,
			OrphanCount:        0,  // All source values exist in target
			ReverseOrphanCount: 20, // 20% of target values don't exist in source (within threshold)
		}, nil
	}

	// Schema with FK candidate column
	sourceTableID := uuid.New()
	targetTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         sourceTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: sourceTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
			},
		},
		{
			ID:         targetTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: targetTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Execute PK match discovery
	_, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: Relationship WAS created because reverse orphan rate (20%) is below 50% threshold
	if len(mocks.schemaRepo.upsertedRelationships) == 0 {
		t.Error("expected relationship to be CREATED (reverse orphan rate 20% < 50% threshold), but none were created")
	}
}

// TestFKDiscovery_NoEntityDependency verifies that FKDiscovery does NOT return early
// when no entities exist. The service should still discover FK relationships from
// ColumnFeatures and schema FK constraints, writing to SchemaRelationship.
func TestFKDiscovery_NoEntityDependency(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()

	// Create mocks with NO entities
	mocks := setupMocks(projectID, ontologyID, datasourceID, uuid.Nil)
	mocks.entityRepo.entities = []*models.OntologyEntity{} // Empty - no entities

	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	userIDColumnID := uuid.New()
	buyerIDColumnID := uuid.New()

	// Setup tables with columns - buyer_id has ColumnFeatures with FK resolved
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.SchemaColumn{
				{
					ID:            userIDColumnID,
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			Columns: []models.SchemaColumn{
				{
					ID:            buyerIDColumnID,
					SchemaTableID: ordersTableID,
					ColumnName:    "buyer_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose":       "identifier",
							"semantic_type": "foreign_key",
							"role":          "foreign_key",
							"identifier_features": map[string]any{
								"identifier_type":  "foreign_key",
								"fk_target_table":  "users",
								"fk_target_column": "id",
								"fk_confidence":    0.9,
							},
						},
					},
				},
			},
		},
	}

	mocks.schemaRepo.relationships = []*models.SchemaRelationship{}

	// Setup join analysis to return valid join (no orphans)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			JoinCount:     100,
			SourceMatched: 100,
			TargetMatched: 50,
			OrphanCount:   0,
		}, nil
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: FKDiscovery succeeded even without entities
	if result.FKRelationships == 0 {
		t.Error("expected FK relationships to be discovered even without entities")
	}

	// Verify: SchemaRelationship was created
	if len(mocks.schemaRepo.upsertedRelationships) == 0 {
		t.Error("expected SchemaRelationship to be created even without entities")
	}

	// Verify: The relationship was created from ColumnFeatures
	rel := mocks.schemaRepo.upsertedRelationships[0]
	if rel.InferenceMethod == nil || *rel.InferenceMethod != models.InferenceMethodColumnFeatures {
		t.Error("expected InferenceMethod=column_features")
	}
}

// TestFKDiscovery_UpsertBehavior verifies that FKDiscovery uses upsert semantics
// when creating relationships, allowing updates to existing relationships.
func TestFKDiscovery_UpsertBehavior(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	userIDColumnID := uuid.New()
	buyerIDColumnID := uuid.New()

	// Setup tables with columns
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.SchemaColumn{
				{
					ID:            userIDColumnID,
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			Columns: []models.SchemaColumn{
				{
					ID:            buyerIDColumnID,
					SchemaTableID: ordersTableID,
					ColumnName:    "buyer_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": "identifier",
							"role":    "foreign_key",
							"identifier_features": map[string]any{
								"fk_target_table":  "users",
								"fk_target_column": "id",
								"fk_confidence":    0.85,
							},
						},
					},
				},
			},
		},
	}

	mocks.schemaRepo.relationships = []*models.SchemaRelationship{}

	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			JoinCount:     100,
			SourceMatched: 100,
			TargetMatched: 50,
			OrphanCount:   0,
		}, nil
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	// Run discovery twice
	_, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("first call unexpected error: %v", err)
	}

	firstCallCount := len(mocks.schemaRepo.upsertedRelationships)
	if firstCallCount == 0 {
		t.Fatal("expected at least one relationship after first call")
	}

	// Run again - should use upsert (UpsertRelationship is called, not Create)
	_, err = service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("second call unexpected error: %v", err)
	}

	// The mock tracks all upsert calls - in production, duplicate upserts would
	// update the existing row, but here we just verify upsert is being called
	secondCallCount := len(mocks.schemaRepo.upsertedRelationships)
	if secondCallCount <= firstCallCount {
		t.Error("expected UpsertRelationship to be called on second run (upsert semantics)")
	}

	// Verify all calls used UpsertRelationship (not Create which goes to relationshipRepo)
	if len(mocks.relationshipRepo.created) > 0 {
		t.Error("expected FKDiscovery to use schemaRepo.UpsertRelationship, not relationshipRepo.Create")
	}
}

// TestFKDiscovery_ErrorPropagation verifies that FKDiscovery propagates repository
// errors immediately (fail-fast pattern per project guidelines).
func TestFKDiscovery_ErrorPropagation(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	userIDColumnID := uuid.New()
	buyerIDColumnID := uuid.New()

	// Setup tables with columns
	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.SchemaColumn{
				{
					ID:            userIDColumnID,
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			Columns: []models.SchemaColumn{
				{
					ID:            buyerIDColumnID,
					SchemaTableID: ordersTableID,
					ColumnName:    "buyer_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": "identifier",
							"role":    "foreign_key",
							"identifier_features": map[string]any{
								"fk_target_table":  "users",
								"fk_target_column": "id",
								"fk_confidence":    0.9,
							},
						},
					},
				},
			},
		},
	}

	mocks.schemaRepo.relationships = []*models.SchemaRelationship{}

	// Configure upsert to return an error
	expectedError := fmt.Errorf("database connection lost")
	mocks.schemaRepo.upsertError = expectedError

	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	_, err := service.DiscoverFKRelationships(context.Background(), projectID, datasourceID, nil)

	// Verify: Error was propagated
	if err == nil {
		t.Fatal("expected error to be propagated, got nil")
	}

	// Verify: The original error is in the chain
	if !strings.Contains(err.Error(), "database connection lost") {
		t.Errorf("expected error to contain original message, got: %v", err)
	}
}

// TestPKMatchDiscovery_NoEntitiesExist verifies that PKMatchDiscovery does NOT return
// empty when no entities exist. After the refactor (plan Step 3.1), the service builds
// candidates from schema metadata instead of entities.
func TestPKMatchDiscovery_NoEntitiesExist(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()

	distinctCount := int64(100)
	rowCount := int64(1000)
	isJoinableTrue := true

	// Create mocks with NO entities
	mocks := setupMocks(projectID, ontologyID, datasourceID, uuid.Nil)
	mocks.entityRepo.entities = []*models.OntologyEntity{} // Empty - no entities

	// Configure AnalyzeJoin to return valid join
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			JoinCount:          100,
			SourceMatched:      100,
			TargetMatched:      50,
			OrphanCount:        0,
			ReverseOrphanCount: 0,
		}, nil
	}

	// Schema with FK candidate column
	sourceTableID := uuid.New()
	targetTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         sourceTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: sourceTableID,
					ColumnName:    "id",
					DataType:      "int8",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
				{
					ID:            uuid.New(),
					SchemaTableID: sourceTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
			},
		},
		{
			ID:         targetTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: targetTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: PKMatchDiscovery succeeded even without entities
	// The old behavior (lines 711-717 in the plan) would return empty here
	if result.InferredRelationships == 0 {
		t.Error("expected PK-match relationships to be discovered even without entities")
	}

	// Verify: SchemaRelationship was created
	if len(mocks.schemaRepo.upsertedRelationships) == 0 {
		t.Error("expected SchemaRelationship to be created even without entities")
	}

	// Verify: inference_method=pk_match
	rel := mocks.schemaRepo.upsertedRelationships[0]
	if rel.InferenceMethod == nil || *rel.InferenceMethod != models.InferenceMethodPKMatch {
		var method string
		if rel.InferenceMethod != nil {
			method = *rel.InferenceMethod
		}
		t.Errorf("expected InferenceMethod=%q, got %q", models.InferenceMethodPKMatch, method)
	}
}

// TestPKMatchDiscovery_ValidationMetricsStored verifies that PKMatchDiscovery stores
// validation metrics (match_rate, source_distinct, target_distinct, matched_count)
// via UpsertRelationshipWithMetrics.
func TestPKMatchDiscovery_ValidationMetricsStored(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()

	distinctCount := int64(100)
	rowCount := int64(1000)
	isJoinableTrue := true

	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Configure AnalyzeJoin to return specific metrics
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			JoinCount:          75, // 75 matching rows
			SourceMatched:      50, // 50 distinct source values matched
			TargetMatched:      40, // 40 distinct target values matched
			OrphanCount:        0,  // 0 orphans (required for relationship creation)
			ReverseOrphanCount: 10, // 10 reverse orphans (20% < 50% threshold)
		}, nil
	}

	// Schema with FK candidate column
	sourceTableID := uuid.New()
	targetTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         sourceTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: sourceTableID,
					ColumnName:    "id",
					DataType:      "int8",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
				{
					ID:            uuid.New(),
					SchemaTableID: sourceTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
					Metadata: map[string]any{
						"column_features": map[string]any{
							"purpose": models.PurposeIdentifier,
						},
					},
				},
			},
		},
		{
			ID:         targetTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &rowCount,
			Columns: []models.SchemaColumn{
				{
					ID:            uuid.New(),
					SchemaTableID: targetTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount,
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.projectService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
		zap.NewNop(),
	)

	_, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: Metrics were stored
	if len(mocks.schemaRepo.upsertedMetrics) == 0 {
		t.Fatal("expected metrics to be stored via UpsertRelationshipWithMetrics")
	}

	// Find the metrics for the relationship
	var metrics *models.DiscoveryMetrics
	for _, m := range mocks.schemaRepo.upsertedMetrics {
		if m != nil {
			metrics = m
			break
		}
	}

	if metrics == nil {
		t.Fatal("expected non-nil metrics")
	}

	// Verify metrics values
	// MatchRate = SourceMatched / (SourceMatched + OrphanCount) = 50 / (50 + 0) = 1.0
	if metrics.MatchRate != 1.0 {
		t.Errorf("expected MatchRate=1.0, got %f", metrics.MatchRate)
	}

	// SourceDistinct = SourceMatched + OrphanCount = 50 + 0 = 50
	if metrics.SourceDistinct != 50 {
		t.Errorf("expected SourceDistinct=50, got %d", metrics.SourceDistinct)
	}

	// TargetDistinct = TargetMatched = 40
	if metrics.TargetDistinct != 40 {
		t.Errorf("expected TargetDistinct=40, got %d", metrics.TargetDistinct)
	}

	// MatchedCount = SourceMatched = 50
	if metrics.MatchedCount != 50 {
		t.Errorf("expected MatchedCount=50, got %d", metrics.MatchedCount)
	}
}
