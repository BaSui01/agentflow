package memory

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// Reflector refines draft observations by checking consistency and accuracy.
type Reflector struct {
	complete CompletionFunc
	logger   *zap.Logger
}

// NewReflector creates a reflector.
func NewReflector(complete CompletionFunc, logger *zap.Logger) *Reflector {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Reflector{complete: complete, logger: logger}
}

const reflectorSystemPrompt = `You are a memory reflector. Given an existing observation log and a new draft observation, produce a refined observation that:

1. Removes any contradictions with prior observations
2. Merges overlapping information
3. Corrects any speculation presented as fact
4. Maintains chronological consistency

Output only the refined observation text, nothing else.`

// Reflect refines a draft observation against existing observations.
func (r *Reflector) Reflect(ctx context.Context, existing []Observation, draft *Observation) (*Observation, error) {
	if draft == nil {
		return nil, nil
	}

	var sb strings.Builder
	sb.WriteString("## Existing Observations\n\n")
	for _, obs := range existing {
		fmt.Fprintf(&sb, "[%s] %s\n\n", obs.Date, obs.Content)
	}
	sb.WriteString("## New Draft Observation\n\n")
	fmt.Fprintf(&sb, "[%s] %s\n", draft.Date, draft.Content)

	refined, err := r.complete(ctx, reflectorSystemPrompt, sb.String())
	if err != nil {
		r.logger.Warn("reflector failed, using draft observation", zap.Error(err))
		return draft, nil
	}

	draft.Content = strings.TrimSpace(refined)
	return draft, nil
}
