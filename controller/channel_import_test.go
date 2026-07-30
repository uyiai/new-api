package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type channelImportProfilesTestResponse struct {
	Success bool `json:"success"`
	Data    struct {
		DefaultProfileID string                   `json:"default_profile_id"`
		Profiles         []AnthropicImportProfile `json:"profiles"`
	} `json:"data"`
}

type channelImportTestResult struct {
	Index        int    `json:"index"`
	Line         int    `json:"line"`
	OK           bool   `json:"ok"`
	Error        string `json:"error"`
	CreatedCount int    `json:"created_count"`
	IDs          []int  `json:"ids"`
}

type channelImportTestResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Results []channelImportTestResult `json:"results"`
	} `json:"data"`
}

func postProfileImport(t *testing.T, payload any) (channelImportTestResponse, string) {
	t.Helper()
	body, err := common.Marshal(payload)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/import", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ImportChannelsFromProfile(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response channelImportTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response, recorder.Body.String()
}

func TestCloudflareProfileImportCreatesOneChannelPerGroupWithoutLeakingToken(t *testing.T) {
	setupModelListControllerTestDB(t)

	const token = "cfut_profile_import_secret"
	response, rawResponse := postProfileImport(t, map[string]any{
		"profile_id": AnthropicImportProfileCloudflare,
		"target":     "channel",
		"rows": []map[string]any{{
			"balance": 12.5,
			"credentials": map[string]string{
				"account_id": "0123456789abcdef0123456789abcdef",
				"api_token":  token,
			},
		}},
		"groups":   []string{"default", "vip"},
		"tag":      "cloudflare-import",
		"priority": 3,
		"weight":   4,
		"models":   "claude-3-5-sonnet-20240620,claude-opus-5",
	})

	require.True(t, response.Success)
	require.Len(t, response.Data.Results, 1)
	require.True(t, response.Data.Results[0].OK, response.Data.Results[0].Error)
	require.Equal(t, 1, response.Data.Results[0].Line)
	require.Equal(t, 2, response.Data.Results[0].CreatedCount)
	require.NotContains(t, rawResponse, token)

	var channels []model.Channel
	require.NoError(t, model.DB.Where("key = ?", token).Order(`"group" asc`).Find(&channels).Error)
	require.Len(t, channels, 2)
	require.Equal(t, "https://gateway.ai.cloudflare.com/v1/0123456789abcdef0123456789abcdef/default/anthropic", channels[0].GetBaseURL())
	require.Equal(t, "claude-3-5-sonnet-20240620,claude-opus-5", channels[0].Models)
	require.NotNil(t, channels[0].TestModel)
	require.Equal(t, "claude-sonnet-4-6", *channels[0].TestModel)
	require.Equal(t, "Cloudflare-89abcdef", channels[0].Name)
	require.Equal(t, "cloudflare-import", channels[0].GetTag())
	require.Equal(t, int64(3), channels[0].GetPriority())
	require.Equal(t, 4, channels[0].GetWeight())
	require.True(t, channels[0].HeaderOverride == nil || strings.TrimSpace(*channels[0].HeaderOverride) == "")

	var settings dto.ChannelOtherSettings
	require.NoError(t, common.UnmarshalJsonStr(channels[0].OtherSettings, &settings))
	require.Equal(t, AnthropicImportProfileCloudflare, settings.UpstreamProvider)
}

