# 贡献指南

感谢你对 AgentFlow 的关注！以下是参与贡献的流程和规范。

## 开发环境

- Go 1.24+
- Docker & Docker Compose（用于本地依赖服务）
- golangci-lint（代码检查）

```bash
# 克隆项目
git clone https://github.com/BaSui01/agentflow.git
cd agentflow

# 安装依赖
go mod download

# 启动本地服务（PostgreSQL, Redis, Qdrant）
make up

# 运行测试
make test

# 代码检查
make lint
```

## 提交流程

1. Fork 仓库并创建特性分支：`git checkout -b feat/your-feature`
2. 编写代码和测试
3. 确保通过所有检查：`make lint && make test`
4. 提交 PR 到 `master` 分支

## Commit 规范

使用 [Conventional Commits](https://www.conventionalcommits.org/) 格式：

```
feat(agent): 添加新的推理模式
fix(llm): 修复流式响应解析
docs(readme): 更新快速开始示例
test(rag): 补充向量检索测试
refactor(workflow): 重构 DAG 执行器
```

## 代码规范

- 遵循项目 `.golangci.yml` 中配置的 21 个 linter 规则
- 导出函数必须有文档注释
- 错误不得静默忽略（`_ = someFunc()` 仅在有充分理由时使用，并附注释说明）
- 新功能必须附带单元测试
- 分层架构约束：`types → llm → agent → workflow → api → cmd`，低层不得导入高层

## 测试

```bash
make test          # 单元测试
make test-short    # 快速测试（跳过集成测试）
make test-cover    # 覆盖率报告
make test-e2e      # 端到端测试（需要 Docker 服务）
make bench         # 性能基准测试
```

## 项目结构

| 目录 | 职责 |
|------|------|
| `types/` | 零依赖核心类型 |
| `llm/` | LLM 抽象层（提供商、路由、缓存） |
| `agent/` | Agent 核心（推理、记忆、工具、协议） |
| `rag/` | RAG 检索增强生成 |
| `workflow/` | DAG 工作流引擎 |
| `api/` | HTTP API 层 |
| `cmd/` | CLI 入口 |
| `pkg/` | 内部基础设施（中间件、遥测、数据库） |

## 问题反馈

- 使用 [GitHub Issues](https://github.com/BaSui01/agentflow/issues) 报告 Bug
- 功能建议请先开 Issue 讨论

## 许可证

贡献的代码将遵循 [MIT License](LICENSE)。
