package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
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

// TestPKMatch_RequiresDistinctCount verifies that columns without DistinctCount
// are skipped and do not create relationships.
func TestPKMatch_RequiresDistinctCount(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()

	distinctCount := int64(100)
	isJoinableTrue := true

	// Create mocks with user entity
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Add order entity - we need both entities to test that the FILTER logic
	// (not the entity lookup) is what prevents relationship creation
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

	// Mock discoverer would return 0 orphans if called - but should NOT be called
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		t.Errorf("AnalyzeJoin should not be called (column without DistinctCount): %s.%s.%s -> %s.%s.%s",
			sourceSchema, sourceTable, sourceColumn,
			targetSchema, targetTable, targetColumn)
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	// Schema: orders.buyer has IsJoinable=true but no DistinctCount - should be skipped as candidate
	// Using column name "buyer" (not "buyer_id") to ensure it's not treated as an entityRefColumn
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
					DistinctCount: &distinctCount, // Has stats
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
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount, // Orders PK
				},
				{
					SchemaTableID: ordersTableID,
					ColumnName:    "buyer", // Not ending in _id, so won't be entityRefColumn
					DataType:      "uuid",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue, // Would be joinable...
					DistinctCount: nil,             // ...but NO stats - should be skipped as candidate
				},
			},
		},
	}

	// Create service
	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
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

	// Verify: NO relationships created because orders.user_id lacks stats
	if result.InferredRelationships != 0 {
		t.Errorf("expected 0 inferred relationships (column without stats should be skipped), got %d", result.InferredRelationships)
	}

	// Verify no relationships were persisted
	if len(mocks.relationshipRepo.created) != 0 {
		t.Errorf("expected 0 relationships to be created, got %d", len(mocks.relationshipRepo.created))
	}
}

