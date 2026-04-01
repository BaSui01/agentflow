
## Project Rules

### 1) 开发阶段：禁止兼容代码（强制）

- **禁止编写兼容代码**：代码修改时不允许为兼容旧逻辑保留分支、兜底或双实现。
- **只保留单一实现**：必须删除被替代的旧实现，只保留修改后唯一且最正确的实现。
- **禁止双轨迁移**：不允许“新老逻辑并存一段时间再删”的方案，除非明确有迁移任务文档并单独批准。

### 2) 架构分层与依赖方向（强制）

- **Layer 0 `types/`**：零依赖核心类型层，只允许被依赖，不反向依赖业务层与适配层。
- **Layer 1 `llm/`**：Provider 抽象与实现层，不得依赖 `agent/`、`workflow/`、`api/`、`cmd/`。
- **Layer 2 `agent/` + `rag/`**：核心能力层，可依赖 `llm/` 与 `types/`，不得依赖 `cmd/`。
- **Layer 3 `workflow/`**：编排层，可依赖 `agent/`、`rag/`、`llm/`、`types/`。
- **适配层 `api/`**：仅做协议转换与入站/出站适配，不承载核心业务决策。
- **组合根 `cmd/`**：只做启动装配、生命周期管理、配置注入；不下沉业务实现。
- **基础设施层 `pkg/`**：不得反向依赖 `api/` 与 `cmd/`。

### 3) 项目链路执行规则（强制）

- 服务启动链路必须保持单入口：`cmd/agentflow/main.go -> internal/app/bootstrap -> cmd/agentflow/server_* -> api/routes -> api/handlers -> domain(agent/rag/workflow/llm)`。
- 新功能必须挂载到现有链路节点，不允许绕过入口链路直接跨层调用。
- Handler 层只能调用用例/领域能力，不得在 Handler 中拼装底层基础设施细节。
- 领域能力对外暴露优先走 `Builder` / `Factory` / `Registry`，避免散落式构造逻辑。

### 4) 代码复用与简洁调用（强制）

- **复用优先**：新增能力前先复用现有 `builder/factory/adapter`，禁止重复造轮子。
- **API 简洁**：对外入口优先保持少量稳定入口（例如顶层便捷构造和 runtime 构造），避免新增并行入口。
- **单一职责**：文件和包职责必须清晰，避免“God Object / God Package”。
- **命名可检索**：模块命名与目录结构要直观表达职责，便于快速定位与调用。

### 5) 变更与校验（强制）

- 所有架构相关改动必须同步更新对应文档（README/ADR/架构说明）中的目录与链路描述。
- 提交前必须通过架构守卫（如 `architecture_guard_test.go`、`scripts/arch_guard.ps1`）对应规则。
- 如果确需突破架构规则，必须先提交 ADR 或架构变更说明，再实施代码改动。

### 6) 测试与质量建议

- **Goroutine 泄漏检测**：建议在关键包（如 `agent/`）的 `TestMain` 中集成 `go.uber.org/goleak` 的 `VerifyTestMain`，以检测测试后的 goroutine 泄漏。若现有测试存在 background goroutines 导致大量误报，可先用 `goleak.IgnoreTopFunction` 忽略已知安全 goroutine，或暂不启用，待测试稳定性提升后再接入。

### 7) 外部参考目录（强制）

- **`CC-Source/` 与 `docs/claude-code/` 仅作外部参考学习资料**：用于借鉴设计与实现思路，不属于当前项目正式实现。
- **默认排除主项目语境**：做当前项目设计、开发、评审、文档同步、架构守卫判断时，默认排除上述目录；仅在明确要求参考外部实现时再读取或引用。
