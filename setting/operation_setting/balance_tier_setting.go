package operation_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	BalanceTierStrategyFixedPriority                        = "fixed_priority"
	BalanceTierStrategyLowerEffectiveBalanceHigherPriority  = "lower_effective_balance_higher_priority"
	BalanceTierStrategyHigherEffectiveBalanceHigherPriority = "higher_effective_balance_higher_priority"
	BalanceTierStrategyKeepPriority                         = "keep_priority"
)

type BalanceTierRule struct {
	Id                  string   `json:"id"`
	Name                string   `json:"name"`
	Enabled             bool     `json:"enabled"`
	ChannelTypes        []int    `json:"channel_types,omitempty"`
	Groups              []string `json:"groups,omitempty"`
	Tags                []string `json:"tags,omitempty"`
	BalanceMin          *float64 `json:"balance_min,omitempty"`
	BalanceMax          *float64 `json:"balance_max,omitempty"`
	EffectiveBalanceMin *float64 `json:"effective_balance_min,omitempty"`
	EffectiveBalanceMax *float64 `json:"effective_balance_max,omitempty"`
	Strategy            string   `json:"strategy"`
	FixedPriority       *int64   `json:"fixed_priority,omitempty"`
	MinPriority         *int64   `json:"min_priority,omitempty"`
	MaxPriority         *int64   `json:"max_priority,omitempty"`
}

type BalanceTierSetting struct {
	Enabled bool              `json:"enabled"`
	Rules   []BalanceTierRule `json:"rules"`
}

var balanceTierSetting = BalanceTierSetting{
	Enabled: false,
	Rules:   []BalanceTierRule{},
}

func init() {
	config.GlobalConfig.Register("balance_tier_setting", &balanceTierSetting)
}

func GetBalanceTierSetting() *BalanceTierSetting {
	if balanceTierSetting.Rules == nil {
		balanceTierSetting.Rules = []BalanceTierRule{}
	}
	return &balanceTierSetting
}

func GetBalanceTierSettingSnapshot() BalanceTierSetting {
	setting := *GetBalanceTierSetting()
	return CloneBalanceTierSetting(setting)
}

func CloneBalanceTierSetting(setting BalanceTierSetting) BalanceTierSetting {
	cloned := setting
	if setting.Rules == nil {
		cloned.Rules = []BalanceTierRule{}
		return cloned
	}
	cloned.Rules = make([]BalanceTierRule, len(setting.Rules))
	for i, rule := range setting.Rules {
		cloned.Rules[i] = rule
		cloned.Rules[i].ChannelTypes = append([]int(nil), rule.ChannelTypes...)
		cloned.Rules[i].Groups = append([]string(nil), rule.Groups...)
		cloned.Rules[i].Tags = append([]string(nil), rule.Tags...)
	}
	return cloned
}

func NormalizeBalanceTierSetting(setting *BalanceTierSetting) {
	if setting == nil {
		return
	}
	if setting.Rules == nil {
		setting.Rules = []BalanceTierRule{}
	}
	for i := range setting.Rules {
		rule := &setting.Rules[i]
		rule.Id = strings.TrimSpace(rule.Id)
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Strategy = strings.TrimSpace(rule.Strategy)
		rule.Groups = normalizeBalanceTierStrings(rule.Groups)
		rule.Tags = normalizeBalanceTierStrings(rule.Tags)
		if rule.ChannelTypes == nil {
			rule.ChannelTypes = []int{}
		}
	}
}

func normalizeBalanceTierStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func IsSupportedBalanceTierStrategy(strategy string) bool {
	switch strategy {
	case BalanceTierStrategyFixedPriority,
		BalanceTierStrategyLowerEffectiveBalanceHigherPriority,
		BalanceTierStrategyHigherEffectiveBalanceHigherPriority,
		BalanceTierStrategyKeepPriority:
		return true
	default:
		return false
	}
}

func ValidateBalanceTierSetting(setting BalanceTierSetting) error {
	setting = CloneBalanceTierSetting(setting)
	NormalizeBalanceTierSetting(&setting)
	seen := make(map[string]bool, len(setting.Rules))
	for i, rule := range setting.Rules {
		if rule.Id == "" {
			return fmt.Errorf("第 %d 条规则缺少 id", i+1)
		}
		if seen[rule.Id] {
			return fmt.Errorf("规则 id 重复：%s", rule.Id)
		}
		seen[rule.Id] = true
		if rule.Name == "" {
			return fmt.Errorf("第 %d 条规则缺少名称", i+1)
		}
		if !IsSupportedBalanceTierStrategy(rule.Strategy) {
			return fmt.Errorf("第 %d 条规则策略无效", i+1)
		}
		if err := validateBalanceTierRange(rule.BalanceMin, rule.BalanceMax, i, "档位余额"); err != nil {
			return err
		}
		if err := validateBalanceTierRange(rule.EffectiveBalanceMin, rule.EffectiveBalanceMax, i, "有效剩余额度"); err != nil {
			return err
		}
		switch rule.Strategy {
		case BalanceTierStrategyFixedPriority:
			if rule.FixedPriority == nil {
				return fmt.Errorf("第 %d 条规则缺少固定优先级", i+1)
			}
		case BalanceTierStrategyLowerEffectiveBalanceHigherPriority,
			BalanceTierStrategyHigherEffectiveBalanceHigherPriority:
			if rule.EffectiveBalanceMin == nil || rule.EffectiveBalanceMax == nil || *rule.EffectiveBalanceMin == *rule.EffectiveBalanceMax {
				return fmt.Errorf("第 %d 条动态规则必须设置不同的有效余额上下限", i+1)
			}
			if rule.MinPriority == nil || rule.MaxPriority == nil {
				return fmt.Errorf("第 %d 条动态规则缺少优先级上下限", i+1)
			}
			if *rule.MinPriority > *rule.MaxPriority {
				return fmt.Errorf("第 %d 条规则最低优先级不能大于最高优先级", i+1)
			}
		}
	}
	return nil
}

func ValidateBalanceTierRulesJSONString(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "[]"
	}
	var rules []BalanceTierRule
	if err := common.Unmarshal([]byte(value), &rules); err != nil {
		return err
	}
	return ValidateBalanceTierSetting(BalanceTierSetting{Rules: rules})
}

func validateBalanceTierRange(minimum, maximum *float64, index int, name string) error {
	if minimum != nil && maximum != nil && *minimum > *maximum {
		return fmt.Errorf("第 %d 条规则%s下限不能大于上限", index+1, name)
	}
	return nil
}
