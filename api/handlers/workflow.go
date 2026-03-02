package handlers

import (
	"context"
	"net/http"

	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow"
	"github.com/BaSui01/agentflow/workflow/dsl"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// WorkflowExecutor defines the workflow execution facade contract used by handler.
type WorkflowExecutor interface {
	ExecuteDAG(ctx context.Context, wf *workflow.DAGWorkflow, input any) (any, error)
}

// WorkflowHandler handles workflow API requests.
type WorkflowHandler struct {
	executor WorkflowExecutor
	parser   *dsl.Parser
	logger   *zap.Logger
}

// NewWorkflowHandler creates a new workflow handler.
func NewWorkflowHandler(executor WorkflowExecutor, parser *dsl.Parser, logger *zap.Logger) *WorkflowHandler {
	return &WorkflowHandler{
		executor: executor,
		parser:   parser,
		logger:   logger,
	}
}

// workflowExecuteRequest is the request body for HandleExecute.
type workflowExecuteRequest struct {
	DSL   string `json:"dsl"`
	Input any    `json:"input"`
}

// HandleExecute handles POST /api/v1/workflows/execute
func (h *WorkflowHandler) HandleExecute(w http.ResponseWriter, r *http.Request) {
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req workflowExecuteRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	if req.DSL == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "dsl is required", h.logger)
		return
	}

	// Parse DSL into workflow
	wf, err := h.parser.Parse([]byte(req.DSL))
	if err != nil {
		apiErr := types.NewError(types.ErrInvalidRequest, "invalid workflow DSL: "+err.Error()).
			WithHTTPStatus(http.StatusBadRequest)
		WriteError(w, apiErr, h.logger)
		return
	}

	// Execute workflow
	result, err := h.executor.ExecuteDAG(r.Context(), wf, req.Input)
	if err != nil {
		apiErr := types.NewError(types.ErrInternalError, "workflow execution failed: "+err.Error())
		WriteError(w, apiErr, h.logger)
		return
	}

	h.logger.Info("workflow executed",
		zap.String("name", wf.Name()),
	)

	WriteSuccess(w, map[string]any{
		"workflow": wf.Name(),
		"result":   result,
	})
}

// workflowParseRequest is the request body for HandleParse.
type workflowParseRequest struct {
	DSL string `json:"dsl"`
}

// HandleParse handles POST /api/v1/workflows/parse (validate DSL)
func (h *WorkflowHandler) HandleParse(w http.ResponseWriter, r *http.Request) {
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req workflowParseRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	if req.DSL == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "dsl is required", h.logger)
		return
	}

	// Validate by parsing
	validator := dsl.NewValidator()
	var dslDef dsl.WorkflowDSL
	if err := yaml.Unmarshal([]byte(req.DSL), &dslDef); err != nil {
		WriteSuccess(w, map[string]any{
			"valid":  false,
			"errors": []string{"invalid YAML: " + err.Error()},
		})
		return
	}

	errs := validator.Validate(&dslDef)
	if len(errs) > 0 {
		errMsgs := make([]string, len(errs))
		for i, e := range errs {
			errMsgs[i] = e.Error()
		}
		WriteSuccess(w, map[string]any{
			"valid":  false,
			"errors": errMsgs,
		})
		return
	}

	WriteSuccess(w, map[string]any{
		"valid": true,
		"name":  dslDef.Name,
	})
}

// HandleList handles GET /api/v1/workflows
func (h *WorkflowHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	// Currently returns an empty list as workflows are not persisted.
	// This endpoint exists for API completeness and future extension.
	WriteSuccess(w, map[string]any{
		"workflows": []any{},
	})
}
