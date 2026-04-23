# Agent 模块架构重构

## 目标

对 agent 模块进行全面的架构重构，解决当前存在的构造入口混乱、职责不清晰、依赖关系混乱、重复代码和对外 API 不稳定等问题。

## 当前问题

### A. 构造入口混乱
- 存在多个并行的构造入口（Builder / Factory / 直接 New）
- 调用方不清楚应该用哪个入口
- 构造逻辑散落在多处

### B. 职责不清晰
- agent 根目录文件过多（32,641 行代码）
- 20+ 个子目录，职责重叠或不明确
- 不清楚哪些是核心能力，哪些是扩展功能

### C. 依赖关系混乱
- agent 子包之间相互依赖
- 违反了分层架构原则
- 难以独立测试和维护

### D. 重复代码
- 多个子包实现了相似的功能
- 缺少统一的抽象层
- 代码复用率低

### E. 对外 API 不稳定
- 暴露了过多内部实现细节
- 缺少清晰的公开 API 边界
- 频繁的破坏性变更

## 现状分析

- **规模**：根目录 32,641 行，20+ 子目录，360 个子目录文件
- **核心接口**：Agent, ContextManager, RetrievalProvider, ToolStateProvider
- **构造入口**：AgentBuilder（builder.go）+ 可能的其他入口

## 重构策略

**选择：全量重构（Big Bang）**

- 一次性重构整个 agent 模块
- 彻底解决所有架构问题
- 建立清晰的目标架构
- 单轨替换，不保留兼容代码

## 目标架构设计

**选择：方案 A - 按职责分层**

```
agent/
├── core/              # 核心层：Agent 接口、基础实现、唯一构造入口
│   ├── agent.go       # Agent 接口定义
│   ├── base.go        # 基础实现
│   └── builder.go     # 唯一构造入口（Builder）
│
├── capabilities/      # 能力层：可插拔的 Agent 能力
│   ├── memory/        # 记忆能力（合并 memory + memorycore）
│   ├── reasoning/     # 推理能力
│   ├── planning/      # 规划能力（合并 planner + deliberation）
│   ├── tools/         # 工具能力（合并 skills + discovery）
│   ├── guardrails/    # 护栏能力（合并 guardrails + guardcore）
│   └── streaming/     # 流式能力
│
├── execution/         # 执行层：Agent 执行引擎
│   ├── runtime/       # 运行时（合并 runtime + execution + longrunning）
│   ├── context/       # 上下文管理
│   └── protocol/      # 协议处理
│
├── collaboration/     # 协作层：多 Agent 协作
│   ├── multiagent/    # 多智能体（合并 multiagent + collaboration）
│   ├── team/          # 团队模式（合并 team + teamadapter + crews）
│   ├── hierarchical/  # 层级模式
│   └── federation/    # 联邦模式
│
├── persistence/       # 持久化层：状态管理
│   ├── checkpoint/    # 检查点（从根目录迁移）
│   ├── conversation/  # 对话历史
│   └── artifacts/     # 产物存储
│
├── integration/       # 集成层：外部集成
│   ├── deployment/    # 部署集成
│   ├── hosted/        # 托管服务
│   ├── k8s/           # K8s 集成
│   ├── lsp/           # LSP 集成
│   └── voice/         # 语音集成
│
├── observability/     # 可观测层：监控评估
│   ├── evaluation/    # 评估
│   ├── monitoring/    # 监控（从 observability 重命名）
│   └── hitl/          # 人机交互
│
└── adapters/          # 适配层：协议适配
    ├── declarative/   # 声明式配置
    ├── structured/    # 结构化输出
    └── handoff/       # 交接协议
```

**架构原则：**
1. **单一入口**：`agent.Builder` 是唯一对外构造入口
2. **清晰分层**：7 个顶层目录，每层职责明确
3. **能力可插拔**：capabilities 下的能力可独立启用/禁用
4. **依赖方向**：core → capabilities/execution → collaboration/persistence → integration/observability/adapters

## 迁移策略

**选择：新分支全量迁移**

### 执行步骤

1. **创建重构分支**
   ```bash
   git checkout -b refactor/agent-architecture
   ```

2. **创建目标目录结构**
   - 创建 7 个顶层目录（core, capabilities, execution, collaboration, persistence, integration, observability, adapters）
   - 创建各层的子目录

3. **迁移代码（按依赖顺序）**
   - Phase 1: 迁移 core 层（agent.go, base.go, builder.go）
   - Phase 2: 迁移 capabilities 层（memory, reasoning, planning, tools, guardrails, streaming）
   - Phase 3: 迁移 execution 层（runtime, context, protocol）
   - Phase 4: 迁移 collaboration 层（multiagent, team, hierarchical, federation）
   - Phase 5: 迁移 persistence 层（checkpoint, conversation, artifacts）
   - Phase 6: 迁移 integration 层（deployment, hosted, k8s, lsp, voice）
   - Phase 7: 迁移 observability 层（evaluation, monitoring, hitl）
   - Phase 8: 迁移 adapters 层（declarative, structured, handoff）

