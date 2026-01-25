package agent

import "errors"

var (
	// ErrProviderNotSet LLM Provider 未设置
	ErrProviderNotSet = errors.New("llm provider not set")

	// ErrAgentNotReady Agent 未就绪
	ErrAgentNotReady = errors.New("agent not ready")

	// ErrAgentBusy Agent 正在执行中
	ErrAgentBusy = errors.New("agent is busy")

	// ErrToolNotFound 工具未找到
	ErrToolNotFound = errors.New("tool not found")

	// ErrMemoryNotFound 记忆未找到
	ErrMemoryNotFound = errors.New("memory not found")

	// ErrConfigInvalid 配置无效
	ErrConfigInvalid = errors.New("invalid agent config")
)
