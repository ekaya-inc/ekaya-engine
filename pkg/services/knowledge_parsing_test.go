package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockKnowledgeService implements KnowledgeService for testing.
type mockKnowledgeService struct {
	storeWithSourceFunc func(ctx context.Context, projectID uuid.UUID, factType, value, contextInfo, source string) (*models.KnowledgeFact, error)
	storeCalls          []storeWithSourceCall
}

type storeWithSourceCall struct {
	ProjectID   uuid.UUID
	FactType    string
	Value       string
	ContextInfo string
	Source      string
}

func (m *mockKnowledgeService) StoreWithSource(ctx context.Context, projectID uuid.UUID, factType, value, contextInfo, source string) (*models.KnowledgeFact, error) {
	m.storeCalls = append(m.storeCalls, storeWithSourceCall{
		ProjectID:   projectID,
		FactType:    factType,
		Value:       value,
		ContextInfo: contextInfo,
		Source:      source,
	})
	if m.storeWithSourceFunc != nil {
		return m.storeWithSourceFunc(ctx, projectID, factType, value, contextInfo, source)
	}
	return &models.KnowledgeFact{
		ID:        uuid.New(),
		ProjectID: projectID,
		FactType:  factType,
		Value:     value,
		Context:   contextInfo,
		Source:    source,
	}, nil
}

func (m *mockKnowledgeService) Store(ctx context.Context, projectID uuid.UUID, factType, value, contextInfo string) (*models.KnowledgeFact, error) {
	return nil, nil
}

func (m *mockKnowledgeService) Update(ctx context.Context, projectID, id uuid.UUID, factType, value, contextInfo string) (*models.KnowledgeFact, error) {
	return nil, nil
}

func (m *mockKnowledgeService) GetAll(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	return nil, nil
}

func (m *mockKnowledgeService) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	return nil, nil
}

func (m *mockKnowledgeService) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockKnowledgeService) DeleteAll(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func TestKnowledgeParsing_EmptyText(t *testing.T) {
	logger := zap.NewNop()
	factory := llm.NewMockClientFactory()
	ks := &mockKnowledgeService{}

	svc := NewKnowledgeParsingService(ks, factory, logger)

	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"tabs and newlines", "\t\n  \n\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := svc.ParseAndStore(context.Background(), uuid.New(), tt.input)
			require.Error(t, err)
			assert.Nil(t, facts)
			assert.Contains(t, err.Error(), "free-form text cannot be empty")
			// LLM should not have been called
			assert.Equal(t, int64(0), factory.MockClient.GenerateResponseCalls.Load())
		})
	}
}

func TestKnowledgeParsing_LLMFactoryError(t *testing.T) {
	logger := zap.NewNop()
	factory := llm.NewMockClientFactory()
	factory.CreateForProjectFunc = func(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
		return nil, fmt.Errorf("config not found")
	}
	ks := &mockKnowledgeService{}

	svc := NewKnowledgeParsingService(ks, factory, logger)

	facts, err := svc.ParseAndStore(context.Background(), uuid.New(), "some valid text")
	require.Error(t, err)
	assert.Nil(t, facts)
	assert.Contains(t, err.Error(), "create LLM client")
	assert.Contains(t, err.Error(), "config not found")
	assert.Empty(t, ks.storeCalls)
}

func TestKnowledgeParsing_LLMGenerateError(t *testing.T) {
	logger := zap.NewNop()
	factory := llm.NewMockClientFactory()
	factory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	ks := &mockKnowledgeService{}

	svc := NewKnowledgeParsingService(ks, factory, logger)

	facts, err := svc.ParseAndStore(context.Background(), uuid.New(), "some valid text")
	require.Error(t, err)
	assert.Nil(t, facts)
	assert.Contains(t, err.Error(), "LLM generate response")
	assert.Contains(t, err.Error(), "rate limit exceeded")
	assert.Empty(t, ks.storeCalls)
}

