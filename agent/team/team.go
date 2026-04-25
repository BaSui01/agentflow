package team

import (
	"context"
	"fmt"
	"time"

	agent "github.com/BaSui01/agentflow/agent/runtime"

	"go.uber.org/zap"
)

// TeamMode defines the collaboration mode for a team.
type TeamMode string

const (
	ModeSupervisor TeamMode = "supervisor"  // Supervisor 路由分配
	ModeRoundRobin TeamMode = "round_robin" // 轮询发言
	ModeSelector   TeamMode = "selector"    // LLM 选择下一个 agent
	ModeSwarm      TeamMode = "swarm"       // 自主协作 + handoff
)

// TeamConfig holds configuration for an AgentTeam.
type TeamConfig struct {
	Mode            TeamMode
	MaxRounds       int
	Timeout         time.Duration
	EnablePlanner   bool
	SelectorPrompt  string
	TerminationFunc func(history []TurnRecord) bool
}

// TurnRecord records a single turn in the team conversation.
type TurnRecord struct {
	AgentID string
	Content string
	Round   int
}

// AgentTeam is the official multi-agent facade for AgentFlow.
type AgentTeam struct {
	id       string
	name     string
	members  []agent.TeamMember
	mode     TeamMode
	strategy teamModeStrategy
	config   TeamConfig
	logger   *zap.Logger
}

type Team interface {
	ID() string
	Members() []agent.TeamMember
	Execute(ctx context.Context, task string, opts ...agent.TeamOption) (*agent.TeamResult, error)
}

// teamModeStrategy is the internal interface for mode-specific execution logic.
type teamModeStrategy interface {
	Execute(ctx context.Context, members []agent.TeamMember, task string, config TeamConfig, opts agent.TeamOptions) (*agent.Output, error)
}

// ID returns the team's unique identifier.
func (t *AgentTeam) ID() string { return t.id }

// Members returns the team's members.
func (t *AgentTeam) Members() []agent.TeamMember { return t.members }

// Execute runs the team on the given task using the configured mode strategy.
func (t *AgentTeam) Execute(ctx context.Context, task string, opts ...agent.TeamOption) (*agent.TeamResult, error) {
	o := &agent.TeamOptions{
		MaxRounds: t.config.MaxRounds,
		Timeout:   t.config.Timeout,
	}
	for _, fn := range opts {
		fn(o)
	}

	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}

	t.logger.Info("team executing",
		zap.String("team_id", t.id),
		zap.String("mode", string(t.mode)),
		zap.Int("members", len(t.members)),
		zap.String("task_preview", truncateStr(task, 80)),
	)

	start := time.Now()
	out, err := t.strategy.Execute(ctx, t.members, task, t.config, *o)
	if err != nil {
		return nil, fmt.Errorf("team %s execution failed: %w", t.id, err)
	}

	result := &agent.TeamResult{
		Content:    out.Content,
		TokensUsed: out.TokensUsed,
		Cost:       out.Cost,
		Duration:   time.Since(start),
		Metadata:   out.Metadata,
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["team_id"] = t.id
	result.Metadata["team_mode"] = string(t.mode)

	t.logger.Info("team execution completed",
		zap.String("team_id", t.id),
		zap.Duration("duration", result.Duration),
	)
	return result, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
