package controller

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type workBuddyOAuthStartRequest struct {
	Realm string `json:"realm"`
	Proxy string `json:"proxy"`
}

type workBuddyOAuthCompleteRequest struct {
	FlowID string `json:"flow_id"`
}

// StartWorkBuddyOAuth opens one CodeBuddy sign-in and returns the page the
// administrator has to open in a browser.
func StartWorkBuddyOAuth(c *gin.Context) {
	var request workBuddyOAuthStartRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, errors.New("invalid request"))
		return
	}
	flow, err := service.StartWorkBuddyAuthorizationFlow(
		c.Request.Context(),
		c.GetInt("id"),
		strings.TrimSpace(request.Realm),
		strings.TrimSpace(request.Proxy),
	)
	if err != nil {
		common.SysError("failed to start workbuddy authorization: " + err.Error())
		common.ApiErrorMsg(c, "Failed to start WorkBuddy sign-in")
		return
	}
	common.ApiSuccess(c, flow)
}

// CompleteWorkBuddyOAuth checks whether the browser sign-in finished and returns
// the credential the channel stores as its key.
func CompleteWorkBuddyOAuth(c *gin.Context) {
	var request workBuddyOAuthCompleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, errors.New("invalid request"))
		return
	}
	result, err := service.PollWorkBuddyAuthorizationFlow(
		c.Request.Context(),
		c.GetInt("id"),
		request.FlowID,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
