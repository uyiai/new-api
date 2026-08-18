package claude

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	messageStartChunk = `{"type":"message_start","message":{"id":"gen-1786973219-3svJTSeqkLOzG4Y9KYUn","type":"message","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":12,"output_tokens":0}}}`
	upstreamErrChunk  = `{"type":"error","error":{"type":"invalid_request_error","message":"Provider returned no content"}}`
)

func newClaudeStreamTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	return c, recorder
}

func newClaudeStreamTestInfo() (*relaycommon.RelayInfo, *ClaudeResponseInfo) {
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		IsStream:    true,
		// UpstreamModelName is reached through the embedded *ChannelMeta pointer, which
		// InitChannelMeta populates in production.
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	claudeInfo := &ClaudeResponseInfo{
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	return info, claudeInfo
}

// An upstream error arriving after bytes have already been flushed must be forwarded verbatim as
// an `event: error` frame, and must suppress retry: failing over to another channel at this point
// would splice a second message_start into the same connection.
func TestHandleStreamResponseData_ForwardsErrorAfterStreamStarted(t *testing.T) {
	c, recorder := newClaudeStreamTestContext()
	info, claudeInfo := newClaudeStreamTestInfo()

	require.Nil(t, HandleStreamResponseData(c, info, claudeInfo, messageStartChunk))
	require.True(t, c.Writer.Written(), "message_start should have been flushed to the client")

	apiErr := HandleStreamResponseData(c, info, claudeInfo, upstreamErrChunk)
	require.NotNil(t, apiErr)

	body := recorder.Body.String()
	assert.Contains(t, body, "event: message_start")
	assert.Contains(t, body, "event: error")
	// The upstream payload must survive byte-for-byte, not be re-serialised.
	assert.Contains(t, body, upstreamErrChunk)
	assert.Contains(t, body, "Provider returned no content")

	assert.True(t, types.IsSkipRetryError(apiErr), "retry must be suppressed once the stream has started")
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyStreamErrorForwarded),
		"the forwarded marker must be set so the relay handler does not emit a duplicate error")
}

// An upstream error arriving before any byte reaches the client leaves the stream clean, so the
// original fail-over behaviour must be preserved. Without this case an unconditional skip-retry
// would still pass the suite while silently killing cross-channel retry.
func TestHandleStreamResponseData_ErrorAsFirstChunkStillRetries(t *testing.T) {
	c, recorder := newClaudeStreamTestContext()
	info, claudeInfo := newClaudeStreamTestInfo()

	apiErr := HandleStreamResponseData(c, info, claudeInfo, upstreamErrChunk)
	require.NotNil(t, apiErr)

	assert.Empty(t, recorder.Body.String(), "nothing should be forwarded before the stream starts")
	assert.False(t, types.IsSkipRetryError(apiErr), "a pre-first-byte error must remain retryable")
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyStreamErrorForwarded))
}

// The OpenAI-format branch converts chunks rather than passing them through, so a raw Claude error
// payload must never be written into an OpenAI stream.
func TestHandleStreamResponseData_DoesNotForwardRawClaudeErrorToOpenAIFormat(t *testing.T) {
	c, recorder := newClaudeStreamTestContext()
	info, claudeInfo := newClaudeStreamTestInfo()
	info.RelayFormat = types.RelayFormatOpenAI

	require.Nil(t, HandleStreamResponseData(c, info, claudeInfo, messageStartChunk))
	require.True(t, c.Writer.Written())

	apiErr := HandleStreamResponseData(c, info, claudeInfo, upstreamErrChunk)
	require.NotNil(t, apiErr)

	assert.NotContains(t, recorder.Body.String(), "event: error")
	assert.NotContains(t, recorder.Body.String(), "invalid_request_error")
	// Retry is still suppressed: the connection is dirty regardless of the wire format.
	assert.True(t, types.IsSkipRetryError(apiErr))
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyStreamErrorForwarded))
}

// Normal chunks must be unaffected by the error branch.
func TestHandleStreamResponseData_PassesThroughNormalChunks(t *testing.T) {
	c, recorder := newClaudeStreamTestContext()
	info, claudeInfo := newClaudeStreamTestInfo()

	const deltaChunk = `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`
	require.Nil(t, HandleStreamResponseData(c, info, claudeInfo, messageStartChunk))
	require.Nil(t, HandleStreamResponseData(c, info, claudeInfo, deltaChunk))

	body := recorder.Body.String()
	assert.Contains(t, body, "event: content_block_delta")
	assert.NotContains(t, body, "event: error")
	assert.Equal(t, "hello", claudeInfo.ResponseText.String())
}
