package model

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

func applyExplicitLogTextFilter(tx *gorm.DB, column string, value string) (*gorm.DB, error) {
	if value == "" {
		return tx, nil
	}
	if strings.Contains(value, "%") {
		pattern, err := sanitizeLikePattern(value)
		if err != nil {
			return nil, err
		}
		return tx.Where(column+" LIKE ? ESCAPE '!'", pattern), nil
	}
	return tx.Where(column+" = ?", value), nil
}

type Log struct {
	Id                int    `json:"id" gorm:"index:idx_created_at_id,priority:2;index:idx_user_id_id,priority:2"`
	UserId            int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:1;index:idx_created_at_type"`
	Type              int    `json:"type" gorm:"index:idx_created_at_type"`
	Content           string `json:"content"`
	Username          string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName         string `json:"token_name" gorm:"index;default:''"`
	ModelName         string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota             int    `json:"quota" gorm:"default:0"`
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0"`
	UseTime           int    `json:"use_time" gorm:"default:0"`
	IsStream          bool   `json:"is_stream"`
	ChannelId         int    `json:"channel" gorm:"index"`
	ChannelName       string `json:"channel_name" gorm:"->"`
	TokenId           int    `json:"token_id" gorm:"default:0;index"`
	Group             string `json:"group" gorm:"index"`
	Ip                string `json:"ip" gorm:"index;default:''"`
	RequestId         string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	Other             string `json:"other"`
}

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
)

type ChannelTagStatsGranularity string

const (
	ChannelTagStatsHour ChannelTagStatsGranularity = "hour"
	ChannelTagStatsDay  ChannelTagStatsGranularity = "day"
	ChannelTagStatsWeek ChannelTagStatsGranularity = "week"
)

const (
	defaultChannelTagStatsTrendLimit = 12
	maxChannelTagStatsTrendLimit     = 30
)

type ChannelTagStatsFilter struct {
	StartTimestamp int64
	EndTimestamp   int64
	Granularity    ChannelTagStatsGranularity
	TrendLimit     int
}

type ChannelTagStatsSummary struct {
	TotalQuota           int64   `json:"total_quota"`
	RequestCount         int64   `json:"request_count"`
	PromptTokens         int64   `json:"prompt_tokens"`
	CompletionTokens     int64   `json:"completion_tokens"`
	Tokens               int64   `json:"tokens"`
	AverageUseTime       float64 `json:"average_use_time"`
	TagCount             int     `json:"tag_count"`
	TagGroupCount        int     `json:"tag_group_count"`
	ChannelCount         int     `json:"channel_count"`
	UntaggedQuota        int64   `json:"untagged_quota"`
	UntaggedRequestCount int64   `json:"untagged_request_count"`
}

type ChannelTagStatsItem struct {
	TagKey           string                       `json:"tag_key"`
	TagName          string                       `json:"tag_name"`
	Quota            int64                        `json:"quota"`
	RequestCount     int64                        `json:"request_count"`
	PromptTokens     int64                        `json:"prompt_tokens"`
	CompletionTokens int64                        `json:"completion_tokens"`
	Tokens           int64                        `json:"tokens"`
	AverageUseTime   float64                      `json:"average_use_time"`
	ChannelCount     int                          `json:"channel_count"`
	LastLogAt        int64                        `json:"last_log_at"`
	Channels         []ChannelTagStatsChannelItem `json:"channels"`
}

type ChannelTagStatsChannelItem struct {
	ChannelId        int     `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	ChannelType      int     `json:"channel_type"`
	ChannelStatus    int     `json:"channel_status"`
	Quota            int64   `json:"quota"`
	RequestCount     int64   `json:"request_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	Tokens           int64   `json:"tokens"`
	AverageUseTime   float64 `json:"average_use_time"`
	LastLogAt        int64   `json:"last_log_at"`
}

