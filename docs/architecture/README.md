# AgentFlow Architecture Index

This directory contains the current architecture contracts for AgentFlow. Files
under `docs/archive/` are historical background only and must not be treated as
the current public surface.

## Official Entrypoints

| Scope | Official entrypoint | Notes |
|---|---|---|
| Repository SDK | `sdk.New(opts).Build(ctx)` | Top-level construction surface. |
| Single Agent | `agent/runtime` | Runtime builder and single-agent execution. |
| Multi Agent | `agent/team` | TeamBuilder, Team modes, execution facade, SharedState. |
| Explicit workflow | `workflow/runtime` | Deterministic workflow and DAG orchestration. |
| Authorization | `internal/usecase/authorization_service.go` | Unified authorization boundary. |

## Current Architecture Contracts

| Document | Use when |
|---|---|
| [AgentFlow全面重构计划-2026-04-26.md](./AgentFlow全面重构计划-2026-04-26.md) | Planning or tracking the full-scope refactoring across all layers. |
| [Agent框架现状与收口改进计划-2026-04-25.md](./Agent框架现状与收口改进计划-2026-04-25.md) | Checking current Agent framework capability, gaps, and closure checklist. |
| [Workflow-Agent与Agentic-Agent现状建议补充-2026-04-25.md](./Workflow-Agent与Agentic-Agent现状建议补充-2026-04-25.md) | Checking Workflow Agent and Agentic Agent completion status with `[X]` / `[ ]` checklists. |
| [运行时状态模型契约-2026-04-25.md](./运行时状态模型契约-2026-04-25.md) | Changing run config, run state, stream events, checkpoint/resume, replay, approval, or audit event semantics. |
| [ADRs/004-多Agent团队抽象.md](./ADRs/004-多Agent团队抽象.md) | Changing `agent/team` public surface or multi-agent execution boundaries. |
| [启动装配链路与组合根说明.md](./启动装配链路与组合根说明.md) | Changing startup wiring, handler assembly, hot reload, or composition root code. |
| [FunctionCalling回归矩阵说明-2026-04-25.md](./FunctionCalling回归矩阵说明-2026-04-25.md) | Adding or validating provider tool/function-calling behavior. |
| [Provider工具负载映射说明.md](./Provider工具负载映射说明.md) | Changing tool payload mapping between gateway, provider, SDK, and runtime. |
| [Provider原生Token计数说明.md](./Provider原生Token计数说明.md) | Changing token counting, budget admission, or provider-native usage behavior. |
| [原生Provider与SDK边界说明.md](./原生Provider与SDK边界说明.md) | Changing OpenAI, Anthropic, Gemini, or native SDK boundaries. |
| [闭环Agent回归守卫说明.md](./闭环Agent回归守卫说明.md) | Changing the default single-agent closed-loop execution path. |

## Routing And Provider Architecture

| Document | Use when |
|---|---|
| [Channel路由扩展架构说明.md](./Channel路由扩展架构说明.md) | Designing or changing channel-based provider routing. |
| [Channel路由外部接入模板-中文版.md](./Channel路由外部接入模板-中文版.md) | Integrating AgentFlow into a Chinese external project with existing channel/key/model mapping. |
| [Channel路由外部接入模板-英文版.md](./Channel路由外部接入模板-英文版.md) | Integrating AgentFlow into an English external project. |

## Authorization Architecture

| Document | Use when |
|---|---|
| [权限控制系统重构与引入方案-2026-04-24.md](./权限控制系统重构与引入方案-2026-04-24.md) | Planning authorization, HITL approval, tool permissions, and audit hardening. |
| [权限控制系统详细设计-2026-04-24.md](./权限控制系统详细设计-2026-04-24.md) | Implementing authorization packages, interfaces, execution hooks, or migration slices. |

## Design Reference

| Document | Use when |
|---|---|
| [我的Agent框架设计参考-2026-04-23.md](./我的Agent框架设计参考-2026-04-23.md) | Comparing custom Agent framework design with external frameworks. |

## Current Guardrails

- Do not re-export internal `agent/team/internal/engines/*` types through `agent/team`.
- Do not introduce a second multi-agent execution path beside `agent/team.ExecuteAgents(...)` / `team.ModeExecutor`.
- Do not put business decisions in `api/handlers`; handlers should delegate to usecases or domain entrypoints.
- Do not treat archived plans as current contracts unless a current architecture document explicitly re-promotes them.
