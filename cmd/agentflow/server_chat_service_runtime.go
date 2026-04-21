package main

import (
	"time"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
)

const defaultChatServiceTimeout = 30 * time.Second

func (s *Server) buildChatService(
	provider llm.Provider,
	policyManager *llmpolicy.Manager,
	ledger observability.Ledger,
) usecase.ChatService {
	var runtime usecase.ChatRuntime
	if provider != nil {
		gateway := llmgateway.New(llmgateway.Config{
			ChatProvider:  provider,
			PolicyManager: policyManager,
			Ledger:        ledger,
			Logger:        s.logger,
		})
		runtime = usecase.ChatRuntime{
			Gateway:      gateway,
			ChatProvider: llmgateway.NewChatProviderAdapter(gateway, provider),
			ToolManager:  s.currentChatToolManager(),
		}
	}

	if existing, ok := s.chatService.(*usecase.DefaultChatService); ok {
		existing.UpdateRuntime(runtime)
		return existing
	}
	if provider == nil {
		return nil
	}

	converter := handlers.NewUsecaseChatConverter(handlers.NewDefaultChatConverter(defaultChatServiceTimeout))
	return usecase.NewDefaultChatService(
		runtime,
		converter,
		s.logger,
	)
}
