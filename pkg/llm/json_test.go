package llm

import (
	"testing"
)

func TestExtractJSON_PlainObject(t *testing.T) {
	input := `{"name": "test", "value": 123}`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestExtractJSON_PlainArray(t *testing.T) {
	input := `[{"name": "test"}, {"name": "test2"}]`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestExtractJSON_NestedObject(t *testing.T) {
	input := `{"outer": {"inner": {"deep": "value"}}}`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestExtractJSON_NestedArraysAndObjects(t *testing.T) {
	input := `{"items": [{"nested": {"array": [1, 2, 3]}}]}`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestExtractJSON_WithThinkTags(t *testing.T) {
	input := `<think>
Let me analyze this request...
I should return a JSON object.
</think>
{"name": "test", "value": 123}`

	expected := `{"name": "test", "value": 123}`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExtractJSON_WithThinkTagsAndNestedJSON(t *testing.T) {
	input := `<think>
Processing the schema...
</think>
{"entities": {"users": {"columns": ["id", "name"]}}}`

	expected := `{"entities": {"users": {"columns": ["id", "name"]}}}`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExtractJSON_WithLeadingWhitespaceAndThinkTags(t *testing.T) {
	input := `
<think>Some thinking here</think>
  {"result": "success"}`

	expected := `{"result": "success"}`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExtractJSON_WithTextBeforeJSON(t *testing.T) {
	input := `Here is the JSON response:
{"name": "test"}`

	expected := `{"name": "test"}`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExtractJSON_WithTextAfterJSON(t *testing.T) {
	input := `{"name": "test"}
Let me know if you need anything else.`

	expected := `{"name": "test"}`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExtractJSON_BracketsInStrings(t *testing.T) {
	input := `{"message": "Use {braces} and [brackets] in text", "count": 1}`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestExtractJSON_EscapedQuotesInStrings(t *testing.T) {
	input := `{"message": "He said \"hello\"", "valid": true}`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestExtractJSON_ArrayBeforeObject(t *testing.T) {
	// When array appears before object in the text, extract the array
	input := `[{"item": 1}, {"item": 2}]`
	result, err := ExtractJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestExtractJSON_NoJSON(t *testing.T) {
	input := `This is just plain text with no JSON.`
	_, err := ExtractJSON(input)
	if err == nil {
		t.Error("expected error for input with no JSON")
	}
}

func TestExtractJSON_InvalidJSON(t *testing.T) {
	input := `{"unclosed": "object"`
	_, err := ExtractJSON(input)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestExtractJSON_EmptyInput(t *testing.T) {
	input := ``
	_, err := ExtractJSON(input)
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParseJSONResponse_Object(t *testing.T) {
	type testStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	input := `<think>thinking</think>{"name": "test", "value": 42}`
	result, err := ParseJSONResponse[testStruct](input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "test" {
		t.Errorf("expected name 'test', got %q", result.Name)
	}
	if result.Value != 42 {
		t.Errorf("expected value 42, got %d", result.Value)
	}
}

func TestParseJSONResponse_Array(t *testing.T) {
	type item struct {
		ID string `json:"id"`
	}

	input := `[{"id": "a"}, {"id": "b"}]`
	result, err := ParseJSONResponse[[]item](input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}
	if result[0].ID != "a" {
		t.Errorf("expected first id 'a', got %q", result[0].ID)
	}
}