// TestPKMatch_WorksWithoutRowCount verifies that columns with DistinctCount
// but missing RowCount can still pass the cardinality filter.
func TestPKMatch_WorksWithoutRowCount(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()

	distinctCount := int64(50) // Meets absolute threshold (>= 20)
	isJoinableTrue := true

	// Create mocks with user entity
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Add order entity - PK match needs TWO entities: source (orders) and target (users)
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

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

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   nil, // NO row count - ratio check should be skipped
			Columns: []models.SchemaColumn{
				{
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
					SchemaTableID: ordersTableID,
					ColumnName:    "id",
					DataType:      "int8", // Different type - prevents bidirectional matching
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &distinctCount, // Orders PK
				},
				{
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

	// Verify: 2 logical relationships discovered (bidirectional matching due to high-cardinality columns)
	// Missing RowCount doesn't block when DistinctCount >= 20
	// Each logical relationship creates 2 rows (forward + reverse) = 4 total rows
	if result.InferredRelationships != 2 {
		t.Errorf("expected 2 inferred relationships (column has sufficient DistinctCount, bidirectional), got %d", result.InferredRelationships)
	}

	// Verify relationships were persisted (2 logical x 2 directions = 4 rows)
	if len(mocks.relationshipRepo.created) != 4 {
		t.Fatalf("expected 4 relationships to be created (2 logical x 2 directions), got %d", len(mocks.relationshipRepo.created))
	}

	// Verify both logical relationships exist (each appears twice: once forward, once reverse)
	orderToUserCount := 0
	userToOrderCount := 0
	for _, rel := range mocks.relationshipRepo.created {
		if rel.SourceEntityID == orderEntityID && rel.TargetEntityID == userEntityID {
			orderToUserCount++
		}
		if rel.SourceEntityID == userEntityID && rel.TargetEntityID == orderEntityID {
			userToOrderCount++
		}
	}
	// Each logical relationship appears twice (forward + reverse)
	if orderToUserCount != 2 {
		t.Errorf("expected 2 order -> user relationships (forward + reverse of each logical rel), got %d", orderToUserCount)
	}
	if userToOrderCount != 2 {
		t.Errorf("expected 2 user -> order relationships (forward + reverse of each logical rel), got %d", userToOrderCount)
	}
}

// TestIsPKMatchExcludedName verifies that the name exclusion function catches
// all patterns that should not be considered as FK candidates.
func TestIsPKMatchExcludedName(t *testing.T) {
	tests := []struct {
		name     string
		column   string
		excluded bool
	}{
		// Count patterns with num_ prefix
		{"num_users should be excluded", "num_users", true},
		{"num_items should be excluded", "num_items", true},
		{"NUM_ORDERS should be excluded (case insensitive)", "NUM_ORDERS", true},

		// Count patterns with total_ prefix
		{"total_amount should be excluded", "total_amount", true},
		{"total_sales should be excluded", "total_sales", true},
		{"TOTAL_REVENUE should be excluded (case insensitive)", "TOTAL_REVENUE", true},

		// Existing count suffix patterns
		{"user_count should be excluded", "user_count", true},
		{"order_count should be excluded", "order_count", true},

		// Amount/total suffixes
		{"order_amount should be excluded", "order_amount", true},
		{"sale_total should be excluded", "sale_total", true},

		// Aggregate function suffixes
		{"revenue_sum should be excluded", "revenue_sum", true},
		{"price_avg should be excluded", "price_avg", true},
		{"score_min should be excluded", "score_min", true},
		{"value_max should be excluded", "value_max", true},

		// Rating patterns
		{"rating should be excluded", "rating", true},
		{"user_rating should be excluded", "user_rating", true},
		{"product_rating should be excluded", "product_rating", true},
		{"RATING should be excluded (case insensitive)", "RATING", true},

		// Score patterns
		{"score should be excluded", "score", true},
		{"credit_score should be excluded", "credit_score", true},
		{"quality_score should be excluded", "quality_score", true},

		// Level patterns
		{"level should be excluded", "level", true},
		{"mod_level should be excluded", "mod_level", true},
		{"access_level should be excluded", "access_level", true},

		// Valid FK column names should NOT be excluded
		{"user_id should NOT be excluded", "user_id", false},
		{"account_id should NOT be excluded", "account_id", false},
		{"id should NOT be excluded", "id", false},
		{"owner_id should NOT be excluded", "owner_id", false},
		{"host_id should NOT be excluded", "host_id", false},

		// Edge cases - columns with excluded patterns as substrings
		{"document_id should NOT be excluded (ment != amount)", "document_id", false},
		{"internal should NOT be excluded (internal != num_)", "internal", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPKMatchExcludedName(tt.column)
			if result != tt.excluded {
				t.Errorf("isPKMatchExcludedName(%q) = %v, want %v", tt.column, result, tt.excluded)
			}
		})
	}
}

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

		// Verify: 2 logical relationships discovered (bidirectional matching due to high-cardinality columns)
		// orders.buyer (uuid, high distinct) -> users.id (uuid, PK)
		// users.id (uuid, candidate) -> orders.buyer (uuid, entityRef due to high distinct)
		// This is expected behavior: high-cardinality columns become both candidates and entityRefColumns
		// Each logical relationship creates 2 rows (forward + reverse) = 4 total rows
		if result.InferredRelationships != 2 {
			t.Errorf("expected 2 inferred relationships (IsJoinable=true columns form bidirectional matches), got %d", result.InferredRelationships)
		}

		if len(mocks.relationshipRepo.created) != 4 {
			t.Fatalf("expected 4 relationships to be created (2 logical x 2 directions), got %d", len(mocks.relationshipRepo.created))
		}

		// Verify both logical relationships exist (each appears twice: once forward, once reverse)
		orderToUserCount := 0
		userToOrderCount := 0
		for _, rel := range mocks.relationshipRepo.created {
			if rel.SourceEntityID == orderEntityID && rel.TargetEntityID == userEntityID {
				orderToUserCount++
			}
			if rel.SourceEntityID == userEntityID && rel.TargetEntityID == orderEntityID {
				userToOrderCount++
			}
		}
		if orderToUserCount != 2 {
			t.Errorf("expected 2 order -> user relationships (forward + reverse of each logical rel), got %d", orderToUserCount)
		}
		if userToOrderCount != 2 {
			t.Errorf("expected 2 user -> order relationships (forward + reverse of each logical rel), got %d", userToOrderCount)
		}
	})
}

