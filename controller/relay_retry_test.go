package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	clineapi "github.com/QuantumNous/new-api/pkg/cline"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const retryTestModel = "retry-contract"

func setupRelayRetryTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousCache, previousRedis := common.MemoryCacheEnabled, common.RedisEnabled
	previousRetryTimes, previousConsumeLog := common.RetryTimes, common.LogConsumeEnabled
	previousCountToken, previousErrorLog := constant.CountToken, constant.ErrorLogEnabled
	previousStreamingTimeout := constant.StreamingTimeout
	previousFreePreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	previousRatios := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.MemoryCacheEnabled, common.RedisEnabled = previousCache, previousRedis
		common.RetryTimes, common.LogConsumeEnabled = previousRetryTimes, previousConsumeLog
		constant.CountToken, constant.ErrorLogEnabled = previousCountToken, previousErrorLog
		constant.StreamingTimeout = previousStreamingTimeout
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = previousFreePreConsume
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatios))
	})
	db := setupModelListControllerTestDB(t)
	common.MemoryCacheEnabled, common.RedisEnabled = false, false
	common.RetryTimes, common.LogConsumeEnabled = 0, false
	constant.CountToken, constant.ErrorLogEnabled = false, false
	constant.StreamingTimeout = 30
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"retry-contract":0}`))
	require.NoError(t, i18n.Init())
	service.InitHttpClient()
	require.NoError(t, db.Create(&model.User{
		Id: 1001, Username: "relay-retry-user", Group: "default", Quota: 1000000,
		Status: common.UserStatusEnabled,
	}).Error)

	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserId, 1001)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
		common.SetContextKey(c, constant.ContextKeyUserQuota, 1000000)
		defer common.CleanupBodyStorage(c)
		c.Next()
	}, middleware.Distribute(), func(c *gin.Context) {
		Relay(c, types.RelayFormatOpenAI)
	})
	return db, router
}

func createRetryChannel(t *testing.T, db *gorm.DB, baseURL string, index, channelType int) model.Channel {
	t.Helper()
	key := "fallback"
	if channelType == constant.ChannelTypeCline {
		encoded, err := common.Marshal(clineapi.Credential{
			AccessToken: fmt.Sprintf("account-%d", index), RefreshToken: "unused-refresh",
			ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
		})
		require.NoError(t, err)
		key = string(encoded)
	}
	channel := model.Channel{
		Type: channelType, Key: key, Name: fmt.Sprintf("retry-%d", index),
		BaseURL: &baseURL, Group: "default", Models: retryTestModel,
		Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(20 - index)),
		AutoBan: common.GetPointer(0),
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: retryTestModel, ChannelId: channel.Id, Enabled: true,
		Priority: channel.Priority,
	}).Error)
	t.Cleanup(func() { model.MarkChannelModelCooldown(channel.Id, retryTestModel, time.Time{}, "") })
	return channel
}

func TestRelayRetriesAllCandidatesAndSkipsClineCooldowns(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			db, router := setupRelayRetryTest(t)
			var mu sync.Mutex
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				account := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				mu.Lock()
				calls = append(calls, account)
				mu.Unlock()
				var request struct {
					Model    string `json:"model"`
					Stream   bool   `json:"stream"`
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				if !assert.NoError(t, common.DecodeJson(r.Body, &request)) {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				assert.Equal(t, retryTestModel, request.Model)
				assert.Len(t, request.Messages, 1)
				if len(request.Messages) == 1 {
					assert.Equal(t, "hello", request.Messages[0].Content)
				}
				if account == "fallback" {
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "data: {\"id\":\"ok\",\"object\":\"chat.completion.chunk\",\"model\":\"retry-contract\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"ok\",\"object\":\"chat.completion.chunk\",\"model\":\"retry-contract\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":"ok","object":"chat.completion","model":"retry-contract","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
					return
				}
				assert.True(t, request.Stream)
				index, err := strconv.Atoi(strings.TrimPrefix(account, "workos:account-"))
				if !assert.NoError(t, err) {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				errorBody := `{"error":{"code":"rate_limit_error","message":"This request would exceed the rate limit. Retry after 55s."}}`
				if index == 0 {
					errorBody = `{"error":{"code":"INFERENCE_CAP_ERROR","message":"Daily free limit reached. Try again in 20h 4m"}}`
				}
				if index == 1 {
					w.Header().Set("Retry-After", "55")
					errorBody = `{"error":{"message":"Too many requests"}}`
				}
				if index == 2 {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "data: %s\n\n", errorBody)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprint(w, errorBody)
			}))
			t.Cleanup(server.Close)
			channels := make([]model.Channel, 0, 8)
			for i := 0; i < 8; i++ {
				channels = append(channels, createRetryChannel(t, db, server.URL, i, constant.ChannelTypeCline))
			}
			createRetryChannel(t, db, server.URL, 8, constant.ChannelTypeOpenAI)

			requestBody := fmt.Sprintf(`{"model":"retry-contract","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")
			started := time.Now()
			router.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Body.String(), `"content":"ok"`)
			mu.Lock()
			assert.Equal(t, []string{"workos:account-0", "workos:account-1", "workos:account-2", "workos:account-3", "workos:account-4", "workos:account-5", "workos:account-6", "workos:account-7", "fallback"}, calls)
			mu.Unlock()
			for i, channel := range channels {
				cooldowns := model.ChannelModelCooldowns(channel.Id)
				require.Len(t, cooldowns, 1)
				window := 55 * time.Second
				if i == 0 {
					window = 20*time.Hour + 4*time.Minute
				}
				assert.WithinDuration(t, started.Add(window), cooldowns[0].Until, 5*time.Second)
			}

			// A new request must go straight to the fallback while all Cline routes cool.
			recorder = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			mu.Lock()
			assert.Len(t, calls, 10)
			assert.Equal(t, "fallback", calls[len(calls)-1])
			mu.Unlock()
		})
	}
}