func TestCloudflareProfileImportRejectsDuplicateRowAtomicallyAcrossGroups(t *testing.T) {
	setupModelListControllerTestDB(t)

	baseURL := "https://gateway.ai.cloudflare.com/v1/0123456789abcdef0123456789abcdef/default/anthropic"
	require.NoError(t, model.DB.Create(&model.Channel{
		Type:    14,
		Key:     "cfut_duplicate",
		Name:    "existing",
		Status:  common.ChannelStatusEnabled,
		BaseURL: &baseURL,
		Models:  "claude-sonnet-4-6",
		Group:   "default",
	}).Error)

	response, _ := postProfileImport(t, map[string]any{
		"profile_id": AnthropicImportProfileCloudflare,
		"target":     "channel",
		"rows": []map[string]any{{
			"balance": 1,
			"credentials": map[string]string{
				"account_id": "0123456789abcdef0123456789abcdef",
				"api_token":  "cfut_duplicate",
			},
		}},
		"groups": []string{"default", "vip"},
	})

	require.True(t, response.Success)
	require.Len(t, response.Data.Results, 1)
	require.False(t, response.Data.Results[0].OK)
	require.Contains(t, response.Data.Results[0].Error, "分组 default")

	var vipCount int64
	require.NoError(t, model.DB.Model(&model.Channel{}).
		Where(&model.Channel{Key: "cfut_duplicate", Group: "vip"}).
		Count(&vipCount).Error)
	require.Zero(t, vipCount)
}

func TestCloudflareProfileImportAllowsSameTokenForDifferentAccounts(t *testing.T) {
	setupModelListControllerTestDB(t)

	response, _ := postProfileImport(t, map[string]any{
		"profile_id": AnthropicImportProfileCloudflare,
		"target":     "channel",
		"rows": []map[string]any{
			{
				"balance": 1,
				"credentials": map[string]string{
					"account_id": "0123456789abcdef0123456789abcdef",
					"api_token":  "cfut_shared",
				},
			},
			{
				"balance": 2,
				"credentials": map[string]string{
					"account_id": "1123456789abcdef0123456789abcdee",
					"api_token":  "cfut_shared",
				},
			},
		},
		"groups": []string{"default"},
	})

	require.True(t, response.Success)
	require.Len(t, response.Data.Results, 2)
	require.True(t, response.Data.Results[0].OK, response.Data.Results[0].Error)
	require.True(t, response.Data.Results[1].OK, response.Data.Results[1].Error)

	var channels []model.Channel
	require.NoError(t, model.DB.Where("key = ?", "cfut_shared").Order("base_url asc").Find(&channels).Error)
	require.Len(t, channels, 2)
	require.NotEqual(t, channels[0].GetBaseURL(), channels[1].GetBaseURL())
}

func TestCloudflareProfileImportAllowsSameIdentityAcrossTargets(t *testing.T) {
	setupModelListControllerTestDB(t)

	payload := map[string]any{
		"profile_id": AnthropicImportProfileCloudflare,
		"rows": []map[string]any{{
			"balance": 1,
			"credentials": map[string]string{
				"account_id": "0123456789abcdef0123456789abcdef",
				"api_token":  "cfut_cross_target",
			},
		}},
		"groups": []string{"default"},
	}
	payload["target"] = "channel"
	channelResponse, _ := postProfileImport(t, payload)
	require.True(t, channelResponse.Data.Results[0].OK, channelResponse.Data.Results[0].Error)

	payload["target"] = "preparation"
	preparationResponse, _ := postProfileImport(t, payload)
	require.True(t, preparationResponse.Data.Results[0].OK, preparationResponse.Data.Results[0].Error)
}

