package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cline"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClineImportNormalizesDesktopCredentials(t *testing.T) {
	ch := &model.Channel{Type: constant.ChannelTypeCline, Key: `{"providers":{"cline":{"settings":{"auth":{"refreshToken":"r","accessToken":"a"}}},"other":{"secret":"unrelated"}}}`}
	require.NoError(t, validateChannel(ch, true))
	assert.NotContains(t, ch.Key, "unrelated")
	parsed, err := cline.ParseCredential(ch.Key)
	require.NoError(t, err)
	assert.Equal(t, "r", parsed.RefreshToken)
	ch.ChannelInfo.IsMultiKey = true
	assert.Error(t, validateChannel(ch, false))
}
func TestClineUnsavedDiscoveryReturnsRotatedCredentialWhenPlanFails(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/refresh" {
			fmt.Fprintf(w, `{"success":true,"data":{"accessToken":"new","refreshToken":"rotated","expiresAt":%q}}`, time.Now().Add(time.Hour).Format(time.RFC3339))
			return
		}
		w.WriteHeader(503)
		fmt.Fprint(w, `{"success":false}`)
	}))
	defer server.Close()
	raw, err := common.Marshal(map[string]any{"type": constant.ChannelTypeCline, "base_url": server.URL, "key": "{\n\"refreshToken\":\"old\"\n}"})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/fetch_models", strings.NewReader(string(raw)))
	c.Request.Header.Set("Content-Type", "application/json")
	FetchModels(c)
	var result struct {
		Success    bool   `json:"success"`
		Credential string `json:"credential"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	assert.False(t, result.Success)
	credential, err := cline.ParseCredential(result.Credential)
	require.NoError(t, err)
	assert.Equal(t, "rotated", credential.RefreshToken)
}
