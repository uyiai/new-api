package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelTagStatsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := DB
	previousLogDB := LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&Channel{}, &Log{}))

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		DB = previousDB
		LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		common.RedisEnabled = previousRedisEnabled
	})

	return db
}

func channelTagStatsStringPtr(value string) *string {
	return &value
}

func requireChannelTagStatsItem(t *testing.T, items []ChannelTagStatsItem, tagKey string) ChannelTagStatsItem {
	t.Helper()
	for _, item := range items {
		if item.TagKey == tagKey {
			return item
		}
	}
	require.Failf(t, "missing tag stats item", "tag_key=%s items=%v", tagKey, items)
	return ChannelTagStatsItem{}
}

func requireChannelTagStatsTrendPoint(t *testing.T, points []ChannelTagStatsTrendPoint, bucketStart int64, tagKey string) ChannelTagStatsTrendPoint {
	t.Helper()
	for _, point := range points {
		if point.BucketStart == bucketStart && point.TagKey == tagKey {
			return point
		}
	}
	require.Failf(t, "missing tag stats trend point", "bucket_start=%d tag_key=%s points=%v", bucketStart, tagKey, points)
	return ChannelTagStatsTrendPoint{}
}

func TestNormalizeChannelTagTreatsNilEmptyAndWhitespaceAsUntagged(t *testing.T) {
	cases := []*string{
		nil,
		channelTagStatsStringPtr(""),
		channelTagStatsStringPtr("   \t\n"),
	}

	for _, tag := range cases {
		key, name := NormalizeChannelTag(tag)
		require.Equal(t, UntaggedChannelTagKey, key)
		require.Equal(t, UntaggedChannelTagName, name)
	}

	key, name := NormalizeChannelTag(channelTagStatsStringPtr("  paid-tier  "))
	require.Equal(t, "paid-tier", key)
	require.Equal(t, "paid-tier", name)
}

func TestGetChannelTagMetadataByIdsChunksLargeLookups(t *testing.T) {
	setupChannelTagStatsTestDB(t)

	const channelCount = channelTagMetadataLookupChunkSize + 1
	channels := make([]Channel, 0, channelCount)
	ids := make([]int, 0, channelCount)
	for id := 1; id <= channelCount; id++ {
		channels = append(channels, Channel{Id: id, Type: 1, Key: fmt.Sprintf("key-%d", id), Name: fmt.Sprintf("channel-%d", id)})
		ids = append(ids, id)
	}
	require.NoError(t, DB.Create(&channels).Error)

	metadata, err := GetChannelTagMetadataByIds(ids)
	require.NoError(t, err)
	require.Len(t, metadata, channelCount)
	require.Contains(t, metadata, 1)
	require.Contains(t, metadata, channelCount)
}

func TestGetChannelTagStatsAggregatesUntaggedConsumeOnlyAndSkipsMissingChannels(t *testing.T) {
	setupChannelTagStatsTestDB(t)

	channels := []Channel{
		{Id: 1, Type: 1, Key: "key-1", Name: "nil tag"},
		{Id: 2, Type: 1, Key: "key-2", Name: "empty tag", Tag: channelTagStatsStringPtr("")},
		{Id: 3, Type: 1, Key: "key-3", Name: "space tag", Tag: channelTagStatsStringPtr("  ")},
		{Id: 4, Type: 1, Key: "key-4", Name: "alpha tag", Tag: channelTagStatsStringPtr(" alpha ")},
	}
	require.NoError(t, DB.Create(&channels).Error)

	logs := []Log{
		{CreatedAt: 1000, Type: LogTypeConsume, ChannelId: 1, Quota: 100, PromptTokens: 10, CompletionTokens: 1, UseTime: 10},
		{CreatedAt: 1001, Type: LogTypeConsume, ChannelId: 2, Quota: 200, PromptTokens: 20, CompletionTokens: 2, UseTime: 20},
		{CreatedAt: 1002, Type: LogTypeConsume, ChannelId: 3, Quota: 300, PromptTokens: 30, CompletionTokens: 3, UseTime: 30},
		{CreatedAt: 1003, Type: LogTypeConsume, ChannelId: 4, Quota: 400, PromptTokens: 40, CompletionTokens: 4, UseTime: 40},
		{CreatedAt: 1004, Type: LogTypeManage, ChannelId: 4, Quota: 900, PromptTokens: 90, CompletionTokens: 9, UseTime: 90},
		{CreatedAt: 1005, Type: LogTypeConsume, ChannelId: 999, Quota: 50, PromptTokens: 5, CompletionTokens: 1, UseTime: 5},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	result, err := GetChannelTagStats(context.Background(), ChannelTagStatsFilter{Granularity: ChannelTagStatsDay, TrendLimit: 10})
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, int64(1000), result.Summary.TotalQuota)
	require.Equal(t, int64(4), result.Summary.RequestCount)
	require.Equal(t, int64(100), result.Summary.PromptTokens)
	require.Equal(t, int64(10), result.Summary.CompletionTokens)
	require.Equal(t, int64(110), result.Summary.Tokens)
	require.InDelta(t, 25, result.Summary.AverageUseTime, 0.0001)
	require.Equal(t, 1, result.Summary.TagCount)
	require.Equal(t, 2, result.Summary.TagGroupCount)
	require.Equal(t, 4, result.Summary.ChannelCount)
	require.Equal(t, int64(600), result.Summary.UntaggedQuota)
	require.Equal(t, int64(3), result.Summary.UntaggedRequestCount)

	untagged := requireChannelTagStatsItem(t, result.Items, UntaggedChannelTagKey)
	require.Equal(t, UntaggedChannelTagName, untagged.TagName)
	require.Equal(t, int64(600), untagged.Quota)
	require.Equal(t, int64(3), untagged.RequestCount)
	require.Equal(t, 3, untagged.ChannelCount)
	require.Equal(t, int64(1002), untagged.LastLogAt)

	alpha := requireChannelTagStatsItem(t, result.Items, "alpha")
	require.Equal(t, "alpha", alpha.TagName)
	require.Equal(t, int64(400), alpha.Quota)
	require.Equal(t, int64(1), alpha.RequestCount)
	require.Equal(t, 1, alpha.ChannelCount)
}

