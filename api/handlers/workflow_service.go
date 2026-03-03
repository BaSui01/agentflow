package handlers

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow"
	"github.com/BaSui01/agentflow/workflow/dsl"
	workflowobs "github.com/BaSui01/agentflow/workflow/observability"
	"gopkg.in/yaml.v3"
)

// WorkflowExecutor defines the workflow execution facade contract used by handlers.
type WorkflowExecutor interface {
	ExecuteDAG(ctx context.Context, wf *workflow.DAGWorkflow, input any) (any, error)
}

// WorkflowService encapsulates workflow parsing, validation and execution use-cases.
type WorkflowService interface {
	BuildDAGWorkflow(req workflowExecuteRequest) (*workflow.DAGWorkflow, *types.Error)
	Execute(ctx context.Context, wf *workflow.DAGWorkflow, input any, streamEmitter workflow.WorkflowStreamEmitter, nodeEmitter workflowobs.NodeEventEmitter) (any, *types.Error)
	ValidateDSL(rawDSL string) workflowDSLValidationResult
}

type workflowDSLValidationResult struct {
	Valid  bool
	Name   string
	Errors []string
}

type defaultWorkflowService struct {
	executor WorkflowExecutor
	parser   *dsl.Parser
}

func newDefaultWorkflowService(executor WorkflowExecutor, parser *dsl.Parser) WorkflowService {
	return &defaultWorkflowService{
		executor: executor,
		parser:   parser,
	}
}

func (s *defaultWorkflowService) BuildDAGWorkflow(req workflowExecuteRequest) (*workflow.DAGWorkflow, *types.Error) {
	if s.parser == nil {
		return nil, types.NewInternalError("workflow parser is not configured")
	}

	var (
		wf  *workflow.DAGWorkflow
		err error
	)

	switch {
	case req.DAGFile != "":
		var def *workflow.DAGDefinition
		def, err = loadDAGDefinition(req.DAGFile)
		if err == nil {
			wf, err = def.ToDAGWorkflow()
		}
	case req.DAGJSON != "":
		var def *workflow.DAGDefinition
		def, err = workflow.FromJSON(req.DAGJSON)
		if err == nil {
			wf, err = def.ToDAGWorkflow()
		}
	case req.DAGYAML != "":
		var def *workflow.DAGDefinition
		def, err = workflow.FromYAML(req.DAGYAML)
		if err == nil {
			wf, err = def.ToDAGWorkflow()
		}
	case req.DSLFile != "":
		wf, err = s.parser.ParseFile(req.DSLFile)
	default:
		wf, err = s.parser.Parse([]byte(req.DSL))
	}

	if err != nil {
		return nil, types.NewError(types.ErrInvalidRequest, "invalid workflow DSL: "+err.Error()).
			WithHTTPStatus(http.StatusBadRequest)
	}

	return wf, nil
}

func (s *defaultWorkflowService) Execute(
	ctx context.Context,
	wf *workflow.DAGWorkflow,
	input any,
	streamEmitter workflow.WorkflowStreamEmitter,
	nodeEmitter workflowobs.NodeEventEmitter,
) (any, *types.Error) {
	if s.executor == nil {
		return nil, types.NewInternalError("workflow executor is not configured").
			WithHTTPStatus(http.StatusNotImplemented)
	}

	execCtx := workflow.WithWorkflowStreamEmitter(ctx, streamEmitter)
	execCtx = workflowobs.WithNodeEventEmitter(execCtx, nodeEmitter)

	result, err := s.executor.ExecuteDAG(execCtx, wf, input)
	if err != nil {
		return nil, types.NewError(types.ErrInternalError, "workflow execution failed: "+err.Error())
	}
	return result, nil
}

func (s *defaultWorkflowService) ValidateDSL(rawDSL string) workflowDSLValidationResult {
	validator := dsl.NewValidator()
	var dslDef dsl.WorkflowDSL
	if err := yaml.Unmarshal([]byte(rawDSL), &dslDef); err != nil {
		return workflowDSLValidationResult{
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
		return workflowDSLValidationResult{
			Valid:  false,
			Errors: errMsgs,
		}
	}

	return workflowDSLValidationResult{
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