// mockTestServices holds all mock dependencies for testing
type mockTestServices struct {
	datasourceService *mockTestDatasourceService
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
	created []*models.EntityRelationship
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
	tables        []*models.SchemaTable
	columns       []*models.SchemaColumn
	relationships []*models.SchemaRelationship
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

func (m *mockTestSchemaRepo) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string][]*models.SchemaColumn, error) {
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

func (m *mockTestRelationshipRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockTestRelationshipRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
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
	return nil
}

func (m *mockTestSchemaRepo) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	return nil
}

func (m *mockTestSchemaRepo) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return nil
}

func (m *mockTestSchemaRepo) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
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
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
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

	// Verify: 1 logical relationship IS created because target table is also small (lookup table)
	// Each logical relationship creates 2 rows (forward + reverse) = 2 total rows
	if result.InferredRelationships != 1 {
		t.Errorf("expected 1 inferred relationship (small FK to small lookup table is valid), got %d", result.InferredRelationships)
	}

	if len(mocks.relationshipRepo.created) != 2 {
		t.Fatalf("expected 2 relationships in repo (1 logical x 2 directions), got %d", len(mocks.relationshipRepo.created))
	}

	// Verify the relationships were created (don't assert on exact order)
	// The PK match algorithm may discover relationships in either direction
	hasOrdersToStatuses := false
	hasStatusesToOrders := false
	for _, rel := range mocks.relationshipRepo.created {
		if rel.SourceColumnTable == "orders" && rel.TargetColumnTable == "order_statuses" {
			hasOrdersToStatuses = true
		}
		if rel.SourceColumnTable == "order_statuses" && rel.TargetColumnTable == "orders" {
			hasStatusesToOrders = true
		}
	}
	if !hasOrdersToStatuses {
		t.Error("expected orders -> order_statuses relationship")
	}
	if !hasStatusesToOrders {
		t.Error("expected order_statuses -> orders relationship")
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

// TestPKMatch_CountColumns_NeverJoined tests that columns with count/aggregate names
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

	// Mock discoverer should NOT be called because count columns are filtered by name BEFORE cardinality ratio check
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		t.Errorf("AnalyzeJoin should not be called (count columns should be excluded by name): %s.%s.%s -> %s.%s.%s",
			sourceSchema, sourceTable, sourceColumn,
			targetSchema, targetTable, targetColumn)
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	usersTableID := uuid.New()
	accountsTableID := uuid.New()

	// Schema: users.id is a PK (entityRefColumn), accounts has count columns
	// Count columns should NOT be FK candidates - they are excluded by NAME filter
	// before the cardinality ratio check would exclude them
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
					ColumnName:    "num_users", // Count column with num_ prefix
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct, // Has sufficient absolute distinct count (>= 20)
				},
				{
					SchemaTableID: accountsTableID,
					ColumnName:    "user_count", // Count column with _count suffix
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct,
				},
				{
					SchemaTableID: accountsTableID,
					ColumnName:    "total_items", // Count column with total_ prefix
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct,
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
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

	// Verify: NO relationships created because count columns are excluded by name filter
	// Even though they have sufficient DistinctCount, the name filter should prevent them
	// from being FK candidates
	if result.InferredRelationships != 0 {
		t.Errorf("expected 0 inferred relationships (count columns should be excluded), got %d", result.InferredRelationships)
	}

	if len(mocks.relationshipRepo.created) != 0 {
		t.Errorf("expected 0 relationships to be created, got %d", len(mocks.relationshipRepo.created))
	}
}

