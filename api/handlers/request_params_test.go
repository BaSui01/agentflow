package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathUintID_UsesPathValueOrFallbackSegment(t *testing.T) {
	t.Run("path value", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/providers/12", nil)
		req.SetPathValue("id", "12")
		id, ok := pathUintID(req, "id", 3)
		require.True(t, ok)
		assert.Equal(t, uint(12), id)
	})

	t.Run("fallback segment", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/providers/34", nil)
		id, ok := pathUintID(req, "id", 3)
		require.True(t, ok)
		assert.Equal(t, uint(34), id)
	})
}

func TestParsePositiveQueryInt_AndBoundedOrDefault(t *testing.T) {
	parsed, err := parsePositiveQueryInt("7", "page")
	require.Nil(t, err)
	assert.Equal(t, 7, parsed)

	_, err = parsePositiveQueryInt("0", "page")
	require.NotNil(t, err)
	assert.Equal(t, 50, boundedOrDefault(0, 50, 200))
	assert.Equal(t, 200, boundedOrDefault(500, 50, 200))
}
