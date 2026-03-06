package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ObserverConfig controls observation generation behavior.
type ObserverConfig struct {
	MaxMessagesPerBatch       int           `json:"max_messages_per_batch"`
	MinMessagesForObservation int           `json:"min_messages_for_observation"`
	ObservationInterval       time.Duration `json:"observation_interval"`
}

// DefaultObserverConfig returns sensible defaults.
func DefaultObserverConfig() ObserverConfig {
	return ObserverConfig{
		MaxMessagesPerBatch:       50,
		MinMessagesForObservation: 5,
		ObservationInterval:       5 * time.Minute,
	}
}

// CompletionFunc abstracts LLM completion for the observer.
type CompletionFunc func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

// Observer compresses conversation history into dated observation logs.
type Observer struct {
	config   ObserverConfig
	complete CompletionFunc
	logger   *zap.Logger
}

// NewObserver creates an observer agent.
func NewObserver(config ObserverConfig, complete CompletionFunc, logger *zap.Logger) *Observer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Observer{config: config, complete: complete, logger: logger}
}

const observerSystemPrompt = `You are a memory observer. Your task is to compress a batch of conversation messages into a concise dated observation log entry.

Rules:
- Focus on key decisions, facts, preferences, and outcomes
- Distinguish between speculation and confirmed decisions
- Use present tense for ongoing states, past tense for completed actions
- Keep each observation to 2-4 sentences
- Include the date in YYYY-MM-DD format
- Do NOT include greetings, filler, or meta-commentary`

// Observe compresses a batch of messages into an observation.
func (o *Observer) Observe(ctx context.Context, agentID string, messages []types.Message) (*Observation, error) {
	if len(messages) < o.config.MinMessagesForObservation {
		return nil, nil
	}

	batch := messages
	if len(batch) > o.config.MaxMessagesPerBatch {
		batch = batch[len(batch)-o.config.MaxMessagesPerBatch:]
	}

	var sb strings.Builder
	for _, m := range batch {
		fmt.Fprintf(&sb, "[%s]: %s\n", m.Role, m.Content)
	}

	content, err := o.complete(ctx, observerSystemPrompt, sb.String())
	if err != nil {
		return nil, fmt.Errorf("observer completion failed: %w", err)
	}

	now := time.Now()
	obs := &Observation{
		ID:        fmt.Sprintf("obs-%s-%d", agentID, now.UnixNano()),
		AgentID:   agentID,
		Date:      now.Format("2006-01-02"),
		Content:   strings.TrimSpace(content),
		CreatedAt: now,
	}
	return obs, nil
}
