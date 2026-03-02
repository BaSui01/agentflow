# RAG 层重构执行文档（单轨替换，非兼容）

> 文档类型：可执行重构规范  
> 适用范围：`rag/` 全域（检索、索引、向量存储、文档加载、配置桥接）  
> 迁移策略：不兼容旧实现，不保留双轨

---

## 0. 执行状态总览

- [x] 完成 RAG 当前实现盘点（目录、入口、检索策略、向量后端）
- [x] 完成 RAG 分层边界确认（`rag` 保持 Layer 2，与 `agent` 同级）
- [ ] 完成 RAG 单一构建入口与单一执行主链落地
- [ ] 完成 RAG 配置桥接收敛（移除并行工厂语义）
- [ ] 完成 RAG-LLM 调用口收敛（统一能力入口）
- [ ] 完成 RAG 契约向 `types` 的最小化对齐
- [ ] 完成架构守卫与回归测试

---

## 1. 重构目标（必须同时满足）

### 1.1 业务目标

- 单一构建入口：RAG 运行时实例只通过一个 Builder/Factory 主入口构建。
- 单一执行路径：检索执行链统一为 `query transform -> retrieve -> rerank -> compose`。
- 单一配置口径：配置桥接只保留一套，不保留同义工厂路径。
- 单一观测口径：检索耗时、召回条数、重排耗时、上下文 token 统一埋点。

### 1.2 架构目标

- 严格分层：`rag`（Layer 2）仅依赖 `llm` + `types` + `config` + 基础设施，不依赖 `agent/workflow/api/cmd`。
- 保持同级复用：`agent`、`workflow` 可复用 `rag` 能力，`rag` 不下沉到 `agent` 子包。
- 入口简洁：对外只保留少量稳定入口，删除并行构造路径。

---

## 2. 当前实现盘点（重构输入）

## 2.1 当前基线

- `rag/` 生产文件：根目录 `21`、`loader/` `6`、`sources/` `2`。
- 当前能力覆盖：`hybrid/contextual/multi-hop/graph/web retrieval`、多向量存储后端、文档加载与分块。

## 2.2 关键并行/重复点

### A. 构建与配置桥接并行

- `NewHybridRetriever(...)`
- `NewEnhancedRetriever(...)`
- `NewRetrieverFromConfig(...)`
- `NewOpenAIRetriever/NewCohereRetriever/NewVoyageRetriever/NewJinaRetriever(...)`
- `NewVectorStoreFromConfig(...)` + 各后端 `New*Store(...)`

结论：存在“能力构造入口”与“配置桥接入口”并行，语义重复。

### B. Provider 接入路径并行

- `factory.go` 使用 `embedding/rerank` 的 `NewProviderFromConfig(...)`
- `provider_integration.go` 通过 API Key 再次组装配置并回调 `NewRetrieverFromConfig(...)`

结论：同一目标（创建可用 retriever）有多种入口，增加调用方选择成本。

### C. Tokenizer 契约并行（可接受但需显式治理）

- `rag.Tokenizer`（分块最小接口）
- `types.Tokenizer`（框架层，Message/ToolSchema 语义）
- `llm/tokenizer.Tokenizer`（LLM 层，error + 模型感知）

结论：三者语义不同，不建议强行合并；应通过 adapter 显式桥接并统一文档。

## 2.3 当前跨层耦合点（需治理）

- `rag/factory.go`、`rag/provider_integration.go` 直接依赖 `config.Config`，应收敛到 `runtime/config_bridge`。
- API 层当前可直接拼 `store + embedding`（`api/handlers/rag.go`），后续应优先走 `rag` 用例入口，减少 handler 组装逻辑。
- 架构守卫尚未显式约束 `rag` 禁止导入 `agent/workflow/api/cmd`。

---

## 3. 重构原则（强制）

- 禁止兼容代码：新旧入口不并存。
- 禁止双轨迁移：切换主链同阶段删除旧链。
- 复用优先：优先收敛到已有 `builder/factory/adapter`。
- 契约最小化：跨层共享只上收最小稳定契约到 `types`，不搬运实现细节。

---

## 4. 目标架构（重构后唯一形态）

```text
rag/
├── facade.go                     # 对外稳定入口（Builder / Retrieve）
├── core/
│   ├── contracts.go              # 最小检索契约
│   ├── document.go               # 文档与检索结果核心模型
│   ├── errors.go                 # 统一错误映射
│   └── metrics.go                # 统一观测字段定义
├── retrieval/
│   ├── pipeline.go               # 唯一检索主链
│   ├── hybrid.go
│   ├── contextual.go
│   ├── multihop.go
│   └── rerank.go
├── indexing/
│   ├── chunking.go
│   ├── transform.go
│   └── ingest.go
├── stores/
│   ├── memory.go
│   ├── qdrant.go
│   ├── weaviate.go
│   ├── milvus.go
│   └── pinecone.go
├── runtime/
│   ├── builder.go                # 唯一构建器实现
│   ├── config_bridge.go          # config -> runtime 映射
│   └── registry.go               # store/retriever 注册
├── loader/
├── sources/
└── adapters/
    └── tokenizer_adapter.go      # llm/tokenizer -> rag.Tokenizer
```

