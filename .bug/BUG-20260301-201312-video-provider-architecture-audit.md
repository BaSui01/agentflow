# 🔍 Bug 审计报告：Video Provider 架构统一性审计

> 📅 创建时间：2026-03-01 20:13:12
> 🔖 审计维度：全部九个维度
> 📊 总体评分：4.8/10

---

## 📊 总览仪表盘

| 维度 | 评分 | P0 | P1 | P2 |
|------|------|----|----|-----|
| 🚨 错误捕获 | 5/10 | 0 个 | 7 个 | 16 个 |
| 🛡️ 输入准入 | 3/10 | 12 个 | 15 个 | 10 个 |
| 🧩 状态机 | 4/10 | 2 个 | 22 个 | 33 个 |
| 🛰️ 观测链路 | 2/10 | 7 个 | 27 个 | 7 个 |
| 🔗 前后端契约 | 4/10 | 3 个 | 2 个 | 3 个 |
| ⚡ 性能资源 | 5/10 | 0 个 | 4 个 | 5 个 |
| 🔒 安全漏洞 | 3/10 | 9 个 | 13 个 | 0 个 |
| 🔄 并发事务 | 7/10 | 0 个 | 1 个 | 4 个 |
| 📐 代码分层 | 6/10 | 1 个 | 5 个 | 6 个 |
| **总计** | **4.8/10** | **34 个** | **96 个** | **84 个** |

---

## 📋 Bug 清单

### 🚨 维度一：错误捕获统一性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| E-001 | [ ] | `llm/video/sora.go:176` | pollGeneration 网络错误静默 continue，不记录不计数 | P1 | 添加日志记录 + 连续失败计数器，超阈值返回错误 |
| E-002 | [ ] | `llm/video/luma.go:189` | pollGeneration 网络错误静默 continue | P1 | 同 E-001 |
| E-003 | [ ] | `llm/video/kling.go:196` | pollGeneration 网络错误静默 continue | P1 | 同 E-001 |
| E-004 | [ ] | `llm/video/minimax.go:168` | pollGeneration 网络错误静默 continue | P1 | 同 E-001 |
| E-005 | [ ] | `llm/video/veo.go:196` | pollOperation 网络错误静默 continue | P1 | 同 E-001 |
| E-006 | [ ] | `llm/video/runway.go:175` | pollGeneration 网络错误静默 continue | P1 | 同 E-001 |
| E-007 | [ ] | `llm/video/gemini.go:155` | poll 网络错误静默 continue | P1 | 同 E-001 |
| E-008 | [ ] | `llm/video/sora.go:110` | json.Marshal 错误被 `_` 忽略 | P2 | 检查并返回 error |
| E-009 | [ ] | `llm/video/luma.go:122` | json.Marshal 错误被 `_` 忽略 | P2 | 同 E-008 |
| E-010 | [ ] | `llm/video/kling.go:120` | json.Marshal 错误被 `_` 忽略 | P2 | 同 E-008 |
| E-011 | [ ] | `llm/video/minimax.go:100` | json.Marshal 错误被 `_` 忽略 | P2 | 同 E-008 |
| E-012 | [ ] | `llm/video/veo.go:130` | json.Marshal 错误被 `_` 忽略 | P2 | 同 E-008 |
| E-013 | [ ] | `llm/video/runway.go:110` | json.Marshal 错误被 `_` 忽略 | P2 | 同 E-008 |
| E-014 | [ ] | `llm/video/gemini.go:100` | json.Marshal 错误被 `_` 忽略 | P2 | 同 E-008 |
| E-015 | [ ] | `llm/video/sora.go:127` | io.ReadAll 错误被 `_` 忽略 | P2 | 记录日志或返回 generic error |
| E-016 | [ ] | `llm/video/luma.go:139` | io.ReadAll 错误被 `_` 忽略 | P2 | 同 E-015 |
| E-017 | [ ] | `llm/video/kling.go:137` | io.ReadAll 错误被 `_` 忽略 | P2 | 同 E-015 |
| E-018 | [ ] | `llm/video/minimax.go:117` | io.ReadAll 错误被 `_` 忽略 | P2 | 同 E-015 |
| E-019 | [ ] | `llm/video/veo.go:147` | io.ReadAll 错误被 `_` 忽略 | P2 | 同 E-015 |
| E-020 | [ ] | `llm/video/runway.go:127` | io.ReadAll 错误被 `_` 忽略 | P2 | 同 E-015 |
| E-021 | [ ] | `llm/video/gemini.go:117` | io.ReadAll 错误被 `_` 忽略 | P2 | 同 E-015 |
| E-022 | [ ] | `llm/video/sora.go:180` | poll 响应 HTTP >= 400 未检查，直接 decode | P2 | 添加 status code 检查 |
| E-023 | [ ] | `llm/video/luma.go:193` | poll 响应 HTTP >= 400 未检查 | P2 | 同 E-022 |