func TestRelayKeepsLastErrorWhenCandidatesExhausted(t *testing.T) {
	db, router := setupRelayRetryTest(t)
	var mu sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"code":"rate_limit_error","message":"Retry after 55s."}}`)
	}))
	t.Cleanup(server.Close)
	for i := 0; i < 3; i++ {
		createRetryChannel(t, db, server.URL, i, constant.ChannelTypeCline)
	}
	for _, expectedStatus := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"retry-contract","messages":[{"role":"user","content":"hello"}]}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		require.Equal(t, expectedStatus, recorder.Code, recorder.Body.String())
		if expectedStatus == http.StatusTooManyRequests {
			assert.Contains(t, recorder.Body.String(), "Retry after 55s.")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"Bearer workos:account-0", "Bearer workos:account-1", "Bearer workos:account-2"}, calls)
}

func TestRelayRetryStopsAfterCancellationOrOutput(t *testing.T) {
	upstreamErr := types.NewErrorWithStatusCode(fmt.Errorf("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	for _, scenario := range []string{"cancelled", "output", "specific-channel"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			switch scenario {
			case "cancelled":
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			case "output":
				_, err := c.Writer.WriteString("data: started\n\n")
				require.NoError(t, err)
			case "specific-channel":
				c.Set("specific_channel_id", "1")
			}
			assert.False(t, shouldRetry(c, upstreamErr))
		})
	}
}

func TestRelayRetriesWhenChannelMetadataWasNotInitialized(t *testing.T) {
	db, _ := setupRelayRetryTest(t)
	first := createRetryChannel(t, db, "http://unused", 0, constant.ChannelTypeOpenAI)
	second := createRetryChannel(t, db, "http://unused", 1, constant.ChannelTypeOpenAI)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("channel_id", first.Id)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	addUsedChannel(c, first.Id)
	info := &relaycommon.RelayInfo{OriginModelName: retryTestModel, UserGroup: "default", UsingGroup: "default"}
	param := &service.RetryParam{Ctx: c, TokenGroup: "default", ModelName: retryTestModel, Retry: common.GetPointer(1)}
	channel, err := getChannel(c, info, param)
	require.Nil(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, second.Id, channel.Id)
}

func TestAutoGroupRetryExhaustsCandidatesAndRespectsPermission(t *testing.T) {
	for _, crossGroup := range []bool{false, true} {
		t.Run(fmt.Sprintf("cross-group=%t", crossGroup), func(t *testing.T) {
			db, _ := setupRelayRetryTest(t)
			previousAutoGroups := setting.AutoGroups2JsonString()
			previousUsableGroups := setting.UserUsableGroups2JSONString()
			t.Cleanup(func() {
				require.NoError(t, setting.UpdateAutoGroupsByJsonString(previousAutoGroups))
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups))
			})
			require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
			first := createRetryChannel(t, db, "http://unused", 0, constant.ChannelTypeCline)
			second := createRetryChannel(t, db, "http://unused", 1, constant.ChannelTypeCline)
			third := createRetryChannel(t, db, "http://unused", 2, constant.ChannelTypeOpenAI)
			require.NoError(t, db.Model(&third).Update("group", "vip").Error)
			require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", third.Id).Update("group", "vip").Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, crossGroup)
			param := &service.RetryParam{Ctx: c, TokenGroup: "auto", ModelName: retryTestModel, RequestPath: "/v1/chat/completions"}
			for _, id := range []int{first.Id, second.Id} {
				channel, group, err := service.CacheGetRandomSatisfiedChannel(param)
				require.NoError(t, err)
				require.NotNil(t, channel)
				assert.Equal(t, "default", group)
				assert.Equal(t, id, channel.Id)
				addUsedChannel(c, channel.Id)
				param.IncreaseRetry()
			}
			channel, group, err := service.CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			if !crossGroup {
				assert.Nil(t, channel)
				assert.Equal(t, "default", group)
				return
			}
			require.NotNil(t, channel)
			assert.Equal(t, "vip", group)
			assert.Equal(t, third.Id, channel.Id)
			addUsedChannel(c, channel.Id)
			param.IncreaseRetry()
			channel, _, err = service.CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			assert.Nil(t, channel)
		})
	}
}
