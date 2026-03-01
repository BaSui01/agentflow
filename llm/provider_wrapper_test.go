package llm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultProviderFactory_CreateProvider(t *testing.T) {
	factory := NewDefaultProviderFactory()

	t.Run("unregistered provider returns error", func(t *testing.T) {
		_, err := factory.CreateProvider("unknown", "key", "url")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not registered")
	})

	t.Run("registered provider creates successfully", func(t *testing.T) {
		factory.RegisterProvider("test", func(apiKey, baseURL string) (Provider, error) {
			return &testProvider{name: fmt.Sprintf("%s-%s", apiKey, baseURL)}, nil
		})
		p, err := factory.CreateProvider("test", "mykey", "myurl")
		require.NoError(t, err)
		assert.Equal(t, "mykey-myurl", p.Name())
	})

	t.Run("constructor error propagated", func(t *testing.T) {
		factory.RegisterProvider("failing", func(apiKey, baseURL string) (Provider, error) {
			return nil, fmt.Errorf("constructor failed")
		})
		_, err := factory.CreateProvider("failing", "k", "u")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "constructor failed")
	})

	t.Run("overwrite registration", func(t *testing.T) {
		factory.RegisterProvider("dup", func(apiKey, baseURL string) (Provider, error) {
			return &testProvider{name: "v1"}, nil
		})
		factory.RegisterProvider("dup", func(apiKey, baseURL string) (Provider, error) {
			return &testProvider{name: "v2"}, nil
		})
		p, err := factory.CreateProvider("dup", "k", "u")
		require.NoError(t, err)
		assert.Equal(t, "v2", p.Name())
	})
}

