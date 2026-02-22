package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&llm.LLMProvider{}, &llm.LLMProviderAPIKey{}))

	// 种子一个 provider
	require.NoError(t, db.Create(&llm.LLMProvider{
		Code: "openai", Name: "OpenAI", Status: llm.LLMProviderStatusActive,
	}).Error)
	return db
}

func TestMaskAPIKey(t *testing.T) {
	assert.Equal(t, "****", maskAPIKey("abc"))
	assert.Equal(t, "****5678", maskAPIKey("12345678"))
	// "sk-abcdefghijklmnopqrstuvwxycdef" = 30 chars, mask first 26
	key := "sk-abcdefghijklmnopqrstuvwxycdef"
	masked := maskAPIKey(key)
	assert.Equal(t, len(key), len(masked))
	assert.True(t, masked[len(masked)-4:] == key[len(key)-4:])
}

// PLACEHOLDER_TESTS

func TestHandleListProviders(t *testing.T) {
	db := setupTestDB(t)
	h := NewAPIKeyHandler(db, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	w := httptest.NewRecorder()
	h.HandleListProviders(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
}

func TestHandleCreateAndListAPIKeys(t *testing.T) {
	db := setupTestDB(t)
	h := NewAPIKeyHandler(db, zap.NewNop())

	// Create
	body, _ := json.Marshal(createAPIKeyRequest{
		APIKey:  "sk-test-key-1234567890",
		BaseURL: "https://api.openai.com",
		Label:   "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers/1/api-keys", bytes.NewReader(body))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.HandleCreateAPIKey(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var createResp Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	assert.True(t, createResp.Success)

	// 验证脱敏
	data, _ := json.Marshal(createResp.Data)
	var keyResp apiKeyResponse
	json.Unmarshal(data, &keyResp)
	assert.Equal(t, "https://api.openai.com", keyResp.BaseURL)
	assert.NotContains(t, keyResp.APIKeyMasked, "sk-test")
	assert.True(t, len(keyResp.APIKeyMasked) > 0)

	// List
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/providers/1/api-keys", nil)
	req2.SetPathValue("id", "1")
	w2 := httptest.NewRecorder()
	h.HandleListAPIKeys(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestHandleUpdateAPIKey(t *testing.T) {
	db := setupTestDB(t)
	h := NewAPIKeyHandler(db, zap.NewNop())

	// 先创建
	db.Create(&llm.LLMProviderAPIKey{
		ProviderID: 1, APIKey: "sk-update-test", BaseURL: "https://old.url", Label: "old", Priority: 100, Weight: 100, Enabled: true,
	})

	// 更新
	newURL := "https://new.url"
	newLabel := "updated"
	body, _ := json.Marshal(updateAPIKeyRequest{BaseURL: &newURL, Label: &newLabel})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/providers/1/api-keys/1", bytes.NewReader(body))
	req.SetPathValue("id", "1")
	req.SetPathValue("keyId", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateAPIKey(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// 验证更新
	var key llm.LLMProviderAPIKey
	db.First(&key, 1)
	assert.Equal(t, "https://new.url", key.BaseURL)
	assert.Equal(t, "updated", key.Label)
}

// PLACEHOLDER_DELETE_STATS_TESTS

func TestHandleDeleteAPIKey(t *testing.T) {
	db := setupTestDB(t)
	h := NewAPIKeyHandler(db, zap.NewNop())

	db.Create(&llm.LLMProviderAPIKey{
		ProviderID: 1, APIKey: "sk-delete-test", Priority: 100, Weight: 100, Enabled: true,
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/providers/1/api-keys/1", nil)
	req.SetPathValue("id", "1")
	req.SetPathValue("keyId", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteAPIKey(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// 验证已删除
	var count int64
	db.Model(&llm.LLMProviderAPIKey{}).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestHandleDeleteAPIKey_NotFound(t *testing.T) {
	db := setupTestDB(t)
	h := NewAPIKeyHandler(db, zap.NewNop())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/providers/1/api-keys/999", nil)
	req.SetPathValue("id", "1")
	req.SetPathValue("keyId", "999")
	w := httptest.NewRecorder()
	h.HandleDeleteAPIKey(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAPIKeyStats(t *testing.T) {
	db := setupTestDB(t)
	h := NewAPIKeyHandler(db, zap.NewNop())

	db.Create(&llm.LLMProviderAPIKey{
		ProviderID: 1, APIKey: "sk-stats-test", BaseURL: "https://api.test.com",
		Label: "stats", Priority: 10, Weight: 100, Enabled: true,
		TotalRequests: 100, FailedRequests: 5,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers/1/api-keys/stats", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.HandleAPIKeyStats(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
}

func TestHandleCreateAPIKey_Validation(t *testing.T) {
	db := setupTestDB(t)
	h := NewAPIKeyHandler(db, zap.NewNop())

	// 空 api_key 应该返回 400
	body, _ := json.Marshal(createAPIKeyRequest{APIKey: "", Label: "empty"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers/1/api-keys", bytes.NewReader(body))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.HandleCreateAPIKey(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
