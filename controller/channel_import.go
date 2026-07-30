package controller

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	AnthropicImportProfileOfficial   = "anthropic_official"
	AnthropicImportProfileCloudflare = "cloudflare_anthropic_gateway"
)

var cloudflareAnthropicDefaultModels = []string{
	"claude-3-5-sonnet-20240620",
	"claude-3-5-sonnet-20241022",
	"claude-3-7-sonnet-20250219",
	"claude-3-7-sonnet-20250219-thinking",
	"claude-sonnet-4-20250514",
	"claude-sonnet-4-20250514-thinking",
	"claude-opus-4-20250514",
	"claude-opus-4-20250514-thinking",
	"claude-opus-4-1-20250805",
	"claude-opus-4-1-20250805-thinking",
	"claude-sonnet-4-5-20250929",
	"claude-sonnet-4-5-20250929-thinking",
	"claude-opus-4-5-20251101",
	"claude-opus-4-5-20251101-thinking",
	"claude-3-opus-20240229",
	"claude-3-sonnet-20240229",
	"claude-sonnet-4-6-thinking",
	"claude-opus-4-6-thinking",
	"claude-haiku-4-5-20251001",
	"claude-sonnet-4-6",
	"claude-opus-4-7",
	"claude-opus-4-6",
	"claude-opus-4-8",
	"claude-sonnet-5",
	"claude-opus-5",
}

type AnthropicImportProfile struct {
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	ChannelType      int      `json:"channel_type"`
	Columns          []string `json:"columns"`
	DefaultModels    []string `json:"default_models"`
	DefaultTestModel string   `json:"default_test_model"`
	Targets          []string `json:"targets"`
}

type anthropicImportRow struct {
	Balance     float64           `json:"balance"`
	Credentials map[string]string `json:"credentials"`
}

type anthropicImportRequest struct {
	ProfileID  string               `json:"profile_id"`
	Target     string               `json:"target"`
	Rows       []anthropicImportRow `json:"rows"`
	Groups     []string             `json:"groups"`
	Tag        string               `json:"tag"`
	Priority   int64                `json:"priority"`
	Weight     uint                 `json:"weight"`
	Models     string               `json:"models"`
	NameSuffix string               `json:"name_suffix"`
}

type anthropicImportResult struct {
	Index        int    `json:"index"`
	Line         int    `json:"line"`
	OK           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
	CreatedCount int    `json:"created_count,omitempty"`
	IDs          []int  `json:"ids,omitempty"`
}

type builtAnthropicImportRow struct {
	Name          string
	Key           string
	BaseURL       string
	Models        string
	TestModel     string
	OtherSettings string
	Balance       float64
}

var cloudflareAccountIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)

func anthropicImportProfiles() []AnthropicImportProfile {
	return []AnthropicImportProfile{
		{
			ID:               AnthropicImportProfileOfficial,
			Label:            "官方 Anthropic",
			ChannelType:      constant.ChannelTypeAnthropic,
			Columns:          []string{"balance", "api_key"},
			DefaultModels:    append([]string(nil), channelId2Models[constant.ChannelTypeAnthropic]...),
			DefaultTestModel: "claude-sonnet-4-6",
			Targets:          []string{"channel", "preparation"},
		},
		{
			ID:               AnthropicImportProfileCloudflare,
			Label:            "Cloudflare",
			ChannelType:      constant.ChannelTypeAnthropic,
			Columns:          []string{"balance", "account_id", "api_token"},
			DefaultModels:    append([]string(nil), cloudflareAnthropicDefaultModels...),
			DefaultTestModel: "claude-sonnet-4-6",
			Targets:          []string{"channel", "preparation"},
		},
	}
}

func GetAnthropicImportProfile(id string) (AnthropicImportProfile, bool) {
	for _, profile := range anthropicImportProfiles() {
		if profile.ID == id {
			return profile, true
		}
	}
	return AnthropicImportProfile{}, false
}

