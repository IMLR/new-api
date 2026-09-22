// Package workbuddy implements the CodeBuddy / WorkBuddy upstream protocol:
// account credentials with token renewal, the request header families, the
// chat request body pipeline, the model catalog and the credit balance.
//
// The upstream is a Tencent CodeBuddy deployment: CN accounts talk to
// copilot.tencent.com, global accounts to www.workbuddy.ai. Both speak the
// OpenAI chat completions shape with extra request fields and header
// requirements, and both refuse non-streaming requests.
package workbuddy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	// ChatBaseCN and BillingBaseCN serve CN accounts.
	ChatBaseCN    = "https://copilot.tencent.com"
	BillingBaseCN = "https://www.codebuddy.cn"
	// GlobalBase serves global accounts; the international deployment uses one
	// host for chat and billing.
	GlobalBase = "https://www.workbuddy.ai"

	ChatCompletionsPath = "/v2/chat/completions"
	RefreshPath         = "/v2/plugin/auth/token/refresh"
	EnterpriseModelsCN  = "/console/enterprises/personal/models"
	EnterpriseModelsInt = "/v2/enterprises/personal/models"
	ConfigPath          = "/v3/config"
	BillingMeterPath    = "/v2/billing/meter/get-user-resource"
	BillingMeterPathInt = "/billing/meter/get-user-resource"

	// Client versions taken from the official desktop build and its bundled
	// CLI; the upstream rejects requests that look like a plain HTTP client.
	ClientVersion = "5.5.4"
	CliVersion    = "2.137.1"
)

// RealmGlobal and RealmCN are the two upstream deployments.
const (
	RealmCN     = "cn"
	RealmGlobal = "global"
)

// Credential is one CodeBuddy account. The channel key stores this JSON; the
// upstream token rotation writes the refreshed values back.
type Credential struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
	Domain       string `json:"domain,omitempty"`
	Realm        string `json:"realm,omitempty"`
	UID          string `json:"uid,omitempty"`
	EnterpriseID string `json:"enterpriseId,omitempty"`
	Nickname     string `json:"nickname,omitempty"`
	DeviceToken  string `json:"device_token,omitempty"`
}

// nestedCredential mirrors the plugin OAuth export: {"auth":{...},"account":{...}}.
type nestedCredential struct {
	Auth struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"`
		Domain       string `json:"domain"`
		Realm        string `json:"realm"`
	} `json:"auth"`
	Account struct {
		UID          string `json:"uid"`
		EnterpriseID string `json:"enterpriseId"`
		Nickname     string `json:"nickname"`
	} `json:"account"`
	DeviceToken string `json:"device_token"`
}

// flatCredential mirrors the hand written form, which keeps every field on the
// top level.
type flatCredential struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"`
	Domain       string `json:"domain"`
	Realm        string `json:"realm"`
	UID          string `json:"uid"`
	EnterpriseID string `json:"enterpriseId"`
	Nickname     string `json:"nickname"`
	DeviceToken  string `json:"device_token"`
}

