package cline

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func SetHeaders(h http.Header) {
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", "Cline/"+ClientVersion)
	h.Set("X-CLIENT-TYPE", "cline-desktop")
	h.Set("X-CLIENT-VERSION", ClientVersion)
	h.Set("X-PLATFORM", "Cline Desktop")
	h.Set("X-PLATFORM-VERSION", ClientVersion)
}

type HTTPError struct {
	Status int
	Reauth bool
}

func (e *HTTPError) Error() string {
	if e.Reauth {
		return "Cline credential revoked or expired; import a new credential"
	}
	return fmt.Sprintf("Cline upstream HTTP %d", e.Status)
}

func Refresh(ctx context.Context, client *http.Client, base string, old *Credential) (*Credential, error) {
	body, err := common.Marshal(map[string]string{"refreshToken": old.RefreshToken, "grantType": "refresh_token"})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/refresh", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	SetHeaders(req.Header)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Cline refresh transport failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, &HTTPError{Status: resp.StatusCode, Reauth: resp.StatusCode == 400 || resp.StatusCode == 401 || resp.StatusCode == 403}
	}
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresAt    string `json:"expiresAt"`
		} `json:"data"`
	}
	if common.DecodeJson(io.LimitReader(resp.Body, 1<<20), &result) != nil || !result.Success || result.Data.AccessToken == "" {
		return nil, fmt.Errorf("Invalid Cline refresh response")
	}
	expires, err := time.Parse(time.RFC3339Nano, result.Data.ExpiresAt)
	if err != nil || !expires.After(time.Now()) {
		return nil, fmt.Errorf("Invalid Cline token expiry")
	}
	refresh := result.Data.RefreshToken
	if refresh == "" {
		refresh = old.RefreshToken
	}
	return &Credential{AccessToken: result.Data.AccessToken, RefreshToken: refresh, ExpiresAt: expires.UnixMilli()}, nil
}

func get(ctx context.Context, client *http.Client, base, path string, credential *Credential, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return 0, err
	}
	SetHeaders(req.Header)
	req.Header.Set("Authorization", "Bearer "+credential.Bearer())
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("Cline account request failed")
	}
	defer resp.Body.Close()
	if err := common.DecodeJson(io.LimitReader(resp.Body, 4<<20), out); err != nil {
		return resp.StatusCode, fmt.Errorf("Invalid Cline account response (HTTP %d)", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

type Plan struct {
	SubscriptionID   string `json:"subscriptionId"`
	CurrentPeriodEnd string `json:"currentPeriodEnd"`
	Status           string `json:"status"`
	Plan             *struct {
		Type        string `json:"type"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"plan"`
}

func (p *Plan) Active(now time.Time) bool {
	if p == nil || p.Plan == nil || p.SubscriptionID == "" {
		return false
	}
	if p.Status != "" && p.Status != "active" && p.Status != "trialing" {
		return false
	}
	end, err := time.Parse(time.RFC3339Nano, p.CurrentPeriodEnd)
	if p.CurrentPeriodEnd != "" && (err != nil || !end.After(now)) {
		return false
	}
	// ClinePass is the subscription catalog. General paid account plans are not ClinePass.
	name := strings.ToLower(p.Plan.Type + " " + p.Plan.Name + " " + p.Plan.DisplayName)
	return strings.Contains(strings.NewReplacer("-", "", "_", "", " ", "").Replace(name), "clinepass")
}

func Models(ctx context.Context, client *http.Client, base string, credential *Credential) ([]string, error) {
	var plan struct {
		Success bool   `json:"success"`
		Data    *Plan  `json:"data"`
		Error   string `json:"error"`
	}
	status, err := get(ctx, client, base, "/api/v1/users/me/plan", credential, &plan)
	if err != nil {
		return nil, err
	}
	if status == 404 && plan.Error == "no plan history found for user" {
		plan.Data = nil
	} else if status != 200 {
		return nil, &HTTPError{Status: status}
	} else if !plan.Success {
		return nil, fmt.Errorf("Cline subscription lookup failed")
	}
	type entry struct {
		ID string `json:"id"`
	}
	var catalog struct {
		Free []entry `json:"free"`
		Pass []entry `json:"clinePass"`
	}
	status, err = get(ctx, client, base, "/api/v1/ai/cline/recommended-models", credential, &catalog)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, &HTTPError{Status: status}
	}
	entries := catalog.Free
	if plan.Data.Active(time.Now()) {
		entries = append(entries, catalog.Pass...)
	}
	models := make([]string, 0, len(entries))
	seen := map[string]bool{}
	for _, m := range entries {
		id := strings.TrimSpace(m.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			models = append(models, id)
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("Cline returned an empty model catalog")
	}
	return models, nil
}
