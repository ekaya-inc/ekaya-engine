package tunnel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMessage_Register(t *testing.T) {
	data := []byte(`{"type":"register","project_id":"abc-123"}`)
	msg, err := ParseMessage(data)
	require.NoError(t, err)

	reg, ok := msg.(*RegisterMessage)
	require.True(t, ok)
	assert.Equal(t, TypeRegister, reg.Type)
	assert.Equal(t, "abc-123", reg.ProjectID)
}

func TestParseMessage_Registered(t *testing.T) {
	data := []byte(`{"type":"registered","project_id":"abc-123","public_url":"https://mcp.ekaya.ai/mcp/abc-123"}`)
	msg, err := ParseMessage(data)
	require.NoError(t, err)

	reg, ok := msg.(*RegisteredMessage)
	require.True(t, ok)
	assert.Equal(t, TypeRegistered, reg.Type)
	assert.Equal(t, "abc-123", reg.ProjectID)
	assert.Equal(t, "https://mcp.ekaya.ai/mcp/abc-123", reg.PublicURL)
}

func TestParseMessage_Request(t *testing.T) {
	data := []byte(`{
		"type":"request",
		"id":"req-1",
		"method":"POST",
		"headers":{"content-type":"application/json","authorization":"Bearer tok"},
		"body":"eyJmb28iOiJiYXIifQ=="
	}`)
	msg, err := ParseMessage(data)
	require.NoError(t, err)

	req, ok := msg.(*RequestMessage)
	require.True(t, ok)
	assert.Equal(t, TypeRequest, req.Type)
	assert.Equal(t, "req-1", req.ID)
	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "application/json", req.Headers["content-type"])
	assert.Equal(t, "Bearer tok", req.Headers["authorization"])
	assert.Equal(t, "eyJmb28iOiJiYXIifQ==", req.Body)
}

func TestParseMessage_ResponseStart(t *testing.T) {
	data := []byte(`{"type":"response_start","id":"req-1","status":200,"headers":{"content-type":"text/event-stream"}}`)
	msg, err := ParseMessage(data)
	require.NoError(t, err)

	resp, ok := msg.(*ResponseStartMessage)
	require.True(t, ok)
	assert.Equal(t, TypeResponseStart, resp.Type)
	assert.Equal(t, "req-1", resp.ID)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, "text/event-stream", resp.Headers["content-type"])
}

func TestParseMessage_ResponseChunk(t *testing.T) {
	data := []byte(`{"type":"response_chunk","id":"req-1","data":"aGVsbG8="}`)
	msg, err := ParseMessage(data)
	require.NoError(t, err)

	chunk, ok := msg.(*ResponseChunkMessage)
	require.True(t, ok)
	assert.Equal(t, TypeResponseChunk, chunk.Type)
	assert.Equal(t, "req-1", chunk.ID)
	assert.Equal(t, "aGVsbG8=", chunk.Data)
}

func TestParseMessage_ResponseEnd(t *testing.T) {
	data := []byte(`{"type":"response_end","id":"req-1"}`)
	msg, err := ParseMessage(data)
	require.NoError(t, err)

	end, ok := msg.(*ResponseEndMessage)
	require.True(t, ok)
	assert.Equal(t, TypeResponseEnd, end.Type)
	assert.Equal(t, "req-1", end.ID)
}

func TestParseMessage_Error(t *testing.T) {
	data := []byte(`{"type":"error","id":"req-1","message":"request timed out"}`)
	msg, err := ParseMessage(data)
	require.NoError(t, err)

	errMsg, ok := msg.(*ErrorMessage)
	require.True(t, ok)
	assert.Equal(t, TypeError, errMsg.Type)
	assert.Equal(t, "req-1", errMsg.ID)
	assert.Equal(t, "request timed out", errMsg.Message)
}

func TestParseMessage_UnknownType(t *testing.T) {
	data := []byte(`{"type":"unknown"}`)
	_, err := ParseMessage(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown message type")
}

func TestParseMessage_InvalidJSON(t *testing.T) {
	data := []byte(`not json`)
	_, err := ParseMessage(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse message envelope")
}

func TestParseMessage_EmptyType(t *testing.T) {
	data := []byte(`{"type":""}`)
	_, err := ParseMessage(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown message type")
}

func TestMessageRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  any
	}{
		{
			name: "register",
			msg:  &RegisterMessage{Type: TypeRegister, ProjectID: "pid-1"},
		},
		{
			name: "registered",
			msg:  &RegisteredMessage{Type: TypeRegistered, ProjectID: "pid-1", PublicURL: "https://mcp.ekaya.ai/mcp/pid-1"},
		},
		{
			name: "request",
			msg: &RequestMessage{
				Type:    TypeRequest,
				ID:      "req-1",
				Method:  "POST",
				Headers: map[string]string{"content-type": "application/json"},
				Body:    "dGVzdA==",
			},
		},
		{
			name: "response_start",
			msg: &ResponseStartMessage{
				Type:    TypeResponseStart,
				ID:      "req-1",
				Status:  200,
				Headers: map[string]string{"content-type": "text/event-stream"},
			},
		},
		{
			name: "response_chunk",
			msg:  &ResponseChunkMessage{Type: TypeResponseChunk, ID: "req-1", Data: "Y2h1bms="},
		},
		{
			name: "response_end",
			msg:  &ResponseEndMessage{Type: TypeResponseEnd, ID: "req-1"},
		},
		{
			name: "error",
			msg:  &ErrorMessage{Type: TypeError, ID: "req-1", Message: "timeout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.msg)
			require.NoError(t, err)

			parsed, err := ParseMessage(data)
			require.NoError(t, err)
			assert.Equal(t, tt.msg, parsed)
		})
	}
}
