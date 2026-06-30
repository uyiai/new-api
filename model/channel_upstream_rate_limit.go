package model

import (
	"crypto/sha256"
	"encoding/hex"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm/clause"
)

const (
	upstreamRateLimitProviderAnthropic = "anthropic"
	upstreamRateLimitFlushInterval     = 2 * time.Second
	upstreamRateLimitFlushBatchSize    = 100
)

const (
	headerReqLimit              = "Anthropic-Ratelimit-Requests-Limit"
	headerInputTokensLimit      = "Anthropic-Ratelimit-Input-Tokens-Limit"
	headerOutputTokensLimit     = "Anthropic-Ratelimit-Output-Tokens-Limit"
	headerOutputTokensRemaining = "Anthropic-Ratelimit-Output-Tokens-Remaining"
	headerOutputTokensReset     = "Anthropic-Ratelimit-Output-Tokens-Reset"
	headerTokensLimit           = "Anthropic-Ratelimit-Tokens-Limit"
)

type ChannelUpstreamRateLimitStatus struct {
	Id int `json:"id"`

	Provider    string `json:"provider" gorm:"type:varchar(32);uniqueIndex:idx_channel_upstream_rate_limit_identity,priority:1;index"`
	ChannelType int    `json:"channel_type" gorm:"uniqueIndex:idx_channel_upstream_rate_limit_identity,priority:2;index"`
	BaseURL     string `json:"base_url" gorm:"type:varchar(255);uniqueIndex:idx_channel_upstream_rate_limit_identity,priority:3"`
	KeyHash     string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_channel_upstream_rate_limit_identity,priority:4;index"`

	LastChannelId int    `json:"last_channel_id" gorm:"index"`
	StatusCode    int    `json:"status_code"`
	RequestId     string `json:"request_id" gorm:"type:varchar(128)"`

	RequestsLimit     *int  `json:"requests_limit,omitempty"`
	RequestsRemaining *int  `json:"requests_remaining,omitempty"`
	RequestsResetTime int64 `json:"requests_reset_time" gorm:"bigint;default:0"`

	InputTokensLimit     *int  `json:"input_tokens_limit,omitempty"`
	InputTokensRemaining *int  `json:"input_tokens_remaining,omitempty"`
	InputTokensResetTime int64 `json:"input_tokens_reset_time" gorm:"bigint;default:0"`

	OutputTokensLimit     *int  `json:"output_tokens_limit,omitempty"`
	OutputTokensRemaining *int  `json:"output_tokens_remaining,omitempty"`
	OutputTokensResetTime int64 `json:"output_tokens_reset_time" gorm:"bigint;default:0"`

	TokensLimit     *int  `json:"tokens_limit,omitempty"`
	TokensRemaining *int  `json:"tokens_remaining,omitempty"`
	TokensResetTime int64 `json:"tokens_reset_time" gorm:"bigint;default:0"`

	RetryAfterSeconds *int  `json:"retry_after_seconds,omitempty"`
	CooldownUntil     int64 `json:"cooldown_until" gorm:"bigint;default:0"`
	ObservedTime      int64 `json:"observed_time" gorm:"bigint;index"`
	UpdatedTime       int64 `json:"updated_time" gorm:"bigint"`
}

var (
	upstreamRateLimitStatusStore sync.Map // identity -> ChannelUpstreamRateLimitStatus
	upstreamRateLimitDirtyStore  sync.Map // identity -> struct{}
	upstreamRateLimitFlushOnce   sync.Once
	upstreamRateLimitLocks       [64]sync.Mutex
)

func channelUpstreamRateLimitLock(identity string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(identity))
	return &upstreamRateLimitLocks[int(h.Sum32())%len(upstreamRateLimitLocks)]
}

func normalizeUpstreamRateLimitBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func channelUpstreamRateLimitKeyHash(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}

func channelUpstreamRateLimitIdentity(provider string, channelType int, baseURL string, keyHash string) string {
	return provider + "\x00" + strconv.Itoa(channelType) + "\x00" + normalizeUpstreamRateLimitBaseURL(baseURL) + "\x00" + keyHash
}

