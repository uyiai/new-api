package controller

import (
	"math/rand"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func int64Ptr(value int64) *int64 { return &value }
func uintPtr(value uint) *uint    { return &value }

func TestComputeChannelPreparationAutoPromotionCapacityClampsPerChannel(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	channelType := constant.ChannelTypeAnthropic
	channels := []model.Channel{
		{
			Type:      channelType,
			Key:       "sk-overused",
			Name:      "overused",
			Status:    common.ChannelStatusEnabled,
			Group:     "vip",
			Balance:   100,
			UsedQuota: int64(7173 * common.QuotaPerUnit),
		},
		{
			Type:      channelType,
			Key:       "sk-healthy",
			Name:      "healthy",
			Status:    common.ChannelStatusEnabled,
			Group:     "vip",
			Balance:   5000,
			UsedQuota: 0,
		},
		{
			Type:    channelType,
			Key:     "sk-empty",
			Name:    "empty",
			Status:  common.ChannelStatusEnabled,
			Group:   "vip",
			Balance: 0,
		},
		{
			Type:    channelType,
			Key:     "sk-other-group",
			Name:    "other group",
			Status:  common.ChannelStatusEnabled,
			Group:   "svip",
			Balance: 9999,
		},
		{
			Type:    channelType,
			Key:     "sk-disabled",
			Name:    "disabled",
			Status:  common.ChannelStatusManuallyDisabled,
			Group:   "vip",
			Balance: 9999,
		},
	}
	require.NoError(t, db.Create(&channels).Error)

	capacity, err := computeChannelPreparationAutoPromotionCapacity("vip", channelType)
	require.NoError(t, err)
	require.Equal(t, int64(2), capacity.EligibleChannelCount)
	require.Equal(t, int64(1), capacity.UsableChannelCount)
	require.Equal(t, int64(1), capacity.IgnoredNonPositiveBalanceChannelCount)
	require.InDelta(t, 5100, capacity.BalanceSumUSD, 0.000001)
	require.InDelta(t, 7173, capacity.UsedQuotaUSD, 0.000001)
	require.InDelta(t, -2073, capacity.RawEffectiveCapacityUSD, 0.000001)
	require.InDelta(t, 5000, capacity.EffectiveCapacityUSD, 0.000001)
}

func TestChooseChannelPreparationAutoPromotionCandidateRespectsHighestPriorityTier(t *testing.T) {
	preparations := []model.ChannelPreparation{
		{Id: 1, Balance: 100, Priority: int64Ptr(1), Weight: uintPtr(100000)},
		{Id: 2, Balance: 10, Priority: int64Ptr(10), Weight: uintPtr(0)},
	}

	candidate, ok := chooseChannelPreparationAutoPromotionCandidate(
		preparations,
		operation_setting.ChannelPreparationAutoPromotionStrategyPriorityWeighted,
		rand.New(rand.NewSource(1)),
	)

	require.True(t, ok)
	require.Equal(t, 2, candidate.Id)
}

func TestChooseChannelPreparationAutoPromotionCandidateSmallBalanceFirst(t *testing.T) {
	preparations := []model.ChannelPreparation{
		{Id: 1, Balance: 1, Priority: int64Ptr(1)},
		{Id: 2, Balance: 5, Priority: int64Ptr(10)},
		{Id: 3, Balance: 2, Priority: int64Ptr(10)},
		{Id: 4, Balance: 2, Priority: int64Ptr(10)},
	}

	candidate, ok := chooseChannelPreparationAutoPromotionCandidate(
		preparations,
		operation_setting.ChannelPreparationAutoPromotionStrategySmallBalanceFirst,
		nil,
	)

	require.True(t, ok)
	require.Equal(t, 3, candidate.Id)
}

func TestChooseChannelPreparationAutoPromotionCandidateLargeBalanceFirst(t *testing.T) {
	preparations := []model.ChannelPreparation{
		{Id: 1, Balance: 100, Priority: int64Ptr(1)},
		{Id: 2, Balance: 5, Priority: int64Ptr(10)},
		{Id: 3, Balance: 10, Priority: int64Ptr(10)},
		{Id: 4, Balance: 10, Priority: int64Ptr(10)},
	}

	candidate, ok := chooseChannelPreparationAutoPromotionCandidate(
		preparations,
		operation_setting.ChannelPreparationAutoPromotionStrategyLargeBalanceFirst,
		nil,
	)

	require.True(t, ok)
	require.Equal(t, 3, candidate.Id)
}

func TestChooseChannelPreparationAutoPromotionActiveShortage(t *testing.T) {
	capacityFirst := operation_setting.ChannelPreparationAutoPromotionRule{
		GuaranteePriority: operation_setting.ChannelPreparationAutoPromotionGuaranteePriorityCapacityFirst,
	}
	countFirst := operation_setting.ChannelPreparationAutoPromotionRule{
		GuaranteePriority: operation_setting.ChannelPreparationAutoPromotionGuaranteePriorityCountFirst,
	}

	require.Equal(t, channelPreparationAutoPromotionShortageCapacity, chooseChannelPreparationAutoPromotionActiveShortage(capacityFirst, true, true))
	require.Equal(t, channelPreparationAutoPromotionShortageCount, chooseChannelPreparationAutoPromotionActiveShortage(countFirst, true, true))
	require.Equal(t, channelPreparationAutoPromotionShortageCount, chooseChannelPreparationAutoPromotionActiveShortage(capacityFirst, true, false))
	require.Equal(t, channelPreparationAutoPromotionShortageCapacity, chooseChannelPreparationAutoPromotionActiveShortage(countFirst, false, true))
	require.Empty(t, chooseChannelPreparationAutoPromotionActiveShortage(countFirst, false, false))
}

func TestChannelPreparationAutoPromotionCountDeficit(t *testing.T) {
	require.Equal(t, int64(0), channelPreparationAutoPromotionCountDeficit(0, 0))
	require.Equal(t, int64(3), channelPreparationAutoPromotionCountDeficit(5, 2))
	require.Equal(t, int64(0), channelPreparationAutoPromotionCountDeficit(2, 5))
}
