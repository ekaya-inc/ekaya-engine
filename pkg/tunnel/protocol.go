package tunnel

import (
	"encoding/json"
	"fmt"
)

// Message type constants for the tunnel WebSocket protocol.
const (
	TypeRegister      = "register"
	TypeRegistered    = "registered"
	TypeRequest       = "request"
	TypeResponseStart = "response_start"
	TypeResponseChunk = "response_chunk"
	TypeResponseEnd   = "response_end"
	TypeError         = "error"
)

// Envelope is used for initial JSON decoding to determine the message type.
type Envelope struct {
	Type string `json:"type"`
}

// RegisterMessage is sent by the engine to register a tunnel connection.
type RegisterMessage struct {
	Type      string `json:"type"`
	ProjectID string `json:"project_id"`
}

// RegisteredMessage is sent by the tunnel server to confirm registration.
type RegisteredMessage struct {
	Type      string `json:"type"`
	ProjectID string `json:"project_id"`
	PublicURL string `json:"public_url"`
}

// RequestMessage is sent by the tunnel server to relay an MCP client request.
type RequestMessage struct {
	Type    string            `json:"type"`
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"` // base64-encoded
}

// ResponseStartMessage is sent by the engine to begin a response.
type ResponseStartMessage struct {
	Type    string            `json:"type"`
	ID      string            `json:"id"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
}

// ResponseChunkMessage is sent by the engine for streaming response data.
type ResponseChunkMessage struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Data string `json:"data"` // base64-encoded
}

// ResponseEndMessage is sent by the engine to signal the response is complete.
type ResponseEndMessage struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// ErrorMessage is sent by either side to signal an error for a specific request.
type ErrorMessage struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message"`
}

// ParseMessage decodes a JSON message and returns the typed message.
func ParseMessage(data []byte) (any, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("failed to parse message envelope: %w", err)
	}

	switch env.Type {
	case TypeRegister:
		var msg RegisterMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse register message: %w", err)
		}
		return &msg, nil

	case TypeRegistered:
		var msg RegisteredMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse registered message: %w", err)
		}
		return &msg, nil

	case TypeRequest:
		var msg RequestMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse request message: %w", err)
		}
		return &msg, nil

	case TypeResponseStart:
		var msg ResponseStartMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse response_start message: %w", err)
		}
		return &msg, nil

	case TypeResponseChunk:
		var msg ResponseChunkMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse response_chunk message: %w", err)
		}
		return &msg, nil

	case TypeResponseEnd:
		var msg ResponseEndMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse response_end message: %w", err)
		}
		return &msg, nil

	case TypeError:
		var msg ErrorMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse error message: %w", err)
		}
		return &msg, nil

	default:
		return nil, fmt.Errorf("unknown message type: %q", env.Type)
	}
}
