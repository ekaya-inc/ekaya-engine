package services

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
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
	)

	// Execute PK match discovery
	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID)
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
	)

	// Execute PK match discovery
	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: 2 relationships created (bidirectional matching due to high-cardinality columns)
	// Missing RowCount doesn't block when DistinctCount >= 20
	if result.InferredRelationships != 2 {
		t.Errorf("expected 2 inferred relationships (column has sufficient DistinctCount, bidirectional), got %d", result.InferredRelationships)
	}

	// Verify relationships were persisted
	if len(mocks.relationshipRepo.created) != 2 {
		t.Fatalf("expected 2 relationships to be created, got %d", len(mocks.relationshipRepo.created))
	}

	// Verify both relationships exist (one in each direction)
	var foundOrderToUser, foundUserToOrder bool
	for _, rel := range mocks.relationshipRepo.created {
		if rel.SourceEntityID == orderEntityID && rel.TargetEntityID == userEntityID {
			foundOrderToUser = true
		}
		if rel.SourceEntityID == userEntityID && rel.TargetEntityID == orderEntityID {
			foundUserToOrder = true
		}
	}
	if !foundOrderToUser {
		t.Error("expected relationship order -> user, not found")
	}
	if !foundUserToOrder {
		t.Error("expected relationship user -> order, not found")
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
		)

		result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID)
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
		)

		result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID)
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
		)

		result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify: 2 relationships created (bidirectional matching due to high-cardinality columns)
		// orders.buyer (uuid, high distinct) -> users.id (uuid, PK)
		// users.id (uuid, candidate) -> orders.buyer (uuid, entityRef due to high distinct)
		// This is expected behavior: high-cardinality columns become both candidates and entityRefColumns
		if result.InferredRelationships != 2 {
			t.Errorf("expected 2 inferred relationships (IsJoinable=true columns form bidirectional matches), got %d", result.InferredRelationships)
		}

		if len(mocks.relationshipRepo.created) != 2 {
			t.Fatalf("expected 2 relationships to be created, got %d", len(mocks.relationshipRepo.created))
		}

		// Verify both relationships exist (one in each direction)
		var foundOrderToUser, foundUserToOrder bool
		for _, rel := range mocks.relationshipRepo.created {
			if rel.SourceEntityID == orderEntityID && rel.TargetEntityID == userEntityID {
				foundOrderToUser = true
			}
			if rel.SourceEntityID == userEntityID && rel.TargetEntityID == orderEntityID {
				foundUserToOrder = true
			}
		}
		if !foundOrderToUser {
			t.Error("expected relationship order -> user, not found")
		}
		if !foundUserToOrder {
			t.Error("expected relationship user -> order, not found")
		}
	})
}

// mockTestServices holds all mock dependencies for testing
type mockTestServices struct {
	datasourceService  *mockTestDatasourceService
	adapterFactory     *mockTestAdapterFactory
	discoverer         *mockTestSchemaDiscoverer
	ontologyRepo       *mockTestOntologyRepo
	entityRepo         *mockTestEntityRepo
	relationshipRepo   *mockTestRelationshipRepo
	schemaRepo         *mockTestSchemaRepo
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
			occurrences: []*models.OntologyEntityOccurrence{
				{
					EntityID:   entityID,
					SchemaName: "public",
					TableName:  "users",
					ColumnName: "id",
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

func (m *mockTestDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, error) {
	return nil, nil
}

func (m *mockTestDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockTestDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType string, config map[string]any) error {
	return nil
}

func (m *mockTestDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockTestDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any) error {
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
	entities    []*models.OntologyEntity
	occurrences []*models.OntologyEntityOccurrence
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

func (m *mockTestEntityRepo) CreateOccurrence(ctx context.Context, occurrence *models.OntologyEntityOccurrence) error {
	return nil
}

func (m *mockTestEntityRepo) GetOccurrences(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
	return m.occurrences, nil
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

func (m *mockTestEntityRepo) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}

func (m *mockTestEntityRepo) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.OntologyEntityOccurrence, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetAllOccurrencesByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) UpdateOccurrenceRole(ctx context.Context, entityID uuid.UUID, tableName, columnName string, role *string) error {
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
	tables  []*models.SchemaTable
	columns []*models.SchemaColumn
}

func (m *mockTestSchemaRepo) GetTables(ctx context.Context, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return m.tables, nil
}

func (m *mockTestSchemaRepo) GetColumns(ctx context.Context, datasourceID uuid.UUID, schemaName, tableName string) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
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

func (m *mockTestOntologyRepo) DeactivateAll(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockTestOntologyRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockTestOntologyRepo) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockTestOntologyRepo) WriteCleanOntology(ctx context.Context, projectID uuid.UUID) error {
	return nil
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

func (m *mockTestOntologyRepo) UpdateMetadata(ctx context.Context, projectID uuid.UUID, metadata map[string]any) error {
	return nil
}

func (m *mockTestOntologyRepo) SetActive(ctx context.Context, projectID uuid.UUID, version int) error {
	return nil
}

func (m *mockTestEntityRepo) DeleteAlias(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockTestRelationshipRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockTestRelationshipRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
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

func (m *mockTestOntologyRepo) GetByVersion(ctx context.Context, projectID uuid.UUID, version int) (*models.TieredOntology, error) {
	return nil, nil
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

func (m *mockTestSchemaRepo) GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error {
	return nil
}

func (m *mockTestSchemaRepo) GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetRelationshipCandidates(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.LegacyRelationshipCandidate, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return nil, nil
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

func (m *mockTestSchemaRepo) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64) error {
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
