# API端点兼容与Google端点完善重构

## Goal
按 `docs/重构计划/API端点兼容与Google端点完善重构计划-2026-04-24.md` 落地兼容端点与 Google/Gemini 端点完善，确保 HTTP 入站协议、provider 出站协议、测试和文档都收口到现有主链。

## Requirements
- 保持 `/v1/chat/completions` 与 `/v1/responses` 可用，并补齐回归测试。
- 新增 `/v1/messages` Anthropic-compatible 入站，复用现有 `api/routes -> handlers -> ChatService -> llm/gateway` 主链。
- 不新增 `/v1/chat/compatible`、`/v1/meassage` 或项目级 `/v1/google/*` 入站路由。
- Google/Gemini 与 Vertex AI 仅在 provider / vendor / router 层补端点守卫、测试与文档，不把 SDK 类型泄漏到 `api/`、`agent/`、`workflow/`、`cmd/`。
- handler 只做协议 DTO 转换、SSE 输出与错误格式适配，不直接执行工具，也不直接依赖 provider 细节。
- 同步更新相关 README / API 文档 / 重构计划勾选状态，并通过最小相关测试与架构守卫。

## Acceptance Criteria
- [ ] `POST /v1/chat/completions`、`POST /v1/responses`、`POST /v1/messages` 都有路由与回归测试。
- [ ] `/v1/messages` 支持最小 non-stream、stream、error 兼容形状。
- [ ] Gemini Developer API 与 Vertex AI path 的构造在 provider / vendor / router 层有测试或守卫覆盖。
- [ ] 兼容 handler 仍统一走 `ChatService` 主链，不引入双实现或旁路。
- [ ] 相关文档与计划状态已同步，定向测试与 `scripts/arch_guard.ps1` 通过。

## Technical Notes
- 这是跨层改动，优先保持边界：`api/routes -> api/handlers -> internal/usecase -> llm/*`。
- 工具执行与权限控制统一走 `AuthorizationService` / runtime 主链，不在兼容 handler 内新增执行逻辑。
- 默认只跑与当前修改直接相关的测试与校验，除非实现过程中发现需要扩大范围。
