package dsl

// WorkflowDSL 工作流 DSL 顶层结构
type WorkflowDSL struct {
	// Version DSL 版本
	Version string `yaml:"version" json:"version"`
	// Name 工作流名称
	Name string `yaml:"name" json:"name"`
	// Description 工作流描述
	Description string `yaml:"description" json:"description"`

	// Variables 全局变量定义
	Variables map[string]VariableDef `yaml:"variables,omitempty" json:"variables,omitempty"`

	// Agents Agent 定义
	Agents map[string]AgentDef `yaml:"agents,omitempty" json:"agents,omitempty"`

	// Tools 工具定义
	Tools map[string]ToolDef `yaml:"tools,omitempty" json:"tools,omitempty"`

	// Steps 步骤定义（可复用）
	Steps map[string]StepDef `yaml:"steps,omitempty" json:"steps,omitempty"`

	// Workflow 工作流节点定义
	Workflow WorkflowNodesDef `yaml:"workflow" json:"workflow"`

	// Metadata 元数据
	Metadata map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// VariableDef 变量定义
type VariableDef struct {
	Type        string      `yaml:"type" json:"type"`                                  // string, int, float, bool, list, map
	Default     interface{} `yaml:"default,omitempty" json:"default,omitempty"`         // 默认值
	Description string      `yaml:"description,omitempty" json:"description,omitempty"` // 描述
	Required    bool        `yaml:"required,omitempty" json:"required,omitempty"`       // 是否必填
}

// AgentDef Agent 定义
type AgentDef struct {
	Model        string            `yaml:"model" json:"model"`
	Provider     string            `yaml:"provider,omitempty" json:"provider,omitempty"`
	SystemPrompt string            `yaml:"system_prompt,omitempty" json:"system_prompt,omitempty"`
	Temperature  float64           `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens    int               `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	Tools        []string          `yaml:"tools,omitempty" json:"tools,omitempty"` // 引用 tools 中定义的工具
	Metadata     map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// ToolDef 工具定义
type ToolDef struct {
	Type        string                 `yaml:"type" json:"type"` // builtin, mcp, http, code
	Description string                 `yaml:"description" json:"description"`
	Config      map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
	InputSchema map[string]interface{} `yaml:"input_schema,omitempty" json:"input_schema,omitempty"`
}

// StepDef 步骤定义
type StepDef struct {
	Type   string                 `yaml:"type" json:"type"` // llm, tool, human_input, code, passthrough
	Agent  string                 `yaml:"agent,omitempty" json:"agent,omitempty"`   // 引用 agents 中的 agent
	Tool   string                 `yaml:"tool,omitempty" json:"tool,omitempty"`     // 引用 tools 中的工具
	Prompt string                 `yaml:"prompt,omitempty" json:"prompt,omitempty"` // 支持 ${variable} 插值
	Config map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// WorkflowNodesDef 工作流节点定义
type WorkflowNodesDef struct {
	Entry string    `yaml:"entry" json:"entry"`
	Nodes []NodeDef `yaml:"nodes" json:"nodes"`
}

// NodeDef 节点定义
type NodeDef struct {
	ID        string                 `yaml:"id" json:"id"`
	Type      string                 `yaml:"type" json:"type"` // action, condition, loop, parallel, subgraph, checkpoint
	Step      string                 `yaml:"step,omitempty" json:"step,omitempty"`         // 引用 steps 中的步骤
	StepDef   *StepDef               `yaml:"step_def,omitempty" json:"step_def,omitempty"` // 内联步骤定义
	Next      []string               `yaml:"next,omitempty" json:"next,omitempty"`
	Condition string                 `yaml:"condition,omitempty" json:"condition,omitempty"` // 条件表达式
	OnTrue    []string               `yaml:"on_true,omitempty" json:"on_true,omitempty"`
	OnFalse   []string               `yaml:"on_false,omitempty" json:"on_false,omitempty"`
	Loop      *LoopDef               `yaml:"loop,omitempty" json:"loop,omitempty"`
	Parallel  []string               `yaml:"parallel,omitempty" json:"parallel,omitempty"`
	SubGraph  *WorkflowNodesDef      `yaml:"subgraph,omitempty" json:"subgraph,omitempty"`
	Error     *ErrorDef              `yaml:"error,omitempty" json:"error,omitempty"`
	Metadata  map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// LoopDef 循环定义
type LoopDef struct {
	Type          string `yaml:"type" json:"type"` // while, for, foreach
	Condition     string `yaml:"condition,omitempty" json:"condition,omitempty"`           // 条件表达式
	MaxIterations int    `yaml:"max_iterations,omitempty" json:"max_iterations,omitempty"` // 最大迭代次数
	Collection    string `yaml:"collection,omitempty" json:"collection,omitempty"`         // foreach 的集合表达式
	ItemVar       string `yaml:"item_var,omitempty" json:"item_var,omitempty"`             // foreach 的迭代变量名
}

// ErrorDef 错误处理定义
type ErrorDef struct {
	Strategy      string      `yaml:"strategy" json:"strategy"` // fail_fast, skip, retry
	MaxRetries    int         `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	RetryDelayMs  int         `yaml:"retry_delay_ms,omitempty" json:"retry_delay_ms,omitempty"`
	FallbackValue interface{} `yaml:"fallback_value,omitempty" json:"fallback_value,omitempty"`
}
