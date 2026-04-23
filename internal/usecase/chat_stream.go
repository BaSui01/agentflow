package usecase

import "github.com/BaSui01/agentflow/types"

// ChatStreamEvent is the handler-facing streaming contract owned by usecase.
type ChatStreamEvent struct {
	Chunk *ChatStreamChunk
	Err   *types.Error
}

// ChatStreamChunk is the usecase-owned streaming DTO for chat responses.
type ChatStreamChunk struct {
	ID           string
	Provider     string
	Model        string
	Index        int
	Delta        Message
	FinishReason string
	Usage        *ChatUsage
}