func TestCloudflareProfileImportReturnsPartialSuccessByRow(t *testing.T) {
	setupModelListControllerTestDB(t)

	response, rawResponse := postProfileImport(t, map[string]any{
		"profile_id": AnthropicImportProfileCloudflare,
		"target":     "channel",
		"rows": []map[string]any{
			{
				"balance": 1,
				"credentials": map[string]string{
					"account_id": "0123456789abcdef0123456789abcdef",
					"api_token":  "cfut_partial_success",
				},
			},
			{
				"balance": 1,
				"credentials": map[string]string{
					"account_id": "invalid-account",
					"api_token":  "cfut_partial_failure_secret",
				},
			},
		},
		"groups": []string{"default"},
	})

	require.True(t, response.Success)
	require.Len(t, response.Data.Results, 2)
	require.True(t, response.Data.Results[0].OK, response.Data.Results[0].Error)
	require.False(t, response.Data.Results[1].OK)
	require.Equal(t, 2, response.Data.Results[1].Line)
	require.NotContains(t, rawResponse, "cfut_partial_failure_secret")

	var count int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("key = ?", "cfut_partial_success").Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestCloudflareProfileImportRejectsInvalidAccountID(t *testing.T) {
	setupModelListControllerTestDB(t)

	response, rawResponse := postProfileImport(t, map[string]any{
		"profile_id": AnthropicImportProfileCloudflare,
		"target":     "channel",
		"rows": []map[string]any{{
			"balance": 1,
			"credentials": map[string]string{
				"account_id": "not-an-account-id",
				"api_token":  "cfut_invalid_account_secret",
			},
		}},
		"groups": []string{"default"},
	})

	require.True(t, response.Success)
	require.Len(t, response.Data.Results, 1)
	require.False(t, response.Data.Results[0].OK)
	require.Contains(t, response.Data.Results[0].Error, "32 位十六进制")
	require.NotContains(t, rawResponse, "cfut_invalid_account_secret")

	var count int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestCloudflareProfileImportUsesNormalizedBaseURLForDuplicateDetection(t *testing.T) {
	setupModelListControllerTestDB(t)

	baseURL := "https://gateway.ai.cloudflare.com/v1/0123456789abcdef0123456789abcdef/default/anthropic/"
	require.NoError(t, model.DB.Create(&model.Channel{
		Type:    14,
		Key:     "cfut_trailing_slash",
		Name:    "existing",
		Status:  common.ChannelStatusEnabled,
		BaseURL: &baseURL,
		Models:  "claude-sonnet-4-6",
		Group:   "default",
	}).Error)

	response, _ := postProfileImport(t, map[string]any{
		"profile_id": AnthropicImportProfileCloudflare,
		"target":     "channel",
		"rows": []map[string]any{{
			"balance": 1,
			"credentials": map[string]string{
				"account_id": "0123456789abcdef0123456789abcdef",
				"api_token":  "cfut_trailing_slash",
			},
		}},
		"groups": []string{"default"},
	})

	require.False(t, response.Data.Results[0].OK)
	require.Contains(t, response.Data.Results[0].Error, "已存在")
}

func TestOfficialAnthropicProfileImportKeepsOfficialLifecycleRules(t *testing.T) {
	setupModelListControllerTestDB(t)

	response, _ := postProfileImport(t, map[string]any{
		"profile_id": AnthropicImportProfileOfficial,
		"target":     "channel",
		"rows": []map[string]any{{
			"balance": 3,
			"credentials": map[string]string{
				"api_key": "sk-ant-official-import",
			},
		}},
		"groups":      []string{"default"},
		"name_suffix": "official",
		"models":      "claude-3-5-sonnet-20240620,claude-opus-5",
	})

	require.True(t, response.Success)
	require.Len(t, response.Data.Results, 1)
	require.True(t, response.Data.Results[0].OK, response.Data.Results[0].Error)

	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, "key = ?", "sk-ant-official-import").Error)
	require.NotNil(t, channel.BaseURL)
	require.Empty(t, *channel.BaseURL)
	require.Contains(t, channel.Name, "-3-official")
	require.Equal(t, "claude-sonnet-4-6,claude-opus-5", channel.Models)
}