type ChannelTagStatsTrendPoint struct {
	BucketStart      int64  `json:"bucket_start"`
	TagKey           string `json:"tag_key"`
	TagName          string `json:"tag_name"`
	Quota            int64  `json:"quota"`
	RequestCount     int64  `json:"request_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	Tokens           int64  `json:"tokens"`
}

type ChannelTagStatsResult struct {
	Summary     ChannelTagStatsSummary      `json:"summary"`
	Items       []ChannelTagStatsItem       `json:"items"`
	Trend       []ChannelTagStatsTrendPoint `json:"trend"`
	Granularity ChannelTagStatsGranularity  `json:"granularity"`
}

func NormalizeChannelTagStatsFilter(filter ChannelTagStatsFilter) (ChannelTagStatsFilter, error) {
	if filter.StartTimestamp > 0 && filter.EndTimestamp > 0 && filter.StartTimestamp > filter.EndTimestamp {
		return filter, errors.New("开始时间不能晚于结束时间")
	}
	if filter.Granularity == "" {
		filter.Granularity = ChannelTagStatsDay
	}
	switch filter.Granularity {
	case ChannelTagStatsHour, ChannelTagStatsDay, ChannelTagStatsWeek:
	default:
		return filter, errors.New("无效的统计粒度")
	}
	if filter.TrendLimit <= 0 {
		filter.TrendLimit = defaultChannelTagStatsTrendLimit
	}
	if filter.TrendLimit > maxChannelTagStatsTrendLimit {
		filter.TrendLimit = maxChannelTagStatsTrendLimit
	}
	return filter, nil
}

type channelTagChannelAggregate struct {
	ChannelId        int   `gorm:"column:channel_id"`
	RequestCount     int64 `gorm:"column:request_count"`
	Quota            int64 `gorm:"column:quota"`
	PromptTokens     int64 `gorm:"column:prompt_tokens"`
	CompletionTokens int64 `gorm:"column:completion_tokens"`
	UseTimeSum       int64 `gorm:"column:use_time_sum"`
	LastLogAt        int64 `gorm:"column:last_log_at"`
}

type channelTagTrendAggregate struct {
	BucketStart      int64 `gorm:"column:bucket_start"`
	ChannelId        int   `gorm:"column:channel_id"`
	RequestCount     int64 `gorm:"column:request_count"`
	Quota            int64 `gorm:"column:quota"`
	PromptTokens     int64 `gorm:"column:prompt_tokens"`
	CompletionTokens int64 `gorm:"column:completion_tokens"`
}

type channelTagAccumulator struct {
	item              ChannelTagStatsItem
	useTimeSum        int64
	channels          map[int]*ChannelTagStatsChannelItem
	channelUseTimeSum map[int]int64
}

func applyChannelTagStatsLogFilter(tx *gorm.DB, filter ChannelTagStatsFilter) *gorm.DB {
	tx = tx.Where("logs.type = ?", LogTypeConsume)
	if filter.StartTimestamp > 0 {
		tx = tx.Where("logs.created_at >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp > 0 {
		tx = tx.Where("logs.created_at <= ?", filter.EndTimestamp)
	}
	return tx
}

func channelTagStatsBucketSize(granularity ChannelTagStatsGranularity) int64 {
	switch granularity {
	case ChannelTagStatsHour:
		return int64(time.Hour / time.Second)
	case ChannelTagStatsWeek:
		return int64(7 * 24 * time.Hour / time.Second)
	default:
		return int64(24 * time.Hour / time.Second)
	}
}

func channelTagStatsBucketExpression(granularity ChannelTagStatsGranularity) string {
	bucketSize := channelTagStatsBucketSize(granularity)
	if common.UsingMySQL {
		return fmt.Sprintf("FLOOR(logs.created_at / %d) * %d", bucketSize, bucketSize)
	}
	if common.UsingSQLite {
		return fmt.Sprintf("CAST(logs.created_at / %d AS INTEGER) * %d", bucketSize, bucketSize)
	}
	return fmt.Sprintf("(logs.created_at / %d) * %d", bucketSize, bucketSize)
}

func channelTagStatsAllMetadata() ([]ChannelTagMetadata, error) {
	return GetAllChannelTagMetadata()
}

func addChannelTagStatsTotals(acc *channelTagAccumulator, requestCount int64, quota int64, promptTokens int64, completionTokens int64, useTimeSum int64, lastLogAt int64) {
	acc.item.RequestCount += requestCount
	acc.item.Quota += quota
	acc.item.PromptTokens += promptTokens
	acc.item.CompletionTokens += completionTokens
	acc.item.Tokens += promptTokens + completionTokens
	acc.useTimeSum += useTimeSum
	if lastLogAt > acc.item.LastLogAt {
		acc.item.LastLogAt = lastLogAt
	}
}

func ensureChannelTagStatsAccumulator(accumulators map[string]*channelTagAccumulator, key string, name string) *channelTagAccumulator {
	acc, ok := accumulators[key]
	if ok {
		return acc
	}
	acc = &channelTagAccumulator{
		item: ChannelTagStatsItem{
			TagKey:  key,
			TagName: name,
		},
		channels:          make(map[int]*ChannelTagStatsChannelItem),
		channelUseTimeSum: make(map[int]int64),
	}
	accumulators[key] = acc
	return acc
}

func ensureChannelTagStatsChannel(acc *channelTagAccumulator, metadata ChannelTagMetadata) *ChannelTagStatsChannelItem {
	channel, ok := acc.channels[metadata.Id]
	if ok {
		return channel
	}
	channel = &ChannelTagStatsChannelItem{
		ChannelId:     metadata.Id,
		ChannelName:   metadata.Name,
		ChannelType:   metadata.Type,
		ChannelStatus: metadata.Status,
	}
	acc.channels[metadata.Id] = channel
	return channel
}

func addChannelTagStatsChannelTotals(acc *channelTagAccumulator, metadata ChannelTagMetadata, row channelTagChannelAggregate) {
	channel := ensureChannelTagStatsChannel(acc, metadata)
	channel.RequestCount += row.RequestCount
	channel.Quota += row.Quota
	channel.PromptTokens += row.PromptTokens
	channel.CompletionTokens += row.CompletionTokens
	channel.Tokens += row.PromptTokens + row.CompletionTokens
	acc.channelUseTimeSum[row.ChannelId] += row.UseTimeSum
	if row.LastLogAt > channel.LastLogAt {
		channel.LastLogAt = row.LastLogAt
	}
}

func sortedChannelTagStatsChannels(acc *channelTagAccumulator) []ChannelTagStatsChannelItem {
	channels := make([]ChannelTagStatsChannelItem, 0, len(acc.channels))
	for channelID, channel := range acc.channels {
		if channel.RequestCount > 0 {
			channel.AverageUseTime = float64(acc.channelUseTimeSum[channelID]) / float64(channel.RequestCount)
		}
		channels = append(channels, *channel)
	}
	sort.SliceStable(channels, func(i, j int) bool {
		if channels[i].Quota != channels[j].Quota {
			return channels[i].Quota > channels[j].Quota
		}
		if channels[i].RequestCount != channels[j].RequestCount {
			return channels[i].RequestCount > channels[j].RequestCount
		}
		if channels[i].LastLogAt != channels[j].LastLogAt {
			return channels[i].LastLogAt > channels[j].LastLogAt
		}
		if channels[i].ChannelName != channels[j].ChannelName {
			return channels[i].ChannelName < channels[j].ChannelName
		}
		return channels[i].ChannelId < channels[j].ChannelId
	})
	return channels
}

func sortedChannelTagStatsItems(accumulators map[string]*channelTagAccumulator) []ChannelTagStatsItem {
	items := make([]ChannelTagStatsItem, 0, len(accumulators))
	for _, acc := range accumulators {
		if acc.item.RequestCount > 0 {
			acc.item.AverageUseTime = float64(acc.useTimeSum) / float64(acc.item.RequestCount)
		}
		acc.item.Channels = sortedChannelTagStatsChannels(acc)
		acc.item.ChannelCount = len(acc.item.Channels)
		items = append(items, acc.item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Quota != items[j].Quota {
			return items[i].Quota > items[j].Quota
		}
		if items[i].RequestCount != items[j].RequestCount {
			return items[i].RequestCount > items[j].RequestCount
		}
		return items[i].TagName < items[j].TagName
	})
	return items
}

func topChannelTagKeysForTrend(items []ChannelTagStatsItem, limit int) map[string]struct{} {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	topKeys := make(map[string]struct{}, limit)
	for i := 0; i < limit; i++ {
		topKeys[items[i].TagKey] = struct{}{}
	}
	return topKeys
}

func channelTagForChannelID(channelID int, metadataByID map[int]ChannelTagMetadata) (key string, name string, exists bool) {
	metadata, ok := metadataByID[channelID]
	if !ok {
		return "", "", false
	}
	key, name = NormalizeChannelTag(metadata.Tag)
	return key, name, true
}

func channelTagMetadataForChannelID(channelID int, metadataByID map[int]ChannelTagMetadata) (ChannelTagMetadata, string, string, bool) {
	metadata, ok := metadataByID[channelID]
	if !ok {
		return ChannelTagMetadata{}, "", "", false
	}
	key, name := NormalizeChannelTag(metadata.Tag)
	return metadata, key, name, true
}

func GetChannelTagStats(ctx context.Context, filter ChannelTagStatsFilter) (*ChannelTagStatsResult, error) {
	var err error
	filter, err = NormalizeChannelTagStatsFilter(filter)
	if err != nil {
		return nil, err
	}

	var channelRows []channelTagChannelAggregate
	channelQuery := LOG_DB.WithContext(ctx).Table("logs").
		Select(`logs.channel_id,
			COUNT(*) AS request_count,
			COALESCE(SUM(logs.quota), 0) AS quota,
			COALESCE(SUM(logs.prompt_tokens), 0) AS prompt_tokens,
			COALESCE(SUM(logs.completion_tokens), 0) AS completion_tokens,
			COALESCE(SUM(logs.use_time), 0) AS use_time_sum,
			COALESCE(MAX(logs.created_at), 0) AS last_log_at`)
	channelQuery = applyChannelTagStatsLogFilter(channelQuery, filter)
	if err = channelQuery.Group("logs.channel_id").Scan(&channelRows).Error; err != nil {
		return nil, err
	}

	allMetadata, err := channelTagStatsAllMetadata()
	if err != nil {
		return nil, err
	}

	metadataByID := make(map[int]ChannelTagMetadata, len(allMetadata))
	accumulators := make(map[string]*channelTagAccumulator)
	for _, metadata := range allMetadata {
		if metadata.Id <= 0 {
			continue
		}
		metadataByID[metadata.Id] = metadata
		key, name := NormalizeChannelTag(metadata.Tag)
		acc := ensureChannelTagStatsAccumulator(accumulators, key, name)
		ensureChannelTagStatsChannel(acc, metadata)
	}

	summaryUseTimeSum := int64(0)
	for _, row := range channelRows {
		metadata, key, name, exists := channelTagMetadataForChannelID(row.ChannelId, metadataByID)
		if !exists {
			continue
		}
		acc := ensureChannelTagStatsAccumulator(accumulators, key, name)
		addChannelTagStatsTotals(acc, row.RequestCount, row.Quota, row.PromptTokens, row.CompletionTokens, row.UseTimeSum, row.LastLogAt)
		if row.ChannelId > 0 {
			addChannelTagStatsChannelTotals(acc, metadata, row)
		}
		summaryUseTimeSum += row.UseTimeSum
	}

	items := sortedChannelTagStatsItems(accumulators)
	result := &ChannelTagStatsResult{
		Items:       items,
		Trend:       make([]ChannelTagStatsTrendPoint, 0),
		Granularity: filter.Granularity,
	}
	for _, item := range items {
		result.Summary.TotalQuota += item.Quota
		result.Summary.RequestCount += item.RequestCount
		result.Summary.PromptTokens += item.PromptTokens
		result.Summary.CompletionTokens += item.CompletionTokens
		result.Summary.Tokens += item.Tokens
		result.Summary.ChannelCount += item.ChannelCount
		if item.TagKey == UntaggedChannelTagKey {
			result.Summary.UntaggedQuota = item.Quota
			result.Summary.UntaggedRequestCount = item.RequestCount
		} else {
			result.Summary.TagCount++
		}
	}
	result.Summary.TagGroupCount = len(items)
	if result.Summary.RequestCount > 0 {
		result.Summary.AverageUseTime = float64(summaryUseTimeSum) / float64(result.Summary.RequestCount)
	}

	var trendRows []channelTagTrendAggregate
	bucketExpr := channelTagStatsBucketExpression(filter.Granularity)
	trendQuery := LOG_DB.WithContext(ctx).Table("logs").
		Select(bucketExpr + ` AS bucket_start,
			logs.channel_id,
			COUNT(*) AS request_count,
			COALESCE(SUM(logs.quota), 0) AS quota,
			COALESCE(SUM(logs.prompt_tokens), 0) AS prompt_tokens,
			COALESCE(SUM(logs.completion_tokens), 0) AS completion_tokens`)
	trendQuery = applyChannelTagStatsLogFilter(trendQuery, filter)
	if err = trendQuery.Group(bucketExpr + ", logs.channel_id").Scan(&trendRows).Error; err != nil {
		return nil, err
	}

	topKeys := topChannelTagKeysForTrend(items, filter.TrendLimit)
	trendByBucketAndTag := make(map[int64]map[string]*ChannelTagStatsTrendPoint)
	for _, row := range trendRows {
		key, name, exists := channelTagForChannelID(row.ChannelId, metadataByID)
		if !exists {
			continue
		}
		if _, ok := topKeys[key]; !ok {
			key = "__other__"
			name = "其他"
		}
		if _, ok := trendByBucketAndTag[row.BucketStart]; !ok {
			trendByBucketAndTag[row.BucketStart] = make(map[string]*ChannelTagStatsTrendPoint)
		}
		point, ok := trendByBucketAndTag[row.BucketStart][key]
		if !ok {
			point = &ChannelTagStatsTrendPoint{
				BucketStart: row.BucketStart,
				TagKey:      key,
				TagName:     name,
			}
			trendByBucketAndTag[row.BucketStart][key] = point
		}
		point.RequestCount += row.RequestCount
		point.Quota += row.Quota
		point.PromptTokens += row.PromptTokens
		point.CompletionTokens += row.CompletionTokens
		point.Tokens += row.PromptTokens + row.CompletionTokens
	}

	for _, tags := range trendByBucketAndTag {
		for _, point := range tags {
			result.Trend = append(result.Trend, *point)
		}
	}
	sort.SliceStable(result.Trend, func(i, j int) bool {
		if result.Trend[i].BucketStart != result.Trend[j].BucketStart {
			return result.Trend[i].BucketStart < result.Trend[j].BucketStart
		}
		if result.Trend[i].TagKey == "__other__" || result.Trend[j].TagKey == "__other__" {
			return result.Trend[i].TagKey != "__other__"
		}
		return result.Trend[i].TagName < result.Trend[j].TagName
	})

	return result, nil
}

func formatUserLogs(logs []*Log, startIdx int) {
	for i := range logs {
		sanitizeUserLog(logs[i])
		logs[i].Id = startIdx + i + 1
	}
}

func formatUserLogsForExport(logs []*Log) {
	for i := range logs {
		sanitizeUserLog(logs[i])
	}
}

func sanitizeUserLog(log *Log) {
	log.ChannelName = ""
	var otherMap map[string]interface{}
	otherMap, _ = common.StrToMap(log.Other)
	if otherMap != nil {
		// Remove admin-only debug fields.
		delete(otherMap, "admin_info")
		// delete(otherMap, "reject_reason")
		delete(otherMap, "stream_status")
	}
	log.Other = common.MapToJsonStr(otherMap)
}

func GetLogByTokenId(tokenId int) (logs []*Log, err error) {
	err = LOG_DB.Model(&Log{}).Where("token_id = ?", tokenId).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs, 0)
	return logs, err
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// RecordLogWithAdminInfo 记录操作日志，并将管理员相关信息存入 Other.admin_info，
func RecordLogWithAdminInfo(userId int, logType int, content string, adminInfo map[string]interface{}) {
	RecordLogWithAdminInfoAndMetadata(userId, logType, content, 0, "", adminInfo)
}

// RecordLogWithAdminInfoAndMetadata 记录操作日志，并额外写入渠道、分组等可筛选字段。
func RecordLogWithAdminInfoAndMetadata(userId int, logType int, content string, channelId int, group string, adminInfo map[string]interface{}) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username := "system"
	if userId > 0 {
		username, _ = GetUsernameById(userId, false)
	}
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
		ChannelId: channelId,
		Group:     group,
	}
	if len(adminInfo) > 0 {
		other := map[string]interface{}{
			"admin_info": adminInfo,
		}
		log.Other = common.MapToJsonStr(other)
	}
	if err := LOG_DB.Create(log).Error; err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

func RecordTopupLog(userId int, content string, callerIp string, paymentMethod string, callbackPaymentMethod string) {
	username, _ := GetUsernameById(userId, false)
	adminInfo := map[string]interface{}{
		"server_ip":               common.GetIp(),
		"node_name":               common.NodeName,
		"caller_ip":               callerIp,
		"payment_method":          paymentMethod,
		"callback_payment_method": callbackPaymentMethod,
		"version":                 common.Version,
	}
	other := map[string]interface{}{
		"admin_info": adminInfo,
	}
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Ip:        callerIp,
		Other:     common.MapToJsonStr(other),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record topup log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, common.LocalLogPreview(content)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	otherStr := common.MapToJsonStr(params.Other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(userId, username, params.ModelName, params.Quota, common.GetTimestamp(), params.PromptTokens+params.CompletionTokens)
		})
	}
}

type RecordTaskBillingLogParams struct {
	UserId    int
	LogType   int
	Content   string
	ChannelId int
	ModelName string
	Quota     int
	TokenId   int
	Group     string
	Other     map[string]interface{}
}

func RecordTaskBillingLog(params RecordTaskBillingLogParams) {
	if params.LogType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(params.UserId, false)
	tokenName := ""
	if params.TokenId > 0 {
		if token, err := GetTokenById(params.TokenId); err == nil {
			tokenName = token.Name
		}
	}
	log := &Log{
		UserId:    params.UserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      params.LogType,
		Content:   params.Content,
		TokenName: tokenName,
		ModelName: params.ModelName,
		Quota:     params.Quota,
		ChannelId: params.ChannelId,
		TokenId:   params.TokenId,
		Group:     params.Group,
		Other:     common.MapToJsonStr(params.Other),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record task billing log: " + err.Error())
	}
}

func buildAdminLogQuery(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, requestId string, upstreamRequestId string) (*gorm.DB, error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("logs.type = ?", logType)
	}

	var err error
	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", username); err != nil {
		return nil, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	return tx, nil
}

func fillLogChannelNames(logs []*Log) error {
	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() == 0 {
		return nil
	}

	var channels []struct {
		Id   int    `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	if common.MemoryCacheEnabled {
		// Cache get channel
		for _, channelId := range channelIds.Items() {
			if cacheChannel, err := CacheGetChannel(channelId); err == nil {
				channels = append(channels, struct {
					Id   int    `gorm:"column:id"`
					Name string `gorm:"column:name"`
				}{
					Id:   channelId,
					Name: cacheChannel.Name,
				})
			}
		}
	} else {
		// Bulk query channels from DB
		if err := DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
			return err
		}
	}
	channelMap := make(map[int]string, len(channels))
	for _, channel := range channels {
		channelMap[channel.Id] = channel.Name
	}
	for i := range logs {
		logs[i].ChannelName = channelMap[logs[i].ChannelId]
	}
	return nil
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	tx, err := buildAdminLogQuery(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, requestId, upstreamRequestId)
	if err != nil {
		return nil, 0, err
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.created_at desc, logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}
	if err = fillLogChannelNames(logs); err != nil {
		return logs, total, err
	}

	return logs, total, err
}

