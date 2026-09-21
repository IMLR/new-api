package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

// stubChannelCache swaps the in-memory channel index for the duration of one
// test, so selection can run without a database.
func stubChannelCache(t *testing.T) func() {
	t.Helper()
	previousMemoryCache := common.MemoryCacheEnabled
	previousChannels := channelsIDM
	previousGroups := group2model2channels
	common.MemoryCacheEnabled = true
	return func() {
		common.MemoryCacheEnabled = previousMemoryCache
		channelsIDM = previousChannels
		group2model2channels = previousGroups
	}
}

func TestChannelSelectionConsumesLayerBeforeDescending(t *testing.T) {
	restore := stubChannelCache(t)
	defer restore()

	freePriority := int64(5)
	paidPriority := int64(4)
	channelsIDM = map[int]*Channel{
		101: {Id: 101, Priority: &freePriority},
		102: {Id: 102, Priority: &freePriority},
		103: {Id: 103, Priority: &paidPriority},
	}
	group2model2channels = map[string]map[string][]int{
		"default": {"kimi-k3": {101, 102, 103}},
	}

	first, err := GetRandomSatisfiedChannel("default", "kimi-k3", 0, "", nil)
	require.NoError(t, err)
	require.Contains(t, []int{101, 102}, first.Id)

	// A retry stays in the free layer instead of jumping to the paid fallback.
	second, err := GetRandomSatisfiedChannel("default", "kimi-k3", 1, "", []int{first.Id})
	require.NoError(t, err)
	require.NotEqual(t, first.Id, second.Id)
	require.NotEqual(t, 103, second.Id)

	// Once the free layer is used up, the lower priority layer is selected.
	third, err := GetRandomSatisfiedChannel("default", "kimi-k3", 2, "", []int{101, 102})
	require.NoError(t, err)
	require.Equal(t, 103, third.Id)
}

func TestChannelSelectionSkipsCoolingAccountPerModel(t *testing.T) {
	restore := stubChannelCache(t)
	defer restore()

	freePriority := int64(5)
	paidPriority := int64(4)
	channelsIDM = map[int]*Channel{
		201: {Id: 201, Priority: &freePriority},
		202: {Id: 202, Priority: &paidPriority},
	}
	group2model2channels = map[string]map[string][]int{
		"default": {
			"kimi-k3":           {201, 202},
			"deepseek-v4-flash": {201},
		},
	}
	MarkChannelModelCooldown(201, "kimi-k3", time.Now().Add(time.Hour), "")

	channel, err := GetRandomSatisfiedChannel("default", "kimi-k3", 0, "", nil)
	require.NoError(t, err)
	require.Equal(t, 202, channel.Id)

	// The same account keeps serving the model that is not capped.
	channel, err = GetRandomSatisfiedChannel("default", "deepseek-v4-flash", 0, "", nil)
	require.NoError(t, err)
	require.Equal(t, 201, channel.Id)
}

func TestHighestPriorityAbilitiesKeepsTopLayer(t *testing.T) {
	high := int64(5)
	low := int64(2)
	abilities := []Ability{
		{ChannelId: 1, Priority: &low},
		{ChannelId: 2, Priority: &high},
		{ChannelId: 3, Priority: &high},
	}

	require.Equal(t, []Ability{abilities[1], abilities[2]}, highestPriorityAbilities(abilities))
	require.Empty(t, highestPriorityAbilities(nil))
}
