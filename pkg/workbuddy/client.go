package workbuddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPError describes a non 2xx upstream answer.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("workbuddy upstream returned %d: %s", e.Status, truncate(e.Body, 200))
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

// Response envelope shared by every upstream JSON answer.
type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func readEnvelope(resp *http.Response) (json.RawMessage, error) {
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("workbuddy read body failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, &HTTPError{Status: resp.StatusCode, Body: string(raw)}
	}
	var parsed envelope
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("workbuddy response parse failed: %w (body: %s)", err, truncate(string(raw), 160))
	}
	if parsed.Code != 0 {
		return nil, &HTTPError{Status: resp.StatusCode, Body: fmt.Sprintf("code=%d msg=%s", parsed.Code, parsed.Msg)}
	}
	return parsed.Data, nil
}

// Refresh renews the access token. The upstream rotates both tokens, so the
// caller persists the returned credential.
func Refresh(ctx context.Context, client *http.Client, base string, credential *Credential) (*Credential, error) {
	if credential == nil || credential.RefreshToken == "" {
		return nil, fmt.Errorf("workbuddy credential has no refresh token")
	}
	if base == "" {
		base = credential.ChatBase()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+RefreshPath, nil)
	if err != nil {
		return nil, err
	}
	RefreshHeaders(request.Header, credential)
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	data, err := readEnvelope(resp)
	if err != nil {
		return nil, err
	}
	var payload struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int64  `json:"expiresIn"`
		Domain       string `json:"domain"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.AccessToken == "" {
		return nil, fmt.Errorf("workbuddy token refresh returned no access token, sign in again")
	}
	refreshed := *credential
	refreshed.AccessToken = payload.AccessToken
	if payload.RefreshToken != "" {
		refreshed.RefreshToken = payload.RefreshToken
	}
	if payload.Domain != "" {
		refreshed.Domain = payload.Domain
	}
	// The upstream reports 60 days; a missing or implausible value keeps the
	// previous expiry so the account still refreshes on schedule.
	if payload.ExpiresIn > 0 && payload.ExpiresIn < int64((10*365*24*time.Hour)/time.Second) {
		refreshed.ExpiresAt = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).Unix()
	}
	return &refreshed, nil
}

// ModelInfo is one entry of the upstream model catalog.
type ModelInfo struct {
	ID             string
	Name           string
	Description    string
	ContextWindow  int64
	MaxTokens      int64
	MaxAllowedSize int64
	Credits        string
	Vendor         string
	Tags           []string
	Efforts        []string
	DefaultEffort  string
	SupportsImages bool
	SupportsTools  bool
	IsDefault      bool
	Disabled       bool
}

// catalogEntry mirrors the upstream model object, shared by both endpoints.
type catalogEntry struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"descriptionZh"`
	Credits         string   `json:"credits"`
	Tags            []string `json:"tags"`
	Vendor          string   `json:"vendor"`
	IsDefault       bool     `json:"isDefault"`
	MaxInputTokens  int64    `json:"maxInputTokens"`
	MaxOutputTokens int64    `json:"maxOutputTokens"`
	MaxAllowedSize  int64    `json:"maxAllowedSize"`
	Disabled        bool     `json:"disabled"`
	SupportsImages  bool     `json:"supportsImages"`
	SupportsReason  bool     `json:"supportsReasoning"`
	SupportsTool    bool     `json:"supportsToolCall"`
	OnlyReasoning   bool     `json:"onlyReasoning"`
	Reasoning       struct {
		Effort           string   `json:"effort"`
		Summary          string   `json:"summary"`
		DefaultEffort    string   `json:"defaultEffort"`
		SupportedEfforts []string `json:"supportedEfforts"`
	} `json:"reasoning"`
	// The v3 configuration nests the catalog under data.models.
	Models []catalogEntry `json:"models"`
}

func (e catalogEntry) info() ModelInfo {
	return ModelInfo{
		ID:             strings.TrimSpace(e.ID),
		Name:           e.Name,
		Description:    e.Description,
		ContextWindow:  e.MaxInputTokens,
		MaxTokens:      e.MaxOutputTokens,
		MaxAllowedSize: e.MaxAllowedSize,
		Credits:        e.Credits,
		Vendor:         e.Vendor,
		Tags:           e.Tags,
		Efforts:        e.Reasoning.SupportedEfforts,
		DefaultEffort:  e.Reasoning.DefaultEffort,
		SupportsImages: e.SupportsImages,
		SupportsTools:  e.SupportsTool,
		IsDefault:      e.IsDefault,
		Disabled:       e.Disabled,
	}
}

