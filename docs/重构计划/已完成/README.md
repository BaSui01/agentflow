# 已完成的重构计划

本目录存放已完成的重构计划文档，作为项目演进历史的归档记录。

---

## 已完成计划列表

### 2026-04-21: 一次性架构边界重构计划
**文件**: [一次性架构边界重构计划-2026-04-21.md](./一次性架构边界重构计划-2026-04-21.md)

**目标**: 
- API Handler 收口为纯适配层
- 应用层统一收口到 `internal/usecase/`
- Bootstrap 唯一总装配入口 `BuildServeHandlerSet(...)`
- Agent 根包瘦身（checkpoint/pipeline/guardrails 拆分）

**完成度**: 144/144 任务 (100%)

**核心成果**:
- ✅ `api/handlers` 每个 handler 只保留单一构造入口 `NewXxxHandler(service, logger)`
- ✅ `internal/usecase` 完全脱离 `api/` DTO 依赖
- ✅ `internal/app/bootstrap.BuildServeHandlerSet(...)` 唯一总装配
- ✅ `agent/` 根包从 37 个文件瘦身到 38 个（预算 42），拆分出 4 个新文件
- ✅ 删除所有旧构造路径，单轨替换完成
- ✅ 架构守卫、回归测试、文档全部同步更新

**架构边界**:
```
cmd/agentflow
  -> internal/app/bootstrap.BuildServeHandlerSet(...)
    -> api/routes
      -> api/handlers
        -> internal/usecase
          -> domain(agent/rag/workflow/llm)
```

---

### 2026-04-18: Agent 三层运行时模型重构计划
**文件**: [Agent三层运行时模型重构计划-2026-04-18.md](./Agent三层运行时模型重构计划-2026-04-18.md)

**目标**:
- 建立 Agent 三层运行时模型（Runtime/Resolver/Builder）
- 统一 Agent 构造入口
- 解耦配置驱动与代码驱动路径

**完成度**: 48/48 任务 (100%)

**核心成果**:
- ✅ 新增 `agent/runtime` 包，建立三层运行时模型
- ✅ 统一 Agent 构造入口到 `agent/runtime.Builder`
- ✅ 配置驱动与代码驱动路径解耦
- ✅ 多 Agent 模式统一走 runtime 路径

---

## 归档规则

1. **归档时机**: 重构计划所有任务完成 (100%)，且通过所有验证（测试、架构守卫、文档同步）
2. **归档方式**: 使用 `git mv` 移动到 `已完成/` 目录，保留完整 git 历史
3. **文档要求**: 归档前确保计划文档包含完整的变更日志和完成定义 (DoD)
4. **索引维护**: 每次归档后更新本 README，记录核心成果和完成时间

---

## 进行中的重构计划

当前进行中的重构计划请查看上级目录：[../](../)

---

**最后更新**: 2026-04-21
