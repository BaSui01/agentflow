package llm

// 已废弃：此文件包含遗留的路由器实现。
// 请使用多提供者路由器（router_multi_provider.go）替代。
//
// 遗留路由器基于单一提供者/模型架构设计，
// 与新的多提供者数据模型不兼容（新模型中一个模型可由多个提供者提供）。
//
// 迁移指南：
// - 将 NewRouter() 替换为 NewMultiRouter()
// - 使用 SelectProviderWithModel() 而非 SelectProvider()
// - API Key 集合现在由各提供者分别管理
