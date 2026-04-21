package usecase

type WorkflowBuildInput struct {
	DSL     string
	DSLFile string
	DAGJSON string
	DAGYAML string
	DAGFile string
	Source  string
}

type WorkflowExecuteInput struct {
	BuildInput WorkflowBuildInput
	Input      any
}

type WorkflowDSLValidationResult struct {
	Valid  bool
	Name   string
	Errors []string
}
