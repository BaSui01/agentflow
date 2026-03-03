package bootstrap

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/agent/hosted"
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
	return db
}

func TestBuildToolRegistryHandler(t *testing.T) {
	db := setupToolRegistryTestDB(t)
	runtime := &toolRegistryRuntimeStub{targets: []string{"retrieval"}}

	assert.Nil(t, BuildToolRegistryHandler(nil, runtime, zap.NewNop()))
	assert.Nil(t, BuildToolRegistryHandler(db, nil, zap.NewNop()))
	assert.NotNil(t, BuildToolRegistryHandler(db, runtime, zap.NewNop()))
}

func TestRegisterHTTPRoutes_RegistersToolsEndpoints(t *testing.T) {
	db := setupToolRegistryTestDB(t)
	runtime := &toolRegistryRuntimeStub{targets: []string{"retrieval"}}
	toolHandler := BuildToolRegistryHandler(db, runtime, zap.NewNop())
	require.NotNil(t, toolHandler)

	mux := http.NewServeMux()
	RegisterHTTPRoutes(
		mux,
		HTTPRouteHandlers{Tools: toolHandler},
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
}