## 4.1 目标调用链（唯一）

`cmd/agentflow/main.go -> bootstrap -> server_handlers_runtime.initRAG -> rag/runtime.Builder -> rag/retrieval.pipeline -> llm capabilities(embedding/rerank)`

---

## 5. 统一接口设计（目标态）

## 5.1 构建入口统一

- 保留一个主入口：`rag/runtime.Builder.Build(...)`。
- 删除并行构建语义：API Key 快捷工厂与配置工厂合并到同一 builder 路径。

## 5.2 执行主链统一

- 主链固定：`query transform -> retrieve(topK) -> rerank(optional) -> compose context`。
- 所有检索模式都必须挂到统一主链，不允许旁路执行。

## 5.3 契约与错误统一

- 检索结果统一字段：`doc_id/content/score/source/trace`。
- 错误统一映射 `types.ErrorCode`（上游、超时、内部错误）。

## 5.4 观测统一

- 统一输出：`retrieval_latency_ms`、`rerank_latency_ms`、`topk`、`hit_count`、`context_tokens`。
- `trace_id/run_id/version/population` 必须可串联到发布看板。

---

## 6. 执行计划（单轨替换）

状态值约定（机读）：`Done` / `Partial` / `Todo`

| Phase | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| Phase-0 冻结与基线 | Todo | 冻结 + 基线测试 + 基线指标三项齐备 | `docs/rag层重构.md` |
| Phase-1 收敛构建链 | Todo | 唯一构建入口 `runtime.Builder`；并行工厂删除 | `rag/factory.go`、`rag/provider_integration.go` |
| Phase-2 收敛检索主链 | Todo | 统一 pipeline 落地；hybrid/contextual/multi-hop 挂主链 | `rag/retrieval/pipeline.go` |
| Phase-3 收敛配置桥接 | Todo | `config.Config` 依赖仅在 `runtime/config_bridge` | `rag/factory.go`、`rag/runtime/config_bridge.go` |
| Phase-4 统一 LLM 能力接入 | Todo | embedding/rerank 统一经 capability 入口 | `rag/factory.go`、`llm/capabilities/embedding/*` |
| Phase-5 契约与观测对齐 | Todo | 检索契约 + 错误码 + 评估指标 + 观测字段统一 | `types/`、`rag/core/metrics.go` |
| Phase-6 守卫与验收 | Todo | 守卫 + 全量测试 + 文档同步 | `architecture_guard_test.go`、`scripts/arch_guard.ps1` |

## 6.1 Phase-0：冻结与基线

- [ ] 冻结 `rag/` 非重构需求变更。
- [ ] 固化基线测试（检索准确性、向量后端、分块与重排）。
- [ ] 基线指标入库（延迟、召回率@K、MRR、错误率）。

## 6.2 Phase-1：收敛构建链

- [ ] 落地 `runtime.Builder` 作为唯一构建入口。
- [ ] 将 `New*Retriever` 快捷工厂改为 builder 的薄封装或删除。
- [ ] 调用点统一到单入口。

## 6.3 Phase-2：收敛检索主链

- [ ] 建立统一 retrieval pipeline：`query transform -> retrieve(topK=50-200) -> rerank(topK=5-10) -> compose context`。
- [ ] 将 `hybrid/contextual/multi-hop` 挂接为主链策略节点。
- [ ] 明确 Hybrid Search 融合算法：默认使用 RRF（Reciprocal Rank Fusion），可选加权融合 `H = (1-α)K + αV`（α 可配置，默认 0.5）。
- [ ] 删除并行执行旁路。

## 6.4 Phase-3：收敛配置桥接

- [ ] 将 `config.Config` 依赖下沉到 `runtime/config_bridge`。
- [ ] `core/retrieval/indexing` 禁止直接引用 `config`。
- [ ] 清理重复映射函数。

## 6.5 Phase-4：统一 LLM 能力接入

- [ ] embedding/rerank 统一经 capability 入口创建。
- [ ] 清理 provider_integration 与 factory 的重复路径。

## 6.6 Phase-5：契约与观测对齐

