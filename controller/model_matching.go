package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func PreviewModelMatching(c *gin.Context) {
	var mi model.Model
	if err := c.ShouldBindJSON(&mi); err != nil {
		common.ApiError(c, err)
		return
	}
	preview, err := model.GetModelMatchPreview(&mi)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, preview)
}