func TestGetChannelTagStatsAppliesInclusiveDateBounds(t *testing.T) {
	setupChannelTagStatsTestDB(t)

	channel := Channel{Id: 1, Type: 1, Key: "key-1", Name: "alpha", Tag: channelTagStatsStringPtr("alpha")}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{CreatedAt: 999, Type: LogTypeConsume, ChannelId: 1, Quota: 10},
		{CreatedAt: 1000, Type: LogTypeConsume, ChannelId: 1, Quota: 20},
		{CreatedAt: 1500, Type: LogTypeConsume, ChannelId: 1, Quota: 30},
		{CreatedAt: 2000, Type: LogTypeConsume, ChannelId: 1, Quota: 40},
		{CreatedAt: 2001, Type: LogTypeConsume, ChannelId: 1, Quota: 50},
	}).Error)

	result, err := GetChannelTagStats(context.Background(), ChannelTagStatsFilter{
		StartTimestamp: 1000,
		EndTimestamp:   2000,
		Granularity:    ChannelTagStatsDay,
	})
	require.NoError(t, err)
	require.Equal(t, int64(90), result.Summary.TotalQuota)
	require.Equal(t, int64(3), result.Summary.RequestCount)

	alpha := requireChannelTagStatsItem(t, result.Items, "alpha")
	require.Equal(t, int64(90), alpha.Quota)
	require.Equal(t, int64(3), alpha.RequestCount)
	require.Equal(t, int64(2000), alpha.LastLogAt)
}

func TestNormalizeChannelTagStatsFilterRejectsInvalidGranularityAndDateRange(t *testing.T) {
	_, err := NormalizeChannelTagStatsFilter(ChannelTagStatsFilter{Granularity: ChannelTagStatsGranularity("month")})
	require.Error(t, err)

	_, err = NormalizeChannelTagStatsFilter(ChannelTagStatsFilter{
		StartTimestamp: 2000,
		EndTimestamp:   1000,
		Granularity:    ChannelTagStatsDay,
	})
	require.Error(t, err)

	filter, err := NormalizeChannelTagStatsFilter(ChannelTagStatsFilter{})
	require.NoError(t, err)
	require.Equal(t, ChannelTagStatsDay, filter.Granularity)
	require.Equal(t, defaultChannelTagStatsTrendLimit, filter.TrendLimit)
}

func TestGetChannelTagStatsBuildsTrendBucketsAndCollapsesNonTopTags(t *testing.T) {
	setupChannelTagStatsTestDB(t)

	channels := []Channel{
		{Id: 1, Type: 1, Key: "key-1", Name: "alpha", Tag: channelTagStatsStringPtr("alpha")},
		{Id: 2, Type: 1, Key: "key-2", Name: "beta", Tag: channelTagStatsStringPtr("beta")},
	}
	require.NoError(t, DB.Create(&channels).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{CreatedAt: 100, Type: LogTypeConsume, ChannelId: 1, Quota: 100, PromptTokens: 10, CompletionTokens: 1},
		{CreatedAt: 3700, Type: LogTypeConsume, ChannelId: 1, Quota: 50, PromptTokens: 5, CompletionTokens: 1},
		{CreatedAt: 3701, Type: LogTypeConsume, ChannelId: 2, Quota: 70, PromptTokens: 7, CompletionTokens: 1},
		{CreatedAt: 3702, Type: LogTypeConsume, ChannelId: 999, Quota: 30, PromptTokens: 3, CompletionTokens: 1},
	}).Error)

	result, err := GetChannelTagStats(context.Background(), ChannelTagStatsFilter{
		Granularity: ChannelTagStatsHour,
		TrendLimit:  1,
	})
	require.NoError(t, err)
	require.Equal(t, ChannelTagStatsHour, result.Granularity)

	firstBucketAlpha := requireChannelTagStatsTrendPoint(t, result.Trend, 0, "alpha")
	require.Equal(t, int64(100), firstBucketAlpha.Quota)
	require.Equal(t, int64(1), firstBucketAlpha.RequestCount)
	require.Equal(t, int64(11), firstBucketAlpha.Tokens)

	secondBucketAlpha := requireChannelTagStatsTrendPoint(t, result.Trend, 3600, "alpha")
	require.Equal(t, int64(50), secondBucketAlpha.Quota)
	require.Equal(t, int64(1), secondBucketAlpha.RequestCount)

	secondBucketOther := requireChannelTagStatsTrendPoint(t, result.Trend, 3600, "__other__")
	require.Equal(t, "其他", secondBucketOther.TagName)
	require.Equal(t, int64(70), secondBucketOther.Quota)
	require.Equal(t, int64(1), secondBucketOther.RequestCount)
	require.Equal(t, int64(8), secondBucketOther.Tokens)
}
