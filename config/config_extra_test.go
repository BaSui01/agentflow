package config

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// SafeDSN / MaskSensitive / MaskAPIKey
// ============================================================

func TestMaskSensitive(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"short_1", "a", "***"},
		{"short_3", "abc", "***"},
		{"medium_4", "abcd", "a***d"},
		{"medium_6", "abcdef", "a***f"},
		{"long", "mysecretpassword", "mys***ord"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, MaskSensitive(tt.input))
		})
	}
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"short", "sk-1234", "***"},
		{"exactly_8", "sk-12345", "***"},
		{"normal", "sk-1234567890abcdef", "sk-123...def"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, MaskAPIKey(tt.input))
		})
	}
}

func TestSafeDSN_Postgres(t *testing.T) {
	db := &DatabaseConfig{
		Driver:  "postgres",
		Host:    "localhost",
		Port:    5432,
		User:    "admin",
		Password: "supersecret",
		Name:    "mydb",
		SSLMode: "disable",
	}
	dsn := db.SafeDSN()
	assert.Contains(t, dsn, "sup***ret")
	assert.NotContains(t, dsn, "supersecret")
	assert.Contains(t, dsn, "localhost")
}

func TestSafeDSN_MySQL(t *testing.T) {
	db := &DatabaseConfig{
		Driver:  "mysql",
		Host:    "127.0.0.1",
		Port:    3306,
		User:    "root",
		Password: "mypassword",
		Name:    "testdb",
	}
	dsn := db.SafeDSN()
	assert.Contains(t, dsn, "myp***ord")
	assert.NotContains(t, dsn, "mypassword")
}

func TestSafeDSN_SQLite(t *testing.T) {
	db := &DatabaseConfig{
		Driver: "sqlite",
		Name:   "/tmp/test.db",
	}
	assert.Equal(t, "/tmp/test.db", db.SafeDSN())
}

func TestSafeDSN_Unknown(t *testing.T) {
	db := &DatabaseConfig{Driver: "unknown"}
	assert.Equal(t, "", db.SafeDSN())
}

// ============================================================
// HotReloadManager — Rollback, History, Version
// ============================================================

func TestHotReloadManager_WithMaxHistorySize(t *testing.T) {
	cfg := DefaultConfig()
	m := NewHotReloadManager(cfg, WithMaxHistorySize(3))
	assert.Equal(t, 3, m.maxHistorySize)
}

func TestHotReloadManager_WithMaxHistorySize_Zero(t *testing.T) {
	cfg := DefaultConfig()
	m := NewHotReloadManager(cfg, WithMaxHistorySize(0))
	// Zero should not change the default
	assert.Equal(t, 10, m.maxHistorySize)
}

func TestHotReloadManager_WithValidateFunc(t *testing.T) {
	cfg := DefaultConfig()
	called := false
	m := NewHotReloadManager(cfg, WithValidateFunc(func(c *Config) error {
		called = true
		return nil
	}))
	require.NotNil(t, m.validateFunc)
	_ = m.validateFunc(cfg)
	assert.True(t, called)
}

func TestHotReloadManager_GetCurrentVersion(t *testing.T) {
	cfg := DefaultConfig()
	m := NewHotReloadManager(cfg)
	// Initial version should be 1
	assert.Equal(t, 1, m.GetCurrentVersion())
}

func TestHotReloadManager_GetConfigHistory(t *testing.T) {
	cfg := DefaultConfig()
	m := NewHotReloadManager(cfg)
	history := m.GetConfigHistory()
	require.Len(t, history, 1)
	assert.Equal(t, 1, history[0].Version)
	assert.Equal(t, "init", history[0].Source)
}

func TestHotReloadManager_Rollback_NoPrevious(t *testing.T) {
	cfg := DefaultConfig()
	m := NewHotReloadManager(cfg)
	err := m.Rollback()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no previous config")
}

func TestHotReloadManager_RollbackToVersion(t *testing.T) {
	cfg := DefaultConfig()
	m := NewHotReloadManager(cfg)
	// Version 1 exists from init
	err := m.RollbackToVersion(1)
	require.NoError(t, err)
}

func TestHotReloadManager_RollbackToVersion_NotFound(t *testing.T) {
	cfg := DefaultConfig()
	m := NewHotReloadManager(cfg)
	err := m.RollbackToVersion(999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestHotReloadManager_OnRollback(t *testing.T) {
	cfg := DefaultConfig()
	m := NewHotReloadManager(cfg)
	called := false
	m.OnRollback(func(event RollbackEvent) {
		called = true
	})
	// Trigger rollback to version 1
	_ = m.RollbackToVersion(1)
	assert.True(t, called)
}

// ============================================================
// ConfigAPIHandler — exported Handle* methods
// ============================================================

func TestConfigAPIHandler_HandleConfig_GET(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()
	h.HandleConfig(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp, _ := decodeConfigResponse(t, w)
	assert.True(t, resp.Success)
}

func TestConfigAPIHandler_HandleReload_MethodNotAllowed(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/reload", nil)
	w := httptest.NewRecorder()
	h.HandleReload(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestConfigAPIHandler_HandleFields_GET(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/fields", nil)
	w := httptest.NewRecorder()
	h.HandleFields(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp, data := decodeConfigResponse(t, w)
	assert.True(t, resp.Success)
	assert.NotNil(t, data.Fields)
}

func TestConfigAPIHandler_HandleChanges_GET(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/changes", nil)
	w := httptest.NewRecorder()
	h.HandleChanges(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp, _ := decodeConfigResponse(t, w)
	assert.True(t, resp.Success)
}
