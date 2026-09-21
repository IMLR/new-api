package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChannelModelCooldownWindow(t *testing.T) {
	MarkChannelModelCooldown(12, "kimi-k3", time.Now().Add(time.Hour), "daily free limit")
	require.True(t, IsChannelModelCoolingDown(12, "kimi-k3"))
	require.False(t, IsChannelModelCoolingDown(12, "glm-5.3-flash"))
	require.False(t, IsChannelModelCoolingDown(13, "kimi-k3"))

	// A window in the past clears the entry.
	MarkChannelModelCooldown(12, "kimi-k3", time.Now().Add(-time.Minute), "")
	require.False(t, IsChannelModelCoolingDown(12, "kimi-k3"))
}

func TestFilterUnavailableChannels(t *testing.T) {
	MarkChannelModelCooldown(2, "kimi-k3", time.Now().Add(time.Hour), "")

	channels := []int{1, 2, 3}
	require.Equal(t, []int{1, 3}, filterUnavailableChannels(channels, "kimi-k3", nil))

	// Channels already tried by the current request are skipped as well.
	require.Equal(t, []int{1}, filterUnavailableChannels(channels, "kimi-k3", []int{3}))

	// A different model keeps every channel.
	require.Equal(t, channels, filterUnavailableChannels(channels, "deepseek-v4-flash", nil))

	// When every candidate is unavailable the original list is kept so the
	// caller can still report the upstream error.
	require.Equal(t, channels, filterUnavailableChannels(channels, "kimi-k3", []int{1, 3}))
}

func TestChannelModelCooldownExpiresByTime(t *testing.T) {
	MarkChannelModelCooldown(99, "kimi-k3", time.Now().Add(30*time.Millisecond), "")
	require.True(t, IsChannelModelCoolingDown(99, "kimi-k3"))

	// No timer, no cleanup call: once the deadline passes the channel is
	// selectable again on the next lookup.
	require.Eventually(t, func() bool {
		return !IsChannelModelCoolingDown(99, "kimi-k3")
	}, time.Second, 5*time.Millisecond)

	require.Equal(t, []int{99}, filterUnavailableChannels([]int{99}, "kimi-k3", nil))
}

func TestChannelModelCooldownsListsActiveWindows(t *testing.T) {
	MarkChannelModelCooldown(120, "kimi-k3", time.Now().Add(time.Hour), "Error 429: Daily free limit reached")
	MarkChannelModelCooldown(120, "glm-5.3-flash", time.Now().Add(-time.Minute), "expired")
	MarkChannelModelCooldown(121, "kimi-k3", time.Now().Add(time.Hour), "other channel")

	entries := ChannelModelCooldowns(120)
	require.Len(t, entries, 1)
	require.Equal(t, "kimi-k3", entries[0].ModelName)
	require.Equal(t, "Error 429: Daily free limit reached", entries[0].Reason)
	require.WithinDuration(t, time.Now().Add(time.Hour), entries[0].Until, time.Minute)

	require.Empty(t, ChannelModelCooldowns(122))
}
