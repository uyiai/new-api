package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

var retiredDirectAnthropicModelReplacements = map[string]string{
	"claude-2.0":                          "claude-opus-4-8",
	"claude-2.1":                          "claude-opus-4-8",
	"claude-3-sonnet":                     "claude-sonnet-4-6",
	"claude-3-sonnet-20240229":            "claude-sonnet-4-6",
	"claude-3-5-sonnet":                   "claude-sonnet-4-6",
	"claude-3-5-sonnet-20240620":          "claude-sonnet-4-6",
	"claude-3-5-sonnet-20241022":          "claude-sonnet-4-6",
	"claude-3-7-sonnet-20250219":          "claude-sonnet-4-6",
	"claude-3-7-sonnet-20250219-thinking": "claude-sonnet-4-6",
	"claude-sonnet-4-20250514":            "claude-sonnet-4-6",
	"claude-sonnet-4-20250514-thinking":   "claude-sonnet-4-6",
	"claude-3-opus-20240229":              "claude-opus-4-8",
	"claude-opus-4-20250514":              "claude-opus-4-8",
	"claude-opus-4-20250514-thinking":     "claude-opus-4-8",
	"claude-3-haiku-20240307":             "claude-haiku-4-5-20251001",
	"claude-3-5-haiku-20241022":           "claude-haiku-4-5-20251001",
}

func ReplaceRetiredDirectAnthropicModel(modelName string) (string, bool) {
	trimmed := strings.TrimSpace(modelName)
	if trimmed == "" {
		return "", trimmed != modelName
	}
	replacement, retired := retiredDirectAnthropicModelReplacements[trimmed]
	if retired {
		return replacement, true
	}
	return trimmed, trimmed != modelName
}

func NormalizeDirectAnthropicModelList(models string) (string, bool) {
	parts := strings.Split(models, ",")
	seen := make(map[string]struct{}, len(parts))
	normalized := make([]string, 0, len(parts))
	changed := false

	for _, part := range parts {
		modelName, modelChanged := ReplaceRetiredDirectAnthropicModel(part)
		if modelChanged {
			changed = true
		}
		if modelName == "" {
			if strings.TrimSpace(part) != "" {
				changed = true
			}
			continue
		}
		if _, ok := seen[modelName]; ok {
			changed = true
			continue
		}
		seen[modelName] = struct{}{}
		normalized = append(normalized, modelName)
	}

	result := strings.Join(normalized, ",")
	if result != strings.Trim(strings.TrimSpace(models), ",") {
		changed = true
	}
	return result, changed
}

func NormalizeDirectAnthropicChannelModels(channel *Channel) bool {
	if channel == nil || channel.Type != constant.ChannelTypeAnthropic {
		return false
	}
	changed := false
	if normalizedModels, modelsChanged := NormalizeDirectAnthropicModelList(channel.Models); modelsChanged {
		channel.Models = normalizedModels
		changed = true
	}
	if channel.TestModel != nil {
		if modelName, modelChanged := ReplaceRetiredDirectAnthropicModel(*channel.TestModel); modelChanged {
			channel.TestModel = &modelName
			changed = true
		}
	}
	return changed
}

func NormalizeDirectAnthropicPreparationModels(preparation *ChannelPreparation) bool {
	if preparation == nil || preparation.Type != constant.ChannelTypeAnthropic {
		return false
	}
	changed := false
	if normalizedModels, modelsChanged := NormalizeDirectAnthropicModelList(preparation.Models); modelsChanged {
		preparation.Models = normalizedModels
		changed = true
	}
	if preparation.TestModel != nil {
		if modelName, modelChanged := ReplaceRetiredDirectAnthropicModel(*preparation.TestModel); modelChanged {
			preparation.TestModel = &modelName
			changed = true
		}
	}
	return changed
}

func NormalizeRetiredDirectAnthropicModelsInDatabase() error {
	channelCount, err := normalizeRetiredDirectAnthropicChannelModelsInDatabase()
	if err != nil {
		return err
	}
	preparationCount, err := normalizeRetiredDirectAnthropicPreparationModelsInDatabase()
	if err != nil {
		return err
	}
	if channelCount > 0 || preparationCount > 0 {
		common.SysLog(fmt.Sprintf("normalized retired direct Anthropic models: channels=%d, preparations=%d", channelCount, preparationCount))
	}
	return nil
}

func normalizeRetiredDirectAnthropicChannelModelsInDatabase() (int, error) {
	var channels []Channel
	if err := DB.Where("type = ?", constant.ChannelTypeAnthropic).Find(&channels).Error; err != nil {
		return 0, err
	}
	changedCount := 0
	for _, channel := range channels {
		oldModels := channel.Models
		oldTestModel := ""
		if channel.TestModel != nil {
			oldTestModel = *channel.TestModel
		}
		if !NormalizeDirectAnthropicChannelModels(&channel) {
			continue
		}
		updates := map[string]any{}
		modelsChanged := channel.Models != oldModels
		if modelsChanged {
			updates["models"] = channel.Models
		}
		if channel.TestModel != nil && *channel.TestModel != oldTestModel {
			updates["test_model"] = *channel.TestModel
		}
		if len(updates) == 0 {
			continue
		}
		if err := DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error; err != nil {
			return changedCount, err
		}
		if modelsChanged {
			if err := channel.UpdateAbilities(nil); err != nil {
				return changedCount, err
			}
		}
		changedCount++
	}
	return changedCount, nil
}

func normalizeRetiredDirectAnthropicPreparationModelsInDatabase() (int, error) {
	var preparations []ChannelPreparation
	if err := DB.Where("type = ?", constant.ChannelTypeAnthropic).Find(&preparations).Error; err != nil {
		return 0, err
	}
	changedCount := 0
	for _, preparation := range preparations {
		oldModels := preparation.Models
		oldTestModel := ""
		if preparation.TestModel != nil {
			oldTestModel = *preparation.TestModel
		}
		if !NormalizeDirectAnthropicPreparationModels(&preparation) {
			continue
		}
		updates := map[string]any{}
		if preparation.Models != oldModels {
			updates["models"] = preparation.Models
		}
		if preparation.TestModel != nil && *preparation.TestModel != oldTestModel {
			updates["test_model"] = *preparation.TestModel
		}
		if len(updates) == 0 {
			continue
		}
		if err := DB.Model(&ChannelPreparation{}).Where("id = ?", preparation.Id).Updates(updates).Error; err != nil {
			return changedCount, err
		}
		changedCount++
	}
	return changedCount, nil
}
