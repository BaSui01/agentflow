# 2026 前沿功能 (2026 Features)

展示分层记忆、GraphRAG、原生音频推理、Shadow AI 检测。

## 功能

- **分层记忆**：情景记忆（Episodic）+ 工作记忆（Working）+ 程序记忆（Procedural）
- **GraphRAG**：基于知识图谱的检索增强生成
- **原生音频推理**：低延迟音频处理配置（配置展示）
- **Shadow AI 检测**：检测未授权的 AI API 调用和密钥泄露
- **基础设施管理**：缓存管理器 + 数据库连接池（纯 Go SQLite 内存库）
- **Types 工具函数**：核心类型辅助函数演示
- **服务发现**：Agent 服务发现子系统
- **评估框架**：Agent 评估子系统
- **执行引擎**：Agent 执行子系统

## 前置条件

- Go 1.24+
- 无需 API Key

## 运行

```bash
cd examples/19_2026_features
go run main.go
```

## 代码说明

`memory.NewLayeredMemory` 创建三层记忆系统；`rag.NewKnowledgeGraph` 构建知识图谱；`guardrails.NewShadowAIDetector` 扫描域名和内容中的未授权 AI 使用。Infra Managers 示例会先探测本地缓存后端，未就绪时改为本地说明模式；数据库连接池演示使用纯 Go SQLite 内存库。Audio 仅展示配置，需要外部提供商才能实际运行。
