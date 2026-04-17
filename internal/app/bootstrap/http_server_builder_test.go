package bootstrap

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/agent/hitl"
	"github.com/BaSui01/agentflow/agent/hosted"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type toolRegistryRuntimeStub struct {
	targets     []string
	reloadCalls int
}

func (s *toolRegistryRuntimeStub) ReloadBindings(ctx context.Context) error {
	_ = ctx
	s.reloadCalls++
	return nil
}

func (s *toolRegistryRuntimeStub) BaseToolNames() []string {
	return append([]string(nil), s.targets...)
}

func setupToolRegistryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&hosted.ToolRegistration{}))
	require.NoError(t, db.AutoMigrate(&hosted.ToolProviderConfig{}))
	return db
}

func TestBuildToolRegistryHandler(t *testing.T) {
	db := setupToolRegistryTestDB(t)
	runtime := &toolRegistryRuntimeStub{targets: []string{"retrieval"}}

	assert.Nil(t, BuildToolRegistryHandler(nil, runtime, zap.NewNop()))
	assert.Nil(t, BuildToolRegistryHandler(db, nil, zap.NewNop()))
	assert.NotNil(t, BuildToolRegistryHandler(db, runtime, zap.NewNop()))
}

func TestBuildToolProviderHandler(t *testing.T) {
	db := setupToolRegistryTestDB(t)
	runtime := &toolRegistryRuntimeStub{targets: []string{"retrieval"}}

	assert.Nil(t, BuildToolProviderHandler(nil, runtime, zap.NewNop()))
	assert.Nil(t, BuildToolProviderHandler(db, nil, zap.NewNop()))
	assert.NotNil(t, BuildToolProviderHandler(db, runtime, zap.NewNop()))
}

func TestRegisterHTTPRoutes_RegistersToolsEndpoints(t *testing.T) {
	db := setupToolRegistryTestDB(t)
	runtime := &toolRegistryRuntimeStub{targets: []string{"retrieval"}}
	toolHandler := BuildToolRegistryHandler(db, runtime, zap.NewNop())
	providerHandler := BuildToolProviderHandler(db, runtime, zap.NewNop())
	approvalHandler := BuildToolApprovalHandler(
		hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop()),
		"tool_approval",
		ToolApprovalConfig{Backend: "memory"},
		zap.NewNop(),
	)
	require.NotNil(t, toolHandler)
	require.NotNil(t, providerHandler)
	require.NotNil(t, approvalHandler)

	mux := http.NewServeMux()
	RegisterHTTPRoutes(
		mux,
		HTTPRouteHandlers{Tools: toolHandler, ToolProviders: providerHandler, ToolApprovals: approvalHandler},
		"test-version",
		"test-build-time",
		"test-git-commit",
		"",
		zap.NewNop(),
	)

	targetsReq := httptest.NewRequest(http.MethodGet, "/api/v1/tools/targets", nil)
	targetsRec := httptest.NewRecorder()
	mux.ServeHTTP(targetsRec, targetsReq)
	assert.Equal(t, http.StatusOK, targetsRec.Code)
	assert.Contains(t, targetsRec.Body.String(), "retrieval")

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/tools",
		bytes.NewBufferString(`{"name":"knowledge_search","target":"retrieval","enabled":true}`),
	)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	assert.Equal(t, http.StatusCreated, createRec.Code)
	assert.Equal(t, 1, runtime.reloadCalls)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	assert.Equal(t, http.StatusOK, listRec.Code)
	assert.True(t, strings.Contains(listRec.Body.String(), "knowledge_search"))

	providerReq := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/tools/providers/tavily",
		bytes.NewBufferString(`{"api_key":"key","timeout_seconds":15,"priority":10}`),
	)
	providerReq.Header.Set("Content-Type", "application/json")
	providerRec := httptest.NewRecorder()
	mux.ServeHTTP(providerRec, providerReq)
	assert.Equal(t, http.StatusOK, providerRec.Code)

	approvalReq := httptest.NewRequest(http.MethodGet, "/api/v1/tools/approvals", nil)
	approvalRec := httptest.NewRecorder()
	mux.ServeHTTP(approvalRec, approvalReq)
	assert.Equal(t, http.StatusOK, approvalRec.Code)

	historyReq := httptest.NewRequest(http.MethodGet, "/api/v1/tools/approvals/history", nil)
	historyRec := httptest.NewRecorder()
	mux.ServeHTTP(historyRec, historyReq)
	assert.Equal(t, http.StatusOK, historyRec.Code)

	grantsReq := httptest.NewRequest(http.MethodGet, "/api/v1/tools/approvals/grants", nil)
	grantsRec := httptest.NewRecorder()
	mux.ServeHTTP(grantsRec, grantsReq)
	assert.Equal(t, http.StatusOK, grantsRec.Code)

	statsReq := httptest.NewRequest(http.MethodGet, "/api/v1/tools/approvals/stats", nil)
	statsRec := httptest.NewRecorder()
	mux.ServeHTTP(statsRec, statsReq)
	assert.Equal(t, http.StatusOK, statsRec.Code)

	cleanupReq := httptest.NewRequest(http.MethodPost, "/api/v1/tools/approvals/cleanup", nil)
	cleanupRec := httptest.NewRecorder()
	mux.ServeHTTP(cleanupRec, cleanupReq)
	assert.Equal(t, http.StatusOK, cleanupRec.Code)

	revokeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/tools/approvals/grants/fp-1", nil)
	revokeReq.SetPathValue("fingerprint", "fp-1")
	revokeRec := httptest.NewRecorder()
	mux.ServeHTTP(revokeRec, revokeReq)
	assert.NotEqual(t, http.StatusNotFound, revokeRec.Code)
}

func TestRegisterHTTPRoutes_RegistersOpenAICompatChatEndpoints(t *testing.T) {
	chatHandler := handlers.NewChatHandler(nil, nil, zap.NewNop())
	require.NotNil(t, chatHandler)

	mux := http.NewServeMux()
	RegisterHTTPRoutes(
		mux,
		HTTPRouteHandlers{Chat: chatHandler},
		"test-version",
		"test-build-time",
		"test-git-commit",
		"",
		zap.NewNop(),
	)

	compatChatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	compatChatReq.Header.Set("Content-Type", "application/json")
	compatChatRec := httptest.NewRecorder()
	mux.ServeHTTP(compatChatRec, compatChatReq)
	assert.NotEqual(t, http.StatusNotFound, compatChatRec.Code)

	compatRespReq := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-4o","input":"hi"}`))
	compatRespReq.Header.Set("Content-Type", "application/json")
	compatRespRec := httptest.NewRecorder()
	mux.ServeHTTP(compatRespRec, compatRespReq)
	assert.NotEqual(t, http.StatusNotFound, compatRespRec.Code)
}

func TestBuildMetricsServerConfig_DefaultLoopbackBinding(t *testing.T) {
	cfg := config.DefaultServerConfig()

	built := BuildMetricsServerConfig(cfg)

	assert.Equal(t, "127.0.0.1:9091", built.Addr)
}

func TestBuildMetricsServerConfig_ExplicitBindAddress(t *testing.T) {
	cfg := config.DefaultServerConfig()
	cfg.MetricsPort = 10091
	cfg.MetricsBindAddress = "0.0.0.0"

	built := BuildMetricsServerConfig(cfg)

	assert.Equal(t, "0.0.0.0:10091", built.Addr)
}