func TestKnowledgeParsing_SuccessfulParse(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()

	factory := llm.NewMockClientFactory()
	factory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{"facts": [{"fact_type": "business_rule", "value": "All timestamps are stored in UTC", "context": "database layer"}]}`,
		}, nil
	}

	ks := &mockKnowledgeService{}
	svc := NewKnowledgeParsingService(ks, factory, logger)

	facts, err := svc.ParseAndStore(context.Background(), projectID, "We store all timestamps in UTC")
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "business_rule", facts[0].FactType)
	assert.Equal(t, "All timestamps are stored in UTC", facts[0].Value)

	// Verify KnowledgeService.StoreWithSource was called correctly
	require.Len(t, ks.storeCalls, 1)
	assert.Equal(t, projectID, ks.storeCalls[0].ProjectID)
	assert.Equal(t, "business_rule", ks.storeCalls[0].FactType)
	assert.Equal(t, "All timestamps are stored in UTC", ks.storeCalls[0].Value)
	assert.Equal(t, "database layer", ks.storeCalls[0].ContextInfo)
	assert.Equal(t, "manual", ks.storeCalls[0].Source)
}

func TestKnowledgeParsing_MultipleFacts(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()

	factory := llm.NewMockClientFactory()
	factory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{"facts": [
				{"fact_type": "business_rule", "value": "Amounts in cents", "context": "payments"},
				{"fact_type": "terminology", "value": "A channel is a creator", "context": ""}
			]}`,
		}, nil
	}

	ks := &mockKnowledgeService{}
	svc := NewKnowledgeParsingService(ks, factory, logger)

	facts, err := svc.ParseAndStore(context.Background(), projectID, "amounts are in cents, channels mean creators")
	require.NoError(t, err)
	require.Len(t, facts, 2)
	require.Len(t, ks.storeCalls, 2)

	assert.Equal(t, "business_rule", ks.storeCalls[0].FactType)
	assert.Equal(t, "terminology", ks.storeCalls[1].FactType)
}

func TestKnowledgeParsing_MalformedLLMResponse(t *testing.T) {
	logger := zap.NewNop()

	factory := llm.NewMockClientFactory()
	factory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `not valid json at all`,
		}, nil
	}

	ks := &mockKnowledgeService{}
	svc := NewKnowledgeParsingService(ks, factory, logger)

	facts, err := svc.ParseAndStore(context.Background(), uuid.New(), "some text")
	require.Error(t, err)
	assert.Nil(t, facts)
	assert.Contains(t, err.Error(), "parse LLM response")
	assert.Empty(t, ks.storeCalls)
}

func TestKnowledgeParsing_ResponseWithMarkdownCodeBlock(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()

	factory := llm.NewMockClientFactory()
	factory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: "```json\n{\"facts\": [{\"fact_type\": \"convention\", \"value\": \"IDs use UUID v4\"}]}\n```",
		}, nil
	}

	ks := &mockKnowledgeService{}
	svc := NewKnowledgeParsingService(ks, factory, logger)

	facts, err := svc.ParseAndStore(context.Background(), projectID, "we use uuid v4 for IDs")
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "convention", facts[0].FactType)
}

func TestKnowledgeParsing_InvalidFactTypeSkipped(t *testing.T) {
	logger := zap.NewNop()

	factory := llm.NewMockClientFactory()
	factory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{"facts": [{"fact_type": "unknown_type", "value": "something"}]}`,
		}, nil
	}

	ks := &mockKnowledgeService{}
	svc := NewKnowledgeParsingService(ks, factory, logger)

	facts, err := svc.ParseAndStore(context.Background(), uuid.New(), "something")
	require.Error(t, err)
	assert.Nil(t, facts)
	assert.Contains(t, err.Error(), "no valid facts extracted")
	assert.Empty(t, ks.storeCalls)
}

func TestKnowledgeParsing_StoreError(t *testing.T) {
	logger := zap.NewNop()

	factory := llm.NewMockClientFactory()
	factory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{"facts": [{"fact_type": "business_rule", "value": "something important"}]}`,
		}, nil
	}

	ks := &mockKnowledgeService{
		storeWithSourceFunc: func(ctx context.Context, projectID uuid.UUID, factType, value, contextInfo, source string) (*models.KnowledgeFact, error) {
			return nil, fmt.Errorf("database connection lost")
		},
	}
	svc := NewKnowledgeParsingService(ks, factory, logger)

	facts, err := svc.ParseAndStore(context.Background(), uuid.New(), "something important")
	require.Error(t, err)
	assert.Nil(t, facts)
	assert.Contains(t, err.Error(), "store fact")
	assert.Contains(t, err.Error(), "database connection lost")
}
