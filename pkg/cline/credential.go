package cline

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const BaseURL = "https://api.cline.bot"
const ClientVersion = "0.0.32"

type Credential struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"`
}

// ParseCredential accepts an exported credential, a Desktop auth object, or providers.json.
func ParseCredential(raw string) (*Credential, error) {
	var obj map[string]any
	if common.UnmarshalJsonStr(raw, &obj) != nil || obj == nil {
		return nil, fmt.Errorf("Cline credential must be a JSON object")
	}
	if providers, ok := obj["providers"].(map[string]any); ok {
		p, _ := providers["cline"].(map[string]any)
		settings, _ := p["settings"].(map[string]any)
		obj, _ = settings["auth"].(map[string]any)
	} else if auth, ok := obj["auth"].(map[string]any); ok {
		obj = auth
	}
	access, _ := obj["accessToken"].(string)
	if access == "" {
		access, _ = obj["access_token"].(string)
	}
	refresh, _ := obj["refreshToken"].(string)
	if refresh == "" {
		refresh, _ = obj["refresh_token"].(string)
	}
	c := &Credential{AccessToken: strings.TrimSpace(access), RefreshToken: strings.TrimSpace(refresh)}
	if c.RefreshToken == "" {
		return nil, fmt.Errorf("Cline credential requires refreshToken for automatic renewal")
	}
	expiry := obj["expiresAt"]
	if expiry == nil {
		expiry = obj["expires_at"]
	}
	switch v := expiry.(type) {
	case float64:
		c.ExpiresAt = int64(v)
		if c.ExpiresAt < 1e12 {
			c.ExpiresAt *= 1000
		}
	case string:
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			c.ExpiresAt = t.UnixMilli()
		}
	}
	if c.ExpiresAt == 0 {
		parts := strings.Split(strings.TrimPrefix(c.AccessToken, "workos:"), ".")
		if len(parts) == 3 {
			if b, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
				var p struct {
					Exp int64 `json:"exp"`
				}
				if common.Unmarshal(b, &p) == nil {
					c.ExpiresAt = p.Exp * 1000
				}
			}
		}
	}
	return c, nil
}
func (c *Credential) NeedsRefresh(now time.Time) bool {
	return c.AccessToken == "" || c.ExpiresAt <= now.Add(5*time.Minute).UnixMilli()
}
func (c *Credential) Bearer() string { return "workos:" + strings.TrimPrefix(c.AccessToken, "workos:") }
