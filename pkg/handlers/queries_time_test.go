package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestQueriesHandlerToQueryResponseNormalizesTimestampsToUTCRFC3339(t *testing.T) {
	loc := time.FixedZone("UTC+2", 2*60*60)
	createdAt := time.Date(2026, time.March, 12, 14, 15, 16, 987000000, loc)
	updatedAt := createdAt.Add(5 * time.Minute)
	lastUsedAt := createdAt.Add(10 * time.Minute)
	reviewedAt := createdAt.Add(15 * time.Minute)
	suggestedBy := "agent"

	handler := &QueriesHandler{}
	resp := handler.toQueryResponse(&models.Query{
		ID:                    uuid.New(),
		ProjectID:             uuid.New(),
		DatasourceID:          uuid.New(),
		NaturalLanguagePrompt: "Recent orders",
		SQLQuery:              "SELECT * FROM orders",
		Dialect:               "postgres",
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
		LastUsedAt:            &lastUsedAt,
		ReviewedAt:            &reviewedAt,
		Status:                "approved",
		SuggestedBy:           &suggestedBy,
	})

	if resp.CreatedAt != "2026-03-12T12:15:16Z" {
		t.Fatalf("CreatedAt = %q, want %q", resp.CreatedAt, "2026-03-12T12:15:16Z")
	}
	if resp.UpdatedAt != "2026-03-12T12:20:16Z" {
		t.Fatalf("UpdatedAt = %q, want %q", resp.UpdatedAt, "2026-03-12T12:20:16Z")
	}
	if resp.LastUsedAt == nil || *resp.LastUsedAt != "2026-03-12T12:25:16Z" {
		t.Fatalf("LastUsedAt = %v, want %q", resp.LastUsedAt, "2026-03-12T12:25:16Z")
	}
	if resp.ReviewedAt == nil || *resp.ReviewedAt != "2026-03-12T12:30:16Z" {
		t.Fatalf("ReviewedAt = %v, want %q", resp.ReviewedAt, "2026-03-12T12:30:16Z")
	}
}
