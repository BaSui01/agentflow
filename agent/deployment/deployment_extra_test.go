package deployment

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- DockerProvider unit tests (no Docker required) ---

func TestNewDockerProvider(t *testing.T) {
	p := NewDockerProvider(nil)
	require.NotNil(t, p)
	assert.NotNil(t, p.logger)
}

func TestDockerProvider_Deploy_NoImage(t *testing.T) {
	p := NewDockerProvider(nil)
	err := p.Deploy(context.Background(), &Deployment{
		ID:     "test-1",
		Config: DeploymentConfig{Image: ""},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires Config.Image")
}

func TestDockerProvider_Scale_NotSupported(t *testing.T) {
	p := NewDockerProvider(nil)
	err := p.Scale(context.Background(), "dep-1", 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support scaling")
}

func TestContainerName(t *testing.T) {
	d := &Deployment{ID: "dep_abc123"}
	assert.Equal(t, "agentflow-dep_abc123", containerName(d))
}

func TestSplitDockerLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"single line", "hello\n", []string{"hello"}},
		{"multiple lines", "line1\nline2\nline3\n", []string{"line1", "line2", "line3"}},
		{"with carriage return", "line1\r\nline2\r\n", []string{"line1", "line2"}},
		{"empty lines filtered", "line1\n\nline2\n", []string{"line1", "line2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitDockerLines(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// PLACEHOLDER_DEPLOY_EXTRA_PART2