func TestOfficialAnthropicProfileImportRejectsDuplicateInSameGroup(t *testing.T) {
	setupModelListControllerTestDB(t)

	emptyBaseURL := ""
	require.NoError(t, model.DB.Create(&model.Channel{
		Type:    14,
		Key:     "sk-ant-official-duplicate",
		Name:    "existing",
		Status:  common.ChannelStatusEnabled,
		BaseURL: &emptyBaseURL,
		Models:  "claude-sonnet-4-6",
		Group:   "default",
	}).Error)

	response, _ := postProfileImport(t, map[string]any{
		"profile_id": AnthropicImportProfileOfficial,
		"target":     "channel",
		"rows": []map[string]any{{
			"balance": 3,
			"credentials": map[string]string{
				"api_key": "sk-ant-official-duplicate",
			},
		}},
		"groups": []string{"default"},
	})

	require.False(t, response.Data.Results[0].OK)
	require.Contains(t, response.Data.Results[0].Error, "已存在")
}

func TestCloudflareProfileImportCreatesPreparationThatPreservesProfileOnPromotion(t *testing.T) {
	setupModelListControllerTestDB(t)

	response, _ := postProfileImport(t, map[string]any{
		"profile_id": AnthropicImportProfileCloudflare,
		"target":     "preparation",
		"rows": []map[string]any{{
			"balance": 7,
			"credentials": map[string]string{
				"account_id": "0123456789abcdef0123456789abcdef",
				"api_token":  "cfut_preparation",
			},
		}},
		"groups": []string{"default"},
		"tag":    "cf-preparation",
	})

	require.True(t, response.Success)
	require.Len(t, response.Data.Results, 1)
	require.True(t, response.Data.Results[0].OK, response.Data.Results[0].Error)
	require.Len(t, response.Data.Results[0].IDs, 1)

	var preparation model.ChannelPreparation
	require.NoError(t, model.DB.First(&preparation, "id = ?", response.Data.Results[0].IDs[0]).Error)
	require.Equal(t, "cfut_preparation", preparation.Key)
	require.Equal(t, "batch_import", preparation.Source)
	require.Equal(t, "cf-preparation", *preparation.Tag)

	channel := preparation.ToChannel()
	require.NotNil(t, preparation.BaseURL)
	require.Equal(t, *preparation.BaseURL, channel.GetBaseURL())
	require.Equal(t, preparation.OtherSettings, channel.OtherSettings)
	require.Equal(t, preparation.Models, channel.Models)
	require.Equal(t, preparation.TestModel, channel.TestModel)
}

func TestUpdateOptionRejectsUnknownAnthropicImportProfile(t *testing.T) {
	body, err := common.Marshal(OptionUpdateRequest{
		Key:   "claude.default_import_profile",
		Value: "unknown_vendor",
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/", bytes.NewReader(body))

	UpdateOption(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
	require.Contains(t, response.Message, "导入来源")
}

func TestGetChannelImportProfilesReturnsConfiguredDefaultAndCloudflareSchema(t *testing.T) {
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"claude.default_import_profile": AnthropicImportProfileCloudflare,
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"claude.default_import_profile": AnthropicImportProfileOfficial,
		}))
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/import/profiles", nil)

	GetChannelImportProfiles(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response channelImportProfilesTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, AnthropicImportProfileCloudflare, response.Data.DefaultProfileID)
	require.Len(t, response.Data.Profiles, 2)

	cloudflare, ok := GetAnthropicImportProfile(AnthropicImportProfileCloudflare)
	require.True(t, ok)
	require.Equal(t, []string{"balance", "account_id", "api_token"}, cloudflare.Columns)
	require.Equal(t, "claude-sonnet-4-6", cloudflare.DefaultTestModel)
	require.Len(t, cloudflare.DefaultModels, 25)
	require.Contains(t, cloudflare.DefaultModels, "claude-opus-5")
}

func TestGetChannelImportProfilesFallsBackToOfficialForInvalidStoredValue(t *testing.T) {
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"claude.default_import_profile": "removed_profile",
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"claude.default_import_profile": AnthropicImportProfileOfficial,
		}))
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/import/profiles", nil)

	GetChannelImportProfiles(ctx)

	var response channelImportProfilesTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, AnthropicImportProfileOfficial, response.Data.DefaultProfileID)
}
