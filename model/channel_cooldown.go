package model

import (
	"sort"
	"sync"
	"time"
)

// channelModelCooldownKey identifies one model route on one channel.
type channelModelCooldownKey struct {
	channelId int
	modelName string
}

type channelModelCooldownStore struct {
	mu      sync.RWMutex
	expires map[channelModelCooldownKey]channelModelCooldown
}

type channelModelCooldown struct {
	until      time.Time
	reason     string
	recordedAt time.Time
}

// ChannelModelCooldown is one stored quota window for a channel and model.
type ChannelModelCooldown struct {
	ModelName  string
	Until      time.Time
	Reason     string
	RecordedAt time.Time
}

// channelModelCooldowns remembers upstream quota windows that belong to a
// single account and model. Cline free routes report the reset time inside the
// error message; remembering that window lets channel selection skip accounts
// that cannot serve the model right now instead of spending a retry on every
// request.
var channelModelCooldowns = &channelModelCooldownStore{
	expires: make(map[channelModelCooldownKey]channelModelCooldown),
}

// MarkChannelModelCooldown records that a channel cannot serve a model until
// the given time. A time that is not in the future clears the entry. The reason
// keeps the upstream message that produced the window so the channel view can
// explain why an account is resting.
func MarkChannelModelCooldown(channelId int, modelName string, until time.Time, reason string) {
	if channelId <= 0 || modelName == "" {
		return
	}
	key := channelModelCooldownKey{channelId: channelId, modelName: modelName}
	now := time.Now()
	channelModelCooldowns.mu.Lock()
	defer channelModelCooldowns.mu.Unlock()
	channelModelCooldowns.dropExpiredLocked(now)
	if !until.After(now) {
		delete(channelModelCooldowns.expires, key)
		return
	}
	channelModelCooldowns.expires[key] = channelModelCooldown{
		until:      until,
		reason:     reason,
		recordedAt: now,
	}
}

// IsChannelModelCoolingDown reports whether the channel is inside a known
// per-model quota window.
func IsChannelModelCoolingDown(channelId int, modelName string) bool {
	if channelId <= 0 || modelName == "" {
		return false
	}
	key := channelModelCooldownKey{channelId: channelId, modelName: modelName}
	channelModelCooldowns.mu.RLock()
	defer channelModelCooldowns.mu.RUnlock()
	entry, ok := channelModelCooldowns.expires[key]
	return ok && entry.until.After(time.Now())
}

// ChannelModelCooldowns lists the active quota windows of one channel. Expired
// windows are dropped while reading, so the result only contains accounts that
// still cannot serve the model.
func ChannelModelCooldowns(channelId int) []ChannelModelCooldown {
	if channelId <= 0 {
		return nil
	}
	now := time.Now()
	channelModelCooldowns.mu.Lock()
	defer channelModelCooldowns.mu.Unlock()
	channelModelCooldowns.dropExpiredLocked(now)
	entries := make([]ChannelModelCooldown, 0, 4)
	for key, entry := range channelModelCooldowns.expires {
		if key.channelId != channelId {
			continue
		}
		entries = append(entries, ChannelModelCooldown{
			ModelName:  key.modelName,
			Until:      entry.until,
			Reason:     entry.reason,
			RecordedAt: entry.recordedAt,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ModelName < entries[j].ModelName })
	return entries
}

func (s *channelModelCooldownStore) dropExpiredLocked(now time.Time) {
	for key, entry := range s.expires {
		if !entry.until.After(now) {
			delete(s.expires, key)
		}
	}
}

// channelModelAvailable combines the per-model quota window with the channels
// that the current request already tried.
func channelModelAvailable(channelId int, modelName string, used map[int]struct{}) bool {
	if _, ok := used[channelId]; ok {
		return false
	}
	return !IsChannelModelCoolingDown(channelId, modelName)
}

func toChannelIdSet(channelIds []int) map[int]struct{} {
	if len(channelIds) == 0 {
		return nil
	}
	used := make(map[int]struct{}, len(channelIds))
	for _, channelId := range channelIds {
		used[channelId] = struct{}{}
	}
	return used
}

// filterUnavailableChannels drops channels that are cooling down for the model
// or that the current request already tried. When every candidate is filtered
// out, the original list is returned so the caller keeps forwarding the real
// upstream error instead of reporting a missing channel.
func filterUnavailableChannels(channelIds []int, modelName string, usedChannelIds []int) []int {
	if len(channelIds) == 0 {
		return channelIds
	}
	used := toChannelIdSet(usedChannelIds)
	filtered := make([]int, 0, len(channelIds))
	for _, channelId := range channelIds {
		if channelModelAvailable(channelId, modelName, used) {
			filtered = append(filtered, channelId)
		}
	}
	if len(filtered) == 0 {
		return channelIds
	}
	return filtered
}
