# 全面修复项目架构和代码质量问题

## 目标

系统性修复 AgentFlow 项目中发现的所有架构缺陷、代码质量问题和规范违规，提升项目整体健壮性和可维护性。

## 需求

### 🔴 高优先级
1. 为 `openaicompat` 共享基座添加完整测试
2. 修复 `openaicompat/provider.go` Stream 方法缺失 Temperature/TopP 参数
3. 修复 `config/api.go:323` json.Encode 错误被吞
4. 解决 `anthropic/` 目录 vs `package claude` 命名不匹配

### 🟡 中优先级
5. 消除 anthropic/gemini 中重复的错误映射函数，统一使用 providers.MapHTTPError
6. 替换生产代码中的 `log.Printf` 为 `zap`（canary.go, persistence/factory.go）
7. 替换生产代码中的 `panic` 为 error 返回（container.go, patterns.go, factory.go, loader.go）
8. 修复 openai/provider.go 中的裸字符串 context key
9. 修复 gemini/provider.go 中未检查的 json.Marshal 错误
10. 修复其他未检查的错误（mcp/protocol.go, mcp/client.go, agent/builder.go 等）

### 🟢 低优先级
11. 修复 config/api.go CORS 硬编码和 API key query string 安全问题
12. 为缺失的包添加 doc.go（config/, testutil/）
13. 清理项目根目录的 config.test.exe

## 验收标准

- [ ] openaicompat 有完整的单元测试
- [ ] Stream 方法正确传递 Temperature/TopP
- [ ] 所有 json.Encode/Marshal 错误被正确处理
- [ ] 无 `log.Printf` 在非 main/examples 代码中
- [ ] 无生产代码 panic（改为 error 返回）
- [ ] 无重复的错误映射函数
- [ ] 所有 context key 使用 typed key
- [ ] CORS 和 API key 安全问题修复
- [ ] 所有显著包有 doc.go
- [ ] `go build ./...` 和 `go vet ./...` 通过

## 完成定义

- Lint / typecheck / build 通过
- 现有测试不被破坏
- 新增测试覆盖关键修复

## 范围外

- API handlers 的完整实现（那是功能开发，不是修复）
- 可观测性完整接入（tracing middleware, OTel SDK 初始化）
- retry/circuit breaker 集成到 provider 层
- 测试覆盖率全面提升（只补关键缺失）

## 技术说明

- 项目使用 Go 1.24.0
- 质量规范在 .trellis/spec/backend/quality-guidelines.md
- 目录结构规范在 .trellis/spec/backend/directory-structure.md