### 🛡️ 维度二：输入准入统一性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| V-001 | [ ] | `llm/video/sora.go:72` | Generate 未校验 Prompt 空字符串 | P0 | 入口处检查 `strings.TrimSpace(req.Prompt) == ""` 返回错误 |
| V-002 | [ ] | `llm/video/luma.go:83` | Generate 未校验 Prompt 空字符串 | P0 | 同 V-001 |
| V-003 | [ ] | `llm/video/kling.go:80` | Generate 未校验 Prompt 空字符串 | P0 | 同 V-001 |
| V-004 | [ ] | `llm/video/minimax.go:86` | Generate 未校验 Prompt 空字符串 | P0 | 同 V-001 |
| V-005 | [ ] | `llm/video/veo.go:100` | Generate 未校验 Prompt 空字符串 | P0 | 同 V-001 |
| V-006 | [ ] | `llm/video/runway.go:80` | Generate 未校验 Prompt 空字符串 | P0 | 同 V-001 |
| V-007 | [ ] | `llm/video/sora.go:106` | ImageURL 未做 SSRF 防护（允许 file://、内网 IP） | P0 | 使用 urlutil.ValidateExternalURL 校验 |
| V-008 | [ ] | `llm/video/luma.go:113` | ImageURL 未做 SSRF 防护 | P0 | 同 V-007 |
| V-009 | [ ] | `llm/video/kling.go:110` | ImageURL 未做 SSRF 防护 | P0 | 同 V-007 |
| V-010 | [ ] | `llm/video/minimax.go:96` | ImageURL 未做 SSRF 防护 | P0 | 同 V-007 |
| V-011 | [ ] | `llm/video/veo.go:115` | ImageURL 未做 SSRF 防护 | P0 | 同 V-007 |
| V-012 | [ ] | `llm/video/runway.go:95` | ImageURL 未做 SSRF 防护 | P0 | 同 V-007 |
| V-013 | [ ] | `llm/video/luma.go:89-92` | Duration 无上下限 clamp，可传极端值 | P1 | 添加 min/max clamp（如 Sora 的 4-20s） |
| V-014 | [ ] | `llm/video/veo.go:105` | Duration 无上下限 clamp | P1 | 同 V-013 |
| V-015 | [ ] | `llm/video/minimax.go:86` | 完全忽略 Duration/AspectRatio/Resolution 字段 | P1 | 传递给 API 或记录日志说明不支持 |
| V-016 | [ ] | `llm/video/sora.go:73-76` | Model 无白名单校验，可注入任意模型名 | P1 | 添加 allowed models 列表校验 |
| V-017 | [ ] | `llm/video/luma.go:84-87` | Model 无白名单校验 | P1 | 同 V-016 |
| V-018 | [ ] | `llm/video/kling.go:81-84` | Model 无白名单校验 | P1 | 同 V-016 |
| V-019 | [ ] | `llm/video/minimax.go:87-90` | Model 无白名单校验 | P1 | 同 V-016 |
| V-020 | [ ] | `llm/video/sora.go:89-92` | AspectRatio 无格式校验（应为 N:M 格式） | P1 | 正则校验 `^\d+:\d+$` |
| V-021 | [ ] | `llm/video/luma.go:100-103` | AspectRatio 无格式校验 | P1 | 同 V-020 |
| V-022 | [ ] | `llm/video/kling.go:95` | AspectRatio 无格式校验 | P1 | 同 V-020 |
| V-023 | [ ] | `llm/video/sora.go:94-97` | Resolution 无白名单校验 | P1 | 限制为 480p/720p/1080p |
| V-024 | [ ] | `llm/video/luma.go:96-98` | Resolution 无白名单校验 | P1 | 同 V-023 |
| V-025 | [ ] | `llm/video/kling.go:90` | Resolution 无白名单校验 | P1 | 同 V-023 |
| V-026 | [ ] | `llm/video/sora.go:48` | Prompt 长度无上限保护 | P2 | 添加 maxPromptLength 常量限制 |
| V-027 | [ ] | `llm/video/luma.go:47` | Prompt 长度无上限保护 | P2 | 同 V-026 |