func getDefaultAnthropicImportProfileID() string {
	id := model_setting.GetClaudeSettings().DefaultImportProfile
	if _, ok := GetAnthropicImportProfile(id); ok {
		return id
	}
	return AnthropicImportProfileOfficial
}

func GetChannelImportProfiles(c *gin.Context) {
	common.ApiSuccess(c, gin.H{
		"default_profile_id": getDefaultAnthropicImportProfileID(),
		"profiles":           anthropicImportProfiles(),
	})
}

func validateAnthropicImportProfileOption(value string) bool {
	_, ok := GetAnthropicImportProfile(value)
	return ok
}

func anthropicImportProfileSupportsTarget(profile AnthropicImportProfile, target string) bool {
	for _, supportedTarget := range profile.Targets {
		if supportedTarget == target {
			return true
		}
	}
	return false
}

func normalizeAnthropicImportGroups(groups []string) []string {
	return normalizeChannelPreparationCreateGroups("", groups)
}

func buildAnthropicImportRow(profile AnthropicImportProfile, request anthropicImportRequest, row anthropicImportRow) (builtAnthropicImportRow, error) {
	if row.Balance < 0 {
		return builtAnthropicImportRow{}, fmt.Errorf("额度必须是大于等于 0 的数字")
	}
	models := strings.TrimSpace(request.Models)
	if models == "" {
		models = strings.Join(profile.DefaultModels, ",")
	}
	built := builtAnthropicImportRow{
		Models:    models,
		TestModel: profile.DefaultTestModel,
		Balance:   row.Balance,
	}

	switch profile.ID {
	case AnthropicImportProfileOfficial:
		built.Key = strings.TrimSpace(row.Credentials["api_key"])
		if built.Key == "" {
			return built, fmt.Errorf("API Key 不能为空")
		}
		suffix := strings.TrimSpace(request.NameSuffix)
		if suffix == "" {
			suffix = "Anthropic"
		}
		balance := strconv.FormatFloat(row.Balance, 'f', -1, 64)
		built.Name = fmt.Sprintf("%s-%s-%s", time.Now().Format("200601021504"), balance, suffix)
	case AnthropicImportProfileCloudflare:
		accountID := strings.TrimSpace(row.Credentials["account_id"])
		if !cloudflareAccountIDPattern.MatchString(accountID) {
			return built, fmt.Errorf("Cloudflare Account ID 必须是 32 位十六进制字符串")
		}
		built.Key = strings.TrimSpace(row.Credentials["api_token"])
		if built.Key == "" {
			return built, fmt.Errorf("Cloudflare API Token 不能为空")
		}
		built.BaseURL = fmt.Sprintf("https://gateway.ai.cloudflare.com/v1/%s/default/anthropic", strings.ToLower(accountID))
		built.Name = "Cloudflare-" + strings.ToLower(accountID[len(accountID)-8:])
		settingsJSON, err := common.Marshal(dto.ChannelOtherSettings{UpstreamProvider: AnthropicImportProfileCloudflare})
		if err != nil {
			return built, err
		}
		built.OtherSettings = string(settingsJSON)
	default:
		return built, fmt.Errorf("不支持的 Anthropic 导入来源")
	}
	return built, nil
}

func applyAnthropicImportCommonFields(request anthropicImportRequest) (tag *string, priority *int64, weight *uint, autoBan *int) {
	if value := strings.TrimSpace(request.Tag); value != "" {
		tag = &value
	}
	priorityValue := request.Priority
	weightValue := request.Weight
	autoBanValue := 1
	return tag, &priorityValue, &weightValue, &autoBanValue
}

func normalizeAnthropicImportBaseURL(baseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalized == "" {
		return strings.TrimRight(constant.ChannelBaseURLs[constant.ChannelTypeAnthropic], "/")
	}
	return normalized
}

