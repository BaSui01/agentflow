package bootstrap

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/agent/observability/hitl"
	"github.com/BaSui01/agentflow/agent/integration/hosted"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/usecase"
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

func newToolRegistryHandler(db *gorm.DB, runtime usecase.ToolRegistryRuntime) *handlers.ToolRegistryHandler {
	if db == nil || runtime == nil {
		return nil
	}
	return handlers.NewToolRegistryHandler(
		usecase.NewDefaultToolRegistryService(hosted.NewGormToolRegistryStore(db), runtime),
		zap.NewNop(),
	)
}

func newToolProviderHandler(db *gorm.DB, runtime usecase.ToolRegistryRuntime) *handlers.ToolProviderHandler {
	if db == nil || runtime == nil {
		return nil
	}
	return handlers.NewToolProviderHandler(
		usecase.NewDefaultToolProviderService(hosted.NewGormToolProviderStore(db), runtime),
		zap.NewNop(),
	)
}

func newToolApprovalHandlerForTest(manager *hitl.InterruptManager) *handlers.ToolApprovalHandler {
	if manager == nil {
		return nil
	}
	config := ToolApprovalConfig{Backend: "memory"}
	return handlers.NewToolApprovalHandler(
		usecase.NewDefaultToolApprovalService(&toolApprovalRuntime{
			manager: manager,
			store:   defaultToolApprovalGrantStore(config, zap.NewNop()),
			history: defaultToolApprovalHistoryStore(config),
			config:  config,
		}, "tool_approval"),
		zap.NewNop(),
	)
}

func TestToolRegistryHandlerConstruction(t *testing.T) {
	db := setupToolRegistryTestDB(t)
	runtime := &toolRegistryRuntimeStub{targets: []string{"retrieval"}}

	assert.Nil(t, newToolRegistryHandler(nil, runtime))
	assert.Nil(t, newToolRegistryHandler(db, nil))
	assert.NotNil(t, newToolRegistryHandler(db, runtime))
}

func TestToolProviderHandlerConstruction(t *testing.T) {
	db := setupToolRegistryTestDB(t)
	runtime := &toolRegistryRuntimeStub{targets: []string{"retrieval"}}

	assert.Nil(t, newToolProviderHandler(nil, runtime))
	assert.Nil(t, newToolProviderHandler(db, nil))
	assert.NotNil(t, newToolProviderHandler(db, runtime))
}

func TestRegisterHTTPRoutes_RegistersToolsEndpoints(t *testing.T) {
	db := setupToolRegistryTestDB(t)
	runtime := &toolRegistryRuntimeStub{targets: []string{"retrieval"}}
	toolHandler := newToolRegistryHandler(db, runtime)
	providerHandler := newToolProviderHandler(db, runtime)
	approvalHandler := newToolApprovalHandlerForTest(
		hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop()),
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
}

func TestRegisterHTTPRoutes_RegistersOpenAICompatChatEndpoints(t *testing.T) {
	chatHandler := handlers.NewChatHandler(nil, zap.NewNop())
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
