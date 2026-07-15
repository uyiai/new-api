package operation_setting

import "testing"

func float64Pointer(value float64) *float64 { return &value }
func int64Pointer(value int64) *int64       { return &value }

func TestValidateBalanceTierSettingPreservesZeroValues(t *testing.T) {
	setting := BalanceTierSetting{Enabled: true, Rules: []BalanceTierRule{{
		Id: "five-dollar-low", Name: "5刀低余额", Enabled: true,
		BalanceMin: float64Pointer(5), BalanceMax: float64Pointer(5),
		EffectiveBalanceMin: float64Pointer(0), EffectiveBalanceMax: float64Pointer(2),
		Strategy: BalanceTierStrategyFixedPriority, FixedPriority: int64Pointer(0),
	}}}
	if err := ValidateBalanceTierSetting(setting); err != nil {
		t.Fatalf("expected valid setting, got %v", err)
	}
}

func TestValidateBalanceTierSettingRequiresDynamicBounds(t *testing.T) {
	setting := BalanceTierSetting{Rules: []BalanceTierRule{{
		Id: "dynamic", Name: "动态", Enabled: true,
		Strategy:    BalanceTierStrategyHigherEffectiveBalanceHigherPriority,
		MinPriority: int64Pointer(0), MaxPriority: int64Pointer(100),
	}}}
	if err := ValidateBalanceTierSetting(setting); err == nil {
		t.Fatal("expected dynamic rule without effective balance bounds to fail")
	}
}

func TestValidateBalanceTierSettingRejectsDuplicateIds(t *testing.T) {
	rule := BalanceTierRule{
		Id: "same", Name: "规则", Enabled: true,
		Strategy: BalanceTierStrategyKeepPriority,
	}
	setting := BalanceTierSetting{Rules: []BalanceTierRule{rule, rule}}
	if err := ValidateBalanceTierSetting(setting); err == nil {
		t.Fatal("expected duplicate ids to fail")
	}
}
