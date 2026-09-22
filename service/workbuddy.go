package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
)

// workBuddyBase resolves the upstream host for one channel. A base URL set by
// the operator wins; the built-in default follows the account realm, because a
// CN account talks to copilot.tencent.com while a global account talks to
// www.workbuddy.ai, and on CN the billing host differs from the chat host.
func workBuddyBase(ch *model.Channel, credential *workbuddyapi.Credential, billing bool) string {
	if ch != nil {
		if configured := strings.TrimSpace(ch.GetBaseURL()); configured != "" && configured != workbuddyapi.ChatBaseCN {
			return configured
		}
	}
	if credential == nil {
		return workbuddyapi.ChatBaseCN
	}
	if billing {
		return credential.BillingBase()
	}
	return credential.ChatBase()
}

// workBuddyCredentialError spells out the way out of a dead upstream session,
// which the raw identity provider error does not mention.
func workBuddyCredentialError(err error) error {
	if workbuddyapi.IsSessionExpired(err) {
		return fmt.Errorf("WorkBuddy sign-in has expired, sign in again from the channel credentials: %w", err)
	}
	return err
}

// ResolveWorkBuddyCredential re-reads the persisted credential under the refresh
// lock and renews the token when it is about to expire. rejectedToken coalesces
// concurrent retries after an upstream rejection.
func ResolveWorkBuddyCredential(ctx context.Context, id int, rejectedToken string) (*workbuddyapi.Credential, error) {
	var result *workbuddyapi.Credential
	err := model.WithWorkBuddyCredentialLock(ctx, id, func(ch *model.Channel) (string, error) {
		credential, err := workbuddyapi.ParseCredential(ch.Key)
		if err != nil {
			return "", err
		}
		result = credential
		if !credential.NeedsRefresh(time.Now(), 5*time.Minute) && (rejectedToken == "" || credential.AccessToken != rejectedToken) {
			return "", nil
		}
		client, err := workbuddyapi.NewUpstreamClient(ch.GetSetting().Proxy)
		if err != nil {
			return "", err
		}
		refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		refreshed, err := workbuddyapi.Refresh(refreshCtx, client, workBuddyBase(ch, credential, false), credential)
		if err != nil {
			return "", workBuddyCredentialError(err)
		}
		result = refreshed
		encoded, err := common.Marshal(refreshed)
		return string(encoded), err
	})
	return result, err
}

