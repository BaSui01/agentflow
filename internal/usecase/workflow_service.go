package usecase

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/BaSui01/agentflow/types"
	workflow "github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/dsl"
	workflowobs "github.com/BaSui01/agentflow/workflow/observability"
	"gopkg.in/yaml.v3"
)

// WorkflowExecutor defines the workflow execution facade contract used by handlers.
type WorkflowExecutor interface {
	ExecuteDAG(ctx context.Context, wf *workflow.DAGWorkflow, input any) (any, error)
}

type WorkflowService interface {
	BuildDAGWorkflow(req WorkflowBuildInput) (*WorkflowPlan, string, *types.Error)
	Execute(ctx context.Context, wf *WorkflowPlan, input any, streamEmitter WorkflowStreamEmitter, nodeEmitter WorkflowNodeEventEmitter) (any, *types.Error)
	ValidateDSL(rawDSL string) WorkflowDSLValidationResult
}

type defaultWorkflowService struct {
	executor WorkflowExecutor
	parser   *dsl.Parser
}

func NewDefaultWorkflowService(executor WorkflowExecutor, parser *dsl.Parser) WorkflowService {
	return &defaultWorkflowService{
		executor: executor,
		parser:   parser,
	}
}

func (s *defaultWorkflowService) BuildDAGWorkflow(req WorkflowBuildInput) (*WorkflowPlan, string, *types.Error) {
	if s.parser == nil {
		return nil, "", types.NewInternalError("workflow parser is not configured")
	}

	var (
		wf  *workflow.DAGWorkflow
		err error
	)

	source, resolveErr := resolveWorkflowSource(req)
	if resolveErr != nil {
		return nil, "", types.NewError(types.ErrInvalidRequest, resolveErr.Error()).
			WithHTTPStatus(http.StatusBadRequest)
	}

	switch source {
	case workflowSourceDAGFile:
		var def *workflow.DAGDefinition
		def, err = loadDAGDefinition(req.DAGFile)
		if err == nil {
			wf, err = def.ToDAGWorkflow()
		}
	case workflowSourceDAGJSON:
		var def *workflow.DAGDefinition
		def, err = workflow.FromJSON(req.DAGJSON)
		if err == nil {
			wf, err = def.ToDAGWorkflow()
		}
	case workflowSourceDAGYAML:
		var def *workflow.DAGDefinition
		def, err = workflow.FromYAML(req.DAGYAML)
		if err == nil {
			wf, err = def.ToDAGWorkflow()
		}
	case workflowSourceDSLFile:
		wf, err = s.parser.ParseFile(req.DSLFile)
	case workflowSourceDSL:
		wf, err = s.parser.Parse([]byte(req.DSL))
	default:
		return nil, "", types.NewError(types.ErrInvalidRequest, "unsupported workflow source: "+source).
			WithHTTPStatus(http.StatusBadRequest)
	}

	if err != nil {
		return nil, "", types.NewError(types.ErrInvalidRequest, "invalid workflow DSL: "+err.Error()).
			WithCause(err).
			WithHTTPStatus(http.StatusBadRequest)
	}

	return newWorkflowPlan(wf), source, nil
}

func (s *defaultWorkflowService) Execute(
	ctx context.Context,
	wf *WorkflowPlan,
	input any,
	streamEmitter WorkflowStreamEmitter,
	nodeEmitter WorkflowNodeEventEmitter,
) (any, *types.Error) {
	if s.executor == nil {
		return nil, types.NewInternalError("workflow executor is not configured").
			WithHTTPStatus(http.StatusNotImplemented)
	}
	if wf == nil || wf.dag == nil {
		return nil, types.NewInvalidRequestError("workflow is required").
			WithHTTPStatus(http.StatusBadRequest)
	}

	execCtx := workflow.WithWorkflowStreamEmitter(ctx, adaptWorkflowStreamEmitter(streamEmitter))
	execCtx = workflowobs.WithNodeEventEmitter(execCtx, adaptWorkflowNodeEmitter(nodeEmitter))

	result, err := s.executor.ExecuteDAG(execCtx, wf.dag, input)
	if err != nil {
		return nil, types.NewError(types.ErrInternalError, "workflow execution failed: "+err.Error()).
			WithCause(err)
	}
	return result, nil
}

func adaptWorkflowStreamEmitter(emitter WorkflowStreamEmitter) workflow.WorkflowStreamEmitter {
	if emitter == nil {
		return nil
	}
	return func(event workflow.WorkflowStreamEvent) {
		errMsg := ""
		if event.Error != nil {
			errMsg = event.Error.Error()
		}
		emitter(WorkflowStreamEvent{
			Type:     WorkflowStreamEventType(event.Type),
			NodeID:   event.NodeID,
			NodeName: event.NodeName,
			Data:     event.Data,
			Error:    errMsg,
		})
	}
}

