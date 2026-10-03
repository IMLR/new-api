package workbuddy

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// ConvertOpenAIResponsesRequest preserves instructions, input history and
// function calls while changing the wire to the upstream's chat protocol.
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	if !common.WorkBuddySupportsResponses(request.Model) {
		return nil, fmt.Errorf("WorkBuddy Responses conversion only supports GPT chat models, got %q", request.Model)
	}
	result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAI, &request)
	if err != nil {
		return nil, err
	}
	chat, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
	}
	return a.ConvertOpenAIRequest(c, info, chat)
}

// chatRequestBody converts Responses when body pass-through skipped the
// normal adaptor conversion. Other client protocols keep their existing path.
func (a *Adaptor) chatRequestBody(c *gin.Context, info *relaycommon.RelayInfo, raw []byte) ([]byte, error) {
	format := info.GetFinalRequestRelayFormat()
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		// Conversion history can include a prior channel attempt. The current
		// pass-through body still has the caller's original protocol.
		format = info.RelayFormat
	}
	if format == "" && info.RelayMode == relayconstant.RelayModeResponses {
		format = types.RelayFormatOpenAIResponses
	}
	if format != types.RelayFormatOpenAIResponses {
		return raw, nil
	}
	var request dto.OpenAIResponsesRequest
	if err := common.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	converted, err := a.ConvertOpenAIResponsesRequest(c, info, request)
	if err != nil {
		return nil, err
	}
	relaycommon.AppendRequestConversionFromRequest(info, converted)
	return common.Marshal(converted)
}
