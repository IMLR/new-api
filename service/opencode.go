package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	opencodeapi "github.com/QuantumNous/new-api/pkg/opencode"
)

// ResolveOpenCodeGoKey reads the API key of one OpenCode Go channel. The channel
// holds a single key because the subscription and its quota windows belong to
// one account.
func ResolveOpenCodeGoKey(channel *model.Channel) (string, error) {
	if channel == nil || channel.Type != constant.ChannelTypeOpenCodeGo {
		return "", fmt.Errorf("channel type is not OpenCode Go")
	}
	if channel.ChannelInfo.IsMultiKey {
		return "", fmt.Errorf("OpenCode Go requires a single-key channel")
	}
	return opencodeapi.ParseKey(channel.Key)
}

// OpenCodeGoBaseURL returns the upstream base URL of one channel.
func OpenCodeGoBaseURL(channel *model.Channel) string {
	if channel != nil {
		if base := strings.TrimSpace(channel.GetBaseURL()); base != "" {
			return base
		}
	}
	return opencodeapi.BaseURL
}

// FetchOpenCodeGoChannelModels lists the models of one OpenCode Go channel. The
// model endpoint is public, so the catalog is readable before the key is valid.
func FetchOpenCodeGoChannelModels(channel *model.Channel) ([]string, error) {
	key, err := ResolveOpenCodeGoKey(channel)
	if err != nil {
		return nil, err
	}
	client, err := NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return opencodeapi.Models(ctx, client, OpenCodeGoBaseURL(channel), key)
}

// FetchOpenCodeGoChannelUsage reads the subscription windows of one channel.
func FetchOpenCodeGoChannelUsage(channel *model.Channel) (*opencodeapi.Usage, error) {
	key, err := ResolveOpenCodeGoKey(channel)
	if err != nil {
		return nil, err
	}
	client, err := NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return opencodeapi.FetchUsage(ctx, client, OpenCodeGoBaseURL(channel), key)
}