func buildChannelUpstreamRateLimitIdentity(channelType int, baseURL string, key string) (provider string, normalizedBaseURL string, keyHash string, ok bool) {
	if channelType != constant.ChannelTypeAnthropic {
		return "", "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", "", false
	}
	return upstreamRateLimitProviderAnthropic, normalizeUpstreamRateLimitBaseURL(baseURL), channelUpstreamRateLimitKeyHash(key), true
}

func intHeader(header http.Header, name string) (*int, bool) {
	value := strings.TrimSpace(header.Get(name))
	if value == "" {
		return nil, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil, false
	}
	return &parsed, true
}

func resetHeaderMs(header http.Header, name string, nowMs int64) (int64, bool) {
	value := strings.TrimSpace(header.Get(name))
	if value == "" {
		return 0, false
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UnixMilli(), true
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return nowMs + int64(seconds)*1000, true
	}
	return 0, false
}

func retryAfterSeconds(header http.Header, now time.Time) (*int, int64, bool) {
	value := strings.TrimSpace(header.Get(headerRetryAfter))
	if value == "" {
		return nil, 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return &seconds, now.Add(time.Duration(seconds) * time.Second).UnixMilli(), true
	}
	if t, err := http.ParseTime(value); err == nil {
		delta := int(t.Sub(now).Seconds())
		if delta < 0 {
			delta = 0
		}
		return &delta, t.UnixMilli(), true
	}
	return nil, 0, false
}