// TestPKMatch_RatingColumns_NeverJoined tests that columns with rating/score/level names
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

	// Mock discoverer should NOT be called because rating/score/level columns are filtered by name BEFORE cardinality ratio check
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		t.Errorf("AnalyzeJoin should not be called (rating/score/level columns should be excluded by name): %s.%s.%s -> %s.%s.%s",
			sourceSchema, sourceTable, sourceColumn,
			targetSchema, targetTable, targetColumn)
		return &datasource.JoinAnalysis{OrphanCount: 0}, nil
	}

	usersTableID := uuid.New()
	reviewsTableID := uuid.New()

	// Schema: users.id is a PK (entityRefColumn), reviews has rating/score/level columns
	// Rating/score/level columns should NOT be FK candidates - they are excluded by NAME filter
	// before the cardinality ratio check would exclude them
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
					ColumnName:    "rating", // Rating column
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct, // Has sufficient absolute distinct count (>= 20)
				},
				{
					SchemaTableID: reviewsTableID,
					ColumnName:    "mod_level", // Level column
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct,
				},
				{
					SchemaTableID: reviewsTableID,
					ColumnName:    "score", // Score column
					DataType:      "bigint",
					IsPrimaryKey:  false,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &candidateDistinct,
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
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

	// Verify: NO relationships created because rating/score/level columns are excluded by name filter
	// Even though they have sufficient DistinctCount, the name filter should prevent them
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
// relationship_type="manual" create entity relationships with detection_method="manual"
func TestFKDiscovery_ManualRelationshipType(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	channelEntityID := uuid.New()

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Add channel entity
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            channelEntityID,
		OntologyID:    ontologyID,
		Name:          "channel",
		PrimarySchema: "public",
		PrimaryTable:  "channels",
	})

	usersTableID := uuid.New()
	channelsTableID := uuid.New()
	userIDColumnID := uuid.New()
	ownerIDColumnID := uuid.New()

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

	// Add manual schema relationship
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    usersTableID,
			SourceColumnID:   userIDColumnID,
			TargetTableID:    channelsTableID,
			TargetColumnID:   ownerIDColumnID,
			RelationshipType: models.RelationshipTypeManual, // Manual relationship
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
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

	// Verify: 1 FK relationship discovered (creates 2 rows: forward + reverse)
	if result.FKRelationships != 1 {
		t.Errorf("expected 1 FK relationship, got %d", result.FKRelationships)
	}

	// Bidirectional: 2 relationships created (forward + reverse)
	if len(mocks.relationshipRepo.created) != 2 {
		t.Fatalf("expected 2 relationships to be created (bidirectional), got %d", len(mocks.relationshipRepo.created))
	}

	// Verify: forward relationship has detection_method="manual"
	forwardRel := mocks.relationshipRepo.created[0]
	if forwardRel.DetectionMethod != models.DetectionMethodManual {
		t.Errorf("expected forward DetectionMethod=%q, got %q", models.DetectionMethodManual, forwardRel.DetectionMethod)
	}

	// Verify: reverse relationship also has detection_method="manual"
	reverseRel := mocks.relationshipRepo.created[1]
	if reverseRel.DetectionMethod != models.DetectionMethodManual {
		t.Errorf("expected reverse DetectionMethod=%q, got %q", models.DetectionMethodManual, reverseRel.DetectionMethod)
	}
}

// TestFKDiscovery_ForeignKeyRelationshipType tests that schema relationships with
// relationship_type="fk" create entity relationships with detection_method="foreign_key"
func TestFKDiscovery_ForeignKeyRelationshipType(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	userEntityID := uuid.New()
	orderEntityID := uuid.New()

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, userEntityID)

	// Add order entity
	mocks.entityRepo.entities = append(mocks.entityRepo.entities, &models.OntologyEntity{
		ID:            orderEntityID,
		OntologyID:    ontologyID,
		Name:          "order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
	})

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

	// Add FK schema relationship
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    ordersTableID,
			SourceColumnID:   buyerIDColumnID,
			TargetTableID:    usersTableID,
			TargetColumnID:   userIDColumnID,
			RelationshipType: models.RelationshipTypeFK, // FK from DDL
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
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

	// Verify: 1 FK relationship discovered (creates 2 rows: forward + reverse)
	if result.FKRelationships != 1 {
		t.Errorf("expected 1 FK relationship, got %d", result.FKRelationships)
	}

	// Bidirectional: 2 relationships created (forward + reverse)
	if len(mocks.relationshipRepo.created) != 2 {
		t.Fatalf("expected 2 relationships to be created (bidirectional), got %d", len(mocks.relationshipRepo.created))
	}

	// Verify: forward relationship has detection_method="foreign_key"
	forwardRel := mocks.relationshipRepo.created[0]
	if forwardRel.DetectionMethod != models.DetectionMethodForeignKey {
		t.Errorf("expected forward DetectionMethod=%q, got %q", models.DetectionMethodForeignKey, forwardRel.DetectionMethod)
	}

	// Verify: reverse relationship also has detection_method="foreign_key"
	reverseRel := mocks.relationshipRepo.created[1]
	if reverseRel.DetectionMethod != models.DetectionMethodForeignKey {
		t.Errorf("expected reverse DetectionMethod=%q, got %q", models.DetectionMethodForeignKey, reverseRel.DetectionMethod)
	}
}

