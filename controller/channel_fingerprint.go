package controller

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelfingerprint"
	"github.com/gin-gonic/gin"
)

var fingerprintRuns sync.Map

// TestChannelFingerprint performs a bounded, explicit administrator-triggered probe.
func TestChannelFingerprint(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var input struct {
		Model  string `json:"model"`
		Family string `json:"family"`
	}
	if err = c.ShouldBindJSON(&input); err != nil {
		common.ApiError(c, err)
		return
	}
	if input.Family != "gpt" && input.Family != "claude" {
		common.ApiError(c, fmt.Errorf("unsupported reference family"))
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !channel.GetSetting().RelayDetection {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Relay detection is disabled"})
		return
	}
	if !slices.Contains(channel.GetModels(), input.Model) {
		common.ApiError(c, fmt.Errorf("model is not enabled on this channel"))
		return
	}
	if channel.Type != constant.ChannelTypeOpenAI && channel.Type != constant.ChannelTypeAnthropic {
		common.ApiError(c, fmt.Errorf("fingerprint probes support OpenAI and Anthropic channels"))
		return
	}
	if _, busy := fingerprintRuns.LoadOrStore(id, true); busy {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "A fingerprint test is already running on this channel"})
		return
	}
	defer fingerprintRuns.Delete(id)
	userID, err := resolveChannelTestUserID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	challenges, err := modelfingerprint.Challenges()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Minute)
	defer cancel()
	samples := make([]modelfingerprint.Sample, 0, len(challenges))
	for _, challenge := range challenges {
		if ctx.Err() != nil {
			break
		}
		probeCtx, stop := context.WithTimeout(ctx, 70*time.Second)
		result := testChannelWithPrompt(probeCtx, channel, userID, input.Model, "", false, challenge.Prompt)
		stop()
		if result.localErr != nil || result.newAPIError != nil {
			samples = append(samples, modelfingerprint.Sample{ID: challenge.ID, Error: "upstream_request_failed"})
			continue
		}
		text, err := fingerprintResponseText(result.responseBody)
		if err != nil {
			samples = append(samples, modelfingerprint.Sample{ID: challenge.ID, Error: "invalid_response"})
			continue
		}
		sample, err := modelfingerprint.Rank(text, input.Family, challenge.Count)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		sample.ID = challenge.ID
		samples = append(samples, sample)
	}
	source := "OpenRouter Claude reference (2026-09-20)"
	if input.Family == "gpt" {
		source = "ModelTrace GitHub reference (2026-09-20)"
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"model": input.Model, "reference": source, "candidates": modelfingerprint.Aggregate(samples), "samples": samples}})
}

func fingerprintResponseText(body []byte) (string, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return "", err
	}
	if len(response.Choices) > 0 {
		return response.Choices[0].Message.Content, nil
	}
	var text strings.Builder
	for _, part := range response.Content {
		if part.Type == "text" {
			text.WriteString(part.Text)
		}
	}
	for _, out := range response.Output {
		for _, part := range out.Content {
			if part.Type == "output_text" {
				text.WriteString(part.Text)
			}
		}
	}
	return text.String(), nil
}
