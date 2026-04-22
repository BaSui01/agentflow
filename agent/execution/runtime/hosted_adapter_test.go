package runtime

import (
	"testing"
)

func TestMapLanguage_AllSupported(t *testing.T) {
	tests := []struct {
		input string
		want  Language
	}{
		{"python", LangPython},
		{"javascript", LangJavaScript},
		{"typescript", LangTypeScript},
		{"go", LangGo},
		{"rust", LangRust},
		{"bash", LangBash},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := mapLanguage(tt.input)
			if !ok {
				t.Fatalf("expected ok=true for %s", tt.input)
			}
			if got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestMapLanguage_Unsupported(t *testing.T) {
	unsupported := []string{"java", "c++", "ruby", "php", "", "PYTHON"}
	for _, lang := range unsupported {
		t.Run(lang, func(t *testing.T) {
			_, ok := mapLanguage(lang)
			if ok {
				t.Fatalf("expected ok=false for %q", lang)
			}
		})
	}
}

func TestNewHostedAdapter_NilLogger(t *testing.T) {
	adapter := NewHostedAdapter(nil, nil)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if adapter.logger == nil {
		t.Fatal("expected default logger, got nil")
	}
}

func TestNewHostedAdapter_WithLogger(t *testing.T) {
	adapter := NewHostedAdapter(nil, nil)
	if adapter.executor != nil {
		t.Fatal("expected nil executor")
	}
}

func TestHostedAdapter_Execute_UnsupportedLanguage(t *testing.T) {
	adapter := NewHostedAdapter(nil, nil)
	_, err := adapter.Execute(nil, "java", "code", 0)
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
	if got := err.Error(); got != "unsupported language: java" {
		t.Fatalf("unexpected error: %s", got)
	}
}
