package workbuddy

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Upstream connection parameters. The upstream gateway is served over HTTP/2,
// where a half dead stream shows up as "http2: response body closed" in the
// middle of an answer; the same deployment is stable over HTTP/1.1, so the
// chat path pins the protocol. The header timeout matches the reference
// deployment (120s), which leaves room for the slow first token of a large
// context while still failing a dead connection.
const (
	upstreamDialTimeout           = 10 * time.Second
	upstreamDialKeepAlive         = 15 * time.Second
	upstreamTLSHandshakeTimeout   = 10 * time.Second
	upstreamIdleConnTimeout       = 30 * time.Second
	upstreamResponseHeaderTimeout = 120 * time.Second
)

// NewUpstreamClient builds the HTTP client used for chat and account tasks. An
// empty proxy keeps the direct connection.
func NewUpstreamClient(proxyURL string) (*http.Client, error) {
	dialer := &net.Dialer{Timeout: upstreamDialTimeout, KeepAlive: upstreamDialKeepAlive}
	transport := &http.Transport{
		DialContext: dialer.DialContext,
		// A non nil empty map is what actually disables HTTP/2: with
		// ForceAttemptHTTP2=false the standard library still negotiates h2 over
		// ALPN, and the half dead stream problem stays.
		TLSNextProto:          make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		TLSHandshakeTimeout:   upstreamTLSHandshakeTimeout,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       upstreamIdleConnTimeout,
		ResponseHeaderTimeout: upstreamResponseHeaderTimeout,
	}
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed.Host == "" {
			return nil, fmt.Errorf("invalid channel proxy")
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	return &http.Client{Transport: transport}, nil
}
