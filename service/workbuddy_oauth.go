package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
	"github.com/go-redis/redis/v8"
)

const (
	workBuddyOAuthFlowTTL       = 10 * time.Minute
	workBuddyOAuthFlowNamespace = "new-api:workbuddy_oauth_flow:v1"
)

var (
	errWorkBuddyOAuthFlowExpired = errors.New("workbuddy authorization flow not found or expired")
	workBuddyOAuthMemoryFlows    sync.Map
	workBuddyOAuthConsumeScript  = redis.NewScript(`
local value = redis.call("GET", KEYS[1])
if value then
  redis.call("DEL", KEYS[1])
end
return value
`)
)

type workBuddyOAuthFlowRecord struct {
	State     string `json:"state"`
	Realm     string `json:"realm"`
	ProxyURL  string `json:"proxy_url,omitempty"`
	AdminID   int    `json:"admin_id"`
	ExpiresAt int64  `json:"expires_at"`
}

// workBuddyLoginBase resolves the login host per realm; tests replace it.
var workBuddyLoginBase = workbuddyapi.LoginBase

// WorkBuddyAuthorizationFlow is one started sign-in.
type WorkBuddyAuthorizationFlow struct {
	FlowID       string    `json:"flow_id"`
	AuthorizeURL string    `json:"authorize_url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// WorkBuddyAuthorizationResult reports whether the browser sign-in finished.
type WorkBuddyAuthorizationResult struct {
	Status     string                     `json:"status"`
	Credential string                     `json:"credential,omitempty"`
	Realm      string                     `json:"realm,omitempty"`
	Account    *workbuddyapi.LoginAccount `json:"account,omitempty"`
	Domain     string                     `json:"domain,omitempty"`
}

// StartWorkBuddyAuthorizationFlow opens a login session on the upstream and
// remembers its state, so the administrator only has to open the returned page.
func StartWorkBuddyAuthorizationFlow(ctx context.Context, adminID int, realm, proxyURL string) (*WorkBuddyAuthorizationFlow, error) {
	if adminID <= 0 {
		return nil, errors.New("invalid administrator")
	}
	if realm != workbuddyapi.RealmGlobal {
		realm = workbuddyapi.RealmCN
	}
	client, err := NewProxyHttpClient(proxyURL)
	if err != nil {
		return nil, errors.New("invalid channel proxy")
	}
	session, err := workbuddyapi.StartAuthorization(ctx, client, realm, workBuddyLoginBase(realm))
	if err != nil {
		return nil, err
	}
	flowID, err := workBuddyOAuthRandomValue(24)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(workBuddyOAuthFlowTTL)
	record := workBuddyOAuthFlowRecord{
		State:     session.State,
		Realm:     realm,
		ProxyURL:  proxyURL,
		AdminID:   adminID,
		ExpiresAt: expiresAt.Unix(),
	}
	if err := storeWorkBuddyOAuthFlow(ctx, flowID, record); err != nil {
		return nil, errors.New("failed to store workbuddy authorization flow")
	}
	return &WorkBuddyAuthorizationFlow{
		FlowID:       flowID,
		AuthorizeURL: session.AuthURL,
		ExpiresAt:    expiresAt,
	}, nil
}

// PollWorkBuddyAuthorizationFlow asks the upstream whether the sign-in already
// finished. The flow stays available while it is pending, so the caller may
// confirm several times; it is consumed once the credential exists.
func PollWorkBuddyAuthorizationFlow(ctx context.Context, adminID int, flowID string) (*WorkBuddyAuthorizationResult, error) {
	normalized := trimFlowID(flowID)
	if normalized == "" {
		return nil, errWorkBuddyOAuthFlowExpired
	}
	record, err := loadWorkBuddyOAuthFlow(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if record.AdminID != adminID {
		return nil, errors.New("workbuddy authorization flow does not match this administrator")
	}
	if record.ExpiresAt <= time.Now().Unix() {
		removeWorkBuddyOAuthFlow(ctx, normalized)
		return nil, errWorkBuddyOAuthFlowExpired
	}
	client, err := NewProxyHttpClient(record.ProxyURL)
	if err != nil {
		return nil, errors.New("invalid channel proxy")
	}
	tokens, account, err := workbuddyapi.PollAuthorization(ctx, client, record.Realm, workBuddyLoginBase(record.Realm), record.State)
	if errors.Is(err, workbuddyapi.ErrAuthorizationPending) {
		return &WorkBuddyAuthorizationResult{Status: "pending", Realm: record.Realm}, nil
	}
	if err != nil {
		return nil, err
	}
	credential := workbuddyapi.CredentialFromLogin(tokens, account, record.Realm)
	if credential == nil {
		return &WorkBuddyAuthorizationResult{Status: "pending", Realm: record.Realm}, nil
	}
	encoded, err := common.Marshal(credential)
	if err != nil {
		return nil, err
	}
	removeWorkBuddyOAuthFlow(ctx, normalized)
	return &WorkBuddyAuthorizationResult{
		Status:     "ready",
		Credential: string(encoded),
		Realm:      record.Realm,
		Account:    account,
		Domain:     credential.Domain,
	}, nil
}

func trimFlowID(flowID string) string {
	trimmed := strings.TrimSpace(flowID)
	if len(trimmed) == 0 || len(trimmed) > 128 {
		return ""
	}
	return trimmed
}

func workBuddyOAuthRandomValue(size int) (string, error) {
	if size <= 0 {
		return "", errors.New("invalid random value size")
	}
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func workBuddyOAuthKey(flowID string) string {
	return fmt.Sprintf("%s:%s", workBuddyOAuthFlowNamespace, flowID)
}

func storeWorkBuddyOAuthFlow(ctx context.Context, flowID string, record workBuddyOAuthFlowRecord) error {
	if common.RedisEnabled && common.RDB != nil {
		payload, err := common.Marshal(record)
		if err != nil {
			return err
		}
		return common.RDB.Set(ctx, workBuddyOAuthKey(flowID), payload, workBuddyOAuthFlowTTL).Err()
	}
	now := time.Now().Unix()
	workBuddyOAuthMemoryFlows.Range(func(key, value any) bool {
		existing, ok := value.(workBuddyOAuthFlowRecord)
		if !ok || existing.ExpiresAt <= now {
			workBuddyOAuthMemoryFlows.Delete(key)
		}
		return true
	})
	workBuddyOAuthMemoryFlows.Store(flowID, record)
	return nil
}

func loadWorkBuddyOAuthFlow(ctx context.Context, flowID string) (*workBuddyOAuthFlowRecord, error) {
	if common.RedisEnabled && common.RDB != nil {
		payload, err := common.RDB.Get(ctx, workBuddyOAuthKey(flowID)).Result()
		if errors.Is(err, redis.Nil) {
			return nil, errWorkBuddyOAuthFlowExpired
		}
		if err != nil {
			return nil, errors.New("failed to read workbuddy authorization flow")
		}
		var record workBuddyOAuthFlowRecord
		if err := common.Unmarshal([]byte(payload), &record); err != nil {
			return nil, errors.New("invalid workbuddy authorization flow")
		}
		return &record, nil
	}
	value, ok := workBuddyOAuthMemoryFlows.Load(flowID)
	if !ok {
		return nil, errWorkBuddyOAuthFlowExpired
	}
	record, ok := value.(workBuddyOAuthFlowRecord)
	if !ok {
		return nil, errors.New("invalid workbuddy authorization flow")
	}
	return &record, nil
}

func removeWorkBuddyOAuthFlow(ctx context.Context, flowID string) {
	if common.RedisEnabled && common.RDB != nil {
		_, _ = workBuddyOAuthConsumeScript.Run(ctx, common.RDB, []string{workBuddyOAuthKey(flowID)}).Result()
		return
	}
	workBuddyOAuthMemoryFlows.Delete(flowID)
}
