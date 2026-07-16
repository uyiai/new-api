package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func balanceTierFloat64Ptr(value float64) *float64 { return &value }

func TestEvaluateBalanceTierRulesFirstMatchAndSkipsNegativeEffectiveBalance(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	tag := "anthropic"
	channels := []model.Channel{
		{
			Id:        1,
			Type:      constant.ChannelTypeAnthropic,
			Key:       "sk-five",
			Name:      "five dollar",
			Status:    common.ChannelStatusEnabled,
			Group:     "default,vip",
			Tag:       &tag,
			Balance:   5,
			UsedQuota: int64(4 * common.QuotaPerUnit),
			Priority:  int64Ptr(1),
		},
		{
			Id:        2,
			Type:      constant.ChannelTypeAnthropic,
			Key:       "sk-negative",
			Name:      "negative",
			Status:    common.ChannelStatusEnabled,
			Group:     "vip",
			Tag:       &tag,
			Balance:   5,
			UsedQuota: int64(6 * common.QuotaPerUnit),
			Priority:  int64Ptr(7),
		},
		{
			Id:      3,
			Type:    constant.ChannelTypeAnthropic,
			Key:     "sk-disabled",
			Name:    "disabled",
			Status:  common.ChannelStatusManuallyDisabled,
			Group:   "vip",
			Tag:     &tag,
			Balance: 5,
		},
	}
	require.NoError(t, db.Create(&channels).Error)

	setting := operation_setting.BalanceTierSetting{Enabled: true, Rules: []operation_setting.BalanceTierRule{
		{
			Id:                  "five-low-first",
			Name:                "5刀低余额首条规则",
			Enabled:             true,
			ChannelTypes:        []int{constant.ChannelTypeAnthropic},
			Groups:              []string{"vip"},
			Tags:                []string{tag},
			BalanceMin:          balanceTierFloat64Ptr(5),
			BalanceMax:          balanceTierFloat64Ptr(5),
			EffectiveBalanceMin: balanceTierFloat64Ptr(0),
			EffectiveBalanceMax: balanceTierFloat64Ptr(2),
			Strategy:            operation_setting.BalanceTierStrategyLowerEffectiveBalanceHigherPriority,
			MinPriority:         int64Ptr(10),
			MaxPriority:         int64Ptr(100),
		},
		{
			Id:            "five-fallback",
			Name:          "不应命中的后续规则",
			Enabled:       true,
			BalanceMin:    balanceTierFloat64Ptr(5),
			BalanceMax:    balanceTierFloat64Ptr(5),
			Strategy:      operation_setting.BalanceTierStrategyFixedPriority,
			FixedPriority: int64Ptr(1),
		},
	}}

	result, err := evaluateBalanceTierRules(setting)
	require.NoError(t, err)
	require.Equal(t, 2, result.Summary.EnabledChannels)
	require.Equal(t, 1, result.Summary.MatchedChannels)
	require.Equal(t, 1, result.Summary.ChangedChannels)
	require.Len(t, result.Details, 2)
	require.Equal(t, "five-low-first", result.Details[0].MatchedRuleId)
	require.Equal(t, int64(55), result.Details[0].NewPriority)
	require.Contains(t, result.Details[1].Reason, "小于 0")
	require.False(t, result.Details[1].Changed)
}

func TestEvaluateBalanceTierRulesSupportsRemainingRatioMetric(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	channels := []model.Channel{
		{
			Id:        21,
			Type:      constant.ChannelTypeAnthropic,
			Key:       "sk-small-full",
			Name:      "small full",
			Status:    common.ChannelStatusEnabled,
			Group:     "default",
			Balance:   5,
			UsedQuota: 0,
			Priority:  int64Ptr(1),
		},
		{
			Id:        22,
			Type:      constant.ChannelTypeAnthropic,
			Key:       "sk-large-low-ratio",
			Name:      "large low ratio",
			Status:    common.ChannelStatusEnabled,
			Group:     "default",
			Balance:   100,
			UsedQuota: int64(90 * common.QuotaPerUnit),
			Priority:  int64Ptr(1),
		},
	}
	require.NoError(t, db.Create(&channels).Error)

	setting := operation_setting.BalanceTierSetting{Enabled: true, Rules: []operation_setting.BalanceTierRule{{
		Id:                "ratio-low",
		Name:              "区间档按剩余比例",
		Enabled:           true,
		BalanceMin:        balanceTierFloat64Ptr(5),
		BalanceMax:        balanceTierFloat64Ptr(100),
		Metric:            operation_setting.BalanceTierMetricRemainingRatio,
		RemainingRatioMin: balanceTierFloat64Ptr(0),
		RemainingRatioMax: balanceTierFloat64Ptr(20),
		Strategy:          operation_setting.BalanceTierStrategyLowerEffectiveBalanceHigherPriority,
		MinPriority:       int64Ptr(0),
		MaxPriority:       int64Ptr(100),
	}}}

	result, err := evaluateBalanceTierRules(setting)
	require.NoError(t, err)
	require.Len(t, result.Details, 2)
	require.Empty(t, result.Details[0].MatchedRuleId)
	require.Equal(t, float64(100), result.Details[0].RemainingRatio)
	require.Equal(t, "ratio-low", result.Details[1].MatchedRuleId)
	require.Equal(t, float64(10), result.Details[1].RemainingRatio)
	require.Equal(t, int64(50), result.Details[1].NewPriority)
}

func TestApplyBalanceTierRulesSyncsStaleAbilityPriority(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = originalOptionMap })

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	currentPriority := int64(100)
	stalePriority := int64(1)
	channel := model.Channel{
		Id:        11,
		Type:      constant.ChannelTypeAnthropic,
		Key:       "sk-sync-ability",
		Name:      "sync ability",
		Status:    common.ChannelStatusEnabled,
		Group:     "vip",
		Models:    "claude-test",
		Balance:   5,
		UsedQuota: int64(1 * common.QuotaPerUnit),
		Priority:  &currentPriority,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "vip",
		Model:     "claude-test",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &stalePriority,
	}).Error)

	setting := operation_setting.BalanceTierSetting{Enabled: true, Rules: []operation_setting.BalanceTierRule{{
		Id:            "sync",
		Name:          "同步 ability",
		Enabled:       true,
		BalanceMin:    balanceTierFloat64Ptr(5),
		BalanceMax:    balanceTierFloat64Ptr(5),
		Strategy:      operation_setting.BalanceTierStrategyFixedPriority,
		FixedPriority: &currentPriority,
	}}}
	body, err := common.Marshal(setting)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/balance-tier/apply", bytes.NewReader(body))

	ApplyBalanceTierRules(ctx)

	var response struct {
		Success bool              `json:"success"`
		Data    balanceTierResult `json:"data"`
		Message string            `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	require.Equal(t, 1, response.Data.Summary.MatchedChannels)
	require.Equal(t, 0, response.Data.Summary.ChangedChannels)

	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", channel.Id).Error)
	require.NotNil(t, ability.Priority)
	require.Equal(t, currentPriority, *ability.Priority)
}
