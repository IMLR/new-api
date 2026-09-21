package cline

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Account is the signed-in Cline identity behind one credential.
type Account struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

// FetchAccount reads the account that owns the credential.
func FetchAccount(ctx context.Context, client *http.Client, base string, credential *Credential) (*Account, error) {
	var payload struct {
		Success bool     `json:"success"`
		Data    *Account `json:"data"`
	}
	status, err := get(ctx, client, base, "/api/v1/users/me", credential, &payload)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, &HTTPError{Status: status, Reauth: status == http.StatusUnauthorized || status == http.StatusForbidden}
	}
	if payload.Data == nil {
		return nil, &HTTPError{Status: status}
	}
	payload.Data.Email = strings.TrimSpace(payload.Data.Email)
	payload.Data.DisplayName = strings.TrimSpace(payload.Data.DisplayName)
	return payload.Data, nil
}

// FetchCurrentPlan returns the subscription record of the account, or nil when
// Cline reports that the account never had a plan.
func FetchCurrentPlan(ctx context.Context, client *http.Client, base string, credential *Credential) (*Plan, error) {
	var payload struct {
		Success bool   `json:"success"`
		Data    *Plan  `json:"data"`
		Error   string `json:"error"`
	}
	status, err := get(ctx, client, base, "/api/v1/users/me/plan", credential, &payload)
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusNotFound && payload.Error == "no plan history found for user":
		return nil, nil
	case status != http.StatusOK:
		return nil, &HTTPError{Status: status}
	case !payload.Success:
		return nil, fmt.Errorf("Cline subscription lookup failed")
	}
	return payload.Data, nil
}
