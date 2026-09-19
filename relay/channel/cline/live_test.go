package cline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	clineapi "github.com/QuantumNous/new-api/pkg/cline"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Opt-in: rotates and persists credentials in the explicitly supplied private file.
func TestClineLive(t *testing.T) {
	path := os.Getenv("CLINE_LIVE_CREDENTIAL_FILE")
	if path == "" {
		t.Skip("set CLINE_LIVE_CREDENTIAL_FILE to exercise a real account")
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	credential, err := clineapi.ParseCredential(string(raw))
	require.NoError(t, err)
	credential.ExpiresAt = 0 // Exercise renewal, not merely an unexpired access token.
	raw, err = common.Marshal(credential)
	require.NoError(t, err)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	oldDB := model.DB
	model.DB = db
	service.InitHttpClient()
	base := clineapi.BaseURL
	ch := model.Channel{Type: constant.ChannelTypeCline, Key: string(raw), BaseURL: &base}
	require.NoError(t, db.Create(&ch).Error)
	t.Cleanup(func() {
		var saved model.Channel
		if db.First(&saved, ch.Id).Error == nil {
			require.NoError(t, os.WriteFile(path, []byte(saved.Key), 0600))
		}
		model.DB = oldDB
		sqlDB.Close()
	})
	refreshed, err := service.ResolveClineCredential(context.Background(), ch.Id, "")
	require.NoError(t, err)
	assert.NotEmpty(t, refreshed.AccessToken)
	models, err := service.FetchClineChannelModels(&ch)
	require.NoError(t, err)
	require.Contains(t, models, "cline-free/kimi-k3")
	for _, m := range models {
		assert.False(t, strings.HasPrefix(m, "cline-pass/"), "this live fixture is an unsubscribed account")
	}
	t.Logf("renewal and account model discovery passed; models=%d", len(models))
	for _, stream := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
		c.Request.Header.Set("Content-Type", "application/json")
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: ch.Id, ChannelType: constant.ChannelTypeCline, ChannelBaseUrl: base, UpstreamModelName: "cline-free/kimi-k3"}, RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: types.RelayFormatOpenAI, IsStream: stream, DisablePing: true}
		a := &Adaptor{}
		a.Init(info)
		request := `{"model":"cline-free/kimi-k3","messages":[{"role":"user","content":"Reply exactly API_OK"}],"max_tokens":256,"stream_options":{"include_usage":true}}`
		result, err := a.DoRequest(c, info, strings.NewReader(request))
		require.NoError(t, err)
		resp := result.(*http.Response)
		require.Equal(t, 200, resp.StatusCode)
		if stream {
			body, err := collectStream(resp.Body)
			resp.Body.Close()
			require.NoError(t, err)
			assert.Contains(t, string(body), "API_OK")
			assert.Contains(t, string(body), "chat.completion")
		} else {
			_, apiErr := a.DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			assert.Contains(t, recorder.Body.String(), "API_OK")
		}
	}
}
