// 版权所有 2024 AgentFlow Authors. 保留所有权利。
// 此源代码的使用由 MIT 许可规范，该许可可以
// 在 LICENSE 文件中找到。

/*
示例 15_structured_output 演示了 AgentFlow 的结构化输出（Structured Output）能力。

# 演示内容

本示例展示 JSON Schema 生成、校验与手动构建的完整流程：

  - Schema Generation：通过反射从 Go struct 自动生成 JSON Schema，
    支持 required、enum、format、minLength、pattern 等约束标签
  - Schema Validation：对 JSON 数据进行 Schema 校验，
    返回带路径的详细错误信息（ValidationErrors）
  - Manual Schema Building：使用 Fluent API 手动构建嵌套 Schema，
    支持 Object、Array、String、Integer、Boolean、Number 等类型组合

# 运行方式

	go run .
*/
package main
