# Novel Studio — AI 多 Agent 小说创作平台（技术栈 & 架构 PRD）

## 目标

基于 agentflow 框架作为基座（Go module 依赖），构建一个独立的 AI 多 Agent 小说创作平台。支持多租户、认证系统、前后端分离。完全复用 agentflow 已有的认证、多租户、数据库、缓存、向量数据库、RAG、工作流、Agent 协作等基础设施。

## 前置任务：agentflow 基座包导出重构

> **阻塞性依赖** — novel-studio 作为独立 Go module 无法导入 agentflow 的 `internal/` 和 `cmd/` 包。
> 必须先完成以下搬迁，才能开始 novel-studio 开发。

### 需要从 `internal/` 搬迁到 `pkg/` 的包

| 当前位置 | 目标位置 | 核心导出 | 理由 |
|----------|----------|---------|------|
| `internal/database/` | `pkg/database/` | PoolManager, PoolConfig, WithTransaction | 上层应用需要数据库连接池 |
| `internal/cache/` | `pkg/cache/` | Manager, Config, ErrCacheMiss | 上层应用需要 Redis 缓存 |
| `internal/server/` | `pkg/server/` | Manager, Config | 上层应用需要 HTTP 服务器生命周期管理 |
| `internal/metrics/` | `pkg/metrics/` | Collector | 上层应用需要 Prometheus 指标采集 |
| `internal/telemetry/` | `pkg/telemetry/` | Providers, Init | 上层应用需要 OpenTelemetry 初始化 |
| `internal/migration/` | `pkg/migration/` | Migrator, DefaultMigrator, Config | 上层应用需要数据库迁移 |

### 需要从 `cmd/agentflow/` 提取到 `pkg/middleware/` 的中间件

| 中间件 | 功能 |
|--------|------|
| `Recovery` | Panic 恢复 |
| `RequestLogger` | 请求日志 |
| `MetricsMiddleware` | Prometheus 指标 |
| `OTelTracing` | OpenTelemetry 追踪 |
| `APIKeyAuth` | API Key 认证 |
| `RateLimiter` | IP 限流 |
| `CORS` | 跨域 |
| `RequestID` | 请求 ID |
| `SecurityHeaders` | 安全头 |
| `JWTAuth` | JWT 认证 |
| `TenantRateLimiter` | 租户限流 |
| `Chain` | 中间件链组合函数 |

### 不需要搬迁的包

| 包 | 理由 |
|-----|------|
| `internal/bridge/` | 仅用于解决 agentflow 内部循环依赖，不属于基座能力 |

### 搬迁原则

1. **仅移动，不重写** — 包的 API 保持不变，只改变目录位置
2. **agentflow 内部引用同步更新** — `cmd/agentflow/` 等内部代码改为从 `pkg/` 导入
3. **`cmd/agentflow/middleware.go`** — 提取中间件到 `pkg/middleware/`，原文件改为 thin wrapper 导入

## 决策（ADR-lite）

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 文档数据库 | PostgreSQL JSONB | 最简单，agentflow 已有完整 GORM 支持，不引入额外数据库 |
| LLM Key 管理 | 平台统一 Key | 按租户计量计费，用户无需自带 Key |
| 文件存储 | 本地文件系统 + PG 元数据 | 初期最简，后续可切 MinIO/S3 |
| 前端 UI 方案 | Ant Design 5（不混用 TailwindCSS） | 避免样式冲突，Ant Design 自带完整设计体系 |
| 关系图可视化 | React Flow（统一用于 DAG 和角色关系图） | 减少依赖，一个库覆盖两个场景 |

## 已知信息（从 agentflow 代码库确认）

### agentflow 已有基础设施（直接复用，不重写）