func maxPositiveInt64(values ...int64) int64 {
	var max int64
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

func mergeOptionalInt(next **int, previous *int) {
	if *next == nil && previous != nil {
		value := *previous
		*next = &value
	}
}

func mergeUpstreamRateLimitStatus(current ChannelUpstreamRateLimitStatus, next ChannelUpstreamRateLimitStatus) ChannelUpstreamRateLimitStatus {
	mergeOptionalInt(&next.RequestsLimit, current.RequestsLimit)
	mergeOptionalInt(&next.RequestsRemaining, current.RequestsRemaining)
	mergeOptionalInt(&next.InputTokensLimit, current.InputTokensLimit)
	mergeOptionalInt(&next.InputTokensRemaining, current.InputTokensRemaining)
	mergeOptionalInt(&next.OutputTokensLimit, current.OutputTokensLimit)
	mergeOptionalInt(&next.OutputTokensRemaining, current.OutputTokensRemaining)
	mergeOptionalInt(&next.TokensLimit, current.TokensLimit)
	mergeOptionalInt(&next.TokensRemaining, current.TokensRemaining)
	mergeOptionalInt(&next.RetryAfterSeconds, current.RetryAfterSeconds)

	next.RequestsResetTime = maxPositiveInt64(next.RequestsResetTime, current.RequestsResetTime)
	next.InputTokensResetTime = maxPositiveInt64(next.InputTokensResetTime, current.InputTokensResetTime)
	next.OutputTokensResetTime = maxPositiveInt64(next.OutputTokensResetTime, current.OutputTokensResetTime)
	next.TokensResetTime = maxPositiveInt64(next.TokensResetTime, current.TokensResetTime)
	next.CooldownUntil = maxPositiveInt64(next.CooldownUntil, current.CooldownUntil)
	return next
}

func UpdateChannelUpstreamRateLimitStatus(channelId int, channelType int, baseURL string, key string, statusCode int, header http.Header) {
	if header == nil || channelId <= 0 {
		return
	}
	provider, normalizedBaseURL, keyHash, ok := buildChannelUpstreamRateLimitIdentity(channelType, baseURL, key)
	if !ok {
		return
	}

	now := time.Now()
	nowMs := now.UnixMilli()
	status := ChannelUpstreamRateLimitStatus{
		Provider:      provider,
		ChannelType:   channelType,
		BaseURL:       normalizedBaseURL,
		KeyHash:       keyHash,
		LastChannelId: channelId,
		StatusCode:    statusCode,
		RequestId:     header.Get(common.RequestIdKey),
		ObservedTime:  nowMs,
		UpdatedTime:   nowMs,
	}

	var found bool
	if value, ok := intHeader(header, headerReqLimit); ok {
		status.RequestsLimit = value
		found = true
	}
	if value, ok := intHeader(header, headerReqRemaining); ok {
		status.RequestsRemaining = value
		found = true
	}
	if value, ok := resetHeaderMs(header, headerReqReset, nowMs); ok {
		status.RequestsResetTime = value
		found = true
	}
	if value, ok := intHeader(header, headerInputTokensLimit); ok {
		status.InputTokensLimit = value
		found = true
	}
	if value, ok := intHeader(header, headerInputTokensRemaining); ok {
		status.InputTokensRemaining = value
		found = true
	}
	if value, ok := resetHeaderMs(header, headerInputTokensReset, nowMs); ok {
		status.InputTokensResetTime = value
		found = true
	}
	if value, ok := intHeader(header, headerOutputTokensLimit); ok {
		status.OutputTokensLimit = value
		found = true
	}
	if value, ok := intHeader(header, headerOutputTokensRemaining); ok {
		status.OutputTokensRemaining = value
		found = true
	}
	if value, ok := resetHeaderMs(header, headerOutputTokensReset, nowMs); ok {
		status.OutputTokensResetTime = value
		found = true
	}
	if value, ok := intHeader(header, headerTokensLimit); ok {
		status.TokensLimit = value
		found = true
	}
	if value, ok := intHeader(header, headerTokensRemaining); ok {
		status.TokensRemaining = value
		found = true
	}
	if value, ok := resetHeaderMs(header, headerTokensReset, nowMs); ok {
		status.TokensResetTime = value
		found = true
	}
	if seconds, retryUntil, ok := retryAfterSeconds(header, now); ok {
		status.RetryAfterSeconds = seconds
		status.CooldownUntil = retryUntil
		found = true
	}
	if statusCode == http.StatusTooManyRequests {
		status.CooldownUntil = maxPositiveInt64(status.CooldownUntil, status.RequestsResetTime, status.InputTokensResetTime, status.OutputTokensResetTime, status.TokensResetTime)
		found = true
	} else if common.ChannelCooldownProactiveEnabled {
		if common.ChannelCooldownMinRequestsRemaining >= 0 && status.RequestsRemaining != nil && *status.RequestsRemaining <= common.ChannelCooldownMinRequestsRemaining {
			status.CooldownUntil = maxPositiveInt64(status.CooldownUntil, status.RequestsResetTime)
		}
		if common.ChannelCooldownMinInputTokensRemaining > 0 && status.InputTokensRemaining != nil && *status.InputTokensRemaining <= common.ChannelCooldownMinInputTokensRemaining {
			status.CooldownUntil = maxPositiveInt64(status.CooldownUntil, status.InputTokensResetTime)
		}
	}

	if !found {
		return
	}
	identity := channelUpstreamRateLimitIdentity(provider, channelType, normalizedBaseURL, keyHash)
	lock := channelUpstreamRateLimitLock(identity)
	lock.Lock()
	defer lock.Unlock()
	if currentValue, ok := upstreamRateLimitStatusStore.Load(identity); ok {
		if current, ok := currentValue.(ChannelUpstreamRateLimitStatus); ok {
			status = mergeUpstreamRateLimitStatus(current, status)
		}
	}
	upstreamRateLimitStatusStore.Store(identity, status)
	upstreamRateLimitDirtyStore.Store(identity, struct{}{})
}

func collectDirtyChannelUpstreamRateLimitStatuses() []ChannelUpstreamRateLimitStatus {
	statuses := make([]ChannelUpstreamRateLimitStatus, 0)
	upstreamRateLimitDirtyStore.Range(func(key, value any) bool {
		identity, ok := key.(string)
		if !ok {
			return true
		}
		upstreamRateLimitDirtyStore.Delete(identity)
		if statusValue, ok := upstreamRateLimitStatusStore.Load(identity); ok {
			if status, ok := statusValue.(ChannelUpstreamRateLimitStatus); ok {
				statuses = append(statuses, status)
			}
		}
		return true
	})
	return statuses
}

func persistChannelUpstreamRateLimitStatuses(statuses []ChannelUpstreamRateLimitStatus) error {
	if len(statuses) == 0 || DB == nil {
		return nil
	}
	for start := 0; start < len(statuses); start += upstreamRateLimitFlushBatchSize {
		end := start + upstreamRateLimitFlushBatchSize
		if end > len(statuses) {
			end = len(statuses)
		}
		chunk := statuses[start:end]
		if err := DB.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "provider"},
				{Name: "channel_type"},
				{Name: "base_url"},
				{Name: "key_hash"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"last_channel_id",
				"status_code",
				"request_id",
				"requests_limit",
				"requests_remaining",
				"requests_reset_time",
				"input_tokens_limit",
				"input_tokens_remaining",
				"input_tokens_reset_time",
				"output_tokens_limit",
				"output_tokens_remaining",
				"output_tokens_reset_time",
				"tokens_limit",
				"tokens_remaining",
				"tokens_reset_time",
				"retry_after_seconds",
				"cooldown_until",
				"observed_time",
				"updated_time",
			}),
		}).Create(&chunk).Error; err != nil {
			return err
		}
	}
	return nil
}

