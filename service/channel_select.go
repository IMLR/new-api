package service

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx         *gin.Context
	TokenGroup  string
	ModelName   string
	RequestPath string
	Retry       *int
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

// CacheGetRandomSatisfiedChannel selects an untried, non-cooling channel from
// the highest remaining priority. Auto groups exhaust their candidates before
// moving on; cross-group retries follow the token's permission. Retry remains
// the total attempt index and never resets when a group changes.
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	usedChannelIDs := usedChannelIdsFromContext(param.Ctx)
	if param.TokenGroup != "auto" {
		channel, err := model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, param.GetRetry(), param.RequestPath, usedChannelIDs)
		return channel, param.TokenGroup, err
	}
	if len(setting.GetAutoGroups()) == 0 {
		return nil, param.TokenGroup, errors.New("auto groups is not enabled")
	}

	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	autoGroups := GetUserAutoGroup(userGroup)
	startGroupIndex := common.GetContextKeyInt(param.Ctx, constant.ContextKeyAutoGroupIndex)
	crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
	selectGroup := param.TokenGroup
	for i := startGroupIndex; i < len(autoGroups); i++ {
		selectGroup = autoGroups[i]
		channel, err := model.GetRandomSatisfiedChannel(selectGroup, param.ModelName, param.GetRetry(), param.RequestPath, usedChannelIDs)
		if err != nil {
			return nil, selectGroup, err
		}
		if channel != nil {
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, selectGroup)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			logger.LogDebug(param.Ctx, "Auto selected group: %s, attempt: %d", selectGroup, param.GetRetry())
			return channel, selectGroup, nil
		}
		if len(usedChannelIDs) > 0 && !crossGroupRetry {
			return nil, selectGroup, nil
		}
	}
	return nil, selectGroup, nil
}

// usedChannelIdsFromContext lists the channels already tried by the current
// request, which keeps a retry from landing on the same account again.
func usedChannelIdsFromContext(ctx *gin.Context) []int {
	if ctx == nil {
		return nil
	}
	rawIDs := ctx.GetStringSlice("use_channel")
	channelIDs := make([]int, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		channelID, err := strconv.Atoi(rawID)
		if err == nil {
			channelIDs = append(channelIDs, channelID)
		}
	}
	return channelIDs
}
