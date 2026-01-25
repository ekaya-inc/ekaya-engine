package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
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