| 能力 | agentflow 位置（搬迁后） | 复用方式 |
|------|--------------------------|---------|
| JWT + API Key 认证 | `pkg/middleware/` | 导入 JWTAuth / APIKeyAuth 中间件 |
| 多租户（tenant_id 注入 + 限流） | `pkg/middleware/` + `types/context.go` | 导入 TenantRateLimiter + `WithTenantID/WithUserID` |
| PostgreSQL (GORM) | `pkg/database/pool.go` | 导入 PoolManager |
| Redis 缓存 | `pkg/cache/manager.go` | 导入 cache.Manager |
| Redis 消息持久化 | `agent/persistence/redis_message_store.go` | 导入 MessageStore |
| 向量数据库 (Milvus/Qdrant/Weaviate/Pinecone) | `rag/` + `rag/factory.go` | `NewVectorStoreFromConfig()` |
| Embedding (OpenAI/Cohere/Voyage/Jina/Gemini) | `llm/embedding/` + `rag/factory.go` | `NewEmbeddingProviderFromConfig()` |
| 混合检索 (BM25 + 向量) | `rag/hybrid_retrieval.go` | 导入 HybridRetriever |
| 文档加载 (txt/md/csv/json) | `rag/loader/` | 导入 LoaderRegistry |
| LLM 多 Provider (14+) | `llm/factory/` | `NewProviderFromConfig()` |
| Agent 协作 (debate/consensus/pipeline) | `agent/collaboration/` | 导入 MultiAgentSystem |
| 角色编排 (RolePipeline) | `agent/collaboration/roles.go` | 导入 RoleRegistry |
| Crew 团队 | `agent/crews/` | 导入 Crew |
| DAG 工作流 | `workflow/` | 导入 DAGBuilder |
| 人工审批 (HITL) | `agent/hitl/` | 导入 InterruptManager |
| 多层记忆 (短期/长期/情景/语义) | `agent/memory/` | 导入 EnhancedMemorySystem |
| 知识图谱 | `agent/memory/knowledge_graph.go` | 导入 KnowledgeGraph |
| 结构化输出 | `agent/structured/` | 导入 StructuredOutput[T] |
| 流式输出 | `agent/streaming/` | 导入 BidirectionalStream |
| 推理策略 | `agent/reasoning/` | 导入 PlanAndExecute |
| HTTP Server + 中间件链 | `pkg/server/` + `pkg/middleware/` | 导入 ServerManager + Chain + 中间件 |
| 配置热加载 | `config/hotreload.go` | 导入 HotReloadManager |
| OpenTelemetry + Prometheus | `pkg/telemetry/` + `pkg/metrics/` | 导入 |
| LLM Prompt 缓存 (Redis+LRU) | `llm/cache/prompt_cache.go` | 导入 MultiLevelCache |
| 语义缓存 | `rag/vector_store.go` SemanticCache | 导入 |

---

## 技术栈

### 后端

| 组件 | 技术选型 | 理由 |
|------|---------|------|
| 语言 | Go 1.24+ | 与 agentflow 一致 |
| 框架基座 | agentflow (Go module import) | 完全复用，不 fork |
| HTTP Server | agentflow `pkg/server.Manager` | 已有优雅关闭、TLS |
| 路由 | `net/http` ServeMux (Go 1.22+) | agentflow 模式一致 |
| 中间件 | agentflow `pkg/middleware` | JWT/APIKey/CORS/限流/追踪 |
| 认证 | agentflow JWT + API Key | 已有 tenant_id/user_id 注入 |
| 多租户 | agentflow TenantRateLimiter + 数据隔离 | 所有表加 tenant_id |
| ORM | GORM (agentflow `pkg/database`) | 已有连接池、事务重试 |
| 关系数据库 | PostgreSQL 16 | 小说结构化数据、版本管理 |
| 文档数据库 | PostgreSQL JSONB | 世界观设定、角色属性等半结构化数据用 JSONB 列 |
| 缓存 | Redis 7 (agentflow `pkg/cache`) | 已有 Manager + TTL + 健康检查 |
| 消息队列 | Redis Streams (agentflow `agent/persistence`) | Agent 间消息传递 |
| 向量数据库 | Milvus 2.x (agentflow `rag/milvus_store.go`) | RAG 知识库，已有完整实现 |
| Embedding | OpenAI text-embedding-3-small | 性价比高，agentflow 已集成 |
| LLM Providers | Claude/DeepSeek/GPT-4o/Gemini | 多模型协作，agentflow factory |
| 可观测性 | OpenTelemetry + Prometheus (agentflow `pkg/telemetry` + `pkg/metrics`) | 已有完整集成 |
| 日志 | zap (agentflow) | 结构化日志 |

