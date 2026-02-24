// Package plugins provides AgentPlugin implementations for optional agent features.
//
// Each plugin wraps a runner interface from the agent package and integrates
// with the Pipeline via the AgentPlugin system (agent/agent_plugin.go).
package plugins

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/agent"
)

// ObservabilityPlugin wraps the entire execution with tracing and metrics.
// Phase: AroundExecute, Priority: 0 (outermost wrapper).
type ObservabilityPlugin struct {
	runner        agent.ObservabilityRunner
	recordMetrics bool
	recordTrace   bool
}

// NewObservabilityPlugin creates an observability plugin.
func NewObservabilityPlugin(runner agent.ObservabilityRunner, recordMetrics, recordTrace bool) *ObservabilityPlugin {
	return &ObservabilityPlugin{
		runner:        runner,
		recordMetrics: recordMetrics,
		recordTrace:   recordTrace,
	}
}

func (p *ObservabilityPlugin) Name() string              { return "observability" }
func (p *ObservabilityPlugin) Priority() int              { return 0 }
func (p *ObservabilityPlugin) Phase() agent.PluginPhase   { return agent.PhaseAroundExecute }
func (p *ObservabilityPlugin) Init(_ context.Context) error     { return nil }
func (p *ObservabilityPlugin) Shutdown(_ context.Context) error { return nil }

// AroundExecute starts a trace, calls next, then ends the trace and records metrics.
func (p *ObservabilityPlugin) AroundExecute(ctx context.Context, pc *agent.PipelineContext, next func(context.Context, *agent.PipelineContext) error) error {
	traceID := pc.Input.TraceID
	agentID := pc.AgentID()

	p.runner.StartTrace(traceID, agentID)

	err := next(ctx, pc)

	if err != nil {
		p.runner.EndTrace(traceID, "failed", err)
		return err
	}

	if p.recordMetrics {
		p.runner.RecordTask(agentID, true, time.Since(pc.StartTime), pc.TokensUsed, 0, 0.8)
	}
	if p.recordTrace {
		p.runner.EndTrace(traceID, "completed", nil)
	}

	return nil
}
