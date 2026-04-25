# Refactor agent module

## Goal

继续推进 `agent/` 模块 Phase-6 收尾重构，把仍滞留在 root 的执行与能力集成职责下沉到现有 8 层目录，最终删除 `agent/integration.go`，并保持正式入口、调用链和架构守卫口径不变。

## What I already know

- 用户要求继续重构 `agent` 模块，并明确接受并行分析。
- 用户额外约束：不要继续在 `agent/` 根层堆文件；要把功能一次性下沉合并到现有职责目录。
- 仓库已存在正式重构计划：`docs/重构计划/agent模块架构重构-2026-04-22.md`。
- 当前 `agent/` 根层生产文件为 6 个：`base.go`、`builder.go`、`integration.go`、`interfaces.go`、`registry.go`、`request.go`。
- 当前 root 文件仍然过厚：`integration.go`、`interfaces.go`、`builder.go` 都较大，`integration.go` 是计划中明确要删除的 root 文件。
- 现有计划已明确目标：root 最终只保留 5 个文件，正式 runtime 入口继续为 `agent/runtime.Builder`。

## Assumptions

- 本轮优先完成 `integration.go` 相关职责继续下沉，而不是重新设计整套 `agent` 架构。
- 不引入兼容分支、双实现或临时平级 root 文件。
- 对外公开语义允许通过 root 薄门面转发，但真实实现必须落在现有 8 层子目录中。

## Requirements

- 继续按现有 Phase-6 计划推进，不另起炉灶。
- 优先处理 `agent/integration.go` 中尚未下沉的职责簇。
- 实现必须落到现有目录：`adapters/`、`capabilities/`、`collaboration/`、`core/`、`execution/`、`integration/`、`observability/`、`persistence/`。
- 不新增 `agent/` 根层生产文件。
- 保持正式入口不变：
  - 仓库级入口：`sdk.New(opts).Build(ctx)`
  - `agent` 子模块 runtime 入口：`agent/runtime.Builder`
- 必须同步维护相关守卫、测试和文档口径。

## Acceptance Criteria

- [ ] `agent/integration.go` 中剩余的 `BaseAgent` 执行流与能力集成职责继续下沉到现有子目录。
- [ ] `agent` 根层生产文件不增加，且以删除 `agent/integration.go` 为目标继续推进。
- [ ] `go test ./agent/... -count=1` 通过。
- [ ] `go build ./...` 通过。
- [ ] `powershell.exe -ExecutionPolicy Bypass -File scripts/arch_guard.ps1` 通过或至少本轮新增修改不引入新的守卫失败。
- [ ] 相关计划/迁移文档在需要时同步更新，反映当前实际收口状态。

## Definition of Done

- 代码改动完成并通过最小必要验证。
- root 下沉方向与现有计划一致，没有新增架构回潮。
- 未引入兼容逻辑、双轨实现或绕过现有调用链的入口。

## Out of Scope

- 不处理仓库现存的全量 lint 历史问题。
- 不重写 `agent` 模块整体 API 设计。
- 不变更正式 runtime 入口或仓库级入口。

## Technical Approach

第一步聚焦 `agent/integration.go`：

- 把 `EnableXxx / ExecuteEnhanced` 继续归并到 `agent/integration/features.go` 或相邻实现文件。
- 把 `Execute / executeCore / Plan / ChatCompletion / StreamCompletion` 及其执行流辅助逻辑下沉到 `agent/execution/`。
- 复用已存在的 `agent/execution/loop/`、`agent/observability/events/`、`agent/execution/context/`、`agent/integration/features.go`，避免重复抽象。
- root 仅保留必要 public surface 或薄转发，真实实现必须在子目录。

## Decision (ADR-lite)

**Context**: `agent` 模块已经完成大规模收口，但 root 仍残留 6 个生产文件，且 `integration.go` 仍持有执行主流程和能力集成逻辑。  
**Decision**: 继续沿用既有 Phase-6 计划，优先删除 `integration.go`，通过把剩余职责分批下沉到既有 8 层目录完成收尾。  
**Consequences**: 改动会触及 `agent` 根层 public surface、测试守卫、README/迁移文档与若干调用方，需要持续做回归验证和文档同步。

## Technical Notes

- 现有计划文件：`docs/重构计划/agent模块架构重构-2026-04-22.md`
- 迁移指南：`docs/migration/agent-refactor-2026-04-22.md`
- 关键守卫：
  - `agent/architecture_refactor_test.go`
  - `architecture_guard_test.go`
  - `scripts/arch_guard.ps1`
- 当前目标目录中已存在可复用实现：
  - `agent/integration/features.go`
  - `agent/execution/loop/completion.go`
  - `agent/execution/context/input_context.go`
  - `agent/runtime/`
