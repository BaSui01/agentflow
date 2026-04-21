package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/agent/hosted"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

type toolProviderRuntimeStub struct {
	targets     []string
	reloadCalls int
}

func (s *toolProviderRuntimeStub) ReloadBindings(ctx context.Context) error {
	_ = ctx
	s.reloadCalls++
	return nil
}

func (s *toolProviderRuntimeStub) BaseToolNames() []string {
	return append([]string(nil), s.targets...)
}

func setupToolProviderDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&hosted.ToolProviderConfig{}))
	return db
}

func TestToolProviderHandler_UpsertListDelete(t *testing.T) {
	db := setupToolProviderDB(t)
	runtime := &toolProviderRuntimeStub{}
	handler := NewToolProviderHandler(usecase.NewDefaultToolProviderService(NewGormToolProviderStore(db), runtime), zap.NewNop())

	upsertReqBody := []byte(`{"api_key":"tv-key","timeout_seconds":20,"priority":10,"enabled":true}`)
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodPut, "/api/v1/tools/providers/tavily", bytes.NewReader(upsertReqBody))
	r1.Header.Set("Content-Type", "application/json")
	handler.HandleUpsert(w1, r1)
	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, 1, runtime.reloadCalls)

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/v1/tools/providers", nil)
	handler.HandleList(w2, r2)
	assert.Equal(t, http.StatusOK, w2.Code)
	var resp Response
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	w3 := httptest.NewRecorder()
	r3 := httptest.NewRequest(http.MethodDelete, "/api/v1/tools/providers/tavily", nil)
	handler.HandleDelete(w3, r3)
	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Equal(t, 2, runtime.reloadCalls)
}

func TestToolProviderHandler_ValidateProviderSpecificFields(t *testing.T) {
	db := setupToolProviderDB(t)
	runtime := &toolProviderRuntimeStub{}
	handler := NewToolProviderHandler(usecase.NewDefaultToolProviderService(NewGormToolProviderStore(db), runtime), zap.NewNop())

	// firecrawl requires api_key
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodPut, "/api/v1/tools/providers/firecrawl", bytes.NewBufferString(`{"timeout_seconds":15}`))
	r1.Header.Set("Content-Type", "application/json")
	handler.HandleUpsert(w1, r1)
	assert.Equal(t, http.StatusBadRequest, w1.Code)

	// searxng requires base_url
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodPut, "/api/v1/tools/providers/searxng", bytes.NewBufferString(`{"timeout_seconds":15}`))
	r2.Header.Set("Content-Type", "application/json")
	handler.HandleUpsert(w2, r2)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestToolProviderHandler_Upsert_AuditLogFields(t *testing.T) {
	db := setupToolProviderDB(t)
	runtime := &toolProviderRuntimeStub{}
	core, observed := observer.New(zap.InfoLevel)
	handler := NewToolProviderHandler(NewGormToolProviderStore(db), runtime, zap.New(core))

	body := []byte(`{"api_key":"tv-key","timeout_seconds":20,"priority":10,"enabled":true}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/tools/providers/tavily", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Request-ID", "req-provider-upsert")
	r.RemoteAddr = "192.168.0.21:8080"
	handler.HandleUpsert(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	entries := observed.FilterMessage("tool provider request completed").All()
	require.NotEmpty(t, entries)
	fields := entries[len(entries)-1].ContextMap()
	assert.Equal(t, "/api/v1/tools/providers/tavily", fields["path"])
	assert.Equal(t, "PUT", fields["method"])
	assert.Equal(t, "req-provider-upsert", fields["request_id"])
	assert.Equal(t, "192.168.0.21:8080", fields["remote_addr"])
	assert.Equal(t, "tool_provider", fields["resource"])
	assert.Equal(t, "upsert", fields["action"])
	assert.Equal(t, "success", fields["result"])
}
