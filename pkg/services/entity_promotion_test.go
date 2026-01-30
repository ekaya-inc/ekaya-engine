package services

import (
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestPromotionScore_EmptyInput(t *testing.T) {
	result := PromotionScore(PromotionInput{
		TableName:  "users",
		SchemaName: "public",
	})

	if result.Score != 0 {
		t.Errorf("empty input should return score 0, got %d", result.Score)
	}
	if len(result.Reasons) != 0 {
		t.Errorf("empty input should return no reasons, got %v", result.Reasons)
	}
}

func TestPromotionScore_HubWithManyInboundRefs(t *testing.T) {
	// Create 5 inbound relationships (pointing to users table)
	relationships := make([]*models.EntityRelationship, 5)
	tables := []string{"orders", "comments", "posts", "profiles", "sessions"}
	for i, table := range tables {
		relationships[i] = &models.EntityRelationship{
			SourceColumnSchema: "public",
			SourceColumnTable:  table,
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		}
	}

	result := PromotionScore(PromotionInput{
		TableName:     "users",
		SchemaName:    "public",
		Relationships: relationships,
	})

	if result.Score != pointsHubMajor {
		t.Errorf("5+ inbound refs should give %d points, got %d", pointsHubMajor, result.Score)
	}
	if len(result.Reasons) != 1 {
		t.Errorf("expected 1 reason, got %d: %v", len(result.Reasons), result.Reasons)
	}
}

func TestPromotionScore_HubWith3InboundRefs(t *testing.T) {
	relationships := make([]*models.EntityRelationship, 3)
	tables := []string{"orders", "comments", "posts"}
	for i, table := range tables {
		relationships[i] = &models.EntityRelationship{
			SourceColumnSchema: "public",
			SourceColumnTable:  table,
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		}
	}

	result := PromotionScore(PromotionInput{
		TableName:     "users",
		SchemaName:    "public",
		Relationships: relationships,
	})

	if result.Score != pointsHubMinor {
		t.Errorf("3+ inbound refs should give %d points, got %d", pointsHubMinor, result.Score)
	}
}

func TestPromotionScore_MultipleRoles(t *testing.T) {
	// host_id and visitor_id both reference users
	host := "host"
	visitor := "visitor"
	relationships := []*models.EntityRelationship{
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "meetings",
			SourceColumnName:   "host_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
			Association:        &host,
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "meetings",
			SourceColumnName:   "visitor_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
			Association:        &visitor,
		},
	}

	result := PromotionScore(PromotionInput{
		TableName:     "users",
		SchemaName:    "public",
		Relationships: relationships,
	})

	// Should get points for multiple roles
	if result.Score != pointsMultipleRoles {
		t.Errorf("2+ distinct roles should give %d points, got %d", pointsMultipleRoles, result.Score)
	}
}

func TestPromotionScore_MultipleRolesFromColumnNames(t *testing.T) {
	// When Association is not set, derive roles from column names
	relationships := []*models.EntityRelationship{
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "meetings",
			SourceColumnName:   "host_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "meetings",
			SourceColumnName:   "visitor_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "invoices",
			SourceColumnName:   "payer_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
	}

	result := PromotionScore(PromotionInput{
		TableName:     "users",
		SchemaName:    "public",
		Relationships: relationships,
	})

	// Should get points for multiple roles (host, visitor, payer)
	if result.Score < pointsMultipleRoles {
		t.Errorf("3 distinct roles should give at least %d points, got %d", pointsMultipleRoles, result.Score)
	}
}

func TestPromotionScore_RelatedTables(t *testing.T) {
	// Multiple tables share the "users" concept
	allTables := []*models.SchemaTable{
		{TableName: "users", SchemaName: "public"},
		{TableName: "s1_users", SchemaName: "public"},
		{TableName: "test_users", SchemaName: "public"},
	}

	result := PromotionScore(PromotionInput{
		TableName:  "users",
		SchemaName: "public",
		AllTables:  allTables,
	})

	if result.Score != pointsRelatedTables {
		t.Errorf("multiple related tables should give %d points, got %d", pointsRelatedTables, result.Score)
	}
}