// TestFKDiscovery_SelfReferentialRelationship tests that self-referential FK constraints
// (e.g., employee.manager_id → employee.id) are discovered as entity relationships.
// These represent hierarchies/trees and are intentional design patterns.
func TestFKDiscovery_SelfReferentialRelationship(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	employeeEntityID := uuid.New()

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, employeeEntityID)

	// Replace default user entity with employee entity that owns the employees table
	mocks.entityRepo.entities = []*models.OntologyEntity{
		{
			ID:            employeeEntityID,
			OntologyID:    ontologyID,
			Name:          "employee",
			PrimarySchema: "public",
			PrimaryTable:  "employees",
		},
	}

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
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    employeesTableID,
			SourceColumnID:   managerIDColumnID,
			TargetTableID:    employeesTableID, // Same table
			TargetColumnID:   idColumnID,       // References own PK
			RelationshipType: models.RelationshipTypeFK,
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
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

	// Verify: 1 FK relationship discovered (creates 2 rows: forward + reverse)
	if result.FKRelationships != 1 {
		t.Errorf("expected 1 FK relationship, got %d", result.FKRelationships)
	}

	// Bidirectional: 2 relationships created (forward + reverse)
	if len(mocks.relationshipRepo.created) != 2 {
		t.Fatalf("expected 2 relationships to be created (bidirectional), got %d", len(mocks.relationshipRepo.created))
	}

	// Verify: both source and target are the same entity (self-reference)
	forwardRel := mocks.relationshipRepo.created[0]
	if forwardRel.SourceEntityID != employeeEntityID {
		t.Errorf("expected forward source entity to be employee, got %v", forwardRel.SourceEntityID)
	}
	if forwardRel.TargetEntityID != employeeEntityID {
		t.Errorf("expected forward target entity to be employee, got %v", forwardRel.TargetEntityID)
	}

	// Verify columns
	if forwardRel.SourceColumnName != "manager_id" {
		t.Errorf("expected forward source column 'manager_id', got %q", forwardRel.SourceColumnName)
	}
	if forwardRel.TargetColumnName != "id" {
		t.Errorf("expected forward target column 'id', got %q", forwardRel.TargetColumnName)
	}

	// Verify reverse relationship
	reverseRel := mocks.relationshipRepo.created[1]
	if reverseRel.SourceEntityID != employeeEntityID || reverseRel.TargetEntityID != employeeEntityID {
		t.Errorf("expected reverse to be self-referential, got source=%v target=%v", reverseRel.SourceEntityID, reverseRel.TargetEntityID)
	}
	if reverseRel.SourceColumnName != "id" || reverseRel.TargetColumnName != "manager_id" {
		t.Errorf("expected reverse columns id→manager_id, got %s→%s", reverseRel.SourceColumnName, reverseRel.TargetColumnName)
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
// 3. Reversed direction is NOT discovered (FK relationships preserve the constraint direction)
//
// This is the end-to-end validation test for BUG-4 fixes.
func TestFKDiscovery_LegitimateFK_EmailNotDiscovered(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	accountEntityID := uuid.New()
	authEntityID := uuid.New()

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, accountEntityID)

	// FK discovery requires entities for BOTH tables (source and target)
	mocks.entityRepo.entities = []*models.OntologyEntity{
		{
			ID:            accountEntityID,
			OntologyID:    ontologyID,
			Name:          "account",
			PrimarySchema: "public",
			PrimaryTable:  "accounts",
		},
		{
			ID:            authEntityID,
			OntologyID:    ontologyID,
			Name:          "account_authentication",
			PrimarySchema: "public",
			PrimaryTable:  "account_authentications",
		},
	}

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
	mocks.schemaRepo.relationships = []*models.SchemaRelationship{
		{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    accountAuthTableID,    // FK holder
			SourceColumnID:   authAccountIDColumnID, // FK column
			TargetTableID:    accountsTableID,       // PK holder
			TargetColumnID:   accountIDColumnID,     // PK column
			RelationshipType: models.RelationshipTypeFK,
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
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

	// Verify: 1 FK relationship discovered (the legitimate one)
	if result.FKRelationships != 1 {
		t.Errorf("expected 1 FK relationship, got %d", result.FKRelationships)
	}

	// Bidirectional: 2 relationships created (forward + reverse)
	if len(mocks.relationshipRepo.created) != 2 {
		t.Fatalf("expected 2 relationships to be created (bidirectional), got %d", len(mocks.relationshipRepo.created))
	}

	// Verify: forward relationship is account_authentications.account_id → accounts.account_id
	forwardRel := mocks.relationshipRepo.created[0]
	if forwardRel.SourceColumnName != "account_id" {
		t.Errorf("expected forward source column 'account_id', got %q", forwardRel.SourceColumnName)
	}
	if forwardRel.TargetColumnName != "account_id" {
		t.Errorf("expected forward target column 'account_id', got %q", forwardRel.TargetColumnName)
	}
	if forwardRel.SourceColumnTable != "account_authentications" {
		t.Errorf("expected forward source table 'account_authentications', got %q", forwardRel.SourceColumnTable)
	}
	if forwardRel.TargetColumnTable != "accounts" {
		t.Errorf("expected forward target table 'accounts', got %q", forwardRel.TargetColumnTable)
	}
	if forwardRel.DetectionMethod != models.DetectionMethodForeignKey {
		t.Errorf("expected DetectionMethod=%q, got %q", models.DetectionMethodForeignKey, forwardRel.DetectionMethod)
	}

	// Verify: NO email→email relationships were created
	// FK discovery only follows actual FK constraints from DDL, so email columns
	// without FK constraints won't be discovered - this validates BUG-4a
	for _, rel := range mocks.relationshipRepo.created {
		if rel.SourceColumnName == "email" && rel.TargetColumnName == "email" {
			t.Errorf("email→email relationship should NOT be discovered (BUG-4a): %s.%s → %s.%s",
				rel.SourceColumnTable, rel.SourceColumnName,
				rel.TargetColumnTable, rel.TargetColumnName)
		}
	}

	// Verify: The reverse relationship is the correct direction for bidirectional navigation
	// (not a spurious "reversed FK direction" relationship)
	reverseRel := mocks.relationshipRepo.created[1]
	if reverseRel.SourceColumnTable != "accounts" || reverseRel.TargetColumnTable != "account_authentications" {
		t.Errorf("expected reverse to be accounts → account_authentications, got %s → %s",
			reverseRel.SourceColumnTable, reverseRel.TargetColumnTable)
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

	rowCount := int64(1000)
	authRowCount := int64(500)
	distinctCount := int64(1000)
	authDistinctCount := int64(500)
	isJoinableTrue := true

	// Setup tables with email columns that have overlapping values
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
					DistinctCount: &distinctCount,
				},
				{
					ID:            uuid.New(),
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
					ID:            uuid.New(),
					SchemaTableID: accountAuthTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					IsJoinable:    &isJoinableTrue,
					DistinctCount: &authDistinctCount,
				},
				{
					ID:            uuid.New(),
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

	// Verify: NO relationships discovered (email columns should be excluded)
	if result.InferredRelationships != 0 {
		t.Errorf("expected 0 inferred relationships (email columns should be excluded), got %d", result.InferredRelationships)
	}

	// Verify: NO email→email relationships in repo
	for _, rel := range mocks.relationshipRepo.created {
		if rel.SourceColumnName == "email" || rel.TargetColumnName == "email" {
			t.Errorf("email column relationship should NOT be discovered: %s.%s → %s.%s",
				rel.SourceColumnTable, rel.SourceColumnName,
				rel.TargetColumnTable, rel.TargetColumnName)
		}
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
					DistinctCount: &visitorDistinct, // 500/100k = 0.5% (below old 5% threshold)
				},
			},
		},
	}

	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
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

	// Verify: visitor_id should have passed the cardinality filter because it ends in _id
	if !joinAnalysisCalled {
		t.Error("expected join analysis to be called for visitor_id column (should bypass cardinality ratio check for _id columns)")
	}

	// Verify: relationship created (visitor_id → user_id)
	if result.InferredRelationships < 1 {
		t.Errorf("expected at least 1 inferred relationship (visitor_id → user_id), got %d", result.InferredRelationships)
	}

	// Verify: at least one relationship exists for billing_engagements.visitor_id → users.user_id
	var foundVisitorRelationship bool
	for _, rel := range mocks.relationshipRepo.created {
		if rel.SourceColumnTable == "billing_engagements" && rel.SourceColumnName == "visitor_id" &&
			rel.TargetColumnTable == "users" && rel.TargetColumnName == "user_id" {
			foundVisitorRelationship = true
			break
		}
	}
	if !foundVisitorRelationship {
		t.Error("expected relationship billing_engagements.visitor_id → users.user_id to be discovered")
	}
}

