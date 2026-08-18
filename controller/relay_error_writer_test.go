package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRelayErrorTestContext(isStream bool) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	common.SetContextKey(c, constant.ContextKeyIsStream, isStream)
	return c, recorder
}

func testRelayError() *types.NewAPIError {
	return types.WithClaudeError(types.ClaudeError{
		Type:    "invalid_request_error",
		Message: "Provider returned no content",
	}, http.StatusInternalServerError)
}

// A non-stream request must keep the original JSON error body, even when a handler already wrote
// part of a response before failing. This is the regression guard for the guard condition itself:
// keying only on c.Writer.Written() would silently convert non-stream errors to SSE frames.
func TestWriteRelayError_NonStreamKeepsJSONBody(t *testing.T) {
	c, recorder := newRelayErrorTestContext(false)
	// Simulate a handler that already committed a response body before erroring.
	_, _ = c.Writer.Write([]byte(`{"partial":true}`))
	require.True(t, c.Writer.Written())

	writeRelayError(c, types.RelayFormatClaude, testRelayError(), nil)

	body := recorder.Body.String()
	assert.NotContains(t, body, "event: error", "non-stream errors must not be SSE-framed")
	assert.Contains(t, body, `"type":"error"`)
	assert.Contains(t, body, "Provider returned no content")
}

// A non-stream request that wrote nothing keeps the plain JSON body and its status code.
func TestWriteRelayError_NonStreamUntouchedResponse(t *testing.T) {
	c, recorder := newRelayErrorTestContext(false)

	writeRelayError(c, types.RelayFormatClaude, testRelayError(), nil)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "event: error")
	assert.Contains(t, recorder.Body.String(), "Provider returned no content")
}

// A stream that already emitted bytes must receive an SSE-framed error instead of an unframed
// JSON blob appended to the stream.
func TestWriteRelayError_StartedClaudeStreamGetsSSEFrame(t *testing.T) {
	c, recorder := newRelayErrorTestContext(true)
	_, _ = c.Writer.Write([]byte("event: message_start\ndata: {}\n\n"))
	require.True(t, c.Writer.Written())

	writeRelayError(c, types.RelayFormatClaude, testRelayError(), nil)

	body := recorder.Body.String()
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, `"type":"error"`)
	assert.Contains(t, body, "Provider returned no content")
}

// The channel layer already forwarded the upstream error verbatim, so no duplicate may be sent.
func TestWriteRelayError_SkipsWhenAlreadyForwarded(t *testing.T) {
	c, recorder := newRelayErrorTestContext(true)
	_, _ = c.Writer.Write([]byte("event: message_start\ndata: {}\n\n"))
	common.SetContextKey(c, constant.ContextKeyStreamErrorForwarded, true)

	writeRelayError(c, types.RelayFormatClaude, testRelayError(), nil)

	// Only the pre-written message_start remains; no second error event was appended.
	assert.Equal(t, "event: message_start\ndata: {}\n\n", recorder.Body.String())
}

// A stream request that failed before writing any byte still gets the plain JSON error body,
// so clients see a real HTTP status rather than a 200 SSE stream carrying an error.
func TestWriteRelayError_StreamBeforeFirstByteKeepsJSONBody(t *testing.T) {
	c, recorder := newRelayErrorTestContext(true)
	require.False(t, c.Writer.Written())

	writeRelayError(c, types.RelayFormatClaude, testRelayError(), nil)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "event: error")
}

// Non-Claude formats get the OpenAI-shaped SSE error chunk, and no [DONE] that could be read as
// a successful completion.
func TestWriteRelayError_StartedOpenAIStreamGetsErrorChunk(t *testing.T) {
	c, recorder := newRelayErrorTestContext(true)
	_, _ = c.Writer.Write([]byte("data: {}\n\n"))

	apiErr := types.NewOpenAIError(errors.New("upstream exploded"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	writeRelayError(c, types.RelayFormatOpenAI, apiErr, nil)

	body := recorder.Body.String()
	assert.Contains(t, body, `"error"`)
	assert.Contains(t, body, "upstream exploded")
	assert.NotContains(t, body, "[DONE]")
}