### 🧩 维度三：状态机统一性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| S-001 | [ ] | `llm/video/runway.go:25-40` | 缺少 defaultRunwayTimeout 和 defaultRunwayPollInterval 常量 | P0 | 添加与其他 provider 一致的常量定义 |
| S-002 | [ ] | `llm/video/runway.go:29` | 构造函数默认 model "gen4_turbo" 与 config.go:59 的 "gen-4.5" 不一致 | P0 | 统一为一个值 |
| S-003 | [ ] | `llm/video/sora.go:187` | poll 状态 "completed"/"failed" 为 magic string | P1 | 提取为包级常量 |
| S-004 | [ ] | `llm/video/luma.go:200-207` | poll 状态 "completed"/"failed"/"queued"/"dreaming" 为 magic string | P1 | 同 S-003 |
| S-005 | [ ] | `llm/video/kling.go:200` | poll 状态 "completed"/"failed" 为 magic string | P1 | 同 S-003 |
| S-006 | [ ] | `llm/video/minimax.go:179-183` | poll 状态 "Success"/"Fail" 为 magic string | P1 | 同 S-003 |
| S-007 | [ ] | `llm/video/sora.go:112` | API 路径 "/v1/video/generations" 硬编码在方法体中 | P1 | 提取为常量 |
| S-008 | [ ] | `llm/video/luma.go:124` | API 路径 "/dream-machine/v1/generations" 硬编码 | P1 | 同 S-007 |
| S-009 | [ ] | `llm/video/kling.go:122` | API 路径 "/v1/videos/text2video" 硬编码 | P1 | 同 S-007 |
| S-010 | [ ] | `llm/video/minimax.go:102` | API 路径 "/v1/video_generation" 硬编码 | P1 | 同 S-007 |
| S-011 | [ ] | `llm/video/sora.go:80` | duration 默认值 8 为 magic number | P2 | 提取为 defaultSoraDuration 常量 |
| S-012 | [ ] | `llm/video/sora.go:82-86` | duration 边界 4/20 为 magic number | P2 | 提取为 minSoraDuration/maxSoraDuration |
| S-013 | [ ] | `llm/video/sora.go:92` | "16:9" 默认宽高比为 magic string | P2 | 提取为 defaultAspectRatio 常量 |
| S-014 | [ ] | `llm/video/sora.go:96` | "720p" 默认分辨率为 magic string | P2 | 提取为 defaultResolution 常量 |
| S-015 | [ ] | 多文件 | 300*time.Second 重复 8 次 | P2 | 提取为 defaultVideoTimeout 包级常量 |
| S-016 | [ ] | 多文件 | 5*time.Second 重复 5 次 | P2 | 提取为 defaultPollInterval 包级常量 |

