package embedding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProviderFromConfig(t *testing.T) {
	tests := []struct {
		name       string
		cfg        FactoryConfig
		wantName   string
		wantErrMsg string
	}{
		{
			name:     "default type uses openai",
			cfg:      FactoryConfig{APIKey: "k"},
			wantName: "openai-embedding",
		},
		{
			name:     "cohere",
			cfg:      FactoryConfig{Type: ProviderCohere, APIKey: "k"},
			wantName: "cohere-embedding",
		},
		{
			name:     "voyage",
			cfg:      FactoryConfig{Type: ProviderVoyage, APIKey: "k"},
			wantName: "voyage-embedding",
		},
		{
			name:     "jina",
			cfg:      FactoryConfig{Type: ProviderJina, APIKey: "k"},
			wantName: "jina-embedding",
		},
		{
			name:     "gemini",
			cfg:      FactoryConfig{Type: ProviderGemini, APIKey: "k"},
			wantName: "gemini-embedding",
		},
		{
			name:       "unsupported provider",
			cfg:        FactoryConfig{Type: ProviderType("unknown"), APIKey: "k"},
			wantErrMsg: "unsupported embedding provider type: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProviderFromConfig(tt.cfg)
			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, tt.wantName, p.Name())
		})
	}
}