### 前端

| 组件 | 技术选型 | 理由 |
|------|---------|------|
| 框架 | React 18 + TypeScript | 用户指定 |
| 构建 | Vite 5 | 快速 HMR |
| 状态管理 | Zustand | 轻量、TypeScript 友好 |
| UI 组件库 | Ant Design 5 | 中文友好、表格/表单/布局完善，自带 CSS-in-JS |
| 流程可视化 | React Flow | DAG 工作流 + 角色关系图（统一一个库） |
| 富文本编辑 | TipTap (ProseMirror) | 小说编辑器，支持协作 |
| Diff 对比 | react-diff-viewer | 版本对比 |
| HTTP 客户端 | Axios | 拦截器、错误处理 |
| WebSocket | 原生 WebSocket + reconnecting-websocket | 实时流式输出 |
| 路由 | React Router v7 | SPA 路由 |

### 基础设施

| 组件 | 技术选型 |
|------|---------|
| 容器化 | Docker + docker-compose |
| PostgreSQL | postgres:16-alpine |
| Redis | redis:7-alpine |
| Milvus | milvusdb/milvus:v2.4-latest |
| 反向代理 | Caddy (开发) / Nginx (生产) |

---

## 项目结构

```
novel-studio/
├── go.mod                          # github.com/BaSui01/novel-studio
├── go.sum                          #   require github.com/BaSui01/agentflow v0.x.x
├── Makefile
├── Dockerfile
├── docker-compose.yml              # postgres + redis + milvus + app
├── uploads/                        # 参考资料文件存储（本地）
├── config/
│   ├── config.yaml                 # 主配置（复用 agentflow config 结构）
│   └── config.go                   # 扩展 agentflow Config
├── cmd/novel-studio/
│   ├── main.go                     # 入口：wire + 启动
│   └── server.go                   # 路由注册（复用 agentflow 中间件链）
├── internal/
│   ├── domain/                     # 领域模型
│   │   ├── novel.go                # Novel, Volume, Chapter
│   │   ├── character.go            # Character, Relationship
│   │   ├── world.go                # WorldSetting, WorldRule (JSONB)
│   │   ├── plot.go                 # PlotThread, PlotPoint
│   │   ├── version.go              # Version, Snapshot
│   │   └── user.go                 # User, Tenant
│   ├── auth/                       # 认证（novel-studio 新增）
│   │   ├── jwt.go                  # JWT 签发（复用 agentflow 验证中间件）
│   │   ├── password.go             # bcrypt 哈希
│   │   └── handler.go              # 注册/登录/刷新 handler
│   ├── store/                      # 持久层 (GORM)
│   │   ├── tenant_scope.go         # GORM Scope: 自动注入 WHERE tenant_id
│   │   ├── user_repo.go
│   │   ├── tenant_repo.go
│   │   ├── novel_repo.go
│   │   ├── chapter_repo.go
│   │   ├── character_repo.go
│   │   ├── version_repo.go
│   │   ├── file_repo.go            # 文件元数据
│   │   └── migrations/
│   ├── agents/                     # 6 个写作 Agent
│   │   ├── registry.go             # 注册所有 Agent 到 agentflow
│   │   ├── plotter.go              # 大纲师 → Claude Sonnet
│   │   ├── worldbuilder.go         # 世界观架构师 → Claude Sonnet
│   │   ├── character_designer.go   # 角色设计师 → Claude Sonnet
│   │   ├── chapter_writer.go       # 章节写手 → DeepSeek Chat
│   │   ├── editor.go               # 编辑润色师 → GPT-4o
│   │   ├── continuity_checker.go   # 连续性检查员 → Gemini Flash
│   │   ├── types.go                # 结构化输出类型
│   │   └── prompts/                # go:embed 系统提示词
│   ├── workflow/                   # DAG 工作流
│   │   ├── novel_workflow.go       # 主创作 DAG
│   │   ├── chapter_workflow.go     # 单章子流程
│   │   └── revision_workflow.go    # 修订循环
│   ├── rag/                        # RAG 知识库配置
│   │   ├── indexer.go              # 索引世界观/角色/章节
│   │   ├── retriever.go            # 上下文检索
│   │   └── collections.go          # Milvus 集合定义
│   ├── memory/                     # 知识图谱封装
│   │   ├── novel_graph.go          # 小说专用知识图谱操作
│   │   └── consistency.go          # 一致性检查逻辑
│   ├── upload/                     # 文件上传
│   │   └── handler.go              # 上传/下载 handler（本地文件系统）
│   ├── api/                        # REST + WebSocket
│   │   ├── router.go
│   │   ├── novel_handler.go
│   │   ├── chapter_handler.go
│   │   ├── character_handler.go
│   │   ├── world_handler.go
│   │   ├── workflow_handler.go
│   │   ├── interrupt_handler.go    # 人工审批
│   │   ├── stream_handler.go       # WebSocket 流式
│   │   └── version_handler.go
│   └── service/                    # 业务编排
│       ├── novel_service.go
│       └── version_service.go
└── web/                            # React 前端
    ├── package.json
    ├── vite.config.ts
    └── src/
        ├── api/                    # Axios 客户端
        ├── store/                  # Zustand stores
        ├── pages/                  # 7 个核心页面
        ├── components/             # 可复用组件
        ├── hooks/                  # WebSocket/Streaming hooks
        └── types/                  # TypeScript 类型
```

