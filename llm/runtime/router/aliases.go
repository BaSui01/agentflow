package router

import (
	llmcore "github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Provider = llmcore.Provider
type Error = llmcore.Error
type HealthStatus = llmcore.HealthStatus

type LLMProvider = llmcore.LLMProvider
type LLMModel = llmcore.LLMModel
type LLMProviderModel = llmcore.LLMProviderModel
type LLMProviderAPIKey = llmcore.LLMProviderAPIKey

type ChatRequest = llmcore.ChatRequest
type ChatResponse = llmcore.ChatResponse
type ChatChoice = llmcore.ChatChoice
type ChatUsage = llmcore.ChatUsage
type StreamChunk = llmcore.StreamChunk
type Message = llmcore.Message
type Model = llmcore.Model
type ProviderEndpoints = llmcore.ProviderEndpoints

const (
	LLMProviderStatusActive = llmcore.LLMProviderStatusActive
	RoleAssistant           = llmcore.RoleAssistant
	RoleUser                = llmcore.RoleUser
)

type CanaryConfig = llmcore.CanaryConfig

func NewCanaryConfig(db *gorm.DB, logger *zap.Logger) *CanaryConfig {
	return llmcore.NewCanaryConfig(db, logger)
}