4. **更新导入路径**
   - 更新所有 import 语句
   - 更新 go.mod（如果需要）

5. **运行测试**
   - 确保所有测试通过
   - 运行架构守卫测试

6. **删除旧代码**
   - 删除旧的子目录
   - 删除根目录中已迁移的文件

7. **更新文档**
   - 更新 README
   - 更新架构文档
   - 更新 API 文档

8. **合并到主分支**
   - 创建 PR
   - Code Review
   - 合并

## 验收标准（DoD - Definition of Done）

### A. 代码质量标准

- [ ] 所有单元测试通过
  - 验证命令：`go test ./agent/... -count=1`
  - 通过标准：退出码为 0，无失败测试

- [ ] 所有集成测试通过
  - 验证命令：`go test ./agent/... -tags=integration -count=1`
  - 通过标准：退出码为 0

- [ ] 代码覆盖率不低于当前水平
  - 验证命令：`go test ./agent/... -cover`
  - 通过标准：覆盖率 >= 重构前基线

- [ ] 无 lint 错误
  - 验证命令：`golangci-lint run ./agent/...`
  - 通过标准：0 错误

- [ ] 无循环依赖
  - 验证命令：`go mod graph | grep "agent.*agent"`
  - 通过标准：0 命中

### B. 架构标准

- [ ] 架构守卫测试通过
  - 验证命令：`go test ./agent -run ArchGuard -count=1`
  - 通过标准：退出码为 0

- [ ] 依赖方向正确
  - 验证命令：手工审查 import 语句
  - 通过标准：core → capabilities → execution → collaboration/persistence → integration/observability/adapters

- [ ] 只有 `agent.Builder` 一个对外构造入口
  - 验证命令：`rg "func New.*Agent" agent/ --type go`
  - 通过标准：只有 `agent/core/builder.go` 中的 `NewBuilder`

- [ ] 根目录代码行数 < 5000 行
  - 验证命令：`wc -l agent/*.go | tail -1`
  - 通过标准：总行数 < 5000（从 32K+ 大幅减少）

- [ ] 每个子包职责单一
  - 验证命令：人工审查各子包职责
  - 通过标准：每个包只负责一个明确的职责域

### C. API 标准

- [ ] 对外 API 清晰
  - 验证命令：`rg "^type.*interface" agent/core/ --type go`
  - 通过标准：只暴露必要的公开接口

- [ ] 所有公开 API 有文档注释
  - 验证命令：`golangci-lint run --enable=godot ./agent/core/`
  - 通过标准：所有导出符号有注释

- [ ] 破坏性变更有迁移指南
  - 验证命令：检查 `docs/migration/agent-refactor.md` 是否存在
  - 通过标准：文档存在且完整

- [ ] 示例代码可运行
  - 验证命令：`go run examples/agent/basic/main.go`
  - 通过标准：示例成功运行

### D. 文档标准

- [ ] README 更新
  - 验证命令：`rg "agent/" README.md`
  - 通过标准：新的目录结构已说明

- [ ] 架构文档更新
  - 验证命令：检查 `docs/architecture/agent.md` 或 ADR
  - 通过标准：文档反映新架构

- [ ] API 文档更新
  - 验证命令：`godoc -http=:6060` 并检查 agent 包文档
  - 通过标准：文档完整且准确

- [ ] 迁移指南
  - 验证命令：检查 `docs/migration/agent-refactor.md`
  - 通过标准：包含所有破坏性变更的迁移步骤

### E. 性能标准

- [ ] 性能不低于重构前
  - 验证命令：`go test ./agent/... -bench=. -benchmem`
  - 通过标准：关键操作性能不低于基线

- [ ] 内存使用不增加
  - 验证命令：benchmark 内存分配对比
  - 通过标准：内存分配不超过基线 +10%

- [ ] 启动时间不增加
  - 验证命令：测量 Agent 初始化时间
  - 通过标准：启动时间不超过基线 +10%

## 需求确认

- [x] 确定重构的优先级和范围 → 全量重构
- [x] 确定目标架构设计 → 方案 A（按职责分层）
- [x] 确定迁移策略 → 新分支全量迁移
- [x] 确定验收标准 → A+B+C+D+E 全面标准

## 技术约束

- 遵循 CLAUDE.md 架构规则：
  - 禁止兼容代码（单轨替换）
  - agent 属于 Layer 2，可依赖 llm 和 types，不得依赖 cmd
  - 对外暴露优先走 Builder/Factory/Registry
  - 单一职责，避免 God Package

## 待讨论

- 重构的优先级？（全量重构 vs 分阶段重构）
- 是否需要保持向后兼容？（根据规则应该是单轨替换）
- 重构期间如何保证现有功能正常运行？
