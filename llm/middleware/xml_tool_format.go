package middleware

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/types"
)

// FormatToolsAsXML 将工具列表转换为 XML 文本描述，注入到 system prompt 中。
// LLM 读到这段文本后会按约定的 <tool_calls> 格式输出工具调用。
func FormatToolsAsXML(tools []types.ToolSchema) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("\n\n# Available Tools\n\n")
	sb.WriteString("You have access to the following tools. To use a tool, respond with a `<tool_calls>` XML block.\n\n")

	for i, tool := range tools {
		sb.WriteString(fmt.Sprintf("## Tool %d: %s\n", i+1, tool.Name))
		if tool.Description != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n", tool.Description))
		}
		if len(tool.Parameters) > 0 {
			sb.WriteString("Parameters schema:\n```json\n")
			// 格式化 JSON 让 LLM 更容易理解参数结构
			var pretty json.RawMessage
			if json.Unmarshal(tool.Parameters, &pretty) == nil {
				formatted, err := json.MarshalIndent(pretty, "", "  ")
				if err == nil {
					sb.Write(formatted)
				} else {
					sb.Write(tool.Parameters)
				}
			} else {
				sb.Write(tool.Parameters)
			}
			sb.WriteString("\n```\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## How to call tools\n\n")
	sb.WriteString("When you want to call a tool, output the following XML block (do NOT wrap it in markdown code fences):\n\n")
	sb.WriteString("<tool_calls>\n")
	sb.WriteString("{\"name\": \"tool_name\", \"arguments\": {\"param1\": \"value1\"}}\n")
	sb.WriteString("</tool_calls>\n\n")
	sb.WriteString("You can call multiple tools by including multiple JSON objects separated by newlines inside a single `<tool_calls>` block:\n\n")
	sb.WriteString("<tool_calls>\n")
	sb.WriteString("{\"name\": \"tool1\", \"arguments\": {\"param1\": \"value1\"}}\n")
	sb.WriteString("{\"name\": \"tool2\", \"arguments\": {\"param2\": \"value2\"}}\n")
	sb.WriteString("</tool_calls>\n\n")
	sb.WriteString("Important rules:\n")
	sb.WriteString("- Always use the exact tool names listed above.\n")
	sb.WriteString("- Arguments must be valid JSON matching the parameter schema.\n")
	sb.WriteString("- Do NOT include any text inside the <tool_calls> block other than the JSON objects.\n")
	sb.WriteString("- You may include thinking/explanation text before or after the <tool_calls> block.\n")

	return sb.String()
}