// Models reads the model catalog. The upstream publishes it on two endpoints;
// the configuration endpoint is authoritative and the enterprise endpoint
// fills the models it does not list. A failure on one side keeps the other.
func Models(ctx context.Context, client *http.Client, base string, credential *Credential) ([]ModelInfo, error) {
	if credential == nil {
		return nil, fmt.Errorf("workbuddy credential unavailable")
	}
	if base == "" {
		base = credential.ChatBase()
	}
	base = strings.TrimRight(base, "/")
	var (
		wg                       sync.WaitGroup
		configModels, listModels []ModelInfo
		configErr, listErr       error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		configModels, configErr = fetchConfigModels(ctx, client, base, credential)
	}()
	go func() {
		defer wg.Done()
		listModels, listErr = fetchEnterpriseModels(ctx, client, base, credential)
	}()
	wg.Wait()
	if configErr != nil && listErr != nil {
		return nil, listErr
	}
	merged := mergeModels(configModels, listModels)
	if len(merged) == 0 {
		return nil, fmt.Errorf("workbuddy model catalog is empty")
	}
	return merged, nil
}

func fetchEnterpriseModels(ctx context.Context, client *http.Client, base string, credential *Credential) ([]ModelInfo, error) {
	path := EnterpriseModelsCN
	if credential.IsGlobal() {
		path = EnterpriseModelsInt
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	ModelHeaders(request.Header, credential)
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Models []catalogEntry `json:"models"`
		Agents []struct {
			Name   string   `json:"name"`
			Models []string `json:"models"`
		} `json:"agents"`
	}
	data, err := readEnvelope(resp)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("workbuddy model catalog parse failed: %w", err)
	}
	var cliModels []string
	for _, agent := range payload.Agents {
		if agent.Name == "cli" {
			cliModels = agent.Models
			break
		}
	}
	byID := make(map[string]catalogEntry, len(payload.Models))
	for _, entry := range payload.Models {
		if nonChatModel(entry) {
			continue
		}
		byID[entry.ID] = entry
	}
	out := make([]ModelInfo, 0, len(cliModels))
	for _, id := range cliModels {
		entry, ok := byID[id]
		if !ok || entry.Disabled {
			continue
		}
		out = append(out, entry.info())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("workbuddy model catalog returned no chat model")
	}
	return out, nil
}

func fetchConfigModels(ctx context.Context, client *http.Client, base string, credential *Credential) ([]ModelInfo, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+ConfigPath, nil)
	if err != nil {
		return nil, err
	}
	ModelHeaders(request.Header, credential)
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Models []catalogEntry `json:"models"`
	}
	data, err := readEnvelope(resp)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("workbuddy config catalog parse failed: %w", err)
	}
	out := make([]ModelInfo, 0, len(payload.Models))
	for _, entry := range payload.Models {
		if nonChatModel(entry) {
			continue
		}
		out = append(out, entry.info())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("workbuddy config catalog returned no chat model")
	}
	return out, nil
}

// mergeModels keeps the configuration entry when both endpoints list a model,
// and appends the models only the enterprise endpoint knows.
func mergeModels(primary, secondary []ModelInfo) []ModelInfo {
	seen := make(map[string]bool, len(primary)+len(secondary))
	out := make([]ModelInfo, 0, len(primary)+len(secondary))
	for _, list := range [][]ModelInfo{primary, secondary} {
		for _, info := range list {
			if info.ID == "" || seen[info.ID] {
				continue
			}
			seen[info.ID] = true
			out = append(out, info)
		}
	}
	return out
}