func inferAnthropicImportProfileID(otherSettings string, baseURL string) string {
	var settings dto.ChannelOtherSettings
	if strings.TrimSpace(otherSettings) != "" && common.UnmarshalJsonStr(otherSettings, &settings) == nil {
		if _, ok := GetAnthropicImportProfile(settings.UpstreamProvider); ok {
			return settings.UpstreamProvider
		}
	}
	normalizedBaseURL := normalizeAnthropicImportBaseURL(baseURL)
	if strings.HasPrefix(normalizedBaseURL, "https://gateway.ai.cloudflare.com/v1/") &&
		strings.HasSuffix(normalizedBaseURL, "/default/anthropic") {
		return AnthropicImportProfileCloudflare
	}
	if normalizedBaseURL == normalizeAnthropicImportBaseURL("") {
		return AnthropicImportProfileOfficial
	}
	return ""
}

func isSameAnthropicImportIdentity(profileID string, built builtAnthropicImportRow, otherSettings string, baseURL string) bool {
	return inferAnthropicImportProfileID(otherSettings, baseURL) == profileID &&
		normalizeAnthropicImportBaseURL(baseURL) == normalizeAnthropicImportBaseURL(built.BaseURL)
}

func ensureAnthropicImportIdentityAvailable(tx *gorm.DB, profileID string, target string, built builtAnthropicImportRow, groups []string) error {
	for _, group := range groups {
		switch target {
		case "channel":
			var channels []model.Channel
			if err := tx.Select("base_url", "settings").
				Where(&model.Channel{Key: built.Key, Group: group}).
				Find(&channels).Error; err != nil {
				return err
			}
			for _, channel := range channels {
				baseURL := ""
				if channel.BaseURL != nil {
					baseURL = *channel.BaseURL
				}
				if isSameAnthropicImportIdentity(profileID, built, channel.OtherSettings, baseURL) {
					return fmt.Errorf("目标池中已存在相同来源、Account/Base URL、Key 和分组 %s 的记录", group)
				}
			}
		case "preparation":
			var preparations []model.ChannelPreparation
			if err := tx.Select("base_url", "settings").
				Where(&model.ChannelPreparation{Key: built.Key, Group: group}).
				Where("status IN ?", []int{model.ChannelPreparationStatusPending, model.ChannelPreparationStatusPromoting}).
				Find(&preparations).Error; err != nil {
				return err
			}
			for _, preparation := range preparations {
				baseURL := ""
				if preparation.BaseURL != nil {
					baseURL = *preparation.BaseURL
				}
				if isSameAnthropicImportIdentity(profileID, built, preparation.OtherSettings, baseURL) {
					return fmt.Errorf("目标池中已存在相同来源、Account/Base URL、Key 和分组 %s 的记录", group)
				}
			}
		default:
			return fmt.Errorf("不支持的导入目标")
		}
	}
	return nil
}

