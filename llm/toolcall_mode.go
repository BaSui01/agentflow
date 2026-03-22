package llm

// ToolCallMode 表示工具调用的模式。
// 当 Provider 支持原生函数调用时使用 Native 模式；
// 当 Provider 不支持时，框架自动降级为 XML 模式——
// 将工具定义注入 system prompt，从 LLM 文本输出中解析 <tool_calls> 标签。
type ToolCallMode string

const (
	// ToolCallModeNative 原生函数调用模式（Provider 原生支持 JSON tool calling）
	ToolCallModeNative ToolCallMode = "native"

	// ToolCallModeXML XML 文本降级模式（工具定义注入 prompt，从文本解析调用）
	ToolCallModeXML ToolCallMode = "xml"
)
