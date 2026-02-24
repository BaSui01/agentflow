# 引入 MongoDB：文档型数据存储层

## 目标

为 AgentFlow 引入 MongoDB 作为文档型数据存储，与现有 PostgreSQL（关系型）、Redis（缓存）、Vector DB（向量检索）形成互补的存储架构。

## 职责划分

| 存储 | 职责 |
|------|------|
| PostgreSQL | 关系型数据（LLM provider、API key、租户） |
| **MongoDB** | **文档型数据（提示词、对话、运行日志、审计）** |
| Redis | 缓存、短期记忆 |
| Vector DB | 向量检索（RAG、长期记忆） |

## 需求

### 1. 基础设施层 (`pkg/mongodb/`)

- MongoDB 客户端连接管理，对标 `pkg/database/pool.go` 模式
- 配置结构 `MongoDBConfig`，集成到 `config.Config`
- 健康检查 goroutine
- 支持 YAML + 环境变量配置（`AGENTFLOW_MONGODB_*`）
- TLS 支持（遵循 §32 TLS 硬化规范）

### 2. Agent 提示词存储 (`agent_prompts` collection)

- 存储完整 `PromptBundle` 结构（SystemPrompt + Tools + Examples 等）
- 版本管理：agent_type + name + version 唯一
- is_active 标记当前生效版本
- tenant_id 多租户隔离
- Store 接口：CRUD + 按 agent_type 查询 + 获取活跃版本

### 3. 对话历史存储 (`conversations` collection)

- 一个文档 = 一个完整对话（含所有 messages）
- 对接现有 `types.Message`、`conversation.ChatMessage` 结构
- 支持对话分支（`ConversationTree` / `Branch`）
- 超长对话分页查询
- 索引：agent_id + tenant_id + created_at
- Store 接口：创建/追加消息/查询/按时间范围检索

### 4. Agent 运行日志存储 (`agent_runs` collection)

- 对接现有 `AsyncExecution`、`Input`/`Output` 结构
- 记录：输入、输出、状态、耗时、token 用量、成本、错误信息
- 索引：agent_id + tenant_id + start_time + status
- Store 接口：记录执行/更新状态/按条件查询/统计

### 5. 审计日志存储 (`audit_logs` collection)

- 实现现有 `AuditBackend` 接口（`llm/tools/audit.go`）
- 对接 `AuditEntry` 结构
- 支持 `AuditFilter` 查询
- 高写入吞吐

## 目录结构

```
pkg/mongodb/
├── client.go          # 连接管理 + 健康检查
└── config.go          # MongoDBConfig

agent/persistence/mongodb/
├── prompt_store.go         # PromptStore 接口 + 实现
├── conversation_store.go   # ConversationStore 接口 + 实现
├── run_store.go            # RunStore 接口 + 实现
└── audit_store.go          # AuditBackend 实现

config/loader.go            # 新增 MongoDB MongoDBConfig 字段
config/defaults.go          # MongoDB 默认值
```

## 验收标准

- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 通过
- [ ] MongoDB 客户端支持连接、健康检查、优雅关闭
- [ ] 4 个 Store 接口定义清晰，与现有类型对齐
- [ ] 配置集成到 config 体系（YAML + env vars）
- [ ] 遵循项目现有模式（context.Context 第一参数、string ID、sentinel errors）
- [ ] 遵循 §32 TLS 硬化规范
- [ ] docker-compose.yml 包含 MongoDB 服务

## 技术说明

- 使用官方 Go driver: `go.mongodb.org/mongo-driver/v2`
- Store 接口定义在各自文件中，实现紧跟其后
- BSON tags 与现有 JSON tags 保持一致
- 对话文档单文档存储，超长对话通过 messages 数组分页查询（$slice）
- 所有 collection 创建索引在客户端初始化时执行