func FlushChannelUpstreamRateLimitStatusOnce() {
	statuses := collectDirtyChannelUpstreamRateLimitStatuses()
	if err := persistChannelUpstreamRateLimitStatuses(statuses); err != nil {
		common.SysLog("failed to flush upstream rate limit statuses: " + err.Error())
		for _, status := range statuses {
			identity := channelUpstreamRateLimitIdentity(status.Provider, status.ChannelType, status.BaseURL, status.KeyHash)
			upstreamRateLimitDirtyStore.Store(identity, struct{}{})
		}
	}
}

func StartChannelUpstreamRateLimitStatusFlushTask() {
	upstreamRateLimitFlushOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(upstreamRateLimitFlushInterval)
			defer ticker.Stop()
			for range ticker.C {
				FlushChannelUpstreamRateLimitStatusOnce()
			}
		}()
	})
}

func AttachChannelUpstreamRateLimitStatuses(channels []*Channel) error {
	if len(channels) == 0 {
		return nil
	}
	identityToChannels := make(map[string][]*Channel)
	keyHashes := make([]string, 0, len(channels))
	seenHashes := make(map[string]bool, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		keys := channel.GetKeys()
		if len(keys) != 1 {
			continue
		}
		provider, baseURL, keyHash, ok := buildChannelUpstreamRateLimitIdentity(channel.Type, channel.GetBaseURL(), keys[0])
		if !ok {
			continue
		}
		identity := channelUpstreamRateLimitIdentity(provider, channel.Type, baseURL, keyHash)
		identityToChannels[identity] = append(identityToChannels[identity], channel)
		if !seenHashes[keyHash] {
			seenHashes[keyHash] = true
			keyHashes = append(keyHashes, keyHash)
		}
	}
	if len(keyHashes) == 0 {
		return nil
	}

	var statuses []ChannelUpstreamRateLimitStatus
	if err := DB.Where("provider = ? AND key_hash IN ?", upstreamRateLimitProviderAnthropic, keyHashes).Find(&statuses).Error; err != nil {
		return err
	}
	for i := range statuses {
		status := statuses[i]
		identity := channelUpstreamRateLimitIdentity(status.Provider, status.ChannelType, status.BaseURL, status.KeyHash)
		for _, channel := range identityToChannels[identity] {
			statusCopy := status
			channel.UpstreamRateLimitStatus = &statusCopy
		}
	}
	upstreamRateLimitStatusStore.Range(func(key, value any) bool {
		identity, ok := key.(string)
		if !ok {
			return true
		}
		channelsForIdentity := identityToChannels[identity]
		if len(channelsForIdentity) == 0 {
			return true
		}
		status, ok := value.(ChannelUpstreamRateLimitStatus)
		if !ok {
			return true
		}
		for _, channel := range channelsForIdentity {
			statusCopy := status
			channel.UpstreamRateLimitStatus = &statusCopy
		}
		return true
	})
	return nil
}
