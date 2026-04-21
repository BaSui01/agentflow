# RAG 层重构执行文档（单轨替换，非兼容）

> 文档类型：可执行重构规范  
> 适用范围：`rag/` 全域（检索、索引、向量存储、文档加载、配置桥接）  
> 迁移策略：不兼容旧实现，不保留双轨

---

## 0. 执行状态总览

- [x] 完成 RAG 当前实现盘点（目录、入口、检索策略、向量后端）
- [x] 完成 RAG 分层边界确认（`rag` 保持 Layer 2，与 `agent` 同级）
- [x] 完成 RAG 单一构建入口与单一执行主链落地
- [x] 完成 RAG 配置桥接收敛（移除并行工厂语义）
- [x] 完成 RAG-LLM 调用口收敛（统一能力入口）
- [x] 完成 RAG 契约向 `types` 的最小化对齐
- [x] 完成架构守卫与回归测试

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

- `factory.go`（已删除）使用 `embedding/rerank` 的 `NewProviderFromConfig(...)`
- `provider_integration.go`（已清理快捷工厂）曾通过 API Key 再次组装配置并回调 `NewRetrieverFromConfig(...)`

结论：同一目标（创建可用 retriever）有多种入口，增加调用方选择成本。

### C. Tokenizer 契约并行（可接受但需显式治理）

- `rag.Tokenizer`（分块最小接口）
- `types.Tokenizer`（框架层，Message/ToolSchema 语义）
- `llm/tokenizer.Tokenizer`（LLM 层，error + 模型感知）

结论：三者语义不同，不建议强行合并；应通过 adapter 显式桥接并统一文档。

## 2.3 当前跨层耦合点（需治理）

- 历史上 `rag/factory.go`、`rag/provider_integration.go` 直接依赖 `config.Config`；现已收敛到 `runtime/config_bridge`。
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

`cmd/agentflow/main.go:runServe -> internal/app/bootstrap.InitializeServeRuntime -> cmd/agentflow/server_handlers_runtime.initHandlers -> bootstrap.BuildRAGHandlerRuntime -> rag/runtime.Builder -> rag/retrieval.ExecuteRetrievalPipeline -> llm capabilities(embedding/rerank)`

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

| Phase                     | 状态    | 完成判据（机读）                                       | 证据路径                                                 |
| ------------------------- | ------- | ------------------------------------------------------ | -------------------------------------------------------- |
| Phase-0 冻结与基线        | Done    | 冻结 + 基线测试 + 基线指标三项齐备                     | `docs/重构计划/rag层重构.md`                             |
| Phase-1 收敛构建链        | Done    | 唯一构建入口 `runtime.Builder`；并行工厂删除           | `rag/runtime/builder.go`、`rag/factory.go`（已删除）     |
| Phase-2 收敛检索主链      | Done    | 统一 pipeline 落地；hybrid/contextual/multi-hop 挂主链 | `rag/retrieval/pipeline.go`、`rag/retrieval/strategy_nodes.go` |
| Phase-3 收敛配置桥接      | Done    | `config.Config` 依赖仅在 `runtime/builder.go`          | `rag/runtime/builder.go`（配置映射函数已迁移）           |
| Phase-4 统一 LLM 能力接入 | Done    | embedding/rerank 统一经 capability 入口                | `rag/runtime/builder.go`、`llm/capabilities/embedding/*` |
| Phase-5 契约与观测对齐    | Done    | 检索契约 + 错误码 + 评估指标 + 观测字段统一           | `types/retrieval.go`、`rag/core/errors.go`、`rag/core/metrics.go`、`rag/core/shared_contract.go` |
| Phase-6 守卫与验收        | Done    | 守卫规则已落地；`rag` 子集回归通过；全量测试通过       | `architecture_guard_test.go`、`scripts/arch_guard.ps1`、`rag/*_test.go`   |

## 6.1 Phase-0：冻结与基线

- [x] 冻结 `rag/` 非重构需求变更。
- [x] 固化基线测试（检索准确性、向量后端、分块与重排）。
- [x] 基线指标入库（延迟、召回率@K、MRR、错误率）。

## 6.2 Phase-1：收敛构建链

- [x] 落地 `runtime.Builder` 作为唯一构建入口。
- [x] 将 `New*Retriever` 快捷工厂改为 builder 的薄封装或删除。
- [x] 调用点统一到单入口。

## 6.3 Phase-2：收敛检索主链

- [x] 建立统一 retrieval pipeline：`query transform -> retrieve(topK=50-200) -> rerank(topK=5-10) -> compose context`。
- [x] 将 `hybrid/contextual/multi-hop` 挂接为主链策略节点。
- [x] 明确 Hybrid Search 融合算法：默认使用 RRF（Reciprocal Rank Fusion），可选加权融合 `H = (1-α)K + αV`（α 可配置，默认 0.5）。
- [x] 删除并行执行旁路。

## 6.4 Phase-3：收敛配置桥接

- [x] 将 `config.Config` 依赖下沉到 `runtime/config_bridge`。
- [x] `core/retrieval/indexing` 禁止直接引用 `config`。
- [x] 清理重复映射函数。

