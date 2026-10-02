package common

import (
	"io"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPassthroughModelMappingKeepsOriginalBodyAndUnknownFields(t *testing.T) {
	original := `{"model":"canonical","custom":{"n":0,"enabled":false},"opaque":null}`
	storage, err := common.CreateBodyStorage([]byte(original))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	info := &RelayInfo{ChannelMeta: &ChannelMeta{IsModelMapped: true}}
	for _, modelName := range []string{"native-primary", "secondary"} {
		info.UpstreamModelName = modelName
		body, err := PassthroughRequestBody(storage, info)
		require.NoError(t, err)
		data, err := io.ReadAll(body)
		require.NoError(t, err)
		assert.Equal(t, modelName, gjson.GetBytes(data, "model").String())
		assert.Equal(t, `{"n":0,"enabled":false}`, gjson.GetBytes(data, "custom").Raw)
		assert.Equal(t, "null", gjson.GetBytes(data, "opaque").Raw)
		assert.Equal(t, int64(len(data)), info.UpstreamRequestBodySize)
	}
	clientBody, err := storage.Bytes()
	require.NoError(t, err)
	assert.Equal(t, original, string(clientBody))
}

func TestPassthroughURLModelDoesNotAddBodyModel(t *testing.T) {
	original := `{"contents":[{"parts":[{"text":"hello"}]}]}`
	storage, err := common.CreateBodyStorage([]byte(original))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	info := &RelayInfo{ChannelMeta: &ChannelMeta{IsModelMapped: true, UpstreamModelName: "gemini-native"}}
	body, err := PassthroughRequestBody(storage, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, original, string(data))
}
