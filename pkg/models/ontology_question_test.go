package models

import "testing"

func TestOntologyQuestion_ComputeContentHash(t *testing.T) {
	tests := []struct {
		name     string
		category string
		text     string
	}{
		{
			name:     "terminology question",
			category: QuestionCategoryTerminology,
			text:     "What does 'tik' mean in tiks_count?",
		},
		{
			name:     "enumeration question",
			category: QuestionCategoryEnumeration,
			text:     "What do status values 'A', 'P', 'C' represent?",
		},
		{
			name:     "empty category",
			category: "",
			text:     "Some question without a category",
		},
		{
			name:     "empty text",
			category: QuestionCategoryDataQuality,
			text:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &OntologyQuestion{
				Category: tt.category,
				Text:     tt.text,
			}

			hash := q.ComputeContentHash()

			// Hash should be exactly 16 characters (first 16 hex chars of SHA256)
			if len(hash) != 16 {
				t.Errorf("ComputeContentHash() returned hash of length %d, want 16", len(hash))
			}

			// Hash should be deterministic
			hash2 := q.ComputeContentHash()
			if hash != hash2 {
				t.Error("ComputeContentHash() is not deterministic")
			}

			// Hash should be valid hex
			for _, c := range hash {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("ComputeContentHash() returned invalid hex character: %c", c)
				}
			}
		})
	}
}

func TestOntologyQuestion_ComputeContentHash_Uniqueness(t *testing.T) {
	// Same text, different category should produce different hash
	q1 := &OntologyQuestion{
		Category: QuestionCategoryTerminology,
		Text:     "What is this?",
	}
	q2 := &OntologyQuestion{
		Category: QuestionCategoryEnumeration,
		Text:     "What is this?",
	}

	if q1.ComputeContentHash() == q2.ComputeContentHash() {
		t.Error("Different categories with same text should produce different hashes")
	}

	// Same category, different text should produce different hash
	q3 := &OntologyQuestion{
		Category: QuestionCategoryTerminology,
		Text:     "Question A",
	}
	q4 := &OntologyQuestion{
		Category: QuestionCategoryTerminology,
		Text:     "Question B",
	}

	if q3.ComputeContentHash() == q4.ComputeContentHash() {
		t.Error("Different text with same category should produce different hashes")
	}

	// Identical questions should produce identical hash
	q5 := &OntologyQuestion{
		Category: QuestionCategoryRelationship,
		Text:     "Is this a self-reference?",
	}
	q6 := &OntologyQuestion{
		Category: QuestionCategoryRelationship,
		Text:     "Is this a self-reference?",
	}

	if q5.ComputeContentHash() != q6.ComputeContentHash() {
		t.Error("Identical category and text should produce identical hashes")
	}
}
