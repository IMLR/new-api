package modelfingerprint

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

// Frozen Python ModelTrace scores from real held-out responses, not recomputed by Go.
func TestRankMatchesFrozenModelTraceScores(t *testing.T) {
	data, err := os.ReadFile("testdata/parity.json")
	require.NoError(t, err)
	var cases []struct {
		Family     string      `json:"family"`
		Text       string      `json:"text"`
		Count      int         `json:"count"`
		Candidates []Candidate `json:"candidates"`
	}
	require.NoError(t, common.Unmarshal(data, &cases))
	for _, tc := range cases {
		t.Run(tc.Family, func(t *testing.T) {
			got, err := Rank(tc.Text, tc.Family, 317)
			require.NoError(t, err)
			require.Empty(t, got.Error)
			assert.Equal(t, tc.Count, got.Count)
			require.Len(t, got.Candidates, len(tc.Candidates))
			for i, c := range tc.Candidates {
				assert.Equal(t, c.Model, got.Candidates[i].Model)
				assert.InDelta(t, c.Score, got.Candidates[i].Score, 1e-9)
			}
		})
	}
}
func TestRefusalDoesNotProduceCandidateRanking(t *testing.T) {
	got, err := Rank("I cannot provide this sequence.", "claude", 317)
	require.NoError(t, err)
	assert.Equal(t, "insufficient_numbers", got.Error)
	assert.Empty(t, got.Candidates)
	assert.Empty(t, Aggregate([]Sample{got}))
}
func TestNumbersSelectLongestRunAndIgnoreOutOfRange(t *testing.T) {
	assert.Equal(t, []int{3, 4, 355}, Numbers("2 examples: 3,4,355,999 end 7"))
}