- [ ] 明确并落地 `types` 侧最小检索契约（仅跨层共享字段）。
- [ ] 错误码统一到 `types.ErrorCode`。
- [ ] 指标字段与 trace 统一。
- [ ] 落地 RAG 评估指标定义：`context_relevance`、`faithfulness`、`answer_relevancy`、`recall@K`、`MRR`。
- [ ] 在 `rag/runtime/` 下增加语义缓存层（embedding-based similarity cache），降低高频相似查询的检索成本。

## 6.7 Phase-6：守卫与验收

- [ ] 增加依赖守卫：`rag` 禁止导入 `agent/workflow/api/cmd`。
- [ ] `go test ./rag/...`、`go test ./...`、`scripts/arch_guard.ps1` 全通过。
- [ ] README/教程同步更新入口与调用示例。

---

## 7. 删除清单（必须执行）

- [ ] 并行 retriever 构造入口（重复语义）  
- [ ] 重复 config 映射与 provider 集成路径  
- [ ] 非主链检索旁路执行入口

---

## 8. 完成定义（DoD）

| DoD 条目 | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| RAG 仅存在一个构建入口 | Todo | `runtime.Builder` 为唯一入口；并行工厂删除 | `rag/runtime/builder.go` |
| RAG 仅存在一个检索主链 | Todo | 统一 pipeline 落地；topK 分层策略可配置 | `rag/retrieval/pipeline.go` |
| Hybrid 融合算法显式选择 | Todo | 默认 RRF；可选加权融合；融合策略可配置 | `rag/retrieval/hybrid.go` |
| 配置桥接仅存在一套映射实现 | Todo | `config.Config` 依赖仅在 `runtime/config_bridge` | `rag/runtime/config_bridge.go` |
| RAG 评估指标定义落地 | Todo | `context_relevance/faithfulness/answer_relevancy/recall@K/MRR` 定义完成 | `rag/core/metrics.go` |
| 语义缓存层落地 | Todo | embedding-based similarity cache 可用 | `rag/runtime/cache.go` |
| 统一错误码与统一观测字段落地 | Todo | 错误码映射 `types.ErrorCode`；观测字段统一 | `rag/core/errors.go`、`rag/core/metrics.go` |
| 架构守卫、回归测试、文档同步全部通过 | Todo | 守卫 + 全量测试 + 文档同步 | `architecture_guard_test.go`、`rag/*_test.go` |

---

## 9. 风险与控制

- 风险 1：入口收敛导致调用方改造面大。
控制：先批量替换调用点，再同阶段删除旧入口。

- 风险 2：检索主链统一影响效果波动。
控制：保留模式策略节点，但统一挂主链；对召回指标做回归对比。

- 风险 3：配置桥接迁移引发环境差异。
控制：桥接层单测覆盖所有后端配置分支。

- 风险 4：Hybrid 融合算法选择影响检索质量。
控制：RRF 作为默认（无需分数归一化，鲁棒性强）；加权融合 α 参数可配置并通过 A/B 评估调优。

- 风险 5：语义缓存命中率低导致额外开销。
控制：缓存相似度阈值可配置（建议 ≥0.95）；缓存 TTL 与索引更新频率联动；提供旁路开关。

---

## 10. Tokenizer 三套并行治理说明

当前存在三套 Tokenizer 接口，语义不同，不建议强行合并：

| 接口 | 位置 | 语义 | 适用场景 |
|---|---|---|---|
| `rag.Tokenizer` | `rag/` | 分块最小接口（`CountTokens(text) int`） | 文档分块时的 token 计数 |
| `types.Tokenizer` | `types/` | 框架层（Message/ToolSchema 语义） | 跨层 token 预算估算 |
| `llm/tokenizer.Tokenizer` | `llm/tokenizer/` | LLM 层（error + 模型感知） | 精确 token 计数与模型适配 |

治理规则：
- `rag/adapters/tokenizer_adapter.go` 负责 `llm/tokenizer -> rag.Tokenizer` 的显式桥接。
- 禁止在 `rag/` 内直接依赖 `llm/tokenizer`，必须经 adapter。
- 三者语义差异在本节文档化，后续新增 tokenizer 需求优先复用已有接口。

---

## 11. 变更日志

- [x] 2026-03-02：创建文档，完成 RAG 层重构目标、现状盘点、目标架构与阶段计划定义。
- [x] 2026-03-02：Review 补充：Phase 表改为机读状态表；Phase-2 补充 topK 分层策略与 Hybrid 融合算法（RRF/加权）；Phase-5 补充 RAG 评估指标定义与语义缓存层；DoD 改为机读判据表并补充融合算法、评估指标、语义缓存条目；新增风险 4（融合算法）与风险 5（语义缓存）；新增 Tokenizer 三套并行治理说明。