const logSearchCountLimit = 10000

func buildUserLogQuery(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, group string, requestId string, upstreamRequestId string) (*gorm.DB, error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("logs.user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("logs.user_id = ? and logs.type = ?", userId, logType)
	}

	var err error
	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	return tx, nil
}

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	tx, err := buildUserLogQuery(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, group, requestId, upstreamRequestId)
	if err != nil {
		return nil, 0, err
	}
	err = tx.Model(&Log{}).Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}

	formatUserLogs(logs, startIdx)
	return logs, total, err
}

type LogExportFilter struct {
	UserId            int
	IsAdmin           bool
	LogType           int
	StartTimestamp    int64
	EndTimestamp      int64
	ModelName         string
	Username          string
	TokenName         string
	Channel           int
	Group             string
	RequestId         string
	UpstreamRequestId string
}

func buildLogExportQuery(filter LogExportFilter) (*gorm.DB, error) {
	if filter.IsAdmin {
		return buildAdminLogQuery(filter.LogType, filter.StartTimestamp, filter.EndTimestamp, filter.ModelName, filter.Username, filter.TokenName, filter.Channel, filter.Group, filter.RequestId, filter.UpstreamRequestId)
	}
	return buildUserLogQuery(filter.UserId, filter.LogType, filter.StartTimestamp, filter.EndTimestamp, filter.ModelName, filter.TokenName, filter.Group, filter.RequestId, filter.UpstreamRequestId)
}

