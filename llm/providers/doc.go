// 包 providers 提供跨模型服务商的通用适配与辅助能力。
//
// 该包包含：
//   - 通用请求/响应转换逻辑
//   - 跨 Provider 的错误映射与重写链路
//   - 流式与工具调用相关的共享辅助函数
//
// 各具体服务商实现位于 `llm/providers/*` 子包。
package providers
