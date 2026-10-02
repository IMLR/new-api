package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChannelModelSelectionTest(t *testing.T) (*Channel, *Channel) {
	t.Helper()
	resetPricingEndpointTestTables(t)
	require.NoError(t, DB.Where("1 = 1").Delete(&ChannelModelExclusion{}).Error)
	t.Cleanup(func() { require.NoError(t, DB.Where("1 = 1").Delete(&ChannelModelExclusion{}).Error) })
	first := &Channel{Id: 801, Name: "Primary", Type: constant.ChannelTypeOpenAI, Key: "test", Models: "claude-sonnet-4,claude-opus-4,gpt-4o", Group: "default,vip", Status: common.ChannelStatusEnabled}
	second := &Channel{Id: 802, Name: "Backup", Type: constant.ChannelTypeOpenAI, Key: "test", Models: "claude-sonnet-4,claude-sonnet-preview", Group: "default", Status: common.ChannelStatusEnabled}
	for _, channel := range []*Channel{first, second} {
		require.NoError(t, DB.Create(channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
	}
	return first, second
}

func TestChannelModelSelectionDisablesOnlySelectedPairAndSurvivesRebuild(t *testing.T) {
	first, second := setupChannelModelSelectionTest(t)
	mi := &Model{ModelName: "Sonnet models", Status: 1, MatchRule: &ModelMatchRule{Include: []string{"claude", "sonnet"}, Exclude: []string{"preview"}}, ChannelSelections: []ChannelModelSelection{{ChannelId: first.Id, Model: "claude-sonnet-4", Selected: false}}}
	require.NoError(t, mi.SaveWithChannelSelections(true))
	var persisted Model
	require.NoError(t, DB.First(&persisted, mi.Id).Error)
	require.NotNil(t, persisted.MatchRule)
	assert.Equal(t, mi.MatchRule, persisted.MatchRule)

	preview, err := GetModelMatchPreview(mi)
	require.NoError(t, err)
	require.Len(t, preview, 1)
	assert.Equal(t, "claude-sonnet-4", preview[0].Model)
	require.Len(t, preview[0].Channels, 2)
	assert.False(t, preview[0].Channels[0].Selected)
	assert.True(t, preview[0].Channels[1].Selected)

	for _, memoryCache := range []bool{true, false} {
		common.MemoryCacheEnabled = memoryCache
		InitChannelCache()
		selected, err := GetRandomSatisfiedChannel("default", "claude-sonnet-4", 0, "", nil)
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.Equal(t, second.Id, selected.Id)
		assert.False(t, IsChannelEnabledForGroupModel("default", "claude-sonnet-4", first.Id))
		assert.False(t, IsChannelEnabledForGroupModel("vip", "claude-sonnet-4", first.Id))
		assert.True(t, IsChannelEnabledForGroupModel("default", "claude-opus-4", first.Id))
		assert.True(t, IsChannelEnabledForGroupModel("default", "gpt-4o", first.Id))
	}
	common.MemoryCacheEnabled = true
	// An ability rebuild may temporarily omit the disabled pair. Its persisted
	// exclusion must still keep it out of the memory routing cache.
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", first.Id, "claude-sonnet-4").Delete(&Ability{}).Error)
	InitChannelCache()
	assert.False(t, IsChannelEnabledForGroupModel("default", "claude-sonnet-4", first.Id))
	require.NoError(t, first.UpdateAbilities(nil))
	require.NoError(t, UpdateAbilityStatus(first.Id, false))
	require.NoError(t, UpdateAbilityStatus(first.Id, true))
	tag := "shared"
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", first.Id).Update("tag", tag).Error)
	require.NoError(t, UpdateAbilityStatusByTag(tag, false))
	require.NoError(t, UpdateAbilityStatusByTag(tag, true))
	_, _, err = FixAbility()
	require.NoError(t, err)
	assert.False(t, IsChannelEnabledForGroupModel("default", "claude-sonnet-4", first.Id))
	assert.True(t, IsChannelEnabledForGroupModel("default", "claude-opus-4", first.Id))

	mi.ChannelSelections[0].Selected = true
	require.NoError(t, mi.SaveWithChannelSelections(false))
	InitChannelCache()
	assert.True(t, IsChannelEnabledForGroupModel("default", "claude-sonnet-4", first.Id))
	assert.True(t, IsChannelEnabledForGroupModel("vip", "claude-sonnet-4", first.Id))
}

func TestChannelModelSelectionRollsBackMetadataWhenChannelModelChanged(t *testing.T) {
	first, _ := setupChannelModelSelectionTest(t)
	mi := &Model{ModelName: "Invalid selection", Status: 1, MatchRule: &ModelMatchRule{Include: []string{"claude"}}, ChannelSelections: []ChannelModelSelection{{ChannelId: first.Id, Model: "removed-model", Selected: false}}}
	require.Error(t, mi.SaveWithChannelSelections(true))
	var count int64
	require.NoError(t, DB.Model(&Model{}).Where("model_name = ?", mi.ModelName).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&ChannelModelExclusion{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestChannelModelSelectionCannotEnableDisabledChannel(t *testing.T) {
	first, _ := setupChannelModelSelectionTest(t)
	require.NoError(t, DB.Model(first).Update("status", common.ChannelStatusManuallyDisabled).Error)
	require.NoError(t, UpdateAbilityStatus(first.Id, false))
	mi := &Model{ModelName: "Disabled channel", Status: 1, MatchRule: &ModelMatchRule{Include: []string{"claude", "sonnet"}}, ChannelSelections: []ChannelModelSelection{{ChannelId: first.Id, Model: "claude-sonnet-4", Selected: true}}}
	require.NoError(t, mi.SaveWithChannelSelections(true))
	InitChannelCache()
	assert.False(t, IsChannelEnabledForGroupModel("default", "claude-sonnet-4", first.Id))
	preview, err := GetModelMatchPreview(mi)
	require.NoError(t, err)
	require.NotEmpty(t, preview)
	assert.True(t, preview[0].Channels[0].Selected)
	assert.False(t, preview[0].Channels[0].Enabled)
}

func TestDeletingChannelRemovesItsSelectionsWithoutAffectingOtherChannels(t *testing.T) {
	first, second := setupChannelModelSelectionTest(t)
	mi := &Model{ModelName: "Sonnet models", MatchRule: &ModelMatchRule{Include: []string{"sonnet"}}, ChannelSelections: []ChannelModelSelection{
		{ChannelId: first.Id, Model: "claude-sonnet-4", Selected: false},
		{ChannelId: second.Id, Model: "claude-sonnet-4", Selected: false},
	}}
	require.NoError(t, mi.SaveWithChannelSelections(true))
	require.NoError(t, first.Delete())
	var count int64
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", first.Id).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&ChannelModelExclusion{}).Where("channel_id = ?", first.Id).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&ChannelModelExclusion{}).Where("channel_id = ?", second.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	assert.False(t, IsChannelEnabledForGroupModel("default", "claude-sonnet-4", second.Id))
}
