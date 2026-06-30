package model

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelUpstreamRateLimitStatusTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := DB
	previousLogDB := LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousCooldownEnabled := common.ChannelCooldownEnabled
	previousCooldownProactiveEnabled := common.ChannelCooldownProactiveEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.ChannelCooldownEnabled = true
	common.ChannelCooldownProactiveEnabled = true
	initCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelUpstreamRateLimitStatus{}))

	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		common.ChannelCooldownEnabled = previousCooldownEnabled
		common.ChannelCooldownProactiveEnabled = previousCooldownProactiveEnabled
		resetChannelUpstreamRateLimitStatusTestStores()
		initCol()

		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func resetChannelUpstreamRateLimitStatusTestStores() {
	upstreamRateLimitStatusStore = sync.Map{}
	upstreamRateLimitDirtyStore = sync.Map{}
	upstreamRateLimitFlushOnce = sync.Once{}
	upstreamRateLimitLocks = [64]sync.Mutex{}
}

func TestChannelUpstreamRateLimitStatusIsSharedBySameAnthropicKey(t *testing.T) {
	setupChannelUpstreamRateLimitStatusTestDB(t)

	resetTime := time.Now().Add(90 * time.Second).UTC().Truncate(time.Second)
	header := http.Header{}
	header.Set(headerReqLimit, "50")
	header.Set(headerReqRemaining, "12")
	header.Set(headerReqReset, resetTime.Format(time.RFC3339))
	header.Set(headerInputTokensLimit, "200000")
	header.Set(headerInputTokensRemaining, "150000")
	header.Set(headerInputTokensReset, resetTime.Format(time.RFC3339))

	UpdateChannelUpstreamRateLimitStatus(
		1,
		constant.ChannelTypeAnthropic,
		constant.ChannelBaseURLs[constant.ChannelTypeAnthropic],
		"sk-shared",
		http.StatusOK,
		header,
	)
	FlushChannelUpstreamRateLimitStatusOnce()

	channels := []*Channel{
		{Id: 1, Type: constant.ChannelTypeAnthropic, Key: "sk-shared", Name: "default", Group: "default"},
		{Id: 2, Type: constant.ChannelTypeAnthropic, Key: " sk-shared ", Name: "vip", Group: "vip"},
	}
	require.NoError(t, AttachChannelUpstreamRateLimitStatuses(channels))
	require.NotNil(t, channels[0].UpstreamRateLimitStatus)
	require.NotNil(t, channels[1].UpstreamRateLimitStatus)
	require.Equal(t, 12, *channels[0].UpstreamRateLimitStatus.RequestsRemaining)
	require.Equal(t, 12, *channels[1].UpstreamRateLimitStatus.RequestsRemaining)
	require.Equal(t, channels[0].UpstreamRateLimitStatus.Id, channels[1].UpstreamRateLimitStatus.Id)
}

func TestChannelUpstreamRateLimitStatusConcurrentUpdatesFlushOneRow(t *testing.T) {
	setupChannelUpstreamRateLimitStatusTestDB(t)

	const concurrency = 1000
	resetTime := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(index int) {
			defer wg.Done()
			header := http.Header{}
			header.Set(headerReqLimit, "5000")
			header.Set(headerReqRemaining, fmt.Sprintf("%d", index%5000))
			header.Set(headerReqReset, resetTime.Format(time.RFC3339))
			header.Set(headerInputTokensLimit, "1000000")
			header.Set(headerInputTokensRemaining, fmt.Sprintf("%d", 1000000-index))
			header.Set(headerInputTokensReset, resetTime.Format(time.RFC3339))
			UpdateChannelUpstreamRateLimitStatus(
				1,
				constant.ChannelTypeAnthropic,
				constant.ChannelBaseURLs[constant.ChannelTypeAnthropic],
				"sk-concurrent",
				http.StatusOK,
				header,
			)
		}(i)
	}
	wg.Wait()

	var beforeFlushCount int64
	require.NoError(t, DB.Model(&ChannelUpstreamRateLimitStatus{}).Count(&beforeFlushCount).Error)
	require.Zero(t, beforeFlushCount)

	FlushChannelUpstreamRateLimitStatusOnce()

	var statuses []ChannelUpstreamRateLimitStatus
	require.NoError(t, DB.Find(&statuses).Error)
	require.Len(t, statuses, 1)
	require.Equal(t, 5000, *statuses[0].RequestsLimit)
	require.Equal(t, 1000000, *statuses[0].InputTokensLimit)
	require.NotNil(t, statuses[0].RequestsRemaining)
	require.NotNil(t, statuses[0].InputTokensRemaining)
}