func adaptWorkflowNodeEmitter(emitter WorkflowNodeEventEmitter) workflowobs.NodeEventEmitter {
	if emitter == nil {
		return nil
	}
	return func(event workflowobs.NodeEvent) {
		emitter(WorkflowNodeEvent{
			Type:       event.Type,
			TraceID:    event.TraceID,
			RunID:      event.RunID,
			WorkflowID: event.WorkflowID,
			NodeID:     event.NodeID,
			NodeType:   event.NodeType,
			LatencyMs:  event.LatencyMs,
			Error:      event.Error,
			Timestamp:  event.Timestamp,
		})
	}
}

func (s *defaultWorkflowService) ValidateDSL(rawDSL string) WorkflowDSLValidationResult {
	validator := dsl.NewValidator()
	var dslDef dsl.WorkflowDSL
	if err := yaml.Unmarshal([]byte(rawDSL), &dslDef); err != nil {
		return WorkflowDSLValidationResult{
			Valid:  false,
			Errors: []string{"invalid YAML: " + err.Error()},
		}
	}

	errs := validator.Validate(&dslDef)
	if len(errs) > 0 {
		errMsgs := make([]string, len(errs))
		for i, e := range errs {
			errMsgs[i] = e.Error()
		}
		return WorkflowDSLValidationResult{
			Valid:  false,
			Errors: errMsgs,
		}
	}

	return WorkflowDSLValidationResult{
		Valid: true,
		Name:  dslDef.Name,
	}
}

func loadDAGDefinition(filename string) (*workflow.DAGDefinition, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".json":
		return workflow.LoadFromJSONFile(filename)
	case ".yml", ".yaml":
		return workflow.LoadFromYAMLFile(filename)
	default:
		return nil, types.NewError(types.ErrInvalidRequest, "dag_file must be .json/.yml/.yaml")
	}
}

const (
	workflowSourceAuto    = "auto"
	workflowSourceDSL     = "dsl"
	workflowSourceDSLFile = "dsl_file"
	workflowSourceDAGJSON = "dag_json"
	workflowSourceDAGYAML = "dag_yaml"
	workflowSourceDAGFile = "dag_file"
)

func resolveWorkflowSource(req WorkflowBuildInput) (string, error) {
	source := strings.ToLower(strings.TrimSpace(req.Source))
	if source == "" || source == workflowSourceAuto {
		available := make([]string, 0, 5)
		if strings.TrimSpace(req.DSL) != "" {
			available = append(available, workflowSourceDSL)
		}
		if strings.TrimSpace(req.DSLFile) != "" {
			available = append(available, workflowSourceDSLFile)
		}
		if strings.TrimSpace(req.DAGJSON) != "" {
			available = append(available, workflowSourceDAGJSON)
		}
		if strings.TrimSpace(req.DAGYAML) != "" {
			available = append(available, workflowSourceDAGYAML)
		}
		if strings.TrimSpace(req.DAGFile) != "" {
			available = append(available, workflowSourceDAGFile)
		}
		if len(available) == 0 {
			return "", fmt.Errorf("dsl/dsl_file/dag_json/dag_yaml/dag_file is required")
		}
		if len(available) > 1 {
			return "", fmt.Errorf("multiple workflow sources provided: %s; please set source explicitly", strings.Join(available, ","))
		}
		return available[0], nil
	}

	switch source {
	case workflowSourceDSL:
		if strings.TrimSpace(req.DSL) == "" {
			return "", fmt.Errorf("source=%s requires dsl", workflowSourceDSL)
		}
		return source, nil
	case workflowSourceDSLFile:
		if strings.TrimSpace(req.DSLFile) == "" {
			return "", fmt.Errorf("source=%s requires dsl_file", workflowSourceDSLFile)
		}
		return source, nil
	case workflowSourceDAGJSON:
		if strings.TrimSpace(req.DAGJSON) == "" {
			return "", fmt.Errorf("source=%s requires dag_json", workflowSourceDAGJSON)
		}
		return source, nil
	case workflowSourceDAGYAML:
		if strings.TrimSpace(req.DAGYAML) == "" {
			return "", fmt.Errorf("source=%s requires dag_yaml", workflowSourceDAGYAML)
		}
		return source, nil
	case workflowSourceDAGFile:
		if strings.TrimSpace(req.DAGFile) == "" {
			return "", fmt.Errorf("source=%s requires dag_file", workflowSourceDAGFile)
		}
		return source, nil
	default:
		return "", fmt.Errorf("unsupported source: %s", source)
	}
}
