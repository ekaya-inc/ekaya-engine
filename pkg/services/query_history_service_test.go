package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockQueryHistoryRepository is a mock for testing.
type mockQueryHistoryRepository struct {
	entries  []*models.QueryHistoryEntry
	feedback map[uuid.UUID]string
}

func newMockQueryHistoryRepo() *mockQueryHistoryRepository {
	return &mockQueryHistoryRepository{
		feedback: make(map[uuid.UUID]string),
	}
}

func (m *mockQueryHistoryRepository) Create(ctx context.Context, entry *models.QueryHistoryEntry) error {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockQueryHistoryRepository) List(ctx context.Context, projectID uuid.UUID, filters models.QueryHistoryFilters) ([]*models.QueryHistoryEntry, int, error) {
	var result []*models.QueryHistoryEntry
	for _, e := range m.entries {
		if e.ProjectID != projectID {
			continue
		}
		if filters.UserID != "" && e.UserID != filters.UserID {
			continue
		}
		if filters.Since != nil && e.CreatedAt.Before(*filters.Since) {
			continue
		}
		result = append(result, e)
	}
	limit := filters.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	total := len(result)
	if len(result) > limit {
		result = result[:limit]
	}
	return result, total, nil
}

func (m *mockQueryHistoryRepository) UpdateFeedback(ctx context.Context, projectID uuid.UUID, entryID uuid.UUID, userID string, feedback string, comment *string) error {
	for _, e := range m.entries {
		if e.ID == entryID && e.ProjectID == projectID && e.UserID == userID {
			e.UserFeedback = &feedback
			e.FeedbackComment = comment
			m.feedback[entryID] = feedback
			return nil
		}
	}
	return fmt.Errorf("query history entry not found or not owned by user")
}

func (m *mockQueryHistoryRepository) DeleteOlderThan(ctx context.Context, projectID uuid.UUID, cutoff time.Time) (int64, error) {
	var remaining []*models.QueryHistoryEntry
	var deleted int64
	for _, e := range m.entries {
		if e.ProjectID == projectID && e.CreatedAt.Before(cutoff) {
			deleted++
		} else {
			remaining = append(remaining, e)
		}
	}
	m.entries = remaining
	return deleted, nil
}

func TestQueryHistoryService_Record(t *testing.T) {
	repo := newMockQueryHistoryRepo()
	svc := NewQueryHistoryService(repo, zap.NewNop())

	entry := &models.QueryHistoryEntry{
		ProjectID:       uuid.New(),
		UserID:          "user-123",
		NaturalLanguage: "Show me top customers by revenue",
		SQL:             "SELECT u.name, SUM(o.total) as revenue FROM users u JOIN orders o ON o.user_id = u.id GROUP BY u.name ORDER BY revenue DESC LIMIT 10",
		ExecutedAt:      time.Now(),
		CreatedAt:       time.Now(),
	}

	err := svc.Record(context.Background(), entry)
	require.NoError(t, err)
	require.Len(t, repo.entries, 1)

	recorded := repo.entries[0]
	assert.Equal(t, "Show me top customers by revenue", recorded.NaturalLanguage)
	assert.NotNil(t, recorded.QueryType)
	assert.Equal(t, "aggregation", *recorded.QueryType)
	assert.Contains(t, recorded.TablesUsed, "users")
	assert.Contains(t, recorded.TablesUsed, "orders")
	assert.Contains(t, recorded.AggregationsUsed, "SUM")
}

func TestQueryHistoryService_RecordFeedback(t *testing.T) {
	repo := newMockQueryHistoryRepo()
	svc := NewQueryHistoryService(repo, zap.NewNop())

	projectID := uuid.New()
	entryID := uuid.New()
	userID := "user-123"

	// Add an entry first
	repo.entries = append(repo.entries, &models.QueryHistoryEntry{
		ID:              entryID,
		ProjectID:       projectID,
		UserID:          userID,
		NaturalLanguage: "test query",
		SQL:             "SELECT 1",
		CreatedAt:       time.Now(),
	})

	// Record feedback
	err := svc.RecordFeedback(context.Background(), projectID, entryID, userID, "helpful", nil)
	require.NoError(t, err)
	assert.Equal(t, "helpful", repo.feedback[entryID])
}

