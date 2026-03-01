package providers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

func TestBaseCapabilityProvider_PostJSONDecode(t *testing.T) {
	expected := map[string]string{"status": "ok"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing or wrong Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("missing Content-Type header")
		}
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	p := providers.NewBaseCapabilityProvider(providers.CapabilityConfig{
		Name:    "test",
		BaseURL: srv.URL,
		APIKey:  "test-key",
	})

	var result map[string]string
	err := p.PostJSONDecode(context.Background(), "/test", map[string]string{"input": "hello"}, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", result["status"])
	}
}

func TestBaseCapabilityProvider_GetJSONDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]int{"count": 42})
	}))
	defer srv.Close()

	p := providers.NewBaseCapabilityProvider(providers.CapabilityConfig{
		Name:    "test",
		BaseURL: srv.URL,
		APIKey:  "key",
	})

	var result map[string]int
	err := p.GetJSONDecode(context.Background(), "/data", &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["count"] != 42 {
		t.Errorf("expected count=42, got %d", result["count"])
	}
}

func TestBaseCapabilityProvider_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "rate limited"},
		})
	}))
	defer srv.Close()

	p := providers.NewBaseCapabilityProvider(providers.CapabilityConfig{
		Name:    "test-provider",
		BaseURL: srv.URL,
		APIKey:  "key",
	})

	var result map[string]any
	err := p.PostJSONDecode(context.Background(), "/api", nil, &result)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	llmErr, ok := err.(*llm.Error)
	if !ok {
		t.Fatalf("expected *llm.Error, got %T", err)
	}
	if llmErr.Code != llm.ErrRateLimit {
		t.Errorf("expected ErrRateLimit, got %s", llmErr.Code)
	}
	if !llmErr.Retryable {
		t.Error("expected retryable=true")
	}
	if llmErr.Provider != "test-provider" {
		t.Errorf("expected provider=test-provider, got %s", llmErr.Provider)
	}
}

func TestBaseCapabilityProvider_CustomHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-key") == "" {
			t.Error("expected x-key header")
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	p := providers.NewBaseCapabilityProvider(providers.CapabilityConfig{
		Name:    "custom",
		BaseURL: srv.URL,
		APIKey:  "my-key",
		BuildHeaders: func(r *http.Request, apiKey string) {
			r.Header.Set("x-key", apiKey)
		},
	})

	_, err := p.GetJSON(context.Background(), "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChooseCapabilityModel(t *testing.T) {
	if m := providers.ChooseCapabilityModel("req-model", "default"); m != "req-model" {
		t.Errorf("expected req-model, got %s", m)
	}
	if m := providers.ChooseCapabilityModel("", "default"); m != "default" {
		t.Errorf("expected default, got %s", m)
	}
}