// ParseCredential accepts both storage shapes of the WorkBuddy credential file
// and requires a refresh token, which is what makes automatic renewal possible.
func ParseCredential(raw string) (*Credential, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("workbuddy credential is empty")
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		return nil, fmt.Errorf("workbuddy credential must be a JSON object")
	}
	credential := &Credential{}
	if _, nested := probe["auth"]; nested {
		var parsed nestedCredential
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil, fmt.Errorf("workbuddy credential parse failed: %w", err)
		}
		credential.AccessToken = parsed.Auth.AccessToken
		credential.RefreshToken = parsed.Auth.RefreshToken
		credential.ExpiresAt = normalizeExpiresAt(parsed.Auth.ExpiresAt)
		credential.Domain = parsed.Auth.Domain
		credential.Realm = parsed.Auth.Realm
		credential.UID = parsed.Account.UID
		credential.EnterpriseID = parsed.Account.EnterpriseID
		credential.Nickname = parsed.Account.Nickname
		credential.DeviceToken = parsed.DeviceToken
	} else {
		var parsed flatCredential
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil, fmt.Errorf("workbuddy credential parse failed: %w", err)
		}
		credential.AccessToken = parsed.AccessToken
		credential.RefreshToken = parsed.RefreshToken
		credential.ExpiresAt = normalizeExpiresAt(parsed.ExpiresAt)
		credential.Domain = parsed.Domain
		credential.Realm = parsed.Realm
		credential.UID = parsed.UID
		credential.EnterpriseID = parsed.EnterpriseID
		credential.Nickname = parsed.Nickname
		credential.DeviceToken = parsed.DeviceToken
	}
	credential.AccessToken = strings.TrimSpace(credential.AccessToken)
	credential.RefreshToken = strings.TrimSpace(credential.RefreshToken)
	credential.Domain = strings.TrimSpace(credential.Domain)
	credential.Realm = strings.ToLower(strings.TrimSpace(credential.Realm))
	credential.UID = strings.TrimSpace(credential.UID)
	credential.EnterpriseID = strings.TrimSpace(credential.EnterpriseID)
	credential.Nickname = strings.TrimSpace(credential.Nickname)
	credential.DeviceToken = strings.TrimSpace(credential.DeviceToken)
	if credential.RefreshToken == "" {
		return nil, fmt.Errorf("workbuddy credential requires refreshToken for automatic renewal")
	}
	if credential.Realm != "" && credential.Realm != RealmCN && credential.Realm != RealmGlobal {
		return nil, fmt.Errorf("workbuddy credential realm must be %q or %q", RealmCN, RealmGlobal)
	}
	return credential, nil
}

// normalizeExpiresAt accepts both seconds and milliseconds; the upstream and
// the plugin export use seconds, hand written files sometimes use milliseconds.
func normalizeExpiresAt(value int64) int64 {
	if value > 1e12 {
		return value / 1000
	}
	return value
}

// EffectiveRealm resolves the deployment: an explicit realm wins, otherwise the
// domain decides, and an unknown domain stays on CN.
func (c *Credential) EffectiveRealm() string {
	if c == nil {
		return RealmCN
	}
	if c.Realm == RealmGlobal {
		return RealmGlobal
	}
	if c.Realm == RealmCN {
		return RealmCN
	}
	if IsGlobalDomain(c.Domain) {
		return RealmGlobal
	}
	return RealmCN
}

// IsGlobal reports whether the account talks to the international deployment.
func (c *Credential) IsGlobal() bool { return c.EffectiveRealm() == RealmGlobal }

// IsGlobalDomain reports whether a domain points at the international host.
func IsGlobalDomain(domain string) bool {
	lowered := strings.ToLower(strings.TrimSpace(domain))
	return lowered == "workbuddy.ai" || strings.HasSuffix(lowered, ".workbuddy.ai")
}

// ChatBase is the host that serves chat and model requests for the account.
func (c *Credential) ChatBase() string {
	if c.IsGlobal() {
		return GlobalBase
	}
	return ChatBaseCN
}

// BillingBase is the host that serves billing requests for the account.
func (c *Credential) BillingBase() string {
	if c.IsGlobal() {
		return GlobalBase
	}
	return BillingBaseCN
}

// NeedsRefresh reports whether the access token expires inside the window, or
// has an unknown expiry.
func (c *Credential) NeedsRefresh(now time.Time, within time.Duration) bool {
	if c == nil || c.AccessToken == "" {
		return true
	}
	if c.ExpiresAt <= 0 {
		return true
	}
	return now.Add(within).Unix() >= c.ExpiresAt
}

// Label is a short account name for logs.
func (c *Credential) Label() string {
	if c == nil {
		return "workbuddy"
	}
	if c.Nickname != "" {
		return c.Nickname
	}
	if c.UID != "" {
		return c.UID
	}
	return c.EffectiveRealm()
}

// MaskUID hides all but the last four characters of the account id.
func (c *Credential) MaskUID() string {
	if c == nil {
		return ""
	}
	uid := c.UID
	if len(uid) <= 4 {
		return uid
	}
	return "****" + uid[len(uid)-4:]
}
