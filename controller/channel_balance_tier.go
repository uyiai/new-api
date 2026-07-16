package controller

import (
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type balanceTierDetail struct {
	ChannelId        int     `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	ChannelType      int     `json:"channel_type"`
	Group            string  `json:"group"`
	Tag              string  `json:"tag"`
	Balance          float64 `json:"balance"`
	UsedQuotaUSD     float64 `json:"used_quota_usd"`
	EffectiveBalance float64 `json:"effective_balance"`
	RemainingRatio   float64 `json:"remaining_ratio"`
	OldPriority      int64   `json:"old_priority"`
	NewPriority      int64   `json:"new_priority"`
	MatchedRuleId    string  `json:"matched_rule_id,omitempty"`
	MatchedRuleName  string  `json:"matched_rule_name,omitempty"`
	Reason           string  `json:"reason"`
	Changed          bool    `json:"changed"`
}

type balanceTierSummary struct {
	EnabledChannels int `json:"enabled_channels"`
	MatchedChannels int `json:"matched_channels"`
	ChangedChannels int `json:"changed_channels"`
	SkippedChannels int `json:"skipped_channels"`
}

type balanceTierResult struct {
	Summary balanceTierSummary  `json:"summary"`
	Details []balanceTierDetail `json:"details"`
}

func GetBalanceTierSetting(c *gin.Context) {
	common.ApiSuccess(c, operation_setting.GetBalanceTierSettingSnapshot())
}

func UpdateBalanceTierSetting(c *gin.Context) {
	var request operation_setting.BalanceTierSetting
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "请求参数解析失败: "+err.Error())
		return
	}
	operation_setting.NormalizeBalanceTierSetting(&request)
	if err := operation_setting.ValidateBalanceTierSetting(request); err != nil {
		common.ApiErrorMsg(c, "余额档位规则配置错误: "+err.Error())
		return
	}
	if err := saveBalanceTierSetting(request); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, "更新余额档位调度规则")
	common.ApiSuccess(c, operation_setting.GetBalanceTierSettingSnapshot())
}

func saveBalanceTierSetting(setting operation_setting.BalanceTierSetting) error {
	operation_setting.NormalizeBalanceTierSetting(&setting)
	rulesJSON, err := common.Marshal(setting.Rules)
	if err != nil {
		return err
	}
	const enabledKey = "balance_tier_setting.enabled"
	const rulesKey = "balance_tier_setting.rules"
	return model.UpdateOptionsBulkOrdered(map[string]string{
		enabledKey: strconv.FormatBool(setting.Enabled),
		rulesKey:   string(rulesJSON),
	}, []string{rulesKey, enabledKey})
}

func PreviewBalanceTierRules(c *gin.Context) {
	setting, _, ok := readBalanceTierSettingRequest(c)
	if !ok {
		return
	}
	result, err := evaluateBalanceTierRules(setting)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func ApplyBalanceTierRules(c *gin.Context) {
	setting, hasRequestSetting, ok := readBalanceTierSettingRequest(c)
	if !ok {
		return
	}
	if hasRequestSetting {
		if err := saveBalanceTierSetting(setting); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	var result balanceTierResult
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = evaluateBalanceTierRulesWithDB(tx, setting)
		if err != nil {
			return err
		}
		for _, detail := range result.Details {
			if detail.MatchedRuleId == "" {
				continue
			}
			if detail.Changed {
				if err := tx.Model(&model.Channel{}).
					Where("id = ? AND status = ?", detail.ChannelId, common.ChannelStatusEnabled).
					Update("priority", detail.NewPriority).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.Ability{}).Where("channel_id = ?", detail.ChannelId).
				Update("priority", detail.NewPriority).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	model.RecordLog(c.GetInt("id"), model.LogTypeManage,
		"应用余额档位调度规则，更新渠道数："+strconv.Itoa(result.Summary.ChangedChannels))
	common.ApiSuccess(c, result)
}

func readBalanceTierSettingRequest(c *gin.Context) (operation_setting.BalanceTierSetting, bool, bool) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.ApiErrorMsg(c, "请求参数读取失败: "+err.Error())
		return operation_setting.BalanceTierSetting{}, false, false
	}
	var request operation_setting.BalanceTierSetting
	hasRequestSetting := strings.TrimSpace(string(body)) != ""
	if hasRequestSetting {
		if err := common.Unmarshal(body, &request); err != nil {
			common.ApiErrorMsg(c, "请求参数解析失败: "+err.Error())
			return request, false, false
		}
	} else {
		request = operation_setting.GetBalanceTierSettingSnapshot()
	}
	operation_setting.NormalizeBalanceTierSetting(&request)
	if err := operation_setting.ValidateBalanceTierSetting(request); err != nil {
		common.ApiErrorMsg(c, "余额档位规则配置错误: "+err.Error())
		return request, false, false
	}
	return request, hasRequestSetting, true
}

func evaluateBalanceTierRules(setting operation_setting.BalanceTierSetting) (balanceTierResult, error) {
	return evaluateBalanceTierRulesWithDB(model.DB, setting)
}

func evaluateBalanceTierRulesWithDB(db *gorm.DB, setting operation_setting.BalanceTierSetting) (balanceTierResult, error) {
	setting = operation_setting.CloneBalanceTierSetting(setting)
	operation_setting.NormalizeBalanceTierSetting(&setting)
	if err := operation_setting.ValidateBalanceTierSetting(setting); err != nil {
		return balanceTierResult{}, err
	}
	var channels []model.Channel
	if err := db.Select("id", "name", "type", "group", "tag", "balance", "used_quota", "priority").
		Where("status = ?", common.ChannelStatusEnabled).Order("id").Find(&channels).Error; err != nil {
		return balanceTierResult{}, err
	}
	result := balanceTierResult{
		Summary: balanceTierSummary{EnabledChannels: len(channels)},
		Details: make([]balanceTierDetail, 0, len(channels)),
	}
	quotaPerUnit := float64(common.QuotaPerUnit)
	for _, channel := range channels {
		oldPriority := int64(0)
		if channel.Priority != nil {
			oldPriority = *channel.Priority
		}
		tag := ""
		if channel.Tag != nil {
			tag = *channel.Tag
		}
		usedQuotaUSD := 0.0
		if quotaPerUnit > 0 {
			usedQuotaUSD = float64(channel.UsedQuota) / quotaPerUnit
		}
		effectiveBalance := channel.Balance - usedQuotaUSD
		remainingRatio := calculateBalanceTierRemainingRatio(channel.Balance, effectiveBalance)
		detail := balanceTierDetail{
			ChannelId: channel.Id, ChannelName: channel.Name, ChannelType: channel.Type,
			Group: channel.Group, Tag: tag, Balance: channel.Balance,
			UsedQuotaUSD: usedQuotaUSD, EffectiveBalance: effectiveBalance, RemainingRatio: remainingRatio,
			OldPriority: oldPriority, NewPriority: oldPriority,
		}
		switch {
		case !setting.Enabled:
			detail.Reason = "规则未启用"
		case effectiveBalance < 0:
			detail.Reason = "有效剩余额度小于 0，跳过"
		default:
			rule := matchBalanceTierRule(setting.Rules, channel, tag, effectiveBalance, remainingRatio)
			if rule == nil {
				detail.Reason = "未命中规则"
			} else {
				detail.MatchedRuleId = rule.Id
				detail.MatchedRuleName = rule.Name
				detail.NewPriority = calculateBalanceTierPriority(*rule, effectiveBalance, remainingRatio, oldPriority)
				detail.Changed = detail.NewPriority != oldPriority
				result.Summary.MatchedChannels++
				if rule.Strategy == operation_setting.BalanceTierStrategyKeepPriority {
					detail.Reason = "命中保留原优先级策略"
				} else if detail.Changed {
					detail.Reason = "命中规则，优先级将更新"
				} else {
					detail.Reason = "命中规则，优先级无需变化"
				}
			}
		}
		if detail.Changed {
			result.Summary.ChangedChannels++
		}
		if detail.MatchedRuleId == "" {
			result.Summary.SkippedChannels++
		}
		result.Details = append(result.Details, detail)
	}
	return result, nil
}

func matchBalanceTierRule(rules []operation_setting.BalanceTierRule, channel model.Channel, tag string, effectiveBalance float64, remainingRatio float64) *operation_setting.BalanceTierRule {
	for i := range rules {
		rule := &rules[i]
		metricValue := balanceTierMetricValue(*rule, effectiveBalance, remainingRatio)
		metricMin, metricMax := balanceTierMetricRange(*rule)
		if !rule.Enabled ||
			!balanceTierContainsInt(rule.ChannelTypes, channel.Type) ||
			!balanceTierContainsGroup(rule.Groups, channel.Group) ||
			!balanceTierContainsString(rule.Tags, tag) ||
			!balanceTierInRange(channel.Balance, rule.BalanceMin, rule.BalanceMax) ||
			!balanceTierInRange(metricValue, metricMin, metricMax) {
			continue
		}
		return rule
	}
	return nil
}

func balanceTierContainsInt(filter []int, value int) bool {
	if len(filter) == 0 {
		return true
	}
	for _, item := range filter {
		if item == value {
			return true
		}
	}
	return false
}

func balanceTierContainsString(filter []string, value string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, item := range filter {
		if item == value {
			return true
		}
	}
	return false
}

func balanceTierContainsGroup(filter []string, groups string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, group := range strings.Split(groups, ",") {
		if balanceTierContainsString(filter, strings.TrimSpace(group)) {
			return true
		}
	}
	return false
}

func balanceTierInRange(value float64, minimum, maximum *float64) bool {
	return (minimum == nil || value >= *minimum) && (maximum == nil || value <= *maximum)
}

func calculateBalanceTierRemainingRatio(balance float64, effectiveBalance float64) float64 {
	if balance <= 0 {
		return 0
	}
	ratio := effectiveBalance / balance * 100
	return math.Max(0, math.Min(100, ratio))
}

func balanceTierMetricValue(rule operation_setting.BalanceTierRule, effectiveBalance float64, remainingRatio float64) float64 {
	if rule.Metric == operation_setting.BalanceTierMetricRemainingRatio {
		return remainingRatio
	}
	return effectiveBalance
}

func balanceTierMetricRange(rule operation_setting.BalanceTierRule) (*float64, *float64) {
	if rule.Metric == operation_setting.BalanceTierMetricRemainingRatio {
		return rule.RemainingRatioMin, rule.RemainingRatioMax
	}
	return rule.EffectiveBalanceMin, rule.EffectiveBalanceMax
}

func calculateBalanceTierPriority(rule operation_setting.BalanceTierRule, effectiveBalance float64, remainingRatio float64, oldPriority int64) int64 {
	switch rule.Strategy {
	case operation_setting.BalanceTierStrategyFixedPriority:
		return *rule.FixedPriority
	case operation_setting.BalanceTierStrategyKeepPriority:
		return oldPriority
	}
	minimumBalance, maximumBalance := balanceTierMetricRange(rule)
	if minimumBalance == nil || maximumBalance == nil || *minimumBalance == *maximumBalance {
		return oldPriority
	}
	metricValue := balanceTierMetricValue(rule, effectiveBalance, remainingRatio)
	ratio := (metricValue - *minimumBalance) / (*maximumBalance - *minimumBalance)
	ratio = math.Max(0, math.Min(1, ratio))
	if rule.Strategy == operation_setting.BalanceTierStrategyLowerEffectiveBalanceHigherPriority {
		ratio = 1 - ratio
	}
	priority := float64(*rule.MinPriority) + ratio*float64(*rule.MaxPriority-*rule.MinPriority)
	return int64(math.Round(priority))
}