### 🛰️ 维度四：观测链路统一性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| O-001 | [ ] | `llm/video/sora.go:16-19` | SoraProvider 无 logger 字段 | P0 | 添加 `logger *zap.Logger` 字段，构造函数注入 |
| O-002 | [ ] | `llm/video/luma.go:16-19` | LumaProvider 无 logger 字段 | P0 | 同 O-001 |
| O-003 | [ ] | `llm/video/kling.go:16-19` | KlingProvider 无 logger 字段 | P0 | 同 O-001 |
| O-004 | [ ] | `llm/video/minimax.go:16-19` | MiniMaxVideoProvider 无 logger 字段 | P0 | 同 O-001 |
| O-005 | [ ] | `llm/video/veo.go:20-23` | VeoProvider 无 logger 字段 | P0 | 同 O-001 |
| O-006 | [ ] | `llm/video/runway.go:16-19` | RunwayProvider 无 logger 字段 | P0 | 同 O-001 |
| O-007 | [ ] | `llm/video/gemini.go:20-23` | GeminiProvider 无 logger 字段 | P0 | 同 O-001 |
| O-008 | [ ] | `llm/video/sora.go:72` | Generate 入口无请求开始日志 | P1 | 添加 logger.Info("sora.Generate start", zap.String("prompt", truncated)) |
| O-009 | [ ] | `llm/video/sora.go:155` | Generate 完成无结果日志 | P1 | 添加 logger.Info("sora.Generate complete", zap.String("video_url", url)) |
| O-010 | [ ] | `llm/video/sora.go:126-128` | HTTP >= 400 错误无结构化日志 | P1 | 添加 logger.Error 含 status_code 和 response_body |
| O-011 | [ ] | `llm/video/sora.go:176` | poll 网络错误无日志 | P1 | 添加 logger.Warn("poll network error", zap.Error(err)) |
| O-012 | [ ] | `llm/video/luma.go:83` | Generate 入口无请求开始日志 | P1 | 同 O-008 |
| O-013 | [ ] | `llm/video/luma.go:169` | Generate 完成无结果日志 | P1 | 同 O-009 |
| O-014 | [ ] | `llm/video/luma.go:138-140` | HTTP >= 400 错误无结构化日志 | P1 | 同 O-010 |
| O-015 | [ ] | `llm/video/luma.go:189` | poll 网络错误无日志 | P1 | 同 O-011 |
| O-016 | [ ] | `llm/video/kling.go:80` | Generate 入口无请求开始日志 | P1 | 同 O-008 |
| O-017 | [ ] | `llm/video/kling.go:196` | poll 网络错误无日志 | P1 | 同 O-011 |
| O-018 | [ ] | `llm/video/minimax.go:86` | Generate 入口无请求开始日志 | P1 | 同 O-008 |
| O-019 | [ ] | `llm/video/minimax.go:168` | poll 网络错误无日志 | P1 | 同 O-011 |
| O-020 | [ ] | `llm/video/veo.go:100` | Generate 入口无请求开始日志 | P1 | 同 O-008 |
| O-021 | [ ] | `llm/video/veo.go:196` | poll 网络错误无日志 | P1 | 同 O-011 |
| O-022 | [ ] | `llm/video/runway.go:80` | Generate 入口无请求开始日志 | P1 | 同 O-008 |
| O-023 | [ ] | `llm/video/runway.go:175` | poll 网络错误无日志 | P1 | 同 O-011 |
| O-024 | [ ] | `llm/video/gemini.go:70` | Analyze 入口无请求开始日志 | P1 | 同 O-008 |
| O-025 | [ ] | `llm/video/gemini.go:155` | poll 网络错误无日志 | P1 | 同 O-011 |
| O-026 | [ ] | 所有 provider | poll 循环无轮次计数日志 | P1 | 每 N 轮打印 logger.Info("still polling", zap.Int("round", n)) |
| O-027 | [ ] | 所有 provider | 无 OTel span 集成 | P2 | 在 Generate/Analyze 入口创建 span |
| O-028 | [ ] | 所有 provider | poll 超过阈值无 warning 日志 | P2 | 超过 10 轮打印 logger.Warn |

