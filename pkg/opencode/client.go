package opencode

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// BaseURL is the OpenCode Go gateway. Every endpoint lives under /v1.
const BaseURL = "https://opencode.ai/zen/go"

// RequestLimit caps how much of one console response is read. Model catalogs
// and usage payloads are a few kilobytes.
const RequestLimit = 4 << 20

// UserAgent identifies this gateway. OpenCode asks clients to send their own
// product name instead of a generic HTTP library name.
var UserAgent = "new-api/" + common.Version

// HTTPError is an upstream failure that the channel view reports as is.
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("OpenCode Go HTTP %d", e.Status)
}

// Endpoint joins a base URL with an OpenCode Go path.
func Endpoint(base, path string) string {
	if strings.TrimSpace(base) == "" {
		base = BaseURL
	}
	return strings.TrimRight(base, "/") + path
}

func doGet(ctx context.Context, client *http.Client, url, key string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenCode Go request failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, RequestLimit))
	if err != nil {
		return nil, fmt.Errorf("OpenCode Go response read failed")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{Status: resp.StatusCode, Message: errorMessage(body, resp.StatusCode)}
	}
	return body, nil
}

// errorMessage extracts the message of the gateway error envelope
// `{"type":"error","error":{"type":"AuthError","message":"..."}}`.
func errorMessage(body []byte, status int) string {
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if common.Unmarshal(body, &payload) == nil && payload.Error.Message != "" {
		if payload.Error.Type != "" {
			return fmt.Sprintf("OpenCode Go %s: %s", payload.Error.Type, payload.Error.Message)
		}
		return payload.Error.Message
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Sprintf("OpenCode Go HTTP %d", status)
	}
	if len(text) > 300 {
		text = text[:300]
	}
	return fmt.Sprintf("OpenCode Go HTTP %d: %s", status, text)
}

// Models fetches the Go model catalog. The endpoint is public, so a missing or
// rejected key still returns the catalog and model discovery keeps working
// while a key is rotated.
func Models(ctx context.Context, client *http.Client, base, key string) ([]string, error) {
	body, err := doGet(ctx, client, Endpoint(base, ModelsPath), key)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if common.Unmarshal(body, &payload) != nil {
		return nil, fmt.Errorf("Invalid OpenCode Go model catalog")
	}
	models := make([]string, 0, len(payload.Data))
	seen := make(map[string]bool, len(payload.Data))
	for _, entry := range payload.Data {
		id := strings.TrimSpace(entry.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, id)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("OpenCode Go returned an empty model catalog")
	}
	return models, nil
}

// FetchUsage fetches the subscription windows of one API key.
func FetchUsage(ctx context.Context, client *http.Client, base, key string) (*Usage, error) {
	body, err := doGet(ctx, client, Endpoint(base, UsagePath), key)
	if err != nil {
		return nil, err
	}
	usage, err := ParseUsage(body)
	if err != nil {
		return nil, err
	}
	return usage, nil
}
