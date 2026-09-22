package opencode

import "testing"

func TestParseKey(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "plain key", raw: "  sk-opencode-123  ", want: "sk-opencode-123"},
		{name: "auth file entry", raw: `{"opencode-go":{"type":"api","key":"sk-auth"}}`, want: "sk-auth"},
		{name: "key object", raw: `{"apiKey":"sk-object"}`, want: "sk-object"},
		{name: "environment name", raw: `{"OPENCODE_API_KEY":"sk-env"}`, want: "sk-env"},
		{name: "empty", raw: "   ", wantErr: true},
		{
			name:    "zen credential",
			raw:     `{"opencode":{"type":"api","key":"sk-zen"}}`,
			wantErr: true,
		},
		{name: "json without key", raw: `{"providers":{}}`, wantErr: true},
		{name: "broken json", raw: `{"key":`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseKey(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseKey(%q) returned %q, want error", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseKey(%q) failed: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("ParseKey(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestMaskKey(t *testing.T) {
	if got := MaskKey("sk-abcdefgh"); got != "****efgh" {
		t.Fatalf("MaskKey = %q, want %q", got, "****efgh")
	}
	if got := MaskKey("abc"); got != "***" {
		t.Fatalf("MaskKey short key = %q, want %q", got, "***")
	}
}