## 6.5 Phase-4：统一 LLM 能力接入

- [x] embedding/rerank 统一经 capability 入口创建。
- [x] 清理 provider_integration 与 factory 的重复路径。

## 6.6 Phase-5：契约与观测对齐

- [x] 明确并落地 `types` 侧最小检索契约（仅跨层共享字段）。
- [x] 错误码统一到 `types.ErrorCode`。
- [x] 指标字段与 trace 统一。
- [x] 落地 RAG 评估指标定义：`context_relevance`、`faithfulness`、`answer_relevancy`、`recall@K`、`MRR`。
- [x] 在 `rag/runtime/` 下增加语义缓存层（embedding-based similarity cache），降低高频相似查询的检索成本。

## 6.7 Phase-6：守卫与验收

- [x] 增加依赖守卫：`rag` 禁止导入 `agent/workflow/api/cmd`。
- [x] `go test ./rag/...`、`go test ./...`、`scripts/arch_guard.ps1` 全通过。
- [x] README/教程同步更新入口与调用示例。

---

## 7. 删除清单（必须执行）

- [x] 并行 retriever 构造入口（重复语义）
- [x] 重复 config 映射与 provider 集成路径
- [x] 非主链检索旁路执行入口

---

## 8. 完成定义（DoD）

| DoD 条目                             | 状态 | 完成判据（机读）                                                        | 证据路径                                      |
| ------------------------------------ | ---- | ----------------------------------------------------------------------- | --------------------------------------------- |
| RAG 仅存在一个构建入口               | Done | `runtime.Builder` 为唯一入口；并行工厂删除                              | `rag/runtime/builder.go`                      |
| RAG 仅存在一个检索主链               | Done | 统一 pipeline 落地；topK 分层策略可配置；`hybrid/contextual/multi-hop` 挂接统一策略节点 | `rag/retrieval/pipeline.go`、`rag/retrieval/strategy_nodes.go`                    |
| Hybrid 融合算法显式选择              | Done | 默认 RRF；可选加权融合；融合策略可配置                                  | `rag/hybrid_retrieval.go`                     |
| 配置桥接仅存在一套映射实现           | Done | `config.Config` 依赖仅在 `runtime/config_bridge`                        | `rag/runtime/config_bridge.go`                |
| RAG 评估指标定义落地                 | Done | `context_relevance/faithfulness/answer_relevancy/recall@K/MRR` 定义完成 | `rag/core/metrics.go`                         |
| 语义缓存层落地                       | Done | embedding-based similarity cache 可用                                   | `rag/runtime/cache.go`                        |
| 统一错误码与统一观测字段落地         | Done | 错误码映射 `types.ErrorCode`；观测字段统一                              | `rag/core/errors.go`、`rag/core/metrics.go`   |
| 架构守卫、回归测试、文档同步全部通过 | Done | 守卫通过 + `rag` 回归通过 + 全量回归通过 + 文档同步完成                 | `architecture_guard_test.go`、`rag/*_test.go`、`README.md` |

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

| 接口                      | 位置             | 语义                                    | 适用场景                  |
| ------------------------- | ---------------- | --------------------------------------- | ------------------------- |
| `rag.Tokenizer`           | `rag/`           | 分块最小接口（`CountTokens(text) int`） | 文档分块时的 token 计数   |
| `types.Tokenizer`         | `types/`         | 框架层（Message/ToolSchema 语义）       | 跨层 token 预算估算       |
| `llm/tokenizer.Tokenizer` | `llm/tokenizer/` | LLM 层（error + 模型感知）              | 精确 token 计数与模型适配 |

治理规则：

- `rag/adapters/tokenizer_adapter.go` 负责 `llm/tokenizer -> rag.Tokenizer` 的显式桥接。
- 禁止在 `rag/` 内直接依赖 `llm/tokenizer`，必须经 adapter。
- 三者语义差异在本节文档化，后续新增 tokenizer 需求优先复用已有接口。

---

## 11. 变更日志