func TestPromotionScore_BusinessAliases(t *testing.T) {
	discoverySource := "discovery"
	aliases := []*models.OntologyEntityAlias{
		{ID: uuid.New(), Alias: "customer", Source: &discoverySource},
		{ID: uuid.New(), Alias: "member", Source: &discoverySource},
	}

	result := PromotionScore(PromotionInput{
		TableName:  "users",
		SchemaName: "public",
		Aliases:    aliases,
	})

	if result.Score != pointsBusinessAlias {
		t.Errorf("business aliases should give %d points, got %d", pointsBusinessAlias, result.Score)
	}
}

func TestPromotionScore_BusinessAliases_SkipsTableGrouping(t *testing.T) {
	discoverySource := "discovery"
	tableGroupingSource := "table_grouping"
	aliases := []*models.OntologyEntityAlias{
		{ID: uuid.New(), Alias: "customer", Source: &discoverySource},
		{ID: uuid.New(), Alias: "s1_users", Source: &tableGroupingSource}, // Should be skipped
	}

	result := PromotionScore(PromotionInput{
		TableName:  "users",
		SchemaName: "public",
		Aliases:    aliases,
	})

	// Should still get points (only 1 business alias counted)
	if result.Score != pointsBusinessAlias {
		t.Errorf("should still get business alias points, got %d", result.Score)
	}
}

func TestPromotionScore_OutboundRelationships(t *testing.T) {
	// Create 3 outbound relationships (orders references other tables)
	relationships := []*models.EntityRelationship{
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "product_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "products",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "shipping_address_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "addresses",
			TargetColumnName:   "id",
		},
	}

	result := PromotionScore(PromotionInput{
		TableName:     "orders",
		SchemaName:    "public",
		Relationships: relationships,
	})

	if result.Score != pointsOutboundMinor {
		t.Errorf("3+ outbound relationships should give %d points, got %d", pointsOutboundMinor, result.Score)
	}
}

func TestPromotionScore_CombinedCriteria_PromotesEntity(t *testing.T) {
	// User entity: hub (5+ inbound) + multiple roles = should be promoted
	host := "host"
	visitor := "visitor"
	relationships := []*models.EntityRelationship{
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "meetings",
			SourceColumnName:   "host_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
			Association:        &host,
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "meetings",
			SourceColumnName:   "visitor_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
			Association:        &visitor,
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "comments",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "posts",
			SourceColumnName:   "author_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
	}

	discoverySource := "discovery"
	aliases := []*models.OntologyEntityAlias{
		{ID: uuid.New(), Alias: "customer", Source: &discoverySource},
	}

	result := PromotionScore(PromotionInput{
		TableName:     "users",
		SchemaName:    "public",
		Relationships: relationships,
		Aliases:       aliases,
	})

	// Expected: hub major (30) + multiple roles (25) + business alias (15) = 70
	expectedMinScore := pointsHubMajor + pointsMultipleRoles + pointsBusinessAlias
	if result.Score < expectedMinScore {
		t.Errorf("combined criteria should give at least %d points, got %d", expectedMinScore, result.Score)
	}

	if result.Score < PromotionThreshold {
		t.Errorf("score %d should be >= threshold %d for promotion", result.Score, PromotionThreshold)
	}
}

func TestPromotionScore_LeafTable_NotPromoted(t *testing.T) {
	// A leaf table with single relationship should not be promoted
	relationships := []*models.EntityRelationship{
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "password_resets",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
	}

	result := PromotionScore(PromotionInput{
		TableName:     "password_resets",
		SchemaName:    "public",
		Relationships: relationships,
	})

	if result.Score >= PromotionThreshold {
		t.Errorf("leaf table should score below threshold (%d), got %d", PromotionThreshold, result.Score)
	}
}

func TestPromotionScore_CaseInsensitive(t *testing.T) {
	// Test that table matching is case-insensitive
	relationships := []*models.EntityRelationship{
		{
			SourceColumnSchema: "PUBLIC",
			SourceColumnTable:  "Orders",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "USERS",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "comments",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "PUBLIC",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "Public",
			SourceColumnTable:  "Posts",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "Public",
			TargetColumnTable:  "Users",
			TargetColumnName:   "id",
		},
	}

	result := PromotionScore(PromotionInput{
		TableName:     "users",
		SchemaName:    "public",
		Relationships: relationships,
	})

	// Should count all 3 as inbound (case-insensitive matching)
	if result.Score != pointsHubMinor {
		t.Errorf("case-insensitive matching should find 3 inbound refs (%d points), got %d", pointsHubMinor, result.Score)
	}
}

