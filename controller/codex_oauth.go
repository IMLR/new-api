package controller

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type codexOAuthStartRequest struct {
	Proxy string `json:"proxy"`
}

type codexOAuthCompleteRequest struct {
	FlowID string `json:"flow_id"`
	Input  string `json:"input"`
}

func StartCodexOAuth(c *gin.Context) {
	startCodexOAuth(c, 0)
}

func StartCodexOAuthForChannel(c *gin.Context) {
	channelID, err := parseCodexOAuthChannelID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	startCodexOAuth(c, channelID)
}

func startCodexOAuth(c *gin.Context, channelID int) {
	var request codexOAuthStartRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, errors.New("invalid request"))
		return
	}

	proxyURL := strings.TrimSpace(request.Proxy)
	if channelID > 0 {
		channel, err := getCodexOAuthChannel(channelID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		proxyURL = channel.GetSetting().Proxy
	}

	flow, err := service.StartCodexOAuthAuthorizationFlow(
		c.Request.Context(),
		c.GetInt("id"),
		channelID,
		proxyURL,
	)
	if err != nil {
		common.SysError("failed to start codex oauth flow: " + err.Error())
		common.ApiErrorMsg(c, "Failed to start Codex authorization")
		return
	}
	common.ApiSuccess(c, flow)
}

func CompleteCodexOAuth(c *gin.Context) {
	completeCodexOAuth(c, 0)
}

func CompleteCodexOAuthForChannel(c *gin.Context) {
	channelID, err := parseCodexOAuthChannelID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	completeCodexOAuth(c, channelID)
}

func completeCodexOAuth(c *gin.Context, channelID int) {
	var request codexOAuthCompleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, errors.New("invalid request"))
		return
	}

	var channel *model.Channel
	if channelID > 0 {
		var err error
		channel, err = getCodexOAuthChannel(channelID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	token, err := service.CompleteCodexOAuthAuthorizationFlow(
		ctx,
		c.GetInt("id"),
		channelID,
		request.FlowID,
		request.Input,
	)
	if err != nil {
		common.SysError("failed to complete codex oauth flow: " + err.Error())
		common.ApiErrorMsg(c, codexOAuthPublicError(err))
		return
	}

	accountID, ok := service.ExtractCodexAccountIDFromJWT(token.AccessToken)
	if !ok {
		common.ApiErrorMsg(c, "Codex authorization response did not include an account")
		return
	}
	email, _ := service.ExtractEmailFromJWT(token.AccessToken)
	now := time.Now()
	oauthKey := service.CodexOAuthKey{
		IDToken:      token.IDToken,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		AccountID:    accountID,
		LastRefresh:  now.Format(time.RFC3339),
		Email:        email,
		Type:         "codex",
		Expired:      token.ExpiresAt.Format(time.RFC3339),
	}
	encoded, err := common.Marshal(oauthKey)
	if err != nil {
		common.ApiErrorMsg(c, "Failed to encode Codex credential")
		return
	}

	data := gin.H{
		"account_id":   accountID,
		"email":        email,
		"expires_at":   oauthKey.Expired,
		"last_refresh": oauthKey.LastRefresh,
	}
	if channelID == 0 {
		data["key"] = string(encoded)
		common.ApiSuccess(c, data)
		return
	}

	result := model.DB.Model(&model.Channel{}).
		Where("id = ? AND type = ?", channelID, constant.ChannelTypeCodex).
		Update("key", string(encoded))
	if result.Error != nil {
		common.ApiError(c, result.Error)
		return
	}
	if result.RowsAffected != 1 {
		common.ApiErrorMsg(c, "Codex channel was not found")
		return
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()

	data["channel_id"] = channelID
	recordManageAudit(c, "channel.codex_oauth", map[string]interface{}{
		"id":   channelID,
		"name": channel.Name,
	})
	common.ApiSuccess(c, data)
}

func parseCodexOAuthChannelID(c *gin.Context) (int, error) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		return 0, errors.New("invalid channel id")
	}
	return channelID, nil
}

func getCodexOAuthChannel(channelID int) (*model.Channel, error) {
	channel, err := model.GetChannelById(channelID, false)
	if err != nil {
		return nil, err
	}
	if channel.Type != constant.ChannelTypeCodex {
		return nil, errors.New("channel type is not Codex")
	}
	if channel.ChannelInfo.IsMultiKey {
		return nil, errors.New("multi-key Codex channel is not supported")
	}
	return channel, nil
}

func codexOAuthPublicError(err error) string {
	message := err.Error()
	switch {
	case strings.Contains(message, "not found or expired"):
		return "Authorization expired or was already used; start again"
	case strings.Contains(message, "callback URL"):
		return "Paste the full localhost:1455 callback URL"
	case strings.Contains(message, "state mismatch"):
		return "Authorization state did not match; start again"
	case strings.Contains(message, "authorization was not completed"):
		return "Codex authorization was cancelled or denied"
	case strings.Contains(message, "status="):
		return "Codex rejected the authorization code; start again"
	default:
		return "Failed to complete Codex authorization"
	}
}
