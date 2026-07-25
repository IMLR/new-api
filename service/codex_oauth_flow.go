package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const (
	codexOAuthFlowTTL       = 10 * time.Minute
	codexOAuthFlowNamespace = "new-api:codex_oauth_flow:v1"
	codexOAuthInputMaxBytes = 16 << 10
)

var (
	errCodexOAuthFlowExpired = errors.New("codex oauth flow not found or expired")
	codexOAuthMemoryFlows    sync.Map
	codexOAuthConsumeScript  = redis.NewScript(`
local value = redis.call("GET", KEYS[1])
if value then
  redis.call("DEL", KEYS[1])
end
return value
`)
)

type codexOAuthFlowRecord struct {
	State       string `json:"state"`
	Verifier    string `json:"verifier"`
	RedirectURI string `json:"redirect_uri"`
	ProxyURL    string `json:"proxy_url,omitempty"`
	AdminID     int    `json:"admin_id"`
	ChannelID   int    `json:"channel_id"`
	ExpiresAt   int64  `json:"expires_at"`
}

type CodexOAuthAuthorizationFlow struct {
	FlowID       string    `json:"flow_id"`
	AuthorizeURL string    `json:"authorize_url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func StartCodexOAuthAuthorizationFlow(
	ctx context.Context,
	adminID int,
	channelID int,
	proxyURL string,
) (*CodexOAuthAuthorizationFlow, error) {
	if adminID <= 0 {
		return nil, errors.New("invalid administrator")
	}
	if channelID < 0 {
		return nil, errors.New("invalid channel")
	}

	normalizedProxy := strings.TrimSpace(proxyURL)
	if _, err := getCodexOAuthHTTPClient(normalizedProxy); err != nil {
		return nil, errors.New("invalid channel proxy")
	}

	flowID, err := codexOAuthRandomValue(32)
	if err != nil {
		return nil, err
	}
	state, err := codexOAuthRandomValue(32)
	if err != nil {
		return nil, err
	}
	verifier, err := codexOAuthRandomValue(64)
	if err != nil {
		return nil, err
	}
	challengeSum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeSum[:])
	authorizeURL, err := buildCodexAuthorizeURL(state, challenge)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(codexOAuthFlowTTL)
	record := codexOAuthFlowRecord{
		State:       state,
		Verifier:    verifier,
		RedirectURI: codexOAuthRedirectURI,
		ProxyURL:    normalizedProxy,
		AdminID:     adminID,
		ChannelID:   channelID,
		ExpiresAt:   expiresAt.Unix(),
	}
	if err := storeCodexOAuthFlow(ctx, flowID, record); err != nil {
		return nil, errors.New("failed to store codex oauth flow")
	}

	return &CodexOAuthAuthorizationFlow{
		FlowID:       flowID,
		AuthorizeURL: authorizeURL,
		ExpiresAt:    expiresAt,
	}, nil
}

func CompleteCodexOAuthAuthorizationFlow(
	ctx context.Context,
	adminID int,
	channelID int,
	flowID string,
	callbackURL string,
) (*CodexOAuthTokenResult, error) {
	normalizedFlowID := strings.TrimSpace(flowID)
	if normalizedFlowID == "" || len(normalizedFlowID) > 128 {
		return nil, errCodexOAuthFlowExpired
	}

	record, err := consumeCodexOAuthFlow(ctx, normalizedFlowID)
	if err != nil {
		return nil, err
	}
	if record.AdminID != adminID || record.ChannelID != channelID {
		return nil, errors.New("codex oauth flow does not match this administrator or channel")
	}
	if record.ExpiresAt <= time.Now().Unix() {
		return nil, errCodexOAuthFlowExpired
	}

	code, state, err := parseCodexOAuthCallbackURL(callbackURL)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(state), []byte(record.State)) != 1 {
		return nil, errors.New("codex oauth state mismatch")
	}

	client, err := getCodexOAuthHTTPClient(record.ProxyURL)
	if err != nil {
		return nil, errors.New("invalid channel proxy")
	}
	return exchangeCodexAuthorizationCode(
		ctx,
		client,
		codexOAuthTokenURL,
		codexOAuthClientID,
		code,
		record.Verifier,
		record.RedirectURI,
	)
}

func parseCodexOAuthCallbackURL(input string) (string, string, error) {
	value := strings.TrimSpace(input)
	if value == "" || len(value) > codexOAuthInputMaxBytes {
		return "", "", errors.New("invalid codex oauth callback URL")
	}
	u, err := url.Parse(value)
	if err != nil ||
		!strings.EqualFold(u.Scheme, "http") ||
		!strings.EqualFold(u.Hostname(), "localhost") ||
		u.Port() != "1455" ||
		u.Path != "/auth/callback" ||
		u.User != nil ||
		u.Fragment != "" {
		return "", "", errors.New("invalid codex oauth callback URL")
	}

	query := u.Query()
	if strings.TrimSpace(query.Get("error")) != "" {
		return "", "", errors.New("codex oauth authorization was not completed")
	}
	code := strings.TrimSpace(query.Get("code"))
	state := strings.TrimSpace(query.Get("state"))
	if code == "" || state == "" {
		return "", "", errors.New("codex oauth callback URL is missing code or state")
	}
	return code, state, nil
}

func codexOAuthRandomValue(size int) (string, error) {
	if size <= 0 {
		return "", errors.New("invalid random value size")
	}
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func storeCodexOAuthFlow(ctx context.Context, flowID string, record codexOAuthFlowRecord) error {
	if common.RedisEnabled && common.RDB != nil {
		payload, err := common.Marshal(record)
		if err != nil {
			return err
		}
		return common.RDB.Set(
			ctx,
			fmt.Sprintf("%s:%s", codexOAuthFlowNamespace, flowID),
			payload,
			codexOAuthFlowTTL,
		).Err()
	}

	now := time.Now().Unix()
	codexOAuthMemoryFlows.Range(func(key, value any) bool {
		record, ok := value.(codexOAuthFlowRecord)
		if !ok || record.ExpiresAt <= now {
			codexOAuthMemoryFlows.Delete(key)
		}
		return true
	})
	codexOAuthMemoryFlows.Store(flowID, record)
	return nil
}

func consumeCodexOAuthFlow(ctx context.Context, flowID string) (*codexOAuthFlowRecord, error) {
	if common.RedisEnabled && common.RDB != nil {
		rawPayload, err := codexOAuthConsumeScript.Run(
			ctx,
			common.RDB,
			[]string{fmt.Sprintf("%s:%s", codexOAuthFlowNamespace, flowID)},
		).Result()
		if errors.Is(err, redis.Nil) {
			return nil, errCodexOAuthFlowExpired
		}
		if err != nil {
			return nil, errors.New("failed to consume codex oauth flow")
		}
		payload, ok := rawPayload.(string)
		if !ok {
			return nil, errors.New("invalid codex oauth flow")
		}
		var record codexOAuthFlowRecord
		if err := common.Unmarshal([]byte(payload), &record); err != nil {
			return nil, errors.New("invalid codex oauth flow")
		}
		return &record, nil
	}

	value, ok := codexOAuthMemoryFlows.LoadAndDelete(flowID)
	if !ok {
		return nil, errCodexOAuthFlowExpired
	}
	record, ok := value.(codexOAuthFlowRecord)
	if !ok {
		return nil, errors.New("invalid codex oauth flow")
	}
	return &record, nil
}