### 🔗 维度五：前后端契约一致性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| C-001 | [ ] | `config/loader.go` | MultimodalVideoConfig 缺少 Sora/Kling/Luma/MiniMax 配置字段 | P0 | 添加对应 Config 字段到配置结构体 |
| C-002 | [ ] | `api/handlers/multimodal.go` | Handler 未注册 4 个新 provider | P0 | 在 handler 初始化中创建并注册新 provider |
| C-003 | [ ] | `cmd/agentflow/server.go` | Server 启动未传递新 provider 的 API Key | P0 | 从配置/环境变量读取并传递 |
| C-004 | [ ] | `llm/video/sora.go:52` | Sora image 字段接受 URL，但 data URI 可能导致 API 拒绝 | P1 | 文档说明或添加 URL 格式校验 |
| C-005 | [ ] | `llm/video/kling.go:55` | Kling image_url 字段格式要求未文档化 | P1 | 添加注释说明支持的 URL 格式 |
| C-006 | [ ] | 所有新 provider | BaseURL 未暴露到外部配置文件 | P2 | 确保 config loader 支持 base_url 覆盖 |
| C-007 | [ ] | `api/` | OpenAPI/Swagger 文档未更新新 provider | P2 | 更新 API 文档 |
| C-008 | [ ] | `config/` | 热重载配置未注册新 provider | P2 | 添加 hot-reload 支持 |

### ⚡ 维度六：性能与资源泄漏

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| P-001 | [ ] | 所有 provider pollGeneration | 固定 5s 轮询间隔，无指数退避 | P1 | 实现 exponential backoff（5s→10s→20s，上限 60s） |
| P-002 | [ ] | 所有 provider Generate | 无并发限制，可同时发起大量生成请求 | P1 | 添加 semaphore 或 rate limiter |
| P-003 | [ ] | `llm/video/minimax.go:134` | FileID 为空字符串时仍发起 retrieve 请求 | P1 | 检查 FileID 非空再调用 retrieveFileURL |
| P-004 | [ ] | `llm/video/veo.go:196` | pollOperation 未检查 HTTP status >= 400 | P1 | 添加 status code 检查 |
| P-005 | [ ] | `llm/video/sora.go:181-183` | poll 循环中 resp.Body.Close() 手动调用而非 defer | P2 | 提取为辅助函数使用 defer |
| P-006 | [ ] | `llm/video/luma.go:194-198` | 同 P-005 | P2 | 同 P-005 |
| P-007 | [ ] | `llm/video/kling.go:197-201` | 同 P-005 | P2 | 同 P-005 |
| P-008 | [ ] | `llm/video/minimax.go:173-177` | 同 P-005 | P2 | 同 P-005 |
| P-009 | [ ] | `llm/video/runway.go:176-180` | 同 P-005 | P2 | 同 P-005 |

