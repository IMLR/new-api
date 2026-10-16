// Package resethint reads the time at which an upstream account can serve a
// model again. Providers state it in different ways and not always with a 429:
// a relative wait ("Try again in 20h 4m", "Retry after 55s", "请 30 秒后重试"),
// an absolute reset time ("your usage will reset at 2026-10-17 18:09:40 UTC+8")
// or the standard Retry-After header.
package resethint

import (
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Maximum bounds every window, so a misread or absurd hint cannot take an
// account out of rotation for longer than a weekly quota period.
const Maximum = 7 * 24 * time.Hour

var (
	relativeTrigger = regexp.MustCompile(`(?i)\b(?:try again|retry|resets?|available)\s+(?:in|after)\s+`)
	relativeUnit    = regexp.MustCompile(`(?i)^[\s,]*(?:and\s+)?(\d+(?:\.\d+)?)\s*(milliseconds?|ms|seconds?|secs?|minutes?|mins?|hours?|hrs?|days?|s|m|h|d)`)
	chineseRelative = regexp.MustCompile(`((?:\d+(?:\.\d+)?\s*(?:毫秒|秒钟|秒鐘|秒|分钟|分鐘|分|个小时|個小時|小时|小時|天)\s*)+)[后後]\s*(?:再|可|可以)?(?:重试|重試|试|試|恢复|恢復|重置|可用|解除|使用)`)
	chineseUnit     = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(毫秒|秒钟|秒鐘|秒|分钟|分鐘|分|个小时|個小時|小时|小時|天)`)
	absoluteTime    = regexp.MustCompile(`(\d{4})[-/](\d{1,2})[-/](\d{1,2})[ T](\d{1,2}):(\d{2})(?::(\d{2}))?(?:\.\d+)?\s*(Z|(?i:UTC|GMT))?\s*(?:([+-])(\d{1,2})(?::?(\d{2}))?)?`)
	resetBefore     = regexp.MustCompile(`(?i)(?:reset|renew|retry|try again|available|until|恢复|恢復|重置|解除|可用|重试|重試)[^0-9]{0,24}$`)
	resetAfter      = regexp.MustCompile(`^[^0-9]{0,8}(?:后|後|之后|之後|以后|以後)?\s*(?:恢复|恢復|重置|解除|可用|重试|重試)`)
)

// Parse returns the reset time stated by an upstream error message or its
// Retry-After header. When both state one, the later time wins. Times without
// a zone use the server's local zone; reset times in the past are ignored.
func Parse(message, retryAfter string, now time.Time) (time.Time, bool) {
	until, ok := fromMessage(message, now)
	if header, headerOK := FromRetryAfter(retryAfter, now); headerOK && (!ok || header.After(until)) {
		until, ok = header, true
	}
	return until, ok
}

// FromRetryAfter reads a Retry-After header holding seconds or an HTTP date
// (RFC 9110, section 10.2.3).
func FromRetryAfter(value string, now time.Time) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		return afterWindow(now, seconds*float64(time.Second))
	}
	if date, err := http.ParseTime(value); err == nil {
		return atTime(now, date)
	}
	return time.Time{}, false
}

func fromMessage(message string, now time.Time) (time.Time, bool) {
	if message == "" {
		return time.Time{}, false
	}
	var latest time.Time
	found := false
	keep := func(until time.Time, ok bool) {
		if ok && (!found || until.After(latest)) {
			latest, found = until, true
		}
	}
	for _, position := range relativeTrigger.FindAllStringIndex(message, -1) {
		keep(afterWindow(now, englishWindow(message[position[1]:])))
	}
	for _, match := range chineseRelative.FindAllStringSubmatch(message, -1) {
		keep(afterWindow(now, chineseWindow(match[1])))
	}
	for _, match := range absoluteTime.FindAllStringSubmatchIndex(message, -1) {
		before := message[max(0, match[0]-48):match[0]]
		after := message[match[1]:min(len(message), match[1]+24)]
		if !resetBefore.MatchString(before) && !resetAfter.MatchString(after) {
			continue
		}
		if resetAt, ok := parseAbsolute(message, match); ok {
			keep(atTime(now, resetAt))
		}
	}
	return latest, found
}

// englishWindow adds up the adjacent amounts after a trigger such as
// "1d 2h 30m", "1h30m" or "2.5s". A unit that runs into more letters ("3
// models") ends the window.
func englishWindow(text string) float64 {
	var window float64
	for {
		match := relativeUnit.FindStringSubmatchIndex(text)
		if match == nil {
			return window
		}
		end := match[1]
		if end < len(text) && isLetter(text[end]) {
			return window
		}
		amount, _ := strconv.ParseFloat(text[match[2]:match[3]], 64)
		window += amount * float64(unitScale(strings.ToLower(text[match[4]:match[5]])))
		text = text[end:]
	}
}

func chineseWindow(text string) float64 {
	var window float64
	for _, match := range chineseUnit.FindAllStringSubmatch(text, -1) {
		amount, _ := strconv.ParseFloat(match[1], 64)
		window += amount * float64(unitScale(match[2]))
	}
	return window
}

func unitScale(unit string) time.Duration {
	switch {
	case unit == "ms" || strings.HasPrefix(unit, "millisecond") || unit == "毫秒":
		return time.Millisecond
	case strings.HasPrefix(unit, "s") || strings.HasPrefix(unit, "秒"):
		return time.Second
	case strings.HasPrefix(unit, "m") || strings.HasPrefix(unit, "分"):
		return time.Minute
	case strings.HasPrefix(unit, "h") || strings.HasSuffix(unit, "时") || strings.HasSuffix(unit, "時"):
		return time.Hour
	default:
		return 24 * time.Hour
	}
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// parseAbsolute converts one absoluteTime match. An explicit offset wins over
// a bare UTC/GMT/Z, and a time without either uses the server's zone.
func parseAbsolute(message string, match []int) (time.Time, bool) {
	group := func(i int) string {
		if match[2*i] < 0 {
			return ""
		}
		return message[match[2*i]:match[2*i+1]]
	}
	parts := make([]int, 6)
	for i := range parts {
		parts[i], _ = strconv.Atoi(group(i + 1))
	}
	zone := time.Local
	if sign := group(8); sign != "" {
		hours, _ := strconv.Atoi(group(9))
		minutes, _ := strconv.Atoi(group(10))
		if hours > 14 || minutes > 59 {
			return time.Time{}, false
		}
		offset := hours*3600 + minutes*60
		if sign == "-" {
			offset = -offset
		}
		zone = time.FixedZone("", offset)
	} else if group(7) != "" {
		zone = time.UTC
	}
	if parts[1] < 1 || parts[1] > 12 || parts[2] < 1 || parts[2] > 31 || parts[3] > 23 || parts[4] > 59 || parts[5] > 60 {
		return time.Time{}, false
	}
	return time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], 0, zone), true
}

func afterWindow(now time.Time, window float64) (time.Time, bool) {
	if math.IsNaN(window) || window <= 0 {
		return time.Time{}, false
	}
	if window >= float64(Maximum) {
		return now.Add(Maximum), true
	}
	return now.Add(time.Duration(window)), true
}

func atTime(now, resetAt time.Time) (time.Time, bool) {
	if !resetAt.After(now) {
		return time.Time{}, false
	}
	if resetAt.Sub(now) > Maximum {
		return now.Add(Maximum), true
	}
	return resetAt, true
}
