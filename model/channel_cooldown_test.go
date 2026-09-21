package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChannelModelCooldownWindow(t *testing.T) {
	MarkChannelModelCooldown(12, "kimi-k3", time.Now().Add(time.Hour))
	require.True(t, IsChannelModelCoolingDown(12, "kimi-k3"))
	require.False(t, IsChannelModelCoolingDown(12, "glm-5.3-flash"))
	require.False(t, IsChannelModelCoolingDown(13, "kimi-k3"))

	// A window in the past clears the entry.
	MarkChannelModelCooldown(12, "kimi-k3", time.Now().Add(-time.Minute))
	require.False(t, IsChannelModelCoolingDown(12, "kimi-k3"))
}

func TestFilterUnavailableChannels(t *testing.T) {
	MarkChannelModelCooldown(2, "kimi-k3", time.Now().Add(time.Hour))

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
