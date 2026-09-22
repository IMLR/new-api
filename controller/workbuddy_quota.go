package controller

import (
	"errors"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

var (
	errChannelNotWorkBuddy = errors.New("channel type is not WorkBuddy")
	errWorkBuddyMultiKey   = errors.New("multi-key channel is not supported")
)

// workBuddyQuotaPackage is one credit package of the account.
type workBuddyQuotaPackage struct {
	Name        string `json:"name"`
	Remain      int64  `json:"remain"`
	Used        int64  `json:"used"`
	Size        int64  `json:"size"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	ExpiresSoon bool   `json:"expires_soon,omitempty"`
}

type workBuddyQuotaData struct {
	ChannelId   int                     `json:"channel_id"`
	ChannelName string                  `json:"channel_name"`
	Account     string                  `json:"account,omitempty"`
	Realm       string                  `json:"realm"`
	Remain      int64                   `json:"remain"`
	Used        int64                   `json:"used"`
	Size        int64                   `json:"size"`
	Packages    []workBuddyQuotaPackage `json:"packages"`
	CheckedAt   string                  `json:"checked_at"`
}

// GetWorkBuddyChannelQuota reports the credit balance of one WorkBuddy channel.
// The balance call does not consume credits.
func GetWorkBuddyChannelQuota(c *gin.Context) {
	ch, err := workBuddyChannelFromRequest(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	credits, err := service.FetchWorkBuddyChannelCredits(ch)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	credential, _ := workbuddyapi.ParseCredential(ch.Key)
	data := workBuddyQuotaData{
		ChannelId:   ch.Id,
		ChannelName: ch.Name,
		Remain:      credits.Remain,
		Used:        credits.Used,
		Size:        credits.Size,
		Packages:    workBuddyQuotaPackages(credits, time.Now()),
		CheckedAt:   time.Now().Format(time.RFC3339),
	}
	if credential != nil {
		data.Realm = credential.EffectiveRealm()
		data.Account = credential.MaskUID()
	}
	common.ApiSuccess(c, data)
}

func workBuddyChannelFromRequest(c *gin.Context) (*model.Channel, error) {
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
	if ch.Type != constant.ChannelTypeWorkBuddy {
		return nil, errChannelNotWorkBuddy
	}
	if ch.ChannelInfo.IsMultiKey {
		return nil, errWorkBuddyMultiKey
	}
	return ch, nil
}

// workBuddyQuotaPackages lists the credit packages, marking the ones that
// expire inside the coming week so the caller can spend them first.
func workBuddyQuotaPackages(credits *workbuddyapi.Credits, now time.Time) []workBuddyQuotaPackage {
	if credits == nil {
		return nil
	}
	out := make([]workBuddyQuotaPackage, 0, len(credits.Packages))
	for _, pkg := range credits.Packages {
		entry := workBuddyQuotaPackage{
			Name:   pkg.Name,
			Remain: pkg.Remain,
			Used:   pkg.Used,
			Size:   pkg.Size,
		}
		if pkg.EndTime != "" {
			if expiry, err := time.ParseInLocation("2006-01-02 15:04:05", pkg.EndTime, time.FixedZone("CST", 8*3600)); err == nil {
				entry.ExpiresAt = expiry.Format(time.RFC3339)
				entry.ExpiresSoon = entry.Remain > 0 && !expiry.After(now.Add(7*24*time.Hour))
			}
		}
		out = append(out, entry)
	}
	return out
}
