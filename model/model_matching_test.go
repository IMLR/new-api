package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMatchingRequiresAllLiteralIncludesAndRejectsAnyExclude(t *testing.T) {
	tests := []struct {
		name    string
		rule    ModelMatchRule
		model   string
		matches bool
	}{
		{"all included in either order", ModelMatchRule{Include: []string{"claude", "sonnet"}}, "Sonnet-CLAUDE-4", true},
		{"missing an included term", ModelMatchRule{Include: []string{"claude", "sonnet"}}, "claude-opus-4", false},
		{"any excluded term rejects", ModelMatchRule{Include: []string{"claude", "sonnet"}, Exclude: []string{"preview", "thinking"}}, "claude-sonnet-4-thinking", false},
		{"exclude uses same case option", ModelMatchRule{Include: []string{"claude"}, Exclude: []string{"preview"}}, "CLAUDE-PREVIEW", false},
		{"case sensitive rejects different case", ModelMatchRule{Include: []string{"claude"}, CaseSensitive: true}, "CLAUDE-sonnet", false},
		{"case sensitive accepts same case", ModelMatchRule{Include: []string{"claude"}, CaseSensitive: true}, "claude-sonnet", true},
		{"punctuation remains literal", ModelMatchRule{Include: []string{"gpt.*[4]"}}, "vendor/gpt.*[4]-mini", true},
		{"punctuation cannot act as regex", ModelMatchRule{Include: []string{"gpt.*[4]"}}, "gpt-anything4", false},
		{"unicode text", ModelMatchRule{Include: []string{"模型", "测试"}}, "测试-模型-v1", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mi := Model{MatchRule: &test.rule}
			matcher, err := mi.CompileMatcher()
			require.NoError(t, err)
			assert.Equal(t, test.matches, matcher.Matches(test.model))
		})
	}
}

func TestModelMatchingRejectsEmptyAndOversizedFilters(t *testing.T) {
	for _, rule := range []ModelMatchRule{
		{Include: []string{"  "}},
		{Include: []string{strings.Repeat("x", 129)}},
		{Include: make([]string, 33)},
	} {
		_, err := (&Model{MatchRule: &rule}).CompileMatcher()
		require.Error(t, err)
	}
}

func TestModelMatchingNormalizesTermsBeforePersistence(t *testing.T) {
	rule := ModelMatchRule{Include: []string{" claude ", "CLAUDE", "sonnet"}, Exclude: []string{" preview ", ""}}
	require.NoError(t, rule.Normalize())
	assert.Equal(t, []string{"claude", "sonnet"}, rule.Include)
	assert.Equal(t, []string{"preview"}, rule.Exclude)
}

func TestLiteralMatchingAppliesMetadataToCatalogModels(t *testing.T) {
	resetPricingEndpointTestTables(t)
	insertPricingEndpointChannel(t, 901, constant.ChannelTypeOpenAI, dto.ChannelOtherSettings{})
	for _, name := range []string{"claude-sonnet-4", "CLAUDE-SONNET-4", "claude-sonnet-preview", "claude-opus-4"} {
		insertPricingEndpointAbility(t, 901, name)
	}
	require.NoError(t, DB.Create(&Model{
		ModelName: "Sonnet family", Description: "Matched Sonnet description", Status: 1,
		MatchRule: &ModelMatchRule{Include: []string{"claude", "sonnet"}, Exclude: []string{"preview"}},
	}).Error)
	descriptions := make(map[string]string)
	for _, pricing := range GetPricing() {
		descriptions[pricing.ModelName] = pricing.Description
	}
	assert.Equal(t, "Matched Sonnet description", descriptions["claude-sonnet-4"])
	assert.Equal(t, "Matched Sonnet description", descriptions["CLAUDE-SONNET-4"])
	assert.Empty(t, descriptions["claude-sonnet-preview"])
	assert.Empty(t, descriptions["claude-opus-4"])
}
