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

func TestDefaultRatiosIncludeClaudeFable5(t *testing.T) {
	modelRatio, ok := defaultModelRatio["claude-fable-5"]
	require.True(t, ok)
	require.Equal(t, 5.0, modelRatio)

	completionRatio, ok := defaultCompletionRatio["claude-fable-5"]
	require.True(t, ok)
	require.Equal(t, 5.0, completionRatio)

	cacheRatio, ok := defaultCacheRatio["claude-fable-5"]
	require.True(t, ok)
	require.Equal(t, 0.1, cacheRatio)
}

func TestDefaultRatiosIncludeClaudeOpus5(t *testing.T) {
	modelRatio, ok := defaultModelRatio["claude-opus-5"]
	require.True(t, ok)
	require.Equal(t, 2.5, modelRatio)

	completionRatio, ok := defaultCompletionRatio["claude-opus-5"]
	require.True(t, ok)
	require.Equal(t, 5.0, completionRatio)

	cacheRatio, ok := defaultCacheRatio["claude-opus-5"]
	require.True(t, ok)
	require.Equal(t, 0.1, cacheRatio)

	createCacheRatio, ok := defaultCreateCacheRatio["claude-opus-5"]
	require.True(t, ok)
	require.Equal(t, 1.25, createCacheRatio)
	require.Equal(t, 2.0, createCacheRatio*1.6)
}

func TestThinkingRatiosPreferExactThenFallbackToBaseModel(t *testing.T) {
	savedModelRatios := modelRatioMap.ReadAll()
	savedCompletionRatios := completionRatioMap.ReadAll()
	savedCacheRatios := cacheRatioMap.ReadAll()
	savedCreateCacheRatios := createCacheRatioMap.ReadAll()
	t.Cleanup(func() {
		modelRatioMap.Clear()
		modelRatioMap.AddAll(savedModelRatios)
		completionRatioMap.Clear()
		completionRatioMap.AddAll(savedCompletionRatios)
		cacheRatioMap.Clear()
		cacheRatioMap.AddAll(savedCacheRatios)
		createCacheRatioMap.Clear()
		createCacheRatioMap.AddAll(savedCreateCacheRatios)
	})

	modelRatioMap.Clear()
	modelRatioMap.Set("claude-test", 2)
	modelRatioMap.Set("claude-exact-thinking", 9)
	completionRatioMap.Clear()
	completionRatioMap.Set("claude-test", 5)
	completionRatioMap.Set("claude-exact-thinking", 7)
	cacheRatioMap.Clear()
	cacheRatioMap.Set("claude-test", 0.1)
	cacheRatioMap.Set("claude-exact-thinking", 0.9)
	createCacheRatioMap.Clear()
	createCacheRatioMap.Set("claude-test", 1.25)
	createCacheRatioMap.Set("claude-exact-thinking", 3)

	modelRatio, ok, _ := GetModelRatio("claude-test-thinking")
	require.True(t, ok)
	require.Equal(t, 2.0, modelRatio)
	require.Equal(t, 5.0, GetCompletionRatio("claude-test-thinking"))
	cacheRatio, ok := GetCacheRatio("claude-test-thinking")
	require.True(t, ok)
	require.Equal(t, 0.1, cacheRatio)
	createCacheRatio, ok := GetCreateCacheRatio("claude-test-thinking")
	require.True(t, ok)
	require.Equal(t, 1.25, createCacheRatio)

	modelRatio, ok, _ = GetModelRatio("claude-exact-thinking")
	require.True(t, ok)
	require.Equal(t, 9.0, modelRatio)
	require.Equal(t, 7.0, GetCompletionRatio("claude-exact-thinking"))
	cacheRatio, ok = GetCacheRatio("claude-exact-thinking")
	require.True(t, ok)
	require.Equal(t, 0.9, cacheRatio)
	createCacheRatio, ok = GetCreateCacheRatio("claude-exact-thinking")
	require.True(t, ok)
	require.Equal(t, 3.0, createCacheRatio)
}
