package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	clineapi "github.com/QuantumNous/new-api/pkg/cline"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

var (
	errChannelNotFound = errors.New("channel not found")
	errChannelNotCline = errors.New("channel type is not Cline")
	errClineMultiKey   = errors.New("multi-key channel is not supported")
)

// clineQuotaModel is the per-model quota state shown in the channel view.
type clineQuotaModel struct {
	Model          string `json:"model"`
	UpstreamModel  string `json:"upstream_model"`
	CoolingDown    bool   `json:"cooling_down"`
	ResetAt        string `json:"reset_at,omitempty"`
	ResetInSeconds int64  `json:"reset_in_seconds"`
	Reason         string `json:"reason,omitempty"`
}

// clineQuotaData is the payload of the Cline channel view.
type clineQuotaData struct {
	ChannelId    int               `json:"channel_id"`
	ChannelName  string            `json:"channel_name"`
	Account      *clineapi.Account `json:"account,omitempty"`
	AccountError string            `json:"account_error,omitempty"`
	Plan         *clineQuotaPlan   `json:"plan,omitempty"`
	Credential   clineQuotaCred    `json:"credential"`
	Models       []clineQuotaModel `json:"models"`
	CheckedAt    string            `json:"checked_at"`
}

type clineQuotaPlan struct {
	Active           bool   `json:"active"`
	Type             string `json:"type,omitempty"`
	Name             string `json:"name,omitempty"`
	DisplayName      string `json:"display_name,omitempty"`
	Status           string `json:"status,omitempty"`
	CurrentPeriodEnd string `json:"current_period_end,omitempty"`
}

type clineQuotaCred struct {
	ExpiresAt        string `json:"expires_at,omitempty"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
	AutoRefresh      bool   `json:"auto_refresh"`
}

// GetClineChannelQuota reports the free-model quota windows of one Cline
// channel. Windows come from the daily cap errors the relay already parses, so
// reading this endpoint never spends upstream quota.
func GetClineChannelQuota(c *gin.Context) {
	ch, err := clineChannelFromRequest(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	credential, client, base, err := resolveClineRequestClient(ctx, ch)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	now := time.Now()
	data := clineQuotaData{
		ChannelId:   ch.Id,
		ChannelName: ch.Name,
		Credential:  clineQuotaCred{AutoRefresh: true},
		Models:      clineQuotaModels(ch, now),
		CheckedAt:   now.Format(time.RFC3339),
	}
	if credential.ExpiresAt > 0 {
		expires := time.UnixMilli(credential.ExpiresAt)
		data.Credential.ExpiresAt = expires.Format(time.RFC3339)
		data.Credential.ExpiresInSeconds = int64(time.Until(expires).Seconds())
	}
	account, err := clineapi.FetchAccount(ctx, client, base, credential)
	if err != nil {
		data.AccountError = clineErrorMessage(err)
		common.ApiSuccess(c, data)
		return
	}
	data.Account = account
	plan, err := clineapi.FetchCurrentPlan(ctx, client, base, credential)
	switch {
	case err != nil:
		data.AccountError = clineErrorMessage(err)
	case plan != nil:
		data.Plan = &clineQuotaPlan{
			Active:           plan.Active(now),
			Status:           plan.Status,
			CurrentPeriodEnd: plan.CurrentPeriodEnd,
		}
		if plan.Plan != nil {
			data.Plan.Type = plan.Plan.Type
			data.Plan.Name = plan.Plan.Name
			data.Plan.DisplayName = plan.Plan.DisplayName
		}
	}
	common.ApiSuccess(c, data)
}

// ProbeClineChannelQuota spends one minimal upstream request to learn whether a
// model still has free quota, and refreshes the stored window when it does not.
func ProbeClineChannelQuota(c *gin.Context) {
	ch, err := clineChannelFromRequest(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var request struct {
		Model string `json:"model"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "请求格式错误")
		return
	}
	request.Model = strings.TrimSpace(request.Model)
	if request.Model == "" {
		common.ApiErrorMsg(c, "缺少模型名称")
		return
	}
	upstreamModel, ok := resolveUpstreamModel(ch, request.Model)
	if !ok {
		common.ApiErrorMsg(c, "该模型不属于这个渠道")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	credential, client, base, err := resolveClineRequestClient(ctx, ch)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := clineapi.ProbeModel(ctx, client, base, credential, upstreamModel)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	now := time.Now()
	entry := clineQuotaModel{
		Model:         request.Model,
		UpstreamModel: upstreamModel,
		Reason:        result.Message,
	}
	if until, limited := result.CooldownUntil(now); limited {
		model.MarkChannelModelCooldown(ch.Id, request.Model, until, result.Message)
		if upstreamModel != request.Model {
			model.MarkChannelModelCooldown(ch.Id, upstreamModel, until, result.Message)
		}
		entry.CoolingDown = true
		entry.ResetAt = until.Format(time.RFC3339)
		entry.ResetInSeconds = int64(time.Until(until).Seconds())
	}
	common.ApiSuccess(c, gin.H{
		"status_code": result.StatusCode,
		"latency_ms":  result.Latency.Milliseconds(),
		"model":       entry,
	})
}

