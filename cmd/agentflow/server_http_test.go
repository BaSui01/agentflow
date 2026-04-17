package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildMetricsMux_PProfDisabledByDefault(t *testing.T) {
	mux := buildMetricsMux(false)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestBuildMetricsMux_PProfEnabled(t *testing.T) {
	mux := buildMetricsMux(true)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusNotFound, rec.Code)
}
