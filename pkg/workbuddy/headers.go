package workbuddy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// ChatMeta carries the per-request identity headers. The upstream groups a
// conversation by X-Conversation-Request-ID, so one logical turn keeps one id
// across retries.
type ChatMeta struct {
	ConversationID        string
	ConversationRequestID string
	TraceID               string
}

const (
	originCN     = "https://www.codebuddy.cn"
	originGlobal = "https://www.workbuddy.ai"
)

// CommonHeaders sets the headers every upstream call shares.
func CommonHeaders(h http.Header, cred *Credential) {
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "application/json")
	h.Set("X-Requested-With", "XMLHttpRequest")
	origin := originFor(cred)
	h.Set("Origin", origin)
	h.Set("Referer", origin+"/")
	h.Set("User-Agent", UserAgent(cred))
	h.Set("X-CodeBuddy-Request", "1")
	h.Set("Accept-Language", acceptLanguage(cred))
	injectAccountStableHeaders(h, cred)
}

// UserAgent is the official desktop user agent shape, whose platform segment
// switches with the deployment.
func UserAgent(cred *Credential) string {
	platform := "WorkBuddy"
	if cred != nil && cred.IsGlobal() {
		platform = "WorkBuddy AI"
	}
	return "WorkBuddy/" + ClientVersion + " " + platform + "/" + ClientVersion + " CLI/" + CliVersion
}

func originFor(cred *Credential) string {
	if cred != nil && cred.IsGlobal() {
		return originGlobal
	}
	return originCN
}

func acceptLanguage(cred *Credential) string {
	if cred != nil && cred.IsGlobal() {
		return "en-US"
	}
	return "zh-CN"
}

// ChatHeaders adds the chat specific account headers on top of the common set.
func ChatHeaders(h http.Header, cred *Credential, meta ChatMeta) {
	CommonHeaders(h, cred)
	h.Set("Accept", "application/json, text/event-stream")
	if cred != nil && cred.AccessToken != "" {
		h.Set("Authorization", "Bearer "+cred.AccessToken)
	} else {
		h.Set("X-No-Authorization", "1")
	}
	if cred != nil && cred.UID != "" {
		h.Set("X-User-Id", cred.UID)
	} else {
		h.Set("X-No-User-Id", "1")
	}
	if cred != nil && !cred.IsGlobal() {
		if cred.EnterpriseID != "" {
			h.Set("X-Enterprise-Id", cred.EnterpriseID)
		} else {
			h.Set("X-No-Enterprise-Id", "1")
		}
		if cred.Domain != "" {
			h.Set("X-Domain", cred.Domain)
		} else {
			h.Set("X-No-Department-Info", "1")
		}
	} else {
		// Global accounts have no enterprise id and declare their domain
		// instead; the upstream rejects the CN shape for them.
		h.Set("X-No-Enterprise-Id", "1")
		h.Set("X-Domain", "www.workbuddy.ai")
	}
	injectAttribution(h)
	injectDeviceToken(h, cred)
	injectConversationHeaders(h, meta)
}

// BillingHeaders serves the credit balance call, which does not share the chat
// header set.
func BillingHeaders(h http.Header, cred *Credential) {
	if cred == nil {
		return
	}
	h.Set("Authorization", "Bearer "+cred.AccessToken)
	h.Set("Accept", "application/json")
	h.Set("Content-Type", "application/json")
	h.Set("X-CodeBuddy-Request", "1")
	h.Set("Accept-Language", acceptLanguage(cred))
	h.Set("User-Agent", "WorkBuddy/"+ClientVersion)
	if cred.UID != "" {
		h.Set("X-User-Id", cred.UID)
	}
	if cred.EnterpriseID != "" {
		h.Set("X-Enterprise-Id", cred.EnterpriseID)
		h.Set("X-Tenant-Id", cred.EnterpriseID)
	}
	if cred.Domain != "" {
		h.Set("X-Domain", cred.Domain)
	}
	injectDeviceToken(h, cred)
}

