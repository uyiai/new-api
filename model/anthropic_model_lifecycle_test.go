package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestNormalizeDirectAnthropicModelListReplacesRetiredAndDeduplicates(t *testing.T) {
	models, changed := NormalizeDirectAnthropicModelList(" claude-3-sonnet-20240229,claude-sonnet-4-6,claude-3-opus-20240229 ")

	require.True(t, changed)
	require.Equal(t, "claude-sonnet-4-6,claude-opus-4-8", models)
}

func TestNormalizeDirectAnthropicChannelModelsOnlyTouchesDirectAnthropic(t *testing.T) {
	anthropic := &Channel{
		Type:   constant.ChannelTypeAnthropic,
		Models: "claude-3-sonnet-20240229,claude-3-haiku-20240307",
	}
	require.True(t, NormalizeDirectAnthropicChannelModels(anthropic))
	require.Equal(t, "claude-sonnet-4-6,claude-haiku-4-5-20251001", anthropic.Models)

	aws := &Channel{
		Type:   constant.ChannelTypeAws,
		Models: "claude-3-sonnet-20240229",
	}
	require.False(t, NormalizeDirectAnthropicChannelModels(aws))
	require.Equal(t, "claude-3-sonnet-20240229", aws.Models)
}

func TestNormalizeRetiredDirectAnthropicModelsInDatabase(t *testing.T) {
	setupChannelPreparationModelTestDB(t)
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}, &ChannelPreparation{}))

	testModel := "claude-3-sonnet-20240229"
	channel := Channel{
		Type:      constant.ChannelTypeAnthropic,
		Key:       "sk-test",
		Name:      "anthropic",
		Status:    common.ChannelStatusEnabled,
		Group:     "default",
		Models:    "claude-3-sonnet-20240229,claude-sonnet-4-6,claude-3-opus-20240229",
		TestModel: &testModel,
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	preparationTestModel := "claude-3-haiku-20240307"
	preparation := ChannelPreparation{
		Type:      constant.ChannelTypeAnthropic,
		Key:       "sk-prep",
		Name:      "prep",
		Status:    ChannelPreparationStatusPending,
		Group:     "default",
		Models:    "claude-3-haiku-20240307,claude-haiku-4-5-20251001",
		TestModel: &preparationTestModel,
	}
	require.NoError(t, DB.Create(&preparation).Error)

	require.NoError(t, NormalizeRetiredDirectAnthropicModelsInDatabase())

	var gotChannel Channel
	require.NoError(t, DB.First(&gotChannel, "id = ?", channel.Id).Error)
	require.Equal(t, "claude-sonnet-4-6,claude-opus-4-8", gotChannel.Models)
	require.NotNil(t, gotChannel.TestModel)
	require.Equal(t, "claude-sonnet-4-6", *gotChannel.TestModel)

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Order("model asc").Find(&abilities).Error)
	require.Len(t, abilities, 2)
	require.Equal(t, "claude-opus-4-8", abilities[0].Model)
	require.Equal(t, "claude-sonnet-4-6", abilities[1].Model)

	var gotPreparation ChannelPreparation
	require.NoError(t, DB.First(&gotPreparation, "id = ?", preparation.Id).Error)
	require.Equal(t, "claude-haiku-4-5-20251001", gotPreparation.Models)
	require.NotNil(t, gotPreparation.TestModel)
	require.Equal(t, "claude-haiku-4-5-20251001", *gotPreparation.TestModel)
}
