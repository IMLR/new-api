package model

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChannelEndpointPreferencesValidateProviderAndProtocol(t *testing.T) {
	for _, tc := range []struct {
		name        string
		kind        int
		endpoint    string
		passthrough bool
		valid       bool
	}{
		{"responses", 1, "openai-response", false, true},
		{"chat", 1, "openai", false, true},
		{"unknown protocol", 1, "invalid", false, false},
		{"wrong provider", 14, "openai-response", false, false},
		{"body passthrough", 1, "openai-response", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Channel{Type: tc.kind}
			c.SetSetting(dto.ChannelSettings{ModelEndpoints: map[string]string{"model": tc.endpoint}, PassThroughBodyEnabled: tc.passthrough})
			err := c.ValidateSettings()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
