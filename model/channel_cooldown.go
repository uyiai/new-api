package model

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// channelCooldownUntil maps channelId -> unix-milli timestamp until which the
// channel should be skipped during selection.
//
// It is driven purely by upstream RATE-LIMIT signals (HTTP 429 / Anthropic
// ratelimit headers), so a saturated channel rests until its rate bucket
// refills instead of being hammered again. It is intentionally:
//   - in-memory and short-lived (single node; losing it on restart is harmless),
//   - NOT a disable: credit is preserved and the channel keeps draining once it
//     recovers, so it never reduces total extraction ("超刷").
var channelCooldownUntil sync.Map // map[int]int64

// SetChannelCooldownUntil marks a channel as cooling down until untilMs
// (unix-milli). A timestamp in the past is a no-op.
func SetChannelCooldownUntil(channelId int, untilMs int64) {
	if untilMs <= time.Now().UnixMilli() {
		return
	}
	channelCooldownUntil.Store(channelId, untilMs)
}

// IsChannelCoolingDown reports whether the channel is currently cooling down,
// lazily clearing entries that have already expired.
func IsChannelCoolingDown(channelId int) bool {
	v, ok := channelCooldownUntil.Load(channelId)
	if !ok {
		return false
	}
	until, _ := v.(int64)
	if time.Now().UnixMilli() >= until {
		channelCooldownUntil.Delete(channelId)
		return false
	}
	return true
}

const (
	headerRetryAfter           = "Retry-After"
	headerReqRemaining         = "Anthropic-Ratelimit-Requests-Remaining"
	headerReqReset             = "Anthropic-Ratelimit-Requests-Reset"
	headerInputTokensRemaining = "Anthropic-Ratelimit-Input-Tokens-Remaining"
	headerInputTokensReset     = "Anthropic-Ratelimit-Input-Tokens-Reset"
	headerTokensRemaining      = "Anthropic-Ratelimit-Tokens-Remaining"
	headerTokensReset          = "Anthropic-Ratelimit-Tokens-Reset"
)

// ApplyUpstreamRateLimitHeaders inspects an upstream response's rate-limit
// headers and puts the channel into cooldown until the upstream's exact reset
// moment when it is (429) or is about to be (remaining≈0) rate limited.
//
// It only reacts to RATE limits, never to credit exhaustion (400 credit too
// low), so the channel keeps draining its credit — cooldown paces speed, not
// total volume.
func ApplyUpstreamRateLimitHeaders(channelId int, statusCode int, header http.Header) {
	if !common.ChannelCooldownEnabled || header == nil || channelId <= 0 {
		return
	}
	nowMs := time.Now().UnixMilli()
	clamp := func(ms int64) int64 {
		if common.ChannelCooldownMaxSeconds > 0 {
			capMs := nowMs + int64(common.ChannelCooldownMaxSeconds)*1000
			if ms > capMs {
				return capMs
			}
		}
		return ms
	}

	// Reactive: rate limited now → rest until the upstream says it recovers.
	if statusCode == http.StatusTooManyRequests {
		until := nowMs
		if ra := parseRetryAfterMs(header.Get(headerRetryAfter), nowMs); ra > until {
			until = ra
		}
		for _, h := range []string{headerReqReset, headerInputTokensReset, headerTokensReset} {
			if t := parseResetMs(header.Get(h), nowMs); t > until {
				until = t
			}
		}
		if until > nowMs {
			SetChannelCooldownUntil(channelId, clamp(until))
		}
		return
	}

	// Proactive: a non-429 response whose rate bucket is nearly empty → rest
	// until that bucket resets, before the next request trips a 429.
	if !common.ChannelCooldownProactiveEnabled {
		return
	}
	if thr := common.ChannelCooldownMinRequestsRemaining; thr >= 0 {
		if rem, ok := parseInt(header.Get(headerReqRemaining)); ok && rem <= thr {
			if t := parseResetMs(header.Get(headerReqReset), nowMs); t > nowMs {
				SetChannelCooldownUntil(channelId, clamp(t))
				return
			}
		}
	}
	if thr := common.ChannelCooldownMinInputTokensRemaining; thr > 0 {
		if rem, ok := parseInt(header.Get(headerInputTokensRemaining)); ok && rem <= thr {
			if t := parseResetMs(header.Get(headerInputTokensReset), nowMs); t > nowMs {
				SetChannelCooldownUntil(channelId, clamp(t))
				return
			}
		}
	}
}

func parseInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// parseResetMs parses an Anthropic *-reset header, which is an RFC3339 timestamp
// (e.g. "2026-06-29T12:00:00Z"); some providers send an integer seconds-until.
func parseResetMs(s string, nowMs int64) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli()
	}
	if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
		return nowMs + int64(secs)*1000
	}
	return 0
}

// parseRetryAfterMs parses a Retry-After header (integer seconds, or HTTP-date).
func parseRetryAfterMs(s string, nowMs int64) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
		return nowMs + int64(secs)*1000
	}
	if t, err := http.ParseTime(s); err == nil {
		return t.UnixMilli()
	}
	return 0
}
