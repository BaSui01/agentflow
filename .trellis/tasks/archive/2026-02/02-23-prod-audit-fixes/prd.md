# 生产就绪度审计修复

## 目标

并行修复生产就绪度审计中发现的所有 P1/P2 问题。

## 已修复（本次任务之前）

- [x] P0: `api/handlers/chat.go` 类型转换字段遗漏 (Images, Metadata, Timestamp)
- [x] P1: `cmd/agentflow/server.go` Shutdown() 无全局超时
- [x] P1: `internal/cache/manager.go` ErrCacheMiss 用 fmt.Errorf 定义 + IsCacheMiss 用 == 比较
- [x] P1: `.github/workflows/ci.yml` 测试缺少 -race flag

## 待修复任务

### Task A: APIKeyHandler 引入 service 接口层 (P1)

**问题**: `api/handlers/apikey.go` 的 `APIKeyHandler` 直接持有 `*gorm.DB`，在 handler 中执行 SQL 查询，违反分层原则。

**修复方案**:
1. 在 `api/handlers/apikey.go` 中定义 `APIKeyStore` 接口（包含 handler 需要的所有 DB 操作）
2. 创建 `api/handlers/apikey_store.go` 实现该接口（包装 gorm.DB）
3. 修改 `APIKeyHandler` 依赖接口而非 `*gorm.DB`
4. 更新 `cmd/agentflow/server.go` 中的 handler 创建代码
5. 确保所有测试通过

**验收标准**:
- `APIKeyHandler` 不再直接 import gorm
- 所有现有测试通过
- `go build ./...` 通过

### Task B: config 包解耦 + Content-Type 验证 (P1)

**问题**:
1. `config/api.go` import 了 `api` 包（反向依赖，config 是下层不应依赖上层 api）
2. `config/api.go` 的 POST/PUT handler 缺少 Content-Type 验证

**修复方案**:
1. 将 `config/api.go` 中使用的 `api.Response` / `api.ErrorInfo` 替换为 config 包内部的本地类型（已有 `apiResponse` / `apiError`）
2. 移除 `config/api.go` 对 `api` 包的 import
3. 为 `updateConfig` 和其他接受 JSON body 的 handler 添加 Content-Type 验证
4. 确保所有测试通过

**验收标准**:
- `config/api.go` 不再 import `github.com/BaSui01/agentflow/api`
- POST/PUT handler 有 Content-Type 验证
- 所有现有测试通过

### Task C: internal/tlsutil 提升为公开包 (P2)

**问题**: `internal/tlsutil` 被 agent/*, llm/*, rag/* 等 30+ 个业务包依赖，违反 internal 包的语义（仅限模块内部使用）。

**修复方案**:
1. 将 `internal/tlsutil/` 目录移动到 `pkg/tlsutil/`
2. 更新所有 import 路径：`github.com/BaSui01/agentflow/internal/tlsutil` → `github.com/BaSui01/agentflow/pkg/tlsutil`
3. 确保编译通过

**验收标准**:
- `internal/tlsutil/` 不再存在
- 所有 import 指向 `pkg/tlsutil`
- `go build ./...` 通过

### Task D: cmd 业务逻辑下沉 (P2)

**问题**: `cmd/agentflow/server.go` 的 `buildAgentResolver` 包含 agent 创建、初始化、缓存等业务逻辑，不应在 cmd 层。

**修复方案**:
1. 在 `agent/` 包中创建 `resolver.go`，定义 `CachingResolver` 结构体
2. 将 `buildAgentResolver` 的逻辑移入 `agent.NewCachingResolver`
3. `cmd/agentflow/server.go` 调用 `agent.NewCachingResolver` 获取 resolver
4. 确保所有测试通过

**验收标准**:
- `cmd/agentflow/server.go` 不再包含 agent 创建/缓存逻辑
- `go build ./...` 通过
- 所有现有测试通过

## 技术约束

- 遵循 `.trellis/spec/backend/quality-guidelines.md` 中的编码规范
- 遵循 `.trellis/spec/backend/error-handling.md` 中的错误处理模式
- 使用 zap logger，不使用 log 包
- 所有 sentinel error 使用 `errors.New`
- 不引入新的外部依赖