// TestIsLikelyFKColumn tests the isLikelyFKColumn helper function
func TestIsLikelyFKColumn(t *testing.T) {
	tests := []struct {
		columnName string
		expected   bool
	}{
		// Should return true
		{"user_id", true},
		{"visitor_id", true},
		{"host_id", true},
		{"account_uuid", true},
		{"session_key", true},
		{"USER_ID", true}, // case insensitive
		{"Visitor_Id", true},

		// Should return false
		{"status", false},
		{"id", false}, // just "id" doesn't match suffix patterns
		{"rating", false},
		{"visitor", false}, // no _id suffix
		{"created_at", false},
		{"is_active", false},
	}

	for _, tt := range tests {
		t.Run(tt.columnName, func(t *testing.T) {
			result := isLikelyFKColumn(tt.columnName)
			if result != tt.expected {
				t.Errorf("isLikelyFKColumn(%q) = %v, expected %v", tt.columnName, result, tt.expected)
			}
		})
	}
}

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
	if result.InferredRelationships != 1 {
		t.Errorf("expected 1 inferred relationship (correct direction), got %d", result.InferredRelationships)
	}

	// Verify: 2 relationships in repo (forward + reverse for navigation)
	if len(mocks.relationshipRepo.created) != 2 {
		t.Errorf("expected 2 relationships (bidirectional), got %d", len(mocks.relationshipRepo.created))
	}

	// Verify: The forward relationship should be the correct FK direction
	// (account_password_resets.account_id → accounts.account_id)
	if len(mocks.relationshipRepo.created) >= 1 {
		forwardRel := mocks.relationshipRepo.created[0]
		if forwardRel.SourceColumnTable != "account_password_resets" || forwardRel.TargetColumnTable != "accounts" {
			t.Errorf("expected forward direction account_password_resets→accounts, got %s→%s",
				forwardRel.SourceColumnTable, forwardRel.TargetColumnTable)
		}
		if forwardRel.DetectionMethod != models.DetectionMethodPKMatch {
			t.Errorf("expected DetectionMethod=%q, got %q", models.DetectionMethodPKMatch, forwardRel.DetectionMethod)
		}
	}

	// Verify: The reverse relationship is for navigation only (not an independently discovered wrong-direction relationship)
	if len(mocks.relationshipRepo.created) >= 2 {
		reverseRel := mocks.relationshipRepo.created[1]
		if reverseRel.SourceColumnTable != "accounts" || reverseRel.TargetColumnTable != "account_password_resets" {
			t.Errorf("expected reverse direction accounts→account_password_resets (for navigation), got %s→%s",
				reverseRel.SourceColumnTable, reverseRel.TargetColumnTable)
		}
	}
}
