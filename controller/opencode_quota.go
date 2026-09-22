package controller

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	opencodeapi "github.com/QuantumNous/new-api/pkg/opencode"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

var (
	errChannelNotOpenCodeGo = errors.New("channel type is not OpenCode Go")
	errOpenCodeGoMultiKey   = errors.New("multi-key channel is not supported")
)

// openCodeGoQuotaWindow is one subscription window: a rolling five hour window,
// a weekly window or a monthly window.
type openCodeGoQuotaWindow struct {
	Name             string  `json:"name"`
	UsedPercent      float64 `json:"used_percent"`
	RemainingPercent float64 `json:"remaining_percent"`
	ResetInSeconds   int64   `json:"reset_in_seconds"`
	ResetAt          string  `json:"reset_at,omitempty"`
}

// openCodeGoQuotaModel is one model of the channel together with the upstream
// endpoint that serves it.
type openCodeGoQuotaModel struct {
	Model    string `json:"model"`
	Endpoint string `json:"endpoint"`
}

type openCodeGoQuotaData struct {
	ChannelId   int                     `json:"channel_id"`
	ChannelName string                  `json:"channel_name"`
	KeyMasked   string                  `json:"key_masked"`
	Windows     []openCodeGoQuotaWindow `json:"windows"`
	Models      []openCodeGoQuotaModel  `json:"models"`
	CheckedAt   string                  `json:"checked_at"`
}

// GetOpenCodeGoChannelQuota reports the subscription windows of one OpenCode Go
// channel. Reading the windows spends no subscription quota.
func GetOpenCodeGoChannelQuota(c *gin.Context) {
	ch, err := openCodeGoChannelFromRequest(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usage, err := service.FetchOpenCodeGoChannelUsage(ch)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	now := time.Now()
	data := openCodeGoQuotaData{
		ChannelId:   ch.Id,
		ChannelName: ch.Name,
		KeyMasked:   openCodeGoMaskedKey(ch),
		Windows:     openCodeGoQuotaWindows(usage, now),
		Models:      openCodeGoQuotaModels(ch),
		CheckedAt:   now.Format(time.RFC3339),
	}
	common.ApiSuccess(c, data)
}

func openCodeGoChannelFromRequest(c *gin.Context) (*model.Channel, error) {
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
	if ch.Type != constant.ChannelTypeOpenCodeGo {
		return nil, errChannelNotOpenCodeGo
	}
	if ch.ChannelInfo.IsMultiKey {
		return nil, errOpenCodeGoMultiKey
	}
	return ch, nil
}

func openCodeGoQuotaWindows(usage *opencodeapi.Usage, now time.Time) []openCodeGoQuotaWindow {
	windows := make([]openCodeGoQuotaWindow, 0, 3)
	for _, window := range usage.UsageWindows() {
		entry := openCodeGoQuotaWindow{
			Name:             window.Name,
			UsedPercent:      window.UsedPercent,
			RemainingPercent: 100 - window.UsedPercent,
			ResetInSeconds:   window.ResetInSeconds,
		}
		if until, ok := usage.ResetAt(window.Name, now); ok {
			entry.ResetAt = until.Format(time.RFC3339)
		}
		windows = append(windows, entry)
	}
	return windows
}

func openCodeGoQuotaModels(ch *model.Channel) []openCodeGoQuotaModel {
	models := ch.GetModels()
	result := make([]openCodeGoQuotaModel, 0, len(models))
	for _, name := range models {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		upstreamModel, _ := resolveUpstreamModel(ch, name)
		result = append(result, openCodeGoQuotaModel{
			Model:    name,
			Endpoint: opencodeapi.WireForModel(upstreamModel).Path(),
		})
	}
	return result
}

// openCodeGoMaskedKey shows the tail of the key so several channels of the same
// subscription can be told apart.
func openCodeGoMaskedKey(ch *model.Channel) string {
	key, err := opencodeapi.ParseKey(ch.Key)
	if err != nil {
		return ""
	}
	return opencodeapi.MaskKey(key)
}
