package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRuleRoutesVariantsByPriorityAndRespectsSelections(t *testing.T) {
	for _, memoryCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory=%t", memoryCache), func(t *testing.T) {
			resetPricingEndpointTestTables(t)
			require.NoError(t, DB.Where("channel_id = ?", 811).Delete(&ChannelModelExclusion{}).Error)
			t.Cleanup(func() { require.NoError(t, DB.Where("channel_id = ?", 811).Delete(&ChannelModelExclusion{}).Error) })
			common.MemoryCacheEnabled = memoryCache
			metadata := Model{ModelName: "deepseek-v4-flash", Status: 1, MatchRule: &ModelMatchRule{
				Include: []string{"deepseek", "flash"}, Exclude: []string{"thinking"},
			}}
			require.NoError(t, metadata.SaveWithChannelSelections(true))
			channels := []*Channel{
				{Id: 811, Name: "WorkBuddy", Models: "deepseek-v4.1-flash,deepseek-v4.1-flash-sg", Group: "default", Priority: common.GetPointer(int64(6))},
				{Id: 812, Name: "Cline", Models: metadata.ModelName, Group: "default", Priority: common.GetPointer(int64(5))},
				{Id: 813, Name: "Other group", Models: "DEEPSEEK-V4.1-FLASH", Group: "vip", Priority: common.GetPointer(int64(20))},
				{Id: 814, Name: "Excluded variant", Models: "deepseek-v4.1-flash-thinking", Group: "default", Priority: common.GetPointer(int64(20))},
			}
			for _, channel := range channels {
				channel.Type, channel.Key, channel.Status = constant.ChannelTypeOpenAI, "test", common.ChannelStatusEnabled
				require.NoError(t, DB.Create(channel).Error)
				require.NoError(t, channel.AddAbilities(nil))
			}
			InitChannelCache()
			selected, err := GetRandomSatisfiedChannel("default", metadata.ModelName, 0, "", nil)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 811, selected.Id)
			native, err := ResolveChannelModel("default", metadata.ModelName, selected.Id, "")
			require.NoError(t, err)
			assert.Equal(t, "deepseek-v4.1-flash", native)
			assert.True(t, IsChannelEnabledForGroupModel("default", metadata.ModelName, 811))
			assert.False(t, IsChannelEnabledForGroupModel("default", metadata.ModelName, 813))
			assert.False(t, IsChannelEnabledForGroupModel("default", metadata.ModelName, 814))

			// Another variant on an attempted account must not become a retry.
			selected, err = GetRandomSatisfiedChannel("default", metadata.ModelName, 1, "", []int{811})
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 812, selected.Id)
			selected, err = GetRandomSatisfiedChannel("default", metadata.ModelName, 2, "", []int{811, 812})
			require.NoError(t, err)
			assert.Nil(t, selected)

			for _, name := range []string{"deepseek-v4.1-flash", "deepseek-v4.1-flash-sg"} {
				MarkChannelModelCooldown(811, name, time.Now().Add(time.Hour), "quota reset")
				t.Cleanup(func() { MarkChannelModelCooldown(811, name, time.Time{}, "") })
			}
			selected, err = GetRandomSatisfiedChannel("default", metadata.ModelName, 0, "", nil)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 812, selected.Id)
			for _, name := range []string{"deepseek-v4.1-flash", "deepseek-v4.1-flash-sg"} {
				MarkChannelModelCooldown(811, name, time.Time{}, "")
			}

			metadata.ChannelSelections = []ChannelModelSelection{{ChannelId: 811, Model: "deepseek-v4.1-flash", Selected: false}}
			require.NoError(t, metadata.SaveWithChannelSelections(false))
			InitChannelCache()
			native, err = ResolveChannelModel("default", metadata.ModelName, 811, "")
			require.NoError(t, err)
			assert.Equal(t, "deepseek-v4.1-flash-sg", native)
			metadata.ChannelSelections = append(metadata.ChannelSelections, ChannelModelSelection{ChannelId: 811, Model: "deepseek-v4.1-flash-sg", Selected: false})
			require.NoError(t, metadata.SaveWithChannelSelections(false))
			InitChannelCache()
			selected, err = GetRandomSatisfiedChannel("default", metadata.ModelName, 0, "", nil)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 812, selected.Id)

			selected, err = GetRandomSatisfiedChannel("vip", metadata.ModelName, 0, "", nil)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 813, selected.Id)
			metadata.MatchRule.CaseSensitive = true
			metadata.ChannelSelections = nil
			require.NoError(t, metadata.SaveWithChannelSelections(false))
			InitChannelCache()
			selected, err = GetRandomSatisfiedChannel("vip", metadata.ModelName, 0, "", nil)
			require.NoError(t, err)
			assert.Nil(t, selected)
		})
	}
}

func TestModelRuleUsesNativeModelForEndpointSelection(t *testing.T) {
	for _, memoryCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory=%t", memoryCache), func(t *testing.T) {
			resetPricingEndpointTestTables(t)
			common.MemoryCacheEnabled = memoryCache
			metadata := Model{ModelName: "unified-deepseek", Status: 1, MatchRule: &ModelMatchRule{Include: []string{"deepseek", "flash"}}}
			require.NoError(t, metadata.SaveWithChannelSelections(true))
			channel := Channel{Id: 815, Name: "Native route", Type: constant.ChannelTypeAdvancedCustom, Key: "test", Status: 1, Models: "deepseek-v4.1-flash", Group: "default"}
			channel.SetOtherSettings(pricingEndpointAdvancedCustomConfig(dto.AdvancedCustomRoute{
				IncomingPath: "/v1/chat/completions", UpstreamPath: "/native/chat", Models: []string{"deepseek-v4.1-flash"},
			}))
			require.NoError(t, DB.Create(&channel).Error)
			require.NoError(t, channel.AddAbilities(nil))
			InitChannelCache()
			selected, err := GetRandomSatisfiedChannel("default", metadata.ModelName, 0, "/v1/chat/completions", nil)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 815, selected.Id)
			native, err := ResolveChannelModel("default", metadata.ModelName, 815, "/v1/chat/completions")
			require.NoError(t, err)
			assert.Equal(t, channel.Models, native)
			selected, err = GetRandomSatisfiedChannel("default", metadata.ModelName, 0, "/v1/responses", nil)
			require.NoError(t, err)
			assert.Nil(t, selected)
		})
	}
}
