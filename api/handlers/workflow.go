package handlers

import (
	"net/http"

	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// WorkflowHandler handles workflow API requests.
type WorkflowHandler struct {
	BaseHandler[usecase.WorkflowService]
}

func NewWorkflowHandler(service usecase.WorkflowService, logger *zap.Logger) *WorkflowHandler {
	return &WorkflowHandler{BaseHandler: NewBaseHandler(service, logger)}
}

// workflowExecuteRequest is the request body for HandleExecute.
type workflowExecuteRequest struct {
	DSL     string `json:"dsl"`
	DSLFile string `json:"dsl_file,omitempty"`
	DAGJSON string `json:"dag_json,omitempty"`
	DAGYAML string `json:"dag_yaml,omitempty"`
	DAGFile string `json:"dag_file,omitempty"`
	Source  string `json:"source,omitempty"`
	Input   any    `json:"input"`
}

const maxWorkflowDSLBytes = 512 * 1024

func (r *workflowExecuteRequest) Validate() *types.Error {
	if r.DSL == "" && r.DSLFile == "" && r.DAGJSON == "" && r.DAGYAML == "" && r.DAGFile == "" {
		return types.NewInvalidRequestError("dsl/dsl_file/dag_json/dag_yaml/dag_file is required")
	}
	if len(r.DSL) > maxWorkflowDSLBytes || len(r.DAGJSON) > maxWorkflowDSLBytes || len(r.DAGYAML) > maxWorkflowDSLBytes {
		return types.NewInvalidRequestError("dsl/dag_json/dag_yaml exceeds maximum length of 512KB")
	}
	return nil
}

// HandleExecute handles POST /api/v1/workflows/execute
func (h *WorkflowHandler) HandleExecute(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost, h.logger) {
		return
	}
	var req workflowExecuteRequest
	if !ValidateRequest(w, r, &req, h.logger) {
		return
	}
	// V-014: Input size is bounded by DecodeJSONBody's MaxBytesReader (1MB in common.go)

	service, svcErr := h.currentServiceOrUnavailable("workflow")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	wf, source, apiErr := service.BuildDAGWorkflow(usecase.WorkflowBuildInput{
		DSL:     req.DSL,
		DSLFile: req.DSLFile,
		DAGJSON: req.DAGJSON,
		DAGYAML: req.DAGYAML,
		DAGFile: req.DAGFile,
		Source:  req.Source,
	})
	if apiErr != nil {
		WriteError(w, apiErr, h.logger)
		return
	}

	result, execErr := service.Execute(r.Context(), wf, req.Input, func(event usecase.WorkflowStreamEvent) {
		h.logger.Debug("workflow stream event",
			zap.String("type", string(event.Type)),
			zap.String("node_id", event.NodeID),
		)
	}, func(event usecase.WorkflowNodeEvent) {
		h.logger.Debug("workflow node event",
			zap.String("type", string(event.Type)),
			zap.String("node_id", event.NodeID),
			zap.String("workflow_id", event.WorkflowID),
		)
	})
	if execErr != nil {
		WriteError(w, execErr, h.logger)
		return
	}

	h.logger.Info("workflow executed",
		zap.String("name", wf.Name()),
		zap.String("source", source),
	)

	WriteSuccess(w, map[string]any{
		"workflow":        wf.Name(),
		"workflow_source": source,
		"result":          result,
	})
}

// workflowParseRequest is the request body for HandleParse.
type workflowParseRequest struct {
	DSL string `json:"dsl" binding:"required"`
}

// HandleParse handles POST /api/v1/workflows/parse (validate DSL)
func (h *WorkflowHandler) HandleParse(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost, h.logger) {
		return
	}
	var req workflowParseRequest
	if !ValidateRequest(w, r, &req, h.logger) {
		return
	}

	service, svcErr := h.currentServiceOrUnavailable("workflow")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	result := service.ValidateDSL(req.DSL)
	WriteSuccess(w, map[string]any{
		"valid":  result.Valid,
		"name":   result.Name,
		"errors": result.Errors,
	})
}

// HandleList handles GET /api/v1/workflows
func (h *WorkflowHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, h.logger) {
		return
	}
	// Currently returns an empty list as workflows are not persisted.
	// This endpoint exists for API completeness and future extension.
	WriteSuccess(w, map[string]any{
		"workflows": []any{},
	})
}

// HandleCapabilities handles GET /api/v1/workflows/capabilities
func (h *WorkflowHandler) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, h.logger) {
		return
	}
	WriteSuccess(w, map[string]any{
		"sources": []string{
			"auto",
			"dsl",
			"dsl_file",
			"dag_json",
			"dag_yaml",
			"dag_file",
		},
		"default_source": "auto",
	})
}
