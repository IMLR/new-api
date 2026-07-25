package codex

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetupRequestHeaderUsesEventStreamForForcedCodexUpstreamStream(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	tests := []struct {
		name               string
		infoStream         bool
		forceUpstream      bool
		wantAccept         string
		clientAcceptHeader string
	}{
		{
			name:       "non-stream defaults to json",
			wantAccept: "application/json",
		},
		{
			name:          "forced upstream stream uses event stream",
			forceUpstream: true,
			wantAccept:    "text/event-stream",
		},
		{
			name:       "client stream uses event stream",
			infoStream: true,
			wantAccept: "text/event-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			if tt.clientAcceptHeader != "" {
				c.Request.Header.Set("Accept", tt.clientAcceptHeader)
			}
			if tt.forceUpstream {
				common.SetContextKey(c, constant.ContextKeyCodexUpstreamStream, true)
			}

			headers := http.Header{}
			info := &relaycommon.RelayInfo{
				IsStream: tt.infoStream,
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiKey: `{"access_token":"token","account_id":"account"}`,
				},
			}

			err := (&Adaptor{}).SetupRequestHeader(c, &headers, info)

			require.NoError(t, err)
			require.Equal(t, tt.wantAccept, headers.Get("Accept"))
			require.Equal(t, "application/json", headers.Get("Content-Type"))
		})
	}
}