func CountLogsForExport(ctx context.Context, filter LogExportFilter) (int64, error) {
	tx, err := buildLogExportQuery(filter)
	if err != nil {
		return 0, err
	}
	var total int64
	if err = tx.WithContext(ctx).Model(&Log{}).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func GetLogsForExportBatch(ctx context.Context, filter LogExportFilter, lastCreatedAt int64, lastID int, limit int, startIdx int) (logs []*Log, err error) {
	if limit <= 0 {
		return []*Log{}, nil
	}
	tx, err := buildLogExportQuery(filter)
	if err != nil {
		return nil, err
	}
	if lastCreatedAt > 0 || lastID > 0 {
		tx = tx.Where("logs.created_at < ? OR (logs.created_at = ? AND logs.id < ?)", lastCreatedAt, lastCreatedAt, lastID)
	}
	err = tx.WithContext(ctx).Order("logs.created_at desc, logs.id desc").Limit(limit).Find(&logs).Error
	if err != nil {
		return nil, err
	}
	if filter.IsAdmin {
		if err = fillLogChannelNames(logs); err != nil {
			return logs, err
		}
	} else {
		formatUserLogsForExport(logs)
	}
	return logs, nil
}

type Stat struct {
	Quota int `json:"quota"`
	Rpm   int `json:"rpm"`
	Tpm   int `json:"tpm"`
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (stat Stat, err error) {
	tx := LOG_DB.Table("logs").Select("sum(quota) quota")

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, sum(prompt_tokens) + sum(completion_tokens) tpm")

	if tx, err = applyExplicitLogTextFilter(tx, "username", username); err != nil {
		return stat, err
	}
	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "username", username); err != nil {
		return stat, err
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if tx, err = applyExplicitLogTextFilter(tx, "model_name", modelName); err != nil {
		return stat, err
	}
	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "model_name", modelName); err != nil {
		return stat, err
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
	}

	tx = tx.Where("type = ?", LogTypeConsume)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 执行查询
	if err := tx.Scan(&stat).Error; err != nil {
		common.SysError("failed to query log stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	if err := rpmTpmQuery.Scan(&stat).Error; err != nil {
		common.SysError("failed to query rpm/tpm stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}

	return stat, nil
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}
