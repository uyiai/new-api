package claude

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCloudflareGatewayUsesOnlyExistingClaudeAPIKeyHeader(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")

	headers := make(http.Header)
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-sonnet-4-6",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://gateway.ai.cloudflare.com/v1/account/default/anthropic",
			ApiKey:         "cfut_header_test",
		},
	}

	require.NoError(t, (&Adaptor{}).SetupRequestHeader(ctx, &headers, info))
	require.Equal(t, "cfut_header_test", headers.Get("x-api-key"))
	require.Empty(t, headers.Get("cf-aig-authorization"))
	require.Empty(t, headers.Get("authorization"))
}