- [x] 2026-03-02：创建文档，完成 RAG 层重构目标、现状盘点、目标架构与阶段计划定义。
- [x] 2026-03-02：Review 补充：Phase 表改为机读状态表；Phase-2 补充 topK 分层策略与 Hybrid 融合算法（RRF/加权）；Phase-5 补充 RAG 评估指标定义与语义缓存层；DoD 改为机读判据表并补充融合算法、评估指标、语义缓存条目；新增风险 4（融合算法）与风险 5（语义缓存）；新增 Tokenizer 三套并行治理说明。
- [x] 2026-03-03：完成依赖守卫验收：`architecture_guard_test.go` 已包含 RAG 禁止依赖 `agent/workflow/api/cmd` 规则（`TestDependencyDirectionGuards`），并通过 `go test -run TestDependencyDirectionGuards -count=1 .` 与 `scripts/arch_guard.ps1`。
- [x] 2026-03-03：完成构建链单入口收敛：删除 `rag/factory.go` 与 `rag/factory_test.go`，清理 `provider_integration` 中 `New*Retriever` 快捷工厂；新增 `rag/runtime/config_bridge.go` 并在 `rag/runtime.Builder` 中落地 `BuildVectorStore/BuildEnhancedRetriever`，`bootstrap` 与示例调用点切到 runtime builder。
- [x] 2026-03-03：完成 runtime 语义缓存层：新增 `rag/runtime/cache.go` 与 `rag/runtime/cache_test.go`，支持阈值命中、缓存写入与清理（`Clearable`/`DocumentLister` 回退），并通过 `go test ./rag/runtime/...`、`go test ./rag/...`。
- [x] 2026-03-03：完成 `config` 依赖域收敛复核：`rag` 中 `config` 导入仅存在 `rag/runtime/*`，`core/retrieval/indexing` 侧无直接 `config` 依赖（`rg \"github.com/BaSui01/agentflow/config\" rag | rg -v \"rag/runtime|_test.go\"`）。
- [x] 2026-03-03：完成契约与观测对齐：新增 `types/retrieval.go` 定义最小跨层检索契约；新增 `rag/core/shared_contract.go` 将 `RetrievalResult/EvalMetrics` 映射到 `types` 契约；新增 `rag/core/errors_test.go` 与 `rag/core/shared_contract_test.go`；`rag/metrics.go` 补充 `span_id` 并由 `rag/metrics_test.go` 覆盖，验证通过 `go test ./types/...`、`go test ./rag/core/...`、`go test ./rag/...`。
- [x] 2026-03-03：完成文档入口同步：`README.md` / `README_EN.md` 的 RAG 入口描述由 `factory` 更新为 `rag/runtime.Builder`；`docs/cn/tutorials/07.检索增强RAG.md` 与 `docs/en/tutorials/07.RAG.md` 新增“生产装配入口（runtime.Builder）”示例。
- [x] 2026-03-03：落地统一检索主链执行器：新增 `rag/retrieval/pipeline.go` 与 `rag/retrieval/pipeline_test.go`，实现 `transform -> retrieve -> rerank -> compose` 可注入管线与 topK 分层截断策略；通过 `go test ./rag/retrieval/...` 与 `go test ./rag/...`。
- [x] 2026-03-03：完成主链策略节点挂接：在 `rag/retrieval/strategy_nodes.go` 与 `rag/retrieval/strategy_node_test.go` 中将 `hybrid/contextual/multi-hop` 收敛为统一 `Retriever` 策略节点（`NewStrategyNode` 工厂）；通过 `go test ./rag/retrieval/...` 与 `go test ./rag/...`。
- [x] 2026-03-03：完成 Hybrid 融合算法显式化：`rag/hybrid_retrieval.go` 新增融合算法常量 `FusionRRF/FusionWeighted` 与配置归一化（非法算法回退 RRF、非法 alpha 回退 0.5、非法 `rrf_k` 回退 60）；`rag/hybrid_fusion_test.go` 新增归一化用例，验证默认 RRF 与加权融合行为。
- [x] 2026-03-03：完成旁路入口收敛：删除 `EnhancedRetriever.RetrieveWithProviders`，统一对外为 `ExecuteRetrievalPipeline`（embed query -> retrieve -> rerank）；示例 `examples/20_multimodal_providers` 已切换新入口；新增 `rag/provider_integration_test.go` 覆盖外部 rerank 与 embedding 失败降级路径。
- [x] 2026-03-03：完成检索主链收敛验收复核：`rag/retrieval/` 下仅保留 `pipeline.go` 与 `strategy_nodes.go` 两类主链实现（无并行旧文件）；`go test ./rag/...` 通过；`go test ./workflow/... ./api/handlers/... ./internal/app/bootstrap ./cmd/agentflow` 通过；`scripts/arch_guard.ps1` 通过（存在历史 warning，不阻断 gate）。
- [x] 2026-03-03：完成全量回归验收闭环：`go test ./llm/providers/doubao/...`、`go test ./llm/providers/glm/...`、`go test ./...` 与 `scripts/arch_guard.ps1` 全通过；总览“架构守卫与回归测试”与 6.7 验收项更新为完成。
- [x] 2026-03-03：完成冻结与基线闭环：新增 `docs/重构计划/evidence/rag_freeze_notice-2026-03-03.md` 冻结声明；新增 `rag/baseline_metrics_test.go` 输出 baseline 指标；新增 `scripts/rag_baseline_capture.py` 采集并落盘 `docs/重构计划/evidence/rag_baseline_metrics_latest.json`，覆盖 latency/recall@K/MRR/error_rate。
- [x] 2026-03-03：完成根包超预算瘦身（22→19）：同职责文件合并并删除旧文件，`vector_convert.go -> vector_store.go`、`tokenizer_adapter.go -> chunking.go`、`metrics.go -> hybrid_retrieval.go`；保持单实现与主链不变；通过 `go test ./rag/...` 与 `scripts/arch_guard.ps1`，`rag` 根包生产文件数降至 `19`（阈值 `<=20`）。
