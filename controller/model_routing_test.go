package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRuleRelayMapsNativeNamesOnEveryRetry(t *testing.T) {
	for _, memoryCache := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("memory=%t/stream=%t", memoryCache, stream), func(t *testing.T) {
				db, router := setupRelayRetryTest(t)
				common.MemoryCacheEnabled = memoryCache
				previousPassthrough := model_setting.GetGlobalSettings().PassThroughRequestEnabled
				model_setting.GetGlobalSettings().PassThroughRequestEnabled = stream
				t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = previousPassthrough })
				var mu sync.Mutex
				var models []string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var request struct {
						Model  string         `json:"model"`
						Stream bool           `json:"stream"`
						Custom map[string]any `json:"custom_payload"`
					}
					if !assert.NoError(t, common.DecodeJson(r.Body, &request)) {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					mu.Lock()
					models = append(models, request.Model)
					mu.Unlock()
					if stream {
						assert.Equal(t, map[string]any{"n": float64(0), "keep": false}, request.Custom)
					}
					if request.Model == "retry-contract-primary" {
						w.WriteHeader(http.StatusServiceUnavailable)
						fmt.Fprint(w, `{"error":{"message":"temporarily unavailable","type":"upstream_error"}}`)
						return
					}
					assert.Equal(t, "retry-contract-secondary", request.Model)
					assert.Equal(t, stream, request.Stream)
					if request.Stream {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "data: {\"id\":\"ok\",\"object\":\"chat.completion.chunk\",\"model\":\"retry-contract-secondary\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"ok\",\"object\":\"chat.completion.chunk\",\"model\":\"retry-contract-secondary\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":"ok","object":"chat.completion","model":"retry-contract-secondary","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
				}))
				t.Cleanup(server.Close)
				for index, native := range []string{"retry-contract-primary,retry-contract-primary-sg", "retry-contract-secondary"} {
					channel := model.Channel{
						Name: fmt.Sprintf("variant-%d", index), Type: constant.ChannelTypeOpenAI, Key: "test",
						BaseURL: &server.URL, Models: native, Group: "default", Status: common.ChannelStatusEnabled,
						Priority: common.GetPointer(int64(10 - index)), AutoBan: common.GetPointer(0),
					}
					require.NoError(t, db.Create(&channel).Error)
					require.NoError(t, channel.AddAbilities(nil))
				}
				// Saving a rule alone must refresh routing, without channel checkbox edits.
				metadataPayload := `{"model_name":"retry-contract","status":1,"match_rule":{"include":["retry","contract"]}}`
				metadataCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
				metadataCtx.Request = httptest.NewRequest(http.MethodPost, "/api/models/", strings.NewReader(metadataPayload))
				metadataCtx.Request.Header.Set("Content-Type", "application/json")
				CreateModelMeta(metadataCtx)
				assert.Equal(t, http.StatusOK, metadataCtx.Writer.Status())
				var persisted model.Model
				require.NoError(t, db.Where("model_name = ?", retryTestModel).First(&persisted).Error)

				body := fmt.Sprintf(`{"model":"retry-contract","messages":[{"role":"user","content":"hello"}],"stream":%t,"custom_payload":{"n":0,"keep":false}}`, stream)
				request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.Contains(t, response.Body.String(), "ok")
				mu.Lock()
				assert.Equal(t, []string{"retry-contract-primary", "retry-contract-secondary"}, models)
				mu.Unlock()
			})
		}
	}
}