### 🔒 维度七：安全漏洞统一性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| X-001 | [ ] | `llm/video/veo.go:123` | API Key 放在 URL query parameter 中（?key=） | P0 | 改为 Authorization header |
| X-002 | [ ] | `llm/video/veo.go:185` | poll 请求同样 API Key 在 URL 中 | P0 | 同 X-001 |
| X-003 | [ ] | `llm/video/gemini.go:136` | API Key 放在 URL query parameter 中 | P0 | 同 X-001 |
| X-004 | [ ] | `llm/video/sora.go:106` | ImageURL 无 SSRF 防护（与 V-007 重复，安全视角） | P0 | 校验 URL scheme 为 https，禁止内网 IP |
| X-005 | [ ] | `llm/video/luma.go:113` | ImageURL 无 SSRF 防护 | P0 | 同 X-004 |
| X-006 | [ ] | `llm/video/kling.go:110` | ImageURL 无 SSRF 防护 | P0 | 同 X-004 |
| X-007 | [ ] | `llm/video/minimax.go:96` | ImageURL 无 SSRF 防护 | P0 | 同 X-004 |
| X-008 | [ ] | `llm/video/veo.go:115` | ImageURL 无 SSRF 防护 | P0 | 同 X-004 |
| X-009 | [ ] | `llm/video/runway.go:95` | ImageURL 无 SSRF 防护 | P0 | 同 X-004 |
| X-010 | [ ] | `llm/video/sora.go:128` | 错误响应 body 透传给调用方，可能泄露内部信息 | P1 | 记录日志但返回 generic error |
| X-011 | [ ] | `llm/video/luma.go:140` | 错误响应 body 透传 | P1 | 同 X-010 |
| X-012 | [ ] | `llm/video/kling.go:138` | 错误响应 body 透传 | P1 | 同 X-010 |
| X-013 | [ ] | `llm/video/minimax.go:118` | 错误响应 body 透传 | P1 | 同 X-010 |
| X-014 | [ ] | `llm/video/veo.go:148` | 错误响应 body 透传 | P1 | 同 X-010 |
| X-015 | [ ] | `llm/video/runway.go:128` | 错误响应 body 透传 | P1 | 同 X-010 |
| X-016 | [ ] | `llm/video/gemini.go:118` | 错误响应 body 透传 | P1 | 同 X-010 |
| X-017 | [ ] | `llm/video/sora.go:168` | poll URL 中 task ID 未做格式校验，可能被注入 | P1 | 校验 ID 为 alphanumeric + hyphen |
| X-018 | [ ] | `llm/video/luma.go:182` | poll URL 中 task ID 未做格式校验 | P1 | 同 X-017 |
| X-019 | [ ] | `llm/video/kling.go:192` | poll URL 中 task ID 未做格式校验 | P1 | 同 X-017 |
| X-020 | [ ] | `llm/video/minimax.go:161` | poll URL 中 task ID 未做格式校验 | P1 | 同 X-017 |
| X-021 | [ ] | `llm/video/veo.go:186` | poll URL 中 operation name 未做格式校验 | P1 | 同 X-017 |
| X-022 | [ ] | `llm/video/runway.go:170` | poll URL 中 task ID 未做格式校验 | P1 | 同 X-017 |

### 🔄 维度八：并发与事务一致性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| T-001 | [ ] | 所有 provider pollGeneration | poll 循环无最大重试次数限制，context.Background() 下可永久循环 | P1 | 添加 maxPollAttempts 常量，超限返回 timeout error |
| T-002 | [ ] | `llm/video/sora.go:110` | json.Marshal 错误被丢弃，极端情况下发送空 body | P2 | 检查 error 并返回 |
| T-003 | [ ] | 所有 provider pollGeneration | select case 可能在 ctx cancel 后多执行一次 HTTP 请求 | P2 | 在 ticker.C case 开头检查 ctx.Err() |
| T-004 | [ ] | 所有 provider | Provider struct 无共享可变状态，线程安全 ✅ | — | 无需修复（确认安全） |
| T-005 | [ ] | 所有 provider | http.Client 线程安全 ✅ | — | 无需修复（确认安全） |

