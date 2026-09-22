package workbuddy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Authorization session endpoints. The login flow issues a server side state,
// the user signs in on the upstream login page, and the relay then exchanges the
// state for the token pair.
const (
	AuthStatePath    = "/v2/plugin/auth/state"
	AuthTokenPath    = "/v2/plugin/auth/token"
	LoginAccountPath = "/v2/plugin/login/account"

	// The login endpoints expect the bundled CLI user agent rather than the
	// desktop one used for chat.
	LoginUserAgent = "CLI/2.63.2 CodeBuddy/2.63.2"

	loginStateParam = "CLI"
)

// ErrAuthorizationPending reports that the browser sign-in has not finished
// yet; the caller keeps the state and asks again.
var ErrAuthorizationPending = errors.New("workbuddy authorization is still pending")

// AuthSession is one started authorization.
type AuthSession struct {
	State   string `json:"state"`
	AuthURL string `json:"authUrl"`
}

// LoginTokens is the token bundle the upstream returns once the sign-in
// completed.
type LoginTokens struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
	Domain       string `json:"domain"`
}

// LoginAccount identifies the account behind the tokens.
type LoginAccount struct {
	UID          string `json:"uid"`
	EnterpriseID string `json:"enterpriseId"`
	Nickname     string `json:"nickname"`
}

// LoginBase is the host that serves the authorization session for a realm.
func LoginBase(realm string) string {
	if realm == RealmGlobal {
		return GlobalBase
	}
	return ChatBaseCN
}

func loginOrigin(realm string) string {
	if realm == RealmGlobal {
		return originGlobal
	}
	return originCN
}

func loginHeaders(header http.Header, realm string) {
	origin := loginOrigin(realm)
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "application/json, text/plain, */*")
	header.Set("X-Requested-With", "XMLHttpRequest")
	header.Set("Origin", origin)
	header.Set("Referer", origin+"/")
	header.Set("User-Agent", LoginUserAgent)
}

// StartAuthorization asks the upstream for a login state and the page the user
// has to open. No PKCE is involved: the state alone identifies the session.
func StartAuthorization(ctx context.Context, client *http.Client, realm, base string) (*AuthSession, error) {
	if strings.TrimSpace(base) == "" {
		base = LoginBase(realm)
	}
	endpoint := strings.TrimRight(base, "/") + AuthStatePath + "?platform=" + loginStateParam
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	loginHeaders(request.Header, realm)
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	data, err := readEnvelope(resp)
	if err != nil {
		return nil, err
	}
	var session AuthSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("workbuddy authorization start failed: %w", err)
	}
	if session.State == "" || session.AuthURL == "" {
		return nil, fmt.Errorf("workbuddy authorization start returned no login url")
	}
	if _, err := url.Parse(session.AuthURL); err != nil {
		return nil, fmt.Errorf("workbuddy authorization start returned an invalid login url")
	}
	return &session, nil
}

// PollAuthorization exchanges the state for the token pair. The upstream
// answers with a non zero business code while the sign-in is unfinished, which
// becomes ErrAuthorizationPending.
func PollAuthorization(ctx context.Context, client *http.Client, realm, base, state string) (*LoginTokens, *LoginAccount, error) {
	trimmed := strings.TrimSpace(state)
	if trimmed == "" {
		return nil, nil, fmt.Errorf("workbuddy authorization state is missing")
	}
	if strings.TrimSpace(base) == "" {
		base = LoginBase(realm)
	}
	base = strings.TrimRight(base, "/")
	endpoint := base + AuthTokenPath + "?state=" + url.QueryEscape(trimmed)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	loginHeaders(request.Header, realm)
	resp, err := client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	data, err := readEnvelope(resp)
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.Status < http.StatusInternalServerError {
			return nil, nil, ErrAuthorizationPending
		}
		return nil, nil, err
	}
	var tokens LoginTokens
	if err := json.Unmarshal(data, &tokens); err != nil || tokens.AccessToken == "" {
		return nil, nil, ErrAuthorizationPending
	}
	return &tokens, fetchLoginAccount(ctx, client, base, realm, trimmed, tokens.AccessToken), nil
}

// fetchLoginAccount reads the account identity. It is optional: a failure here
// still yields a usable credential.
func fetchLoginAccount(ctx context.Context, client *http.Client, base, realm, state, accessToken string) *LoginAccount {
	endpoint := base + LoginAccountPath + "?state=" + url.QueryEscape(state)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	loginHeaders(request.Header, realm)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(request)
	if err != nil {
		return nil
	}
	data, err := readEnvelope(resp)
	if err != nil {
		return nil
	}
	var account LoginAccount
	if err := json.Unmarshal(data, &account); err != nil {
		return nil
	}
	return &account
}

// CredentialFromLogin assembles the channel credential from one authorization.
func CredentialFromLogin(tokens *LoginTokens, account *LoginAccount, realm string) *Credential {
	if tokens == nil {
		return nil
	}
	credential := &Credential{
		AccessToken:  strings.TrimSpace(tokens.AccessToken),
		RefreshToken: strings.TrimSpace(tokens.RefreshToken),
		Domain:       strings.TrimSpace(tokens.Domain),
		Realm:        realm,
	}
	if tokens.ExpiresIn > 0 {
		credential.ExpiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).Unix()
	}
	if account != nil {
		credential.UID = strings.TrimSpace(account.UID)
		credential.EnterpriseID = strings.TrimSpace(account.EnterpriseID)
		credential.Nickname = strings.TrimSpace(account.Nickname)
	}
	return credential
}
