package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cline"
)

// ResolveClineCredential always re-reads the persisted key under the refresh lock.
// rejectedToken coalesces concurrent 401 retries against the same rotated token.
func ResolveClineCredential(ctx context.Context, id int, rejectedToken string) (*cline.Credential, error) {
	var result *cline.Credential
	err := model.WithClineCredentialLock(ctx, id, func(ch *model.Channel) (string, error) {
		credential, err := cline.ParseCredential(ch.Key)
		if err != nil {
			return "", err
		}
		result = credential
		if !credential.NeedsRefresh(time.Now()) && (rejectedToken == "" || credential.AccessToken != rejectedToken) {
			return "", nil
		}
		client, err := NewProxyHttpClient(ch.GetSetting().Proxy)
		if err != nil {
			return "", err
		}
		refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		base := ch.GetBaseURL()
		if base == "" {
			base = cline.BaseURL
		}
		result, err = cline.Refresh(refreshCtx, client, base, credential)
		if err != nil {
			return "", err
		}
		encoded, err := common.Marshal(result)
		return string(encoded), err
	})
	return result, err
}

func FetchClineChannelModels(ch *model.Channel) ([]string, error) {
	if ch == nil || ch.Type != constant.ChannelTypeCline || ch.ChannelInfo.IsMultiKey {
		return nil, fmt.Errorf("Cline requires a single-credential channel")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	credential, err := cline.ParseCredential(ch.Key)
	if err != nil {
		return nil, err
	}
	if ch.Id == 0 {
		encoded, err := common.Marshal(credential)
		if err != nil {
			return nil, err
		}
		ch.Key = string(encoded)
	}
	if ch.Id > 0 {
		credential, err = ResolveClineCredential(ctx, ch.Id, "")
	}
	if err != nil {
		return nil, err
	}
	client, err := NewProxyHttpClient(ch.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	base := ch.GetBaseURL()
	if base == "" {
		base = cline.BaseURL
	}
	if ch.Id == 0 && credential.NeedsRefresh(time.Now()) {
		credential, err = cline.Refresh(ctx, client, base, credential)
		if err != nil {
			return nil, err
		}
		encoded, err := common.Marshal(credential)
		if err != nil {
			return nil, err
		}
		ch.Key = string(encoded)
	}
	models, err := cline.Models(ctx, client, base, credential)
	var apiErr *cline.HTTPError
	if errors.As(err, &apiErr) && apiErr.Status == 401 && ch.Id > 0 {
		credential, err = ResolveClineCredential(ctx, ch.Id, credential.AccessToken)
		if err != nil {
			return nil, err
		}
		return cline.Models(ctx, client, base, credential)
	}
	return models, err
}

var clineRefreshOnce sync.Once

func StartClineCredentialAutoRefreshTask() {
	clineRefreshOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		go func() {
			refreshClineChannels()
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				refreshClineChannels()
			}
		}()
	})
}
func refreshClineChannels() {
	lastID := 0
	for {
		var channels []model.Channel
		if err := model.DB.Select("id").Where("type = ? AND status IN ? AND id > ?", constant.ChannelTypeCline, []int{common.ChannelStatusEnabled, common.ChannelStatusAutoDisabled}, lastID).Order("id").Limit(100).Find(&channels).Error; err != nil {
			common.SysError("Cline credential scan failed")
			return
		}
		if len(channels) == 0 {
			return
		}
		for _, ch := range channels {
			lastID = ch.Id
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			_, err := ResolveClineCredential(ctx, ch.Id, "")
			cancel()
			if err != nil {
				common.SysError(fmt.Sprintf("Cline channel %d credential renewal failed: %v", ch.Id, err))
			}
		}
	}
}