// nonChatModel filters the entries that are not conversational models:
// completion and embedding endpoints, tiny output models and image generators.
func nonChatModel(entry catalogEntry) bool {
	id := strings.ToLower(strings.TrimSpace(entry.ID))
	for _, prefix := range []string{"nes-", "completion-", "codewise-"} {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	if entry.MaxOutputTokens > 0 && entry.MaxOutputTokens <= 256 {
		return true
	}
	for _, tag := range entry.Tags {
		if tag == "text-to-image" {
			return true
		}
	}
	return false
}

// CreditPackage is one credit package of the account.
type CreditPackage struct {
	Name    string `json:"name"`
	Remain  int64  `json:"remain"`
	Used    int64  `json:"used"`
	Size    int64  `json:"size"`
	EndTime string `json:"end_time,omitempty"`
}

// Credits is the account balance.
type Credits struct {
	Remain   int64           `json:"remain"`
	Used     int64           `json:"used"`
	Size     int64           `json:"size"`
	Packages []CreditPackage `json:"packages,omitempty"`
}

// FetchCredits reads the credit balance of the account.
func FetchCredits(ctx context.Context, client *http.Client, base string, credential *Credential) (*Credits, error) {
	if credential == nil {
		return nil, fmt.Errorf("workbuddy credential unavailable")
	}
	if base == "" {
		base = credential.BillingBase()
	}
	base = strings.TrimRight(base, "/")
	now := time.Now()
	body := map[string]any{
		"PageNumber":               1,
		"PageSize":                 100,
		"ProductCode":              "p_tcaca",
		"Status":                   []int{0, 3},
		"PackageEndTimeRangeBegin": now.Format("2006-01-02 15:04:05"),
		"PackageEndTimeRangeEnd":   now.Add(365 * 101 * 24 * time.Hour).Format("2006-01-02 15:04:05"),
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	paths := []string{BillingMeterPath}
	if credential.IsGlobal() {
		paths = []string{BillingMeterPathInt, BillingMeterPath}
	}
	var lastErr error
	for _, path := range paths {
		credits, err := requestCredits(ctx, client, base+path, credential, encoded)
		if err == nil {
			return credits, nil
		}
		lastErr = err
		var httpErr *HTTPError
		if !(asHTTPError(err, &httpErr) && httpErr.Status == http.StatusNotFound) {
			return nil, err
		}
	}
	return nil, lastErr
}

func asHTTPError(err error, target **HTTPError) bool {
	typed, ok := err.(*HTTPError)
	if ok {
		*target = typed
	}
	return ok
}

func requestCredits(ctx context.Context, client *http.Client, url string, credential *Credential, body []byte) (*Credits, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	BillingHeaders(request.Header, credential)
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Response struct {
			Data struct {
				Accounts []struct {
					PackageName         string `json:"PackageName"`
					CycleEndTime        string `json:"CycleEndTime"`
					CapacitySize        int64  `json:"CapacitySize"`
					CapacityRemain      int64  `json:"CapacityRemain"`
					CapacityUsed        int64  `json:"CapacityUsed"`
					CycleCapacitySize   int64  `json:"CycleCapacitySize"`
					CycleCapacityRemain int64  `json:"CycleCapacityRemain"`
					CycleCapacityUsed   int64  `json:"CycleCapacityUsed"`
				} `json:"Accounts"`
			} `json:"Data"`
		} `json:"Response"`
	}
	data, err := readEnvelope(resp)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("workbuddy credit parse failed: %w", err)
	}
	credits := &Credits{}
	for _, account := range payload.Response.Data.Accounts {
		remain, used, size := packageAmounts(account.CycleCapacityRemain, account.CycleCapacityUsed, account.CycleCapacitySize)
		if size == 0 {
			remain, used, size = packageAmounts(account.CapacityRemain, account.CapacityUsed, account.CapacitySize)
		}
		if remain < 0 {
			remain = 0
		}
		credits.Remain += remain
		credits.Used += used
		credits.Size += size
		credits.Packages = append(credits.Packages, CreditPackage{
			Name:    account.PackageName,
			Remain:  remain,
			Used:    used,
			Size:    size,
			EndTime: account.CycleEndTime,
		})
	}
	return credits, nil
}

// packageAmounts normalizes one package, whose "cycle" fields are the current
// billing window and the plain capacity fields the package total.
func packageAmounts(remain, used, size int64) (int64, int64, int64) {
	if size <= 0 && remain <= 0 && used <= 0 {
		return 0, 0, 0
	}
	if size <= 0 {
		size = remain + used
	}
	if used < 0 {
		used = 0
	}
	if remain > size {
		remain = size
	}
	return remain, used, size
}

// ChatStream sends one chat request. The upstream always answers with a stream;
// the returned response body carries the raw events.
func ChatStream(ctx context.Context, client *http.Client, base string, credential *Credential, body []byte, meta ChatMeta) (*http.Response, error) {
	if credential == nil {
		return nil, fmt.Errorf("workbuddy credential unavailable")
	}
	if base == "" {
		base = credential.ChatBase()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+ChatCompletionsPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	ChatHeaders(request.Header, credential, meta)
	return client.Do(request)
}
