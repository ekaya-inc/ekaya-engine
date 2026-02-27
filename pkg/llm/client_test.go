package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestContextAwareTransport_InjectsRequestID(t *testing.T) {
	conversationID := uuid.New()
	var receivedHeader string

	// Create a test server that captures the X-Request-Id header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get(requestIDHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create transport with test server's transport
	transport := &contextAwareTransport{base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	// Create request with conversation ID in context
	ctx := WithConversationID(context.Background(), conversationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify header was set
	if receivedHeader != conversationID.String() {
		t.Errorf("expected X-Request-Id header %s, got %s", conversationID, receivedHeader)
	}
}

func TestContextAwareTransport_NoHeaderWhenNoConversationID(t *testing.T) {
	var receivedHeader string
	var headerPresent bool

	// Create a test server that checks for the X-Request-Id header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get(requestIDHeader)
		_, headerPresent = r.Header[requestIDHeader]
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create transport
	transport := &contextAwareTransport{base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	// Create request WITHOUT conversation ID in context
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify header was NOT set
	if headerPresent {
		t.Errorf("expected X-Request-Id header to be absent, got %s", receivedHeader)
	}
}

func TestSupportsChatTemplateKwargs(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"ekaya-community", true},
		{"ekaya-security", true},
		{"qwen3-32b", true},
		{"Qwen3-14B", true},
		{"qwen2.5-coder-7b", true},
		{"nemotron-3-nano-30b", true},
		{"Nemotron-3-Nano-30B-A3B", true},
		{"gpt-4o", false},
		{"gpt-4o-mini", false},
		{"o3-mini", false},
		{"claude-sonnet-4-20250514", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			c := &Client{model: tt.model}
			assert.Equal(t, tt.want, c.supportsChatTemplateKwargs())
		})
	}
}
