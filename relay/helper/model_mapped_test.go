package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMatchingAndExplicitMappingsPreserveClientBillingName(t *testing.T) {
	for _, tc := range []struct {
		name, mapping, expected string
		wantError               bool
	}{
		{name: "automatic native name", expected: "deepseek-v4.1-flash"},
		{name: "native explicit mapping", mapping: `{"deepseek-v4.1-flash":"vendor/deepseek"}`, expected: "vendor/deepseek"},
		{name: "client mapping takes precedence", mapping: `{"deepseek-v4-flash":"manual-model"}`, expected: "manual-model"},
		{name: "identity client mapping takes precedence", mapping: `{"deepseek-v4-flash":"deepseek-v4-flash"}`, expected: "deepseek-v4-flash"},
		{name: "native identity mapping", mapping: `{"deepseek-v4.1-flash":"deepseek-v4.1-flash"}`, expected: "deepseek-v4.1-flash"},
		{name: "cycle rejected", mapping: `{"deepseek-v4.1-flash":"cycle","cycle":"deepseek-v4.1-flash"}`, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(ctx, constant.ContextKeyChannelMatchedModel, "deepseek-v4.1-flash")
			common.SetContextKey(ctx, constant.ContextKeyChannelModelMapping, tc.mapping)
			info := &relaycommon.RelayInfo{OriginModelName: "deepseek-v4-flash"}
			request := &dto.GeneralOpenAIRequest{Model: info.OriginModelName}
			err := ModelMappedHelper(ctx, info, request)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, info.UpstreamModelName)
			assert.Equal(t, tc.expected, request.Model)
			assert.Equal(t, "deepseek-v4-flash", info.OriginModelName)
			assert.Equal(t, tc.expected != info.OriginModelName, info.IsModelMapped)
		})
	}
}

func TestCompactAutomaticModelMatchingPreservesCanonicalPricing(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyChannelMatchedModel, "gpt-native")
	info := &relaycommon.RelayInfo{OriginModelName: ratio_setting.WithCompactModelSuffix("gpt-canonical"), RelayMode: relayconstant.RelayModeResponsesCompact}
	request := &dto.OpenAIResponsesRequest{Model: "gpt-canonical"}
	require.NoError(t, ModelMappedHelper(ctx, info, request))
	assert.Equal(t, "gpt-native", request.Model)
	assert.Equal(t, ratio_setting.WithCompactModelSuffix("gpt-canonical"), info.OriginModelName)
}