// RefreshHeaders serves the token renewal call. X-Refresh-Token only ever
// appears on this request.
func RefreshHeaders(h http.Header, cred *Credential) {
	CommonHeaders(h, cred)
	if cred == nil {
		return
	}
	h.Set("X-Refresh-Token", cred.RefreshToken)
	if cred.EnterpriseID != "" {
		h.Set("X-Enterprise-Id", cred.EnterpriseID)
	}
	h.Set("X-Auth-Refresh-Source", "plugin")
}

// ModelHeaders serves the model catalog calls: common headers plus the bearer
// token.
func ModelHeaders(h http.Header, cred *Credential) {
	CommonHeaders(h, cred)
	if cred != nil {
		h.Set("Authorization", "Bearer "+cred.AccessToken)
	}
}

// deriveAccountStableID builds a stable 36 hex identifier per account and
// purpose, so one account always presents the same device to the upstream.
func deriveAccountStableID(uid, purpose string) string {
	sum := sha256.Sum256([]byte("wb2a:" + purpose + ":" + uid))
	return hex.EncodeToString(sum[:18])
}

func injectAccountStableHeaders(h http.Header, cred *Credential) {
	if cred == nil || cred.UID == "" {
		return
	}
	h.Set("X-Machine-ID", deriveAccountStableID(cred.UID, "machine"))
	h.Set("X-Session-ID", deriveAccountStableID(cred.UID, "session"))
}

func injectDeviceToken(h http.Header, cred *Credential) {
	if cred != nil && cred.DeviceToken != "" {
		h.Set("X-Device-Token", cred.DeviceToken)
	}
}

// injectAttribution mirrors the desktop client's usage attribution headers.
func injectAttribution(h http.Header) {
	h.Set("X-Agent-Purpose", "conversation")
	h.Set("X-IDE-Name", "WorkBuddy")
	h.Set("X-IDE-Type", "WorkBuddy")
	h.Set("X-IDE-Version", ClientVersion)
	h.Set("X-Product", "WorkBuddy")
}

// injectConversationHeaders writes the conversation family: the conversation
// id when known, the turn level request id, a fresh message id and the B3
// trace headers.
func injectConversationHeaders(h http.Header, meta ChatMeta) {
	requestID := strings.TrimSpace(meta.ConversationRequestID)
	if requestID == "" {
		requestID = NewMessageID()
	}
	messageID := NewMessageID()
	if conv := strings.TrimSpace(meta.ConversationID); conv != "" {
		h.Set("X-Conversation-ID", conv)
	}
	h.Set("X-Conversation-Request-ID", requestID)
	h.Set("X-Conversation-Message-ID", messageID)
	h.Set("X-Request-ID", messageID)
	h.Set("X-Root-Request-ID", requestID)
	traceID := strings.TrimSpace(meta.TraceID)
	if traceID == "" {
		traceID = requestID
	}
	h.Set("X-Trace-ID", traceID)
	b3Trace := requestID
	if !validTraceID(b3Trace) {
		b3Trace = messageID
	}
	h.Set("X-B3-TraceId", b3Trace)
	h.Set("X-B3-SpanId", messageID[:16])
	h.Set("X-B3-Sampled", "1")
}

// NewMessageID returns a 32 hex random id, the shape the upstream expects for
// message level identifiers.
func NewMessageID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strings.Repeat("0", 32)
	}
	return hex.EncodeToString(buf)
}

// validTraceID reports whether a value is a legal B3 trace id.
func validTraceID(value string) bool {
	if len(value) != 16 && len(value) != 32 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// CacheKey builds the per account prompt cache key, which cuts the upstream
// charge for repeated prefixes by an order of magnitude. The account id prefix
// keeps two accounts from sharing a cache entry.
func CacheKey(uid, conversation string) string {
	short := uid
	if len(short) > 8 {
		short = short[:8]
	}
	if short == "" {
		short = "-"
	}
	sum := sha256.Sum256([]byte(uid + "|" + conversation))
	return "wb2a-" + short + "-" + hex.EncodeToString(sum[:16])
}
