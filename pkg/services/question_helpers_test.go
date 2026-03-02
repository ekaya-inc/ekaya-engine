package services

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestConvertQuestionInputs_EmptyInput(t *testing.T) {
	projectID := uuid.New()

	result := ConvertQuestionInputs(nil, projectID, nil)
	assert.Nil(t, result)

	result = ConvertQuestionInputs([]OntologyQuestionInput{}, projectID, nil)
	assert.Nil(t, result)
}

func TestConvertQuestionInputs_ValidQuestions(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()

	inputs := []OntologyQuestionInput{
		{
			Category: "terminology",
			Priority: 1,
			Question: "What does 'tik' mean in tiks_count?",
			Context:  "Column tiks_count appears to track some count.",
		},
		{
			Category: "enumeration",
			Priority: 2,
			Question: "What do status values 'A', 'P', 'C' represent?",
			Context:  "Column status has values A, P, C.",
		},
		{
			Category: "data_quality",
			Priority: 3,
			Question: "Column phone has 80% NULL - is this expected?",
			Context:  "Low cardinality in phone column.",
		},
	}

	result := ConvertQuestionInputs(inputs, projectID, &workflowID)

	require.Len(t, result, 3)

	// First question (priority 1 = critical = required)
	assert.Equal(t, projectID, result[0].ProjectID)
	assert.Equal(t, &workflowID, result[0].WorkflowID)
	assert.Equal(t, "What does 'tik' mean in tiks_count?", result[0].Text)
	assert.Equal(t, 1, result[0].Priority)
	assert.True(t, result[0].IsRequired)
	assert.Equal(t, "terminology", result[0].Category)
	assert.Equal(t, "Column tiks_count appears to track some count.", result[0].Reasoning)
	assert.Equal(t, models.QuestionStatusPending, result[0].Status)

	// Second question (priority 2 = important = not required)
	assert.Equal(t, 2, result[1].Priority)
	assert.False(t, result[1].IsRequired)

	// Third question (priority 3 = nice-to-have = not required)
	assert.Equal(t, 3, result[2].Priority)
	assert.False(t, result[2].IsRequired)
}

func TestConvertQuestionInputs_SkipsEmptyQuestions(t *testing.T) {
	projectID := uuid.New()

	inputs := []OntologyQuestionInput{
		{Category: "terminology", Priority: 1, Question: "Valid question?", Context: "ctx"},
		{Category: "terminology", Priority: 1, Question: "", Context: "empty question"},
		{Category: "terminology", Priority: 1, Question: "Another valid?", Context: "ctx2"},
	}

	result := ConvertQuestionInputs(inputs, projectID, nil)

	require.Len(t, result, 2)
	assert.Equal(t, "Valid question?", result[0].Text)
	assert.Equal(t, "Another valid?", result[1].Text)
}

func TestConvertQuestionInputs_PriorityClamping(t *testing.T) {
	projectID := uuid.New()

	inputs := []OntologyQuestionInput{
		{Category: "test", Priority: 0, Question: "Zero priority?", Context: ""},
		{Category: "test", Priority: -1, Question: "Negative priority?", Context: ""},
		{Category: "test", Priority: 10, Question: "High priority?", Context: ""},
	}

	result := ConvertQuestionInputs(inputs, projectID, nil)

	require.Len(t, result, 3)
	// Zero and negative priority should default to 3
	assert.Equal(t, 3, result[0].Priority)
	assert.Equal(t, 3, result[1].Priority)
	// Priority > 5 should be clamped to 5
	assert.Equal(t, 5, result[2].Priority)
}

func TestConvertQuestionInputs_AffectsPopulated(t *testing.T) {
	projectID := uuid.New()

	inputs := []OntologyQuestionInput{
		{
			Category: "enumeration",
			Priority: 1,
			Question: "What do offer_type values 1, 2, 3 represent?",
			Context:  "Column: offers.offer_type",
			Tables:   []string{"offers"},
			Columns:  []string{"offers.offer_type"},
		},
		{
			Category: "business_rules",
			Priority: 2,
			Question: "When is a user marked as is_available?",
			Context:  "Column: users.is_available",
			Tables:   []string{"users"},
		},
		{
			Category: "terminology",
			Priority: 3,
			Question: "No table context question?",
			Context:  "Some context",
			// No Tables or Columns - Affects should be nil
		},
	}

	result := ConvertQuestionInputs(inputs, projectID, nil)
	require.Len(t, result, 3)

	// First: both tables and columns set
	require.NotNil(t, result[0].Affects)
	assert.Equal(t, []string{"offers"}, result[0].Affects.Tables)
	assert.Equal(t, []string{"offers.offer_type"}, result[0].Affects.Columns)

	// Second: only tables set
	require.NotNil(t, result[1].Affects)
	assert.Equal(t, []string{"users"}, result[1].Affects.Tables)
	assert.Nil(t, result[1].Affects.Columns)

	// Third: no tables or columns - Affects should be nil
	assert.Nil(t, result[2].Affects)
}

func TestConvertQuestionInputs_WorkflowIDOptional(t *testing.T) {
	projectID := uuid.New()

	inputs := []OntologyQuestionInput{
		{Category: "test", Priority: 1, Question: "Test?", Context: ""},
	}

	// Without workflow ID
	result := ConvertQuestionInputs(inputs, projectID, nil)
	require.Len(t, result, 1)
	assert.Nil(t, result[0].WorkflowID)

	// With workflow ID
	workflowID := uuid.New()
	result = ConvertQuestionInputs(inputs, projectID, &workflowID)
	require.Len(t, result, 1)
	assert.Equal(t, &workflowID, result[0].WorkflowID)
}
