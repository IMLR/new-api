package controller

import (
	"github.com/QuantumNous/new-api/pkg/modelfingerprint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
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

func TestFingerprintSamplesRunConcurrentlyAndPreserveFailures(t *testing.T) {
	challenges := []modelfingerprint.Challenge{{ID: "one"}, {ID: "two"}, {ID: "three"}}
	started := make(chan int, 3)
	release := make(chan struct{})
	defer close(release)
	done := make(chan []modelfingerprint.Sample, 1)
	go func() {
		done <- collectFingerprintSamples(challenges, func(i int, _ modelfingerprint.Challenge) modelfingerprint.Sample {
			started <- i
			<-release
			if i == 1 {
				panic("upstream panic")
			}
			if i == 2 {
				return modelfingerprint.Sample{Error: "invalid_response"}
			}
			return modelfingerprint.Sample{Text: "answer"}
		})
	}()
	// All three requests must enter the probe before any is allowed to complete.
	seen := map[int]bool{}
	for range challenges {
		select {
		case i := <-started:
			seen[i] = true
		case <-time.After(5 * time.Second):
			t.Fatal("probes did not start concurrently")
		}
	}
	require.Len(t, seen, 3)
	for range challenges {
		release <- struct{}{}
	}
	samples := <-done
	require.Len(t, samples, 3)
	assert.Equal(t, "one", samples[0].ID)
	assert.Equal(t, "answer", samples[0].Text)
	assert.Equal(t, "two", samples[1].ID)
	assert.Equal(t, "upstream_request_failed", samples[1].Error)
	assert.Equal(t, "three", samples[2].ID)
	assert.Equal(t, "invalid_response", samples[2].Error)
}
