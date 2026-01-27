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
	ontologyID := uuid.New()

	result := ConvertQuestionInputs(nil, projectID, ontologyID, nil)
	assert.Nil(t, result)

	result = ConvertQuestionInputs([]OntologyQuestionInput{}, projectID, ontologyID, nil)
	assert.Nil(t, result)
}

func TestConvertQuestionInputs_ValidQuestions(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()
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

	result := ConvertQuestionInputs(inputs, projectID, ontologyID, &workflowID)

	require.Len(t, result, 3)

	// First question (priority 1 = critical = required)
	assert.Equal(t, projectID, result[0].ProjectID)
	assert.Equal(t, ontologyID, result[0].OntologyID)
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
	ontologyID := uuid.New()

	inputs := []OntologyQuestionInput{
		{Category: "terminology", Priority: 1, Question: "Valid question?", Context: "ctx"},
		{Category: "terminology", Priority: 1, Question: "", Context: "empty question"},
		{Category: "terminology", Priority: 1, Question: "Another valid?", Context: "ctx2"},
	}

	result := ConvertQuestionInputs(inputs, projectID, ontologyID, nil)

	require.Len(t, result, 2)
	assert.Equal(t, "Valid question?", result[0].Text)
	assert.Equal(t, "Another valid?", result[1].Text)
}

func TestConvertQuestionInputs_PriorityClamping(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()

	inputs := []OntologyQuestionInput{
		{Category: "test", Priority: 0, Question: "Zero priority?", Context: ""},
		{Category: "test", Priority: -1, Question: "Negative priority?", Context: ""},
		{Category: "test", Priority: 10, Question: "High priority?", Context: ""},
	}

	result := ConvertQuestionInputs(inputs, projectID, ontologyID, nil)

	require.Len(t, result, 3)
	// Zero and negative priority should default to 3
	assert.Equal(t, 3, result[0].Priority)
	assert.Equal(t, 3, result[1].Priority)
	// Priority > 5 should be clamped to 5
	assert.Equal(t, 5, result[2].Priority)
}

func TestConvertQuestionInputs_WorkflowIDOptional(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()

	inputs := []OntologyQuestionInput{
		{Category: "test", Priority: 1, Question: "Test?", Context: ""},
	}

	// Without workflow ID
	result := ConvertQuestionInputs(inputs, projectID, ontologyID, nil)
	require.Len(t, result, 1)
	assert.Nil(t, result[0].WorkflowID)

	// With workflow ID
	workflowID := uuid.New()
	result = ConvertQuestionInputs(inputs, projectID, ontologyID, &workflowID)
	require.Len(t, result, 1)
	assert.Equal(t, &workflowID, result[0].WorkflowID)
}
