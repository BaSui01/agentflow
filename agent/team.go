package agent

import (
	"context"
	"time"
)

type TeamMember struct {
	Agent Agent
	Role  string
}

type TeamResult struct {
	Content    string
	TokensUsed int
	Cost       float64
	Duration   time.Duration
	Metadata   map[string]any
}

type TeamOption func(*TeamOptions)
type TeamOptions struct {
	MaxRounds int
	Timeout   time.Duration
	Context   map[string]any
}

func WithMaxRounds(n int) TeamOption {
	return func(o *TeamOptions) { o.MaxRounds = n }
}

func WithTeamTimeout(d time.Duration) TeamOption {
	return func(o *TeamOptions) { o.Timeout = d }
}

func WithTeamContext(ctx map[string]any) TeamOption {
	return func(o *TeamOptions) { o.Context = ctx }
}

type Team interface {
	ID() string
	Members() []TeamMember
	Execute(ctx context.Context, task string, opts ...TeamOption) (*TeamResult, error)
}
