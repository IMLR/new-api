package opencode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelsAndUsageRequests(t *testing.T) {
	var gotAuth []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization")+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case ModelsPath:
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"kimi-k3"},{"id":"kimi-k3"},{"id":""},{"id":"qwen3.7-plus"}]}`))
		case UsagePath:
			_, _ = w.Write([]byte(`{"usage":{"rolling":{"percent":2,"resetInSec":10}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	models, err := Models(ctx, server.Client(), server.URL, "sk-test")
	if err != nil {
		t.Fatalf("Models failed: %v", err)
	}
	if len(models) != 2 || models[0] != "kimi-k3" || models[1] != "qwen3.7-plus" {
		t.Fatalf("unexpected models: %v", models)
	}
	usage, err := FetchUsage(ctx, server.Client(), server.URL, "sk-test")
	if err != nil {
		t.Fatalf("FetchUsage failed: %v", err)
	}
	if usage.Rolling == nil || usage.Rolling.UsedPercent != 2 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	for _, header := range gotAuth {
		if header != "Bearer sk-test "+ModelsPath && header != "Bearer sk-test "+UsagePath {
			t.Fatalf("unexpected request header %q", header)
		}
	}
}

func TestModelsRejectsEmptyCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer server.Close()
	if _, err := Models(context.Background(), server.Client(), server.URL, "sk-test"); err == nil {
		t.Fatal("expected an error for an empty catalog")
	}
}

func TestFetchUsageReportsUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"AuthError","message":"Unauthorized"}}`))
	}))
	defer server.Close()
	_, err := FetchUsage(context.Background(), server.Client(), server.URL, "sk-test")
	if err == nil {
		t.Fatal("expected an error")
	}
	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("error type = %T, want *HTTPError", err)
	}
	if httpErr.Status != http.StatusUnauthorized || httpErr.Message != "OpenCode Go AuthError: Unauthorized" {
		t.Fatalf("unexpected error: %+v", httpErr)
	}
}

func TestEndpointDefaultBase(t *testing.T) {
	if got := Endpoint("", ModelsPath); got != BaseURL+ModelsPath {
		t.Fatalf("Endpoint = %q", got)
	}
	if got := Endpoint("https://example.com/zen/go/", UsagePath); got != "https://example.com/zen/go"+UsagePath {
		t.Fatalf("Endpoint = %q", got)
	}
}
