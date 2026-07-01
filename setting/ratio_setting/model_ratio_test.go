package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelRatioIncludesClaudeSonnet46(t *testing.T) {
	ratio, ok := defaultModelRatio["claude-sonnet-4-6"]
	require.True(t, ok)
	require.Equal(t, 1.5, ratio)
}

func TestDefaultModelRatioIncludesClaudeSonnet5(t *testing.T) {
	ratio, ok := defaultModelRatio["claude-sonnet-5"]
	require.True(t, ok)
	require.Equal(t, 1.0, ratio)
}

func TestDefaultCompletionRatioIncludesClaudeSonnet5(t *testing.T) {
	ratio, ok := defaultCompletionRatio["claude-sonnet-5"]
	require.True(t, ok)
	require.Equal(t, 5.0, ratio)
}
