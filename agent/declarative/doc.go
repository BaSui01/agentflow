// Copyright 2026 AgentFlow Authors
// Use of this source code is governed by the project license.

/*
# 概述

包 declarative 提供基于 YAML/JSON 的声明式 Agent 定义与加载能力。

用户可通过配置文件（而非 Go 代码）定义 Agent，由本包加载并转换为
与 agent.NewAgentBuilder 兼容的运行时配置。为避免循环依赖，本包
不直接导入 agent 包，而是产出 config map 供调用方自行组装 Builder。

# 核心接口

  - AgentLoader — 从文件或字节流加载 AgentDefinition，支持自动格式检测
  - AgentFactory — 校验定义合法性并转换为 agent.Config 兼容的 map

# 主要类型

  - AgentDefinition — 声明式 Agent 规格，涵盖身份、模型、工具、Memory、
    Guardrails 及 Feature 开关
  - AgentFeatures — 可选能力开关（Reflection、MCP、Observability 等）
  - ToolDefinition / MemoryConfig / GuardrailsConfig — 子配置结构体

# 典型用法

	loader := declarative.NewYAMLLoader()
	def, err := loader.LoadFile("my-agent.yaml")

	factory := declarative.NewAgentFactory(logger)
	if err := factory.Validate(def); err != nil { ... }
	configMap := factory.ToAgentConfig(def)

# 设计约束

  - 不导入 agent 包，通过 map[string]interface{} 传递配置
  - Validate 在转换前检查必填字段与数值范围
  - 支持 YAML (.yaml/.yml) 和 JSON (.json) 两种格式
*/
package declarative
