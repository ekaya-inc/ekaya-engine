package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/google/uuid"
)

func TestExtractColumnNames(t *testing.T) {
	tests := []struct {
		name     string
		question string
		want     []string
	}{
		{
			name:     "single quoted column",
			question: "What does 'marker_at' represent?",
			want:     []string{"marker_at"},
		},
		{
			name:     "double quoted column",
			question: `What is "status" used for?`,
			want:     []string{"status"},
		},
		{
			name:     "backtick quoted column",
			question: "What does `user_id` reference?",
			want:     []string{"user_id"},
		},
		{
			name:     "multiple columns",
			question: "What is the relationship between 'owner_id' and 'entity_id'?",
			want:     []string{"owner_id", "entity_id"},
		},
		{
			name:     "no columns",
			question: "What does this table represent?",
			want:     []string{},
		},
		{
			name:     "mixed case preserved as lowercase",
			question: "What is 'created_at' for?",
			want:     []string{"created_at"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractColumnNames(tt.question)
			if len(got) != len(tt.want) {
				t.Errorf("extractColumnNames() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractColumnNames()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExtractQuestionIntent(t *testing.T) {
	// Note: extractQuestionIntent expects lowercase input (called after strings.ToLower in isSimilarQuestion)
	tests := []struct {
		name     string
		question string
		want     string
	}{
		{
			name:     "what is",
			question: "what is 'status' used for?",
			want:     "what",
		},
		{
			name:     "what does",
			question: "what does 'marker_at' represent?",
			want:     "what",
		},
		{
			name:     "what are",
			question: "what are the possible values for 'type'?",
			want:     "what",
		},
		{
			name:     "why",
			question: "why is 'deleted_at' nullable?",
			want:     "why",
		},
		{
			name:     "how",
			question: "how is 'score' calculated?",
			want:     "how",
		},
		{
			name:     "when",
			question: "when is 'processed_at' updated?",
			want:     "when",
		},
		{
			name:     "does",
			question: "does 'user_id' reference the users table?",
			want:     "does",
		},
		{
			name:     "is question",
			question: "is 'amount' gross or net?",
			want:     "does",
		},
		{
			name:     "no clear intent",
			question: "the 'status' column seems important.",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractQuestionIntent(tt.question)
			if got != tt.want {
				t.Errorf("extractQuestionIntent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSimilarQuestion(t *testing.T) {
	tests := []struct {
		name string
		q1   string
		q2   string
		want bool
	}{
		{
			name: "same column same intent",
			q1:   "What is 'marker_at' used for?",
			q2:   "What does 'marker_at' represent?",
			want: true,
		},
		{
			name: "same column different intent",
			q1:   "What is 'status' used for?",
			q2:   "Why is 'status' nullable?",
			want: false,
		},
		{
			name: "different columns same intent",
			q1:   "What is 'created_at' for?",
			q2:   "What is 'updated_at' for?",
			want: false,
		},
		{
			name: "no column names",
			q1:   "What does this table represent?",
			q2:   "What is the purpose of this entity?",
			want: false,
		},
		{
			name: "one has column one doesnt",
			q1:   "What is 'status' for?",
			q2:   "What does this table represent?",
			want: false,
		},
		{
			name: "different quote styles same column",
			q1:   `What is "marker_at" used for?`,
			q2:   "What does 'marker_at' represent?",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSimilarQuestion(tt.q1, tt.q2)
			if got != tt.want {
				t.Errorf("isSimilarQuestion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeduplicateQuestions(t *testing.T) {
	makeQuestion := func(text string) *models.OntologyQuestion {
		return &models.OntologyQuestion{
			ID:   uuid.New(),
			Text: text,
		}
	}

	tests := []struct {
		name          string
		newQuestions  []string
		existingTexts []string
		wantCount     int
		wantTexts     []string
	}{
		{
			name:          "no existing questions",
			newQuestions:  []string{"What is 'status'?", "What is 'type'?"},
			existingTexts: []string{},
			wantCount:     2,
			wantTexts:     []string{"What is 'status'?", "What is 'type'?"},
		},
		{
			name:          "one duplicate",
			newQuestions:  []string{"What is 'marker_at'?", "What is 'type'?"},
			existingTexts: []string{"What does 'marker_at' represent?"},
			wantCount:     1,
			wantTexts:     []string{"What is 'type'?"},
		},
		{
			name:          "all duplicates",
			newQuestions:  []string{"What is 'marker_at'?"},
			existingTexts: []string{"What does 'marker_at' represent?"},
			wantCount:     0,
			wantTexts:     []string{},
		},
		{
			name:          "no duplicates",
			newQuestions:  []string{"What is 'status'?"},
			existingTexts: []string{"What is 'type'?"},
			wantCount:     1,
			wantTexts:     []string{"What is 'status'?"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newQs := make([]*models.OntologyQuestion, len(tt.newQuestions))
			for i, text := range tt.newQuestions {
				newQs[i] = makeQuestion(text)
			}

			existingQs := make([]*models.OntologyQuestion, len(tt.existingTexts))
			for i, text := range tt.existingTexts {
				existingQs[i] = makeQuestion(text)
			}

			got := deduplicateQuestions(newQs, existingQs)

			if len(got) != tt.wantCount {
				t.Errorf("deduplicateQuestions() returned %d questions, want %d", len(got), tt.wantCount)
				return
			}

			for i, q := range got {
				if q.Text != tt.wantTexts[i] {
					t.Errorf("deduplicateQuestions()[%d].Text = %v, want %v", i, q.Text, tt.wantTexts[i])
				}
			}
		})
	}
}