func importAnthropicProfileRow(request anthropicImportRequest, profile AnthropicImportProfile, row anthropicImportRow, groups []string) (anthropicImportResult, error) {
	built, err := buildAnthropicImportRow(profile, request, row)
	if err != nil {
		return anthropicImportResult{}, err
	}

	tx := model.DB.Begin()
	if tx.Error != nil {
		return anthropicImportResult{}, tx.Error
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			tx.Rollback()
			panic(recovered)
		}
	}()
	if err := ensureAnthropicImportIdentityAvailable(tx, profile.ID, request.Target, built, groups); err != nil {
		tx.Rollback()
		return anthropicImportResult{}, err
	}

	tag, priority, weight, autoBan := applyAnthropicImportCommonFields(request)
	ids := make([]int, 0, len(groups))
	switch request.Target {
	case "channel":
		channels := make([]model.Channel, 0, len(groups))
		for _, group := range groups {
			var baseURL *string
			if built.BaseURL != "" {
				value := built.BaseURL
				baseURL = &value
			}
			testModel := built.TestModel
			channel := model.Channel{
				Type:          profile.ChannelType,
				Key:           built.Key,
				TestModel:     &testModel,
				Status:        common.ChannelStatusEnabled,
				Name:          built.Name,
				Weight:        weight,
				CreatedTime:   common.GetTimestamp(),
				BaseURL:       baseURL,
				Balance:       built.Balance,
				Models:        built.Models,
				Group:         group,
				Priority:      priority,
				AutoBan:       autoBan,
				Tag:           tag,
				OtherSettings: built.OtherSettings,
			}
			if err := validateChannel(&channel, true); err != nil {
				tx.Rollback()
				return anthropicImportResult{}, err
			}
			channels = append(channels, channel)
		}
		if err := model.CreateChannelsWithTx(tx, channels); err != nil {
			tx.Rollback()
			return anthropicImportResult{}, err
		}
		for _, channel := range channels {
			ids = append(ids, channel.Id)
		}
	case "preparation":
		for _, group := range groups {
			var baseURL *string
			if built.BaseURL != "" {
				value := built.BaseURL
				baseURL = &value
			}
			testModel := built.TestModel
			preparation := model.ChannelPreparation{
				Type:          profile.ChannelType,
				Key:           built.Key,
				TestModel:     &testModel,
				Name:          built.Name,
				Weight:        weight,
				BaseURL:       baseURL,
				Balance:       built.Balance,
				Models:        built.Models,
				Group:         group,
				Priority:      priority,
				AutoBan:       autoBan,
				Tag:           tag,
				OtherSettings: built.OtherSettings,
				Source:        "batch_import",
			}
			if err := validateChannelPreparationInput(&preparation, true); err != nil {
				tx.Rollback()
				return anthropicImportResult{}, err
			}
			preparation.NormalizeForCreate()
			if err := tx.Create(&preparation).Error; err != nil {
				tx.Rollback()
				return anthropicImportResult{}, err
			}
			ids = append(ids, preparation.Id)
		}
	default:
		tx.Rollback()
		return anthropicImportResult{}, fmt.Errorf("不支持的导入目标")
	}

	if err := tx.Commit().Error; err != nil {
		return anthropicImportResult{}, err
	}
	return anthropicImportResult{OK: true, CreatedCount: len(ids), IDs: ids}, nil
}

func sanitizeAnthropicImportError(err error, credentials map[string]string) string {
	message := err.Error()
	for _, credential := range credentials {
		if value := strings.TrimSpace(credential); value != "" {
			message = strings.ReplaceAll(message, value, "[REDACTED]")
		}
	}
	return message
}

func ImportChannelsFromProfile(c *gin.Context) {
	var request anthropicImportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	profile, ok := GetAnthropicImportProfile(strings.TrimSpace(request.ProfileID))
	if !ok {
		common.ApiErrorMsg(c, "无效的 Anthropic 导入来源")
		return
	}
	if !anthropicImportProfileSupportsTarget(profile, request.Target) {
		common.ApiErrorMsg(c, "无效的导入目标")
		return
	}
	if len(request.Rows) == 0 {
		common.ApiErrorMsg(c, "导入数据不能为空")
		return
	}

	groups := normalizeAnthropicImportGroups(request.Groups)
	results := make([]anthropicImportResult, 0, len(request.Rows))
	channelCreated := false
	for index, row := range request.Rows {
		result, err := importAnthropicProfileRow(request, profile, row, groups)
		result.Index = index
		result.Line = index + 1
		if err != nil {
			result.OK = false
			result.Error = sanitizeAnthropicImportError(err, row.Credentials)
		} else if request.Target == "channel" {
			channelCreated = true
		}
		results = append(results, result)
	}
	if channelCreated {
		model.InitChannelCache()
		service.ResetProxyClientCache()
	}
	common.ApiSuccess(c, gin.H{"results": results})
}