func clineChannelFromRequest(c *gin.Context) (*model.Channel, error) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return nil, err
	}
	ch, err := model.GetChannelById(channelId, true)
	if err != nil {
		return nil, err
	}
	if ch == nil {
		return nil, errChannelNotFound
	}
	if ch.Type != constant.ChannelTypeCline {
		return nil, errChannelNotCline
	}
	if ch.ChannelInfo.IsMultiKey {
		return nil, errClineMultiKey
	}
	return ch, nil
}

func resolveClineRequestClient(
	ctx context.Context,
	ch *model.Channel,
) (*clineapi.Credential, *http.Client, string, error) {
	credential, err := service.ResolveClineCredential(ctx, ch.Id, "")
	if err != nil {
		return nil, nil, "", err
	}
	client, err := service.NewProxyHttpClient(ch.GetSetting().Proxy)
	if err != nil {
		return nil, nil, "", err
	}
	base := ch.GetBaseURL()
	if base == "" {
		base = clineapi.BaseURL
	}
	return credential, client, base, nil
}

func clineQuotaModels(ch *model.Channel, now time.Time) []clineQuotaModel {
	cooldowns := model.ChannelModelCooldowns(ch.Id)
	byName := make(map[string]model.ChannelModelCooldown, len(cooldowns))
	for _, entry := range cooldowns {
		byName[entry.ModelName] = entry
	}
	models := ch.GetModels()
	result := make([]clineQuotaModel, 0, len(models))
	for _, name := range models {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		upstreamModel, _ := resolveUpstreamModel(ch, name)
		entry := clineQuotaModel{Model: name, UpstreamModel: upstreamModel}
		cooldown, ok := byName[name]
		if !ok && upstreamModel != name {
			cooldown, ok = byName[upstreamModel]
		}
		if ok {
			entry.CoolingDown = true
			entry.ResetAt = cooldown.Until.Format(time.RFC3339)
			entry.ResetInSeconds = int64(cooldown.Until.Sub(now).Seconds())
			entry.Reason = cooldown.Reason
		}
		result = append(result, entry)
	}
	return result
}

// resolveUpstreamModel applies the channel model mapping, following chains the
// same way the relay does.
func resolveUpstreamModel(ch *model.Channel, name string) (string, bool) {
	models := ch.GetModels()
	found := false
	for _, candidate := range models {
		if strings.TrimSpace(candidate) == name {
			found = true
			break
		}
	}
	if !found {
		return "", false
	}
	mapping := strings.TrimSpace(ch.GetModelMapping())
	if mapping == "" || mapping == "{}" {
		return name, true
	}
	table := make(map[string]string)
	if common.UnmarshalJsonStr(mapping, &table) != nil {
		return name, true
	}
	current := name
	visited := map[string]bool{current: true}
	for range 10 {
		next, ok := table[current]
		if !ok || next == "" || visited[next] {
			break
		}
		visited[next] = true
		current = next
	}
	return current, true
}

func clineErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
