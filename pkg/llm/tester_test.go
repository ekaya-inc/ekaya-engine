package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCandidateBaseURLs_AppendsCommonPathsForRootURL(t *testing.T) {
	candidates := candidateBaseURLs("http://localhost:30000")

	assert.Equal(t, []string{
		"http://localhost:30000",
		"http://localhost:30000/v1",
		"http://localhost:30000/api/v1",
		"http://localhost:30000/openai/v1",
		"http://localhost:30000/api/openai/v1",
		"http://localhost:30000/v1beta/openai",
	}, candidates)
}

func TestConnectionTester_Test_ResolvesLLMBaseURLFromRootURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			http.Error(w, "not found", http.StatusNotFound)
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-1",
				"object":  "chat.completion",
				"created": 1,
				"model":   "ekaya-community",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "ok",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     1,
					"completion_tokens": 1,
					"total_tokens":      2,
				},
			}))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	tester := NewConnectionTester()
	result := tester.Test(context.Background(), &TestConfig{
		LLMBaseURL: server.URL,
		LLMModel:   "ekaya-community",
	})

	require.True(t, result.Success)
	assert.True(t, result.LLMSuccess)
	assert.Equal(t, server.URL+"/v1", result.ResolvedLLMBaseURL)
}

func TestConnectionTester_Test_UsesResolvedLLMBaseURLForEmbeddingFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions", "/embeddings":
			http.Error(w, "not found", http.StatusNotFound)
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-1",
				"object":  "chat.completion",
				"created": 1,
				"model":   "ekaya-community",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "ok",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     1,
					"completion_tokens": 1,
					"total_tokens":      2,
				},
			}))
		case "/v1/embeddings":
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"data": []map[string]any{
					{
						"object":    "embedding",
						"embedding": []float32{0.1, 0.2},
						"index":     0,
					},
				},
				"model": "text-embedding-3-small",
				"usage": map[string]any{
					"prompt_tokens": 1,
					"total_tokens":  1,
				},
			}))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	tester := NewConnectionTester()
	result := tester.Test(context.Background(), &TestConfig{
		LLMBaseURL:     server.URL,
		LLMModel:       "ekaya-community",
		EmbeddingModel: "text-embedding-3-small",
	})

	require.True(t, result.Success)
	assert.True(t, result.EmbeddingSuccess)
	assert.Equal(t, server.URL+"/v1", result.ResolvedLLMBaseURL)
	assert.Equal(t, server.URL+"/v1", result.ResolvedEmbeddingBaseURL)
}
