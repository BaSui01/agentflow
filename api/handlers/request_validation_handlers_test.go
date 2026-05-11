package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type validationRAGServiceStub struct{}

func (validationRAGServiceStub) Query(context.Context, usecase.RAGQueryInput) (*usecase.RAGQueryOutput, error) {
	return nil, assert.AnError
}

func (validationRAGServiceStub) Index(context.Context, usecase.RAGIndexInput) error {
	return assert.AnError
}

func (validationRAGServiceStub) SupportedStrategies() []string { return []string{"auto"} }

type validationWorkflowServiceStub struct{}

func (validationWorkflowServiceStub) BuildDAGWorkflow(usecase.WorkflowBuildInput) (*usecase.WorkflowPlan, string, *types.Error) {
	return nil, "", types.NewInternalError("unexpected build")
}

func (validationWorkflowServiceStub) Execute(context.Context, *usecase.WorkflowPlan, any, usecase.WorkflowStreamEmitter, usecase.WorkflowNodeEventEmitter) (any, *types.Error) {
	return nil, types.NewInternalError("unexpected execute")
}

func (validationWorkflowServiceStub) ValidateDSL(string) usecase.WorkflowDSLValidationResult {
	return usecase.WorkflowDSLValidationResult{Valid: true}
}

func TestRAGHandler_HandleQuery_MissingQueryUsesValidateRequest(t *testing.T) {
	handler := NewRAGHandler(validationRAGServiceStub{}, zap.NewNop())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/rag/query", bytes.NewBufferString(`{"collection":"docs"}`))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleQuery(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, "query is required", resp.Error.Message)
}

func TestRAGHandler_HandleIndex_MissingNestedContentUsesValidateRequest(t *testing.T) {
	handler := NewRAGHandler(validationRAGServiceStub{}, zap.NewNop())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/rag/index", bytes.NewBufferString(`{"documents":[{"id":"doc-1","content":"   "}],"collection":"docs"}`))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleIndex(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, "documents[0].content is required", resp.Error.Message)
}

func TestWorkflowHandler_HandleParse_MissingDSLUsesValidateRequest(t *testing.T) {
	handler := NewWorkflowHandler(validationWorkflowServiceStub{}, zap.NewNop())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/parse", bytes.NewBufferString(`{}`))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleParse(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, "dsl is required", resp.Error.Message)
}

func TestWorkflowHandler_HandleExecute_MissingSourceUsesValidateRequest(t *testing.T) {
	handler := NewWorkflowHandler(validationWorkflowServiceStub{}, zap.NewNop())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/execute", bytes.NewBufferString(`{"input":{"topic":"go"}}`))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleExecute(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, "dsl/dsl_file/dag_json/dag_yaml/dag_file is required", resp.Error.Message)
}
