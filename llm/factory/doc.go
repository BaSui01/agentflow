// Package factory 提供 LLM Provider 的集中式工厂，
// 通过名称映射创建 Provider 实例，打破 llm 包与各 provider 子包之间的循环依赖。
package factory
