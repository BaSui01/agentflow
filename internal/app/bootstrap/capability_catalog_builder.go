package bootstrap

import (
	"sort"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/integration/hosted"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/team"
)

// CapabilityCatalog is a read-only runtime view of the currently wired capabilities.
type CapabilityCatalog struct {
	GeneratedAt time.Time             `json:"generated_at"`
	AgentTypes  []CapabilityAgentType `json:"agent_types"`
	Tools       []CapabilityTool      `json:"tools"`
	Modes       []CapabilityMode      `json:"modes"`
}

type CapabilityAgentType struct {
	Name string `json:"name"`
}

type CapabilityTool struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type CapabilityMode struct {
	Name string `json:"name"`
}

// BuildCapabilityCatalog collects the runtime-exposed agent types, hosted tools, and execution modes.
func BuildCapabilityCatalog(
	toolRegistry *hosted.ToolRegistry,
	agentRegistry *agent.AgentRegistry,
	modeRegistry *team.ModeRegistry,
) *CapabilityCatalog {
	catalog := &CapabilityCatalog{
		GeneratedAt: time.Now(),
		AgentTypes:  []CapabilityAgentType{},
		Tools:       []CapabilityTool{},
		Modes:       []CapabilityMode{},
	}

	if agentRegistry != nil {
		for _, agentType := range agentRegistry.ListTypes() {
			name := strings.TrimSpace(string(agentType))
			if name == "" {
				continue
			}
			catalog.AgentTypes = append(catalog.AgentTypes, CapabilityAgentType{Name: name})
		}
		sort.Slice(catalog.AgentTypes, func(i, j int) bool {
			return catalog.AgentTypes[i].Name < catalog.AgentTypes[j].Name
		})
	}

	if toolRegistry != nil {
		for _, tool := range toolRegistry.List() {
			if tool == nil {
				continue
			}
			name := strings.TrimSpace(tool.Name())
			if name == "" {
				continue
			}
			catalog.Tools = append(catalog.Tools, CapabilityTool{
				Name:        name,
				Type:        string(tool.Type()),
				Description: strings.TrimSpace(tool.Description()),
			})
		}
		sort.Slice(catalog.Tools, func(i, j int) bool {
			return catalog.Tools[i].Name < catalog.Tools[j].Name
		})
	}

	if modeRegistry != nil {
		for _, mode := range modeRegistry.List() {
			name := strings.TrimSpace(mode)
			if name == "" {
				continue
			}
			catalog.Modes = append(catalog.Modes, CapabilityMode{Name: name})
		}
		sort.Slice(catalog.Modes, func(i, j int) bool {
			return catalog.Modes[i].Name < catalog.Modes[j].Name
		})
	}

	return catalog
}