## 认证 & 多租户设计

### 认证流程
```
前端 → POST /api/v1/auth/login (email+password)
     ← JWT token (含 tenant_id, user_id, roles)
     → 后续请求 Authorization: Bearer <token>
     → agentflow JWTAuth 中间件自动解析 → ctx 注入 tenant_id/user_id
```

novel-studio 新增:
- `internal/auth/` — 用户注册/登录/密码哈希（bcrypt）
- `internal/store/user_repo.go` — User 表（tenant_id, email, password_hash, role）
- `internal/store/tenant_repo.go` — Tenant 表（name, plan, quota）
- 复用 agentflow 的 `JWTAuth()` 中间件 + `types.WithTenantID()`

### 多租户数据隔离
- 所有业务表加 `tenant_id` 列 + 索引
- GORM Scope 自动注入 `WHERE tenant_id = ?`
- Milvus 集合按 tenant 分区（partition key = tenant_id）
- Redis key 前缀: `novel:{tenant_id}:*`

## 六大 Agent

| Agent | 默认 LLM | agentflow 复用 | 输出类型 |
|-------|---------|---------------|---------|
| 大纲师 | Claude Sonnet | `StructuredOutput[PlotOutline]` + `PlanAndExecute` | PlotOutline |
| 世界观架构师 | Claude Sonnet | `StructuredOutput[WorldBible]` + `KnowledgeGraph` | WorldBible |
| 角色设计师 | Claude Sonnet | `StructuredOutput[CharacterSheet]` + `KnowledgeGraph` | CharacterSheet |
| 章节写手 | DeepSeek Chat | `BidirectionalStream` + `HybridRetriever` | ChapterDraft |
| 编辑润色师 | GPT-4o | `MultiAgentSystem(Debate)` + `StructuredOutput[EditResult]` | EditResult |
| 连续性检查员 | Gemini Flash | `EnhancedMemorySystem` + `HybridRetriever` | ContinuityReport |

## DAG 工作流

```
[概念输入] → [大纲师] → ┬→ [世界观架构师]
                        └→ [角色设计师]
                              ↓
                    ★ [人工审批: 大纲]
                              ↓
                    [章节循环 ForEach]
                              ↓
              ┌─ 无依赖章节并行 (NodeTypeParallel) ─┐
              ↓                                      ↓
        [章节写手] → [连续性检查] → [编辑润色]
              ↑         ↓(有问题)      ↓(评分<7)
              └── 修订 ←┘         Debate 模式
                                       ↓
                              ★ [人工审批: 章节]
                                       ↓
                                 [保存版本]
```

