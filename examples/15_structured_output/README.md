# 结构化输出 (Structured Output)

展示 JSON Schema 生成、验证和手动构建，用于约束 LLM 输出格式。

## 功能

- **Schema 生成**：从 Go 结构体自动生成 JSON Schema（支持 `jsonschema` tag）
- **Schema 验证**：验证 JSON 数据是否符合 Schema（必填字段、枚举、范围等）
- **手动构建**：使用 Builder API 编程式构建复杂 Schema（嵌套对象、数组）

## 前置条件

- Go 1.24+
- 无需 API Key

## 运行

```bash
cd examples/15_structured_output
go run main.go
```

## 代码说明

`structured.NewSchemaGenerator` 通过反射从 Go 类型生成 Schema；`structured.NewValidator` 验证 JSON 数据；`structured.NewObjectSchema` 等 Builder 方法支持链式构建 Schema。
