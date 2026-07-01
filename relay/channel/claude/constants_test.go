package claude

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelListIncludesClaudeSonnet5(t *testing.T) {
	require.Contains(t, ModelList, "claude-sonnet-5")
	require.Equal(t, "claude-sonnet-5", ModelList[0])
}