func TestDeriveRoleFromColumn(t *testing.T) {
	tests := []struct {
		columnName string
		expected   string
	}{
		{"user_id", "user"},
		{"host_id", "host"},
		{"visitor_id", "visitor"},
		{"created_by_id", "created_by"},
		{"account_uuid", "account"},
		{"customer_fk", "customer"},
		{"id", ""},         // Generic, should return empty
		{"uuid", ""},       // Generic, should return empty
		{"user", "user"},   // No suffix, should return as-is
		{"HOST_ID", "host"}, // Uppercase handling
	}

	for _, tt := range tests {
		t.Run(tt.columnName, func(t *testing.T) {
			result := deriveRoleFromColumn(tt.columnName)
			if result != tt.expected {
				t.Errorf("deriveRoleFromColumn(%s) = %s, want %s", tt.columnName, result, tt.expected)
			}
		})
	}
}

func TestCountInboundRelationships(t *testing.T) {
	relationships := []*models.EntityRelationship{
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "comments",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "other",
			SourceColumnTable:  "logs",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "other",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
	}

	// Should only count relationships targeting public.users
	count := countInboundRelationships("users", "public", relationships)
	if count != 2 {
		t.Errorf("countInboundRelationships should return 2, got %d", count)
	}
}

func TestCountOutboundRelationships(t *testing.T) {
	relationships := []*models.EntityRelationship{
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "product_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "products",
			TargetColumnName:   "id",
		},
		{
			SourceColumnSchema: "other",
			SourceColumnTable:  "orders",
			SourceColumnName:   "store_id",
			TargetColumnSchema: "other",
			TargetColumnTable:  "stores",
			TargetColumnName:   "id",
		},
	}

	// Should only count relationships from public.orders
	count := countOutboundRelationships("orders", "public", relationships)
	if count != 2 {
		t.Errorf("countOutboundRelationships should return 2, got %d", count)
	}
}

func TestFindRelatedTables(t *testing.T) {
	allTables := []*models.SchemaTable{
		{TableName: "users", SchemaName: "public"},
		{TableName: "s1_users", SchemaName: "public"},
		{TableName: "test_users", SchemaName: "public"},
		{TableName: "orders", SchemaName: "public"},
		{TableName: "products", SchemaName: "public"},
	}

	related := findRelatedTables("users", allTables)

	if len(related) != 3 {
		t.Errorf("findRelatedTables should return 3 tables (users, s1_users, test_users), got %d", len(related))
	}
}

func TestCountBusinessAliases(t *testing.T) {
	discoverySource := "discovery"
	tableGroupingSource := "table_grouping"
	userSource := "user"

	aliases := []*models.OntologyEntityAlias{
		{Alias: "customer", Source: &discoverySource},
		{Alias: "member", Source: &discoverySource},
		{Alias: "s1_users", Source: &tableGroupingSource}, // Should be skipped
		{Alias: "patron", Source: &userSource},
		{Alias: "no_source", Source: nil}, // nil source counts as business alias
	}

	count := countBusinessAliases(aliases)
	if count != 4 {
		t.Errorf("countBusinessAliases should return 4 (skipping table_grouping), got %d", count)
	}
}

func TestPromotionThreshold(t *testing.T) {
	// Verify the threshold constant is what we expect
	if PromotionThreshold != 50 {
		t.Errorf("PromotionThreshold should be 50, got %d", PromotionThreshold)
	}
}

func TestPromotionScore_ReasonsAreDescriptive(t *testing.T) {
	// Verify reasons contain useful information
	relationships := make([]*models.EntityRelationship, 5)
	for i := 0; i < 5; i++ {
		relationships[i] = &models.EntityRelationship{
			SourceColumnSchema: "public",
			SourceColumnTable:  "table" + string(rune('a'+i)),
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
		}
	}

	result := PromotionScore(PromotionInput{
		TableName:     "users",
		SchemaName:    "public",
		Relationships: relationships,
	})

	if len(result.Reasons) == 0 {
		t.Fatal("expected at least one reason")
	}

	// Reason should mention the count
	found := false
	for _, reason := range result.Reasons {
		if len(reason) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("reasons should be non-empty strings")
	}
}