func TestQueryHistoryService_RecordFeedback_InvalidValue(t *testing.T) {
	repo := newMockQueryHistoryRepo()
	svc := NewQueryHistoryService(repo, zap.NewNop())

	err := svc.RecordFeedback(context.Background(), uuid.New(), uuid.New(), "user", "bad", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid feedback value")
}

func TestQueryHistoryService_RecordFeedback_NotFound(t *testing.T) {
	repo := newMockQueryHistoryRepo()
	svc := NewQueryHistoryService(repo, zap.NewNop())

	err := svc.RecordFeedback(context.Background(), uuid.New(), uuid.New(), "user", "helpful", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestQueryHistoryService_PruneOlderThan(t *testing.T) {
	repo := newMockQueryHistoryRepo()
	svc := NewQueryHistoryService(repo, zap.NewNop())

	projectID := uuid.New()
	now := time.Now()

	// Add old and new entries
	repo.entries = append(repo.entries,
		&models.QueryHistoryEntry{
			ID:        uuid.New(),
			ProjectID: projectID,
			UserID:    "user-1",
			CreatedAt: now.AddDate(0, 0, -100), // 100 days ago
		},
		&models.QueryHistoryEntry{
			ID:        uuid.New(),
			ProjectID: projectID,
			UserID:    "user-1",
			CreatedAt: now, // Today
		},
	)

	cutoff := now.AddDate(0, 0, -90) // 90 days
	count, err := svc.PruneOlderThan(context.Background(), projectID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Len(t, repo.entries, 1)
}

func TestClassifyQuery_Aggregation(t *testing.T) {
	entry := &models.QueryHistoryEntry{
		SQL: "SELECT department, COUNT(*) as headcount, AVG(salary) FROM employees GROUP BY department",
	}
	classifyQuery(entry)

	assert.NotNil(t, entry.QueryType)
	assert.Equal(t, "aggregation", *entry.QueryType)
	assert.Contains(t, entry.AggregationsUsed, "COUNT")
	assert.Contains(t, entry.AggregationsUsed, "AVG")
	assert.Contains(t, entry.TablesUsed, "employees")
}

func TestClassifyQuery_Lookup(t *testing.T) {
	entry := &models.QueryHistoryEntry{
		SQL: "SELECT * FROM users WHERE email = 'test@example.com' LIMIT 1",
	}
	classifyQuery(entry)

	assert.NotNil(t, entry.QueryType)
	assert.Equal(t, "lookup", *entry.QueryType)
	assert.Contains(t, entry.TablesUsed, "users")
}

func TestClassifyQuery_Report(t *testing.T) {
	entry := &models.QueryHistoryEntry{
		SQL: "SELECT * FROM orders WHERE created_at > '2024-01-01' ORDER BY created_at DESC",
	}
	classifyQuery(entry)

	assert.NotNil(t, entry.QueryType)
	assert.Equal(t, "report", *entry.QueryType)
	assert.Contains(t, entry.TablesUsed, "orders")
}

func TestClassifyQuery_Exploration(t *testing.T) {
	entry := &models.QueryHistoryEntry{
		SQL: "SELECT * FROM products",
	}
	classifyQuery(entry)

	assert.NotNil(t, entry.QueryType)
	assert.Equal(t, "exploration", *entry.QueryType)
	assert.Contains(t, entry.TablesUsed, "products")
}

func TestExtractTablesFromSQL(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "simple select",
			sql:      "SELECT * FROM users",
			expected: []string{"users"},
		},
		{
			name:     "join",
			sql:      "SELECT u.name, o.total FROM users u JOIN orders o ON o.user_id = u.id",
			expected: []string{"users", "orders"},
		},
		{
			name:     "schema-qualified",
			sql:      "SELECT * FROM public.users JOIN sales.orders ON orders.user_id = users.id",
			expected: []string{"public.users", "sales.orders"},
		},
		{
			name:     "multiple joins",
			sql:      "SELECT * FROM a JOIN b ON b.id = a.b_id LEFT JOIN c ON c.id = b.c_id",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "CTE",
			sql:      "WITH cte AS (SELECT * FROM source) SELECT * FROM cte JOIN target ON target.id = cte.id",
			expected: []string{"source", "cte", "target"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tables := extractTablesFromSQL(tt.sql)
			assert.Equal(t, tt.expected, tables)
		})
	}
}

func TestExtractAggregations(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "count and sum",
			sql:      "SELECT COUNT(*), SUM(amount) FROM orders",
			expected: []string{"COUNT", "SUM"},
		},
		{
			name:     "avg with min max",
			sql:      "SELECT AVG(price), MIN(price), MAX(price) FROM products",
			expected: []string{"AVG", "MIN", "MAX"},
		},
		{
			name:     "no aggregations",
			sql:      "SELECT * FROM users",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAggregations(tt.sql)
			assert.Equal(t, tt.expected, result)
		})
	}
}