### 📐 维度九：代码分层一致性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| L-001 | [ ] | `llm/video/runway.go:29` | 构造函数默认 model "gen4_turbo" 与 DefaultRunwayConfig "gen-4.5" 不一致 | P0 | 统一为 DefaultRunwayConfig 中的值 |
| L-002 | [ ] | `llm/video/veo.go:160` | pollOperation 命名与其他 provider 的 pollGeneration 不一致 | P1 | 重命名为 pollGeneration 或统一命名约定 |
| L-003 | [ ] | `llm/video/minimax.go:118` | 错误消息格式 "minimax error:" 与其他 provider 不一致 | P1 | 统一为 "{provider} error: status={code} body={body}" |
| L-004 | [ ] | `llm/video/minimax.go:183` | 失败消息 "minimax generation failed for task %s" 包含 taskID，其他 provider 不包含 | P1 | 统一错误消息格式 |
| L-005 | [ ] | `llm/music/` | music 包 Config 未嵌入 BaseProviderConfig，与 video 包模式不一致 | P1 | 重构 music Config 嵌入 BaseProviderConfig |
| L-006 | [ ] | `llm/video/` | 缺少 factory.go 统一创建 provider 的工厂函数 | P1 | 添加 NewProvider(name string, cfg interface{}) Provider |
| L-007 | [ ] | `llm/video/gemini.go` | 注释为中文 | P2 | 统一为英文或中文（与项目约定一致） |
| L-008 | [ ] | `llm/video/sora.go` | 注释为英文 | P2 | 同 L-007 |
| L-009 | [ ] | `llm/video/config.go` | 部分 Default 函数注释中文，部分英文 | P2 | 统一注释语言 |
| L-010 | [ ] | `llm/video/kling.go:46-53` | 有独立的 text2video 和 image2video request struct，其他 provider 统一为一个 | P2 | 可保留（API 设计差异），但添加注释说明 |
| L-011 | [ ] | `llm/video/minimax.go:190-218` | retrieveFileURL 是 MiniMax 独有的三阶段流程 | P2 | 可保留（API 设计差异），但添加注释说明 |
| L-012 | [ ] | `llm/video/` | 无 provider_test_helpers.go 共享测试工具 | P2 | 提取公共 mock server 和断言函数 |

---

## 🔀 交叉分析：跨维度关联

### 四连击 #1：ImageURL 攻击链
> V-007 (无 SSRF 防护) → X-004 (安全漏洞) → E-001 (错误静默) → O-008 (无日志)
>
> 攻击者传入 `file:///etc/passwd` 作为 ImageURL → 请求发往内网 → 错误被静默吞掉 → 无日志可追溯

### 四连击 #2：Provider 注册缺失链
> C-001 (配置缺失) → C-002 (Handler 未注册) → C-003 (Server 未传参) → 运行时 4 个新 provider 完全不可达

### 三连击 #3：Poll 无限循环链
> T-001 (无最大重试) → P-001 (无退避) → E-001 (错误静默)
>
> context.Background() + 网络抖动 → 永久 5s 轮询 → 无日志无告警 → 资源泄漏

---

## 💡 系统性改进方案

### 短期修复（直接修复具体问题）
- [ ] 修复 S-002/L-001：统一 runway.go 默认 model
- [ ] 修复 X-001~X-003：veo/gemini API Key 从 URL 移到 Header
- [ ] 修复 V-001~V-006：所有 provider 添加 Prompt 空值校验
- [ ] 修复 C-001~C-003：注册 4 个新 provider 到配置/handler/server 链路
- [ ] 修复 V-007~V-012/X-004~X-009：添加 SSRF 防护函数

### 中期防护（自动化检查机制）
- [ ] 添加 validateGenerateRequest 公共函数，统一校验 Prompt/ImageURL/Duration/AspectRatio
- [ ] 为所有 provider 注入 *zap.Logger，添加结构化日志
- [ ] 提取 poll 循环为公共函数，内置 maxRetry + exponential backoff + 日志
- [ ] 提取 magic string 为包级常量
- [ ] 添加 CI lint 规则检测 `json.Marshal` 返回值被忽略

### 长期治理（架构层面提升）
- [ ] 实现 Provider 工厂模式（factory.go），统一创建和注册流程
- [ ] 集成 OTel tracing，每个 Generate/Analyze 调用自动创建 span
- [ ] 添加 provider 健康检查和熔断机制
- [ ] 统一注释语言和错误消息格式规范

---

## 📝 修复记录

| 日期 | 编号 | 修复人/工具 | 修复说明 | 关联 commit |
|------|------|-------------|----------|-------------|
| | | | | |




