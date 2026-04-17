package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/agent/hosted"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

type toolRuntimeStub struct {
	targets     []string
	reloadCalls int
}

func (s *toolRuntimeStub) ReloadBindings(ctx context.Context) error {
	_ = ctx
	s.reloadCalls++
	return nil
}

func (s *toolRuntimeStub) BaseToolNames() []string {
	return append([]string(nil), s.targets...)
}

func setupToolRegistryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&hosted.ToolRegistration{}))
	require.NoError(t, db.AutoMigrate(&hosted.ToolProviderConfig{}))
	return db
}

func TestToolRegistryHandler_CRUD_AutoReload(t *testing.T) {
	db := setupToolRegistryDB(t)
	runtime := &toolRuntimeStub{targets: []string{"retrieval", "mcp_search"}}
	handler := NewToolRegistryHandler(hosted.NewGormToolRegistryStore(db), runtime, zap.NewNop())

	createBody := []byte(`{"name":"knowledge_search","target":"retrieval","enabled":true}`)
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodPost, "/api/v1/tools", bytes.NewReader(createBody))
	r1.Header.Set("Content-Type", "application/json")
	handler.HandleCreate(w1, r1)
	assert.Equal(t, http.StatusCreated, w1.Code)
	assert.Equal(t, 1, runtime.reloadCalls)

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil)
	handler.HandleList(w2, r2)
	assert.Equal(t, http.StatusOK, w2.Code)
	var resp Response
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.True(t, resp.Success)
}

func TestToolRegistryHandler_Create_InvalidTarget(t *testing.T) {
	db := setupToolRegistryDB(t)
	runtime := &toolRuntimeStub{targets: []string{"retrieval"}}
	handler := NewToolRegistryHandler(hosted.NewGormToolRegistryStore(db), runtime, zap.NewNop())

	body := []byte(`{"name":"bad_tool","target":"unknown_target"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/tools", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	handler.HandleCreate(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, 0, runtime.reloadCalls)
}

func TestToolRegistryHandler_Create_ReservedOrSelfName(t *testing.T) {
	db := setupToolRegistryDB(t)
	runtime := &toolRuntimeStub{targets: []string{"retrieval"}}
	handler := NewToolRegistryHandler(hosted.NewGormToolRegistryStore(db), runtime, zap.NewNop())

	body := []byte(`{"name":"retrieval","target":"retrieval"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/tools", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	handler.HandleCreate(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, 0, runtime.reloadCalls)
}

func TestToolRegistryHandler_Create_AuditLogFields(t *testing.T) {
	db := setupToolRegistryDB(t)
	runtime := &toolRuntimeStub{targets: []string{"retrieval"}}
	core, observed := observer.New(zap.InfoLevel)
	handler := NewToolRegistryHandler(hosted.NewGormToolRegistryStore(db), runtime, zap.New(core))

	body := []byte(`{"name":"knowledge_search","target":"retrieval","enabled":true}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/tools", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Request-ID", "req-tools-create")
	r.RemoteAddr = "192.168.0.20:8080"
	handler.HandleCreate(w, r)

	require.Equal(t, http.StatusCreated, w.Code)
	entries := observed.FilterMessage("tool registry request completed").All()
	require.NotEmpty(t, entries)
	fields := entries[len(entries)-1].ContextMap()
	assert.Equal(t, "/api/v1/tools", fields["path"])
	assert.Equal(t, "POST", fields["method"])
	assert.Equal(t, "req-tools-create", fields["request_id"])
	assert.Equal(t, "192.168.0.20:8080", fields["remote_addr"])
	assert.Equal(t, "tool_registry", fields["resource"])
	assert.Equal(t, "create", fields["action"])
	assert.Equal(t, "success", fields["result"])
}
