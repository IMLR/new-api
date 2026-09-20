package controller

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFingerprintResponseTextAcrossEndpoints(t *testing.T) {
	for _, body := range []string{
		`{"choices":[{"message":{"content":"1,2,3"}}]}`,
		`{"content":[{"type":"thinking","thinking":"secret"},{"type":"text","text":"1,2,3"}]}`,
		`{"output":[{"type":"reasoning","content":[]},{"content":[{"type":"output_text","text":"1,2,3"}]}]}`,
	} {
		text, err := fingerprintResponseText([]byte(body))
		require.NoError(t, err)
		assert.Equal(t, "1,2,3", text)
	}
	_, err := fingerprintResponseText([]byte("invalid"))
	require.Error(t, err)
}