## 版本管理

- PostgreSQL 存储 Version 记录（content + unified diff + parent_id）
- 分支支持: branch_name 字段（main / alt-ending-1）
- Snapshot = 全书某时刻所有章节版本的快照
- 回滚: 将 Chapter.version_id 指向历史 Version

## RAG 知识库

4 个 Milvus 集合（按 tenant_id 分区）:

| 集合 | 内容 | 分块 |
|------|------|------|
| world_settings | 世界观、规则、地点 | 按条目 |
| character_profiles | 角色卡、关系 | 按角色 |
| chapters | 已写章节 | 语义分块 512 tokens |
| references | 用户上传参考资料 | 标准分块 |

## API 端点

```
# 认证（novel-studio 新增）
POST   /api/v1/auth/register
POST   /api/v1/auth/login
POST   /api/v1/auth/refresh

# 小说 CRUD
POST   /api/v1/novels
GET    /api/v1/novels
GET    /api/v1/novels/:id
PUT    /api/v1/novels/:id
DELETE /api/v1/novels/:id

# 章节 / 角色 / 世界观
GET    /api/v1/novels/:id/chapters
GET    /api/v1/novels/:id/characters
GET    /api/v1/novels/:id/world-settings

# 工作流
POST   /api/v1/novels/:id/workflow/start
GET    /api/v1/novels/:id/workflow/status
POST   /api/v1/novels/:id/workflow/pause

# 人工审批
GET    /api/v1/interrupts
POST   /api/v1/interrupts/:id/resolve

# 版本
GET    /api/v1/chapters/:id/versions
POST   /api/v1/chapters/:id/versions/rollback
POST   /api/v1/novels/:id/snapshots

# WebSocket
WS     /api/v1/ws/stream/:workflow_id
```

## 前端核心页面

| 页面 | 功能 | 关键组件 |
|------|------|---------|
| Dashboard | 项目列表、创建 | Ant Design Table/Card |
| NovelEditor | 主工作台 | TipTap 编辑器 + Agent 状态面板 |
| WorldBuilder | 世界观管理 | 树形结构 + JSONB 编辑 |
| CharacterSheet | 角色管理 | 角色卡片 + React Flow 关系图 |
| WorkflowMonitor | DAG 可视化 | React Flow + 实时状态 |
| ApprovalQueue | 审批队列 | 内容预览 + 通过/驳回 |
| VersionHistory | 版本时间线 | react-diff-viewer |

## 实施步骤

### Phase 0：agentflow 基座重构（前置，在 agentflow 仓库完成）
0. `internal/` → `pkg/` 搬迁（database, cache, server, metrics, telemetry, migration）
1. `cmd/agentflow/middleware.go` → `pkg/middleware/` 提取
2. agentflow 内部引用更新 + `go build ./...` + `go vet ./...` 验证
3. 打 tag 发布新版本（供 novel-studio go.mod require）

### Phase 1：novel-studio 项目搭建
1. 项目骨架: go.mod (import agentflow) + Makefile + docker-compose + config
2. 认证 & 多租户: User/Tenant 表 + 登录注册 + GORM tenant scope
3. 领域模型 + 数据库迁移: domain/ + store/ + migrations

### Phase 2：AI Agent 核心
4. 6 个 Agent 实现: agents/ + 系统提示词 + 结构化输出
5. DAG 工作流: workflow/ + 人工审批集成
6. RAG 知识库: rag/ 索引器 + 检索器
7. 知识图谱 + 一致性: memory/ 层

### Phase 3：API & 版本管理
8. 版本管理: version_service + version_repo
9. API 层: REST handlers + WebSocket streaming

### Phase 4：前端
10. React 前端: Vite + Ant Design + React Flow + TipTap

## 验证方式

- `make test` 单元测试
- `docker-compose up` 启动全栈（postgres + redis + milvus + app）
- API 创建小说 → 启动工作流 → Agent 协作 → 人工审批 → 查看结果
- `cd web && npm run dev` 前端开发服务器
