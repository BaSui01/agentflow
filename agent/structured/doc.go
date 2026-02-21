// Copyright 2026 AgentFlow Authors
// Use of this source code is governed by the project license.

/*
# 概述

包 structured 提供结构化输出的 Schema 建模、生成、解析与校验能力。

该包用于约束 LLM 输出格式，降低自由文本导致的解析失败风险。
当 Provider 支持原生结构化输出时自动启用，否则回退到 Prompt Engineering。

# 核心接口

  - StructuredOutputProvider — 扩展 llm.Provider，声明是否支持原生结构化输出
  - SchemaValidator — 对 JSON 数据按 JSONSchema 进行字段级校验

# 主要类型

  - JSONSchema — 完整的 JSON Schema 定义，支持 object/array/enum/组合关键词
  - StructuredOutput[T] — 泛型结构化输出处理器，自动生成 Schema 并驱动 LLM 生成
  - SchemaGenerator — 通过反射从 Go 类型生成 JSONSchema，支持 jsonschema 标签
  - DefaultValidator — 内置格式校验（email/uri/uuid/datetime/ipv4 等）
  - ParseResult[T] / ParseError / ValidationErrors — 解析与校验结果

# 典型用法

	so, _ := structured.NewStructuredOutput[MyStruct](provider)
	result, _ := so.Generate(ctx, "提取用户信息")

	// 或使用自定义 Schema
	so2, _ := structured.NewStructuredOutputWithSchema[MyStruct](provider, schema)
	pr, _ := so2.GenerateWithParse(ctx, prompt)
	if !pr.IsValid() { // 处理校验错误 }

# 主要能力

  - Schema 建模：定义对象、数组、枚举及约束规则
  - 结构化生成：引导模型按指定结构输出，支持原生模式与 Prompt 回退
  - 结果校验：对输出进行字段级校验与错误报告
  - 反射生成：从 Go struct 标签自动推导 JSONSchema
*/
package structured
