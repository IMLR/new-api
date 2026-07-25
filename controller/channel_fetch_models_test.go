package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/codex"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFetchModelsCodexReturnsStaticModelList(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := fmt.Sprintf(`{"type":%d,"base_url":"https://chatgpt.com","key":"{\"access_token\":\"token\",\"account_id\":\"account\"}"}`, constant.ChannelTypeCodex)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/models", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	FetchModels(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, codex.UpstreamModelList, resp.Data)
}