// FetchWorkBuddyChannelModels reads the upstream model catalog for the channel.
func FetchWorkBuddyChannelModels(ch *model.Channel) ([]string, error) {
	if ch == nil || ch.Type != constant.ChannelTypeWorkBuddy || ch.ChannelInfo.IsMultiKey {
		return nil, fmt.Errorf("WorkBuddy requires a single-credential channel")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	credential, err := workbuddyapi.ParseCredential(ch.Key)
	if err != nil {
		return nil, err
	}
	if ch.Id > 0 {
		credential, err = ResolveWorkBuddyCredential(ctx, ch.Id, "")
		if err != nil {
			return nil, err
		}
	}
	client, err := workbuddyapi.NewUpstreamClient(ch.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	infos, err := workbuddyapi.Models(ctx, client, workBuddyBase(ch, credential, false), credential)
	var apiErr *workbuddyapi.HTTPError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized && ch.Id > 0 {
		credential, err = ResolveWorkBuddyCredential(ctx, ch.Id, credential.AccessToken)
		if err != nil {
			return nil, err
		}
		infos, err = workbuddyapi.Models(ctx, client, workBuddyBase(ch, credential, false), credential)
	}
	if err != nil {
		return nil, err
	}
	cacheWorkBuddyModels(ch.Id, infos)
	ids := make([]string, 0, len(infos))
	for _, info := range infos {
		ids = append(ids, info.ID)
	}
	return ids, nil
}

// FetchWorkBuddyChannelCredits reads the credit balance of the channel account.
func FetchWorkBuddyChannelCredits(ch *model.Channel) (*workbuddyapi.Credits, error) {
	if ch == nil || ch.Type != constant.ChannelTypeWorkBuddy || ch.ChannelInfo.IsMultiKey {
		return nil, fmt.Errorf("WorkBuddy requires a single-credential channel")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	credential, err := workbuddyapi.ParseCredential(ch.Key)
	if err != nil {
		return nil, err
	}
	if ch.Id > 0 {
		credential, err = ResolveWorkBuddyCredential(ctx, ch.Id, "")
		if err != nil {
			return nil, err
		}
	}
	client, err := workbuddyapi.NewUpstreamClient(ch.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	base := workBuddyBase(ch, credential, true)
	credits, err := workbuddyapi.FetchCredits(ctx, client, base, credential)
	var apiErr *workbuddyapi.HTTPError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized && ch.Id > 0 {
		credential, err = ResolveWorkBuddyCredential(ctx, ch.Id, credential.AccessToken)
		if err != nil {
			return nil, err
		}
		credits, err = workbuddyapi.FetchCredits(ctx, client, base, credential)
	}
	return credits, workBuddyCredentialError(err)
}

// The request pipeline lowers reasoning effort to the levels the upstream
// accepts, which needs the catalog. The catalog changes rarely, so it is cached
// per channel and an unavailable cache simply skips the downgrade.
type workBuddyModelCacheEntry struct {
	infos     []workbuddyapi.ModelInfo
	expiresAt time.Time
}

var workBuddyModelCache sync.Map

const workBuddyModelCacheTTL = 10 * time.Minute

func cacheWorkBuddyModels(channelID int, infos []workbuddyapi.ModelInfo) {
	if channelID <= 0 || len(infos) == 0 {
		return
	}
	workBuddyModelCache.Store(channelID, workBuddyModelCacheEntry{
		infos:     infos,
		expiresAt: time.Now().Add(workBuddyModelCacheTTL),
	})
}

// WorkBuddyModelCapabilities returns the thinking levels each model supports,
// keyed by model id, or empty tables when the catalog is unavailable.
func WorkBuddyModelCapabilities(channelID int) (map[string][]string, map[string]string) {
	entry, ok := workBuddyModelCache.Load(channelID)
	if !ok {
		return nil, nil
	}
	cached := entry.(workBuddyModelCacheEntry)
	if time.Now().After(cached.expiresAt) {
		workBuddyModelCache.Delete(channelID)
		return nil, nil
	}
	supported := make(map[string][]string, len(cached.infos))
	defaults := make(map[string]string, len(cached.infos))
	for _, info := range cached.infos {
		if len(info.Efforts) > 0 {
			supported[info.ID] = info.Efforts
		}
		if info.DefaultEffort != "" {
			defaults[info.ID] = info.DefaultEffort
		}
	}
	return supported, defaults
}

// RefreshWorkBuddyChannelModels fetches the catalog for a channel that has no
// in-memory cache entry yet, so a relay can apply the effort downgrade.
func RefreshWorkBuddyChannelModels(channelID int) {
	if _, ok := workBuddyModelCache.Load(channelID); ok {
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil || channel == nil {
		return
	}
	_, _ = FetchWorkBuddyChannelModels(channel)
}

var workBuddyRefreshOnce sync.Once

// StartWorkBuddyCredentialAutoRefreshTask renews the tokens of the stored
// channels in the background: the upstream access token lives 60 days, but the
// upstream also invalidates sessions on its own, so a periodic renewal keeps
// the channel usable without a manual re-import.
func StartWorkBuddyCredentialAutoRefreshTask() {
	workBuddyRefreshOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		go func() {
			refreshWorkBuddyChannels()
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				refreshWorkBuddyChannels()
			}
		}()
	})
}

func refreshWorkBuddyChannels() {
	lastID := 0
	for {
		var channels []model.Channel
		if err := model.DB.Select("id").Where("type = ? AND status IN ? AND id > ?", constant.ChannelTypeWorkBuddy, []int{common.ChannelStatusEnabled, common.ChannelStatusAutoDisabled}, lastID).Order("id").Limit(100).Find(&channels).Error; err != nil {
			common.SysError("WorkBuddy credential scan failed")
			return
		}
		if len(channels) == 0 {
			return
		}
		for _, ch := range channels {
			lastID = ch.Id
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			_, err := ResolveWorkBuddyCredential(ctx, ch.Id, "")
			cancel()
			if err != nil {
				common.SysError(fmt.Sprintf("WorkBuddy channel %d credential renewal failed: %v", ch.Id, err))
			}
			RefreshWorkBuddyChannelModels(ch.Id)
		}
	}
}
