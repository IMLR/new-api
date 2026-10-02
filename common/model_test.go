package common

import "testing"

func TestIsImageGenerationModelMatchesPrefixFamily(t *testing.T) {
	cases := map[string]bool{
		"gpt-image-1":             true,
		"gpt-image-2.5-sunburst":  true,
		"dall-e-3":                true,
		"imagen-3.0-generate-002": true,
		"gpt-5.3-codex":           false,
		"seedance-2.5":            false,
	}
	for model, want := range cases {
		if got := IsImageGenerationModel(model); got != want {
			t.Errorf("IsImageGenerationModel(%q) = %v, want %v", model, got, want)
		}
	}
}
