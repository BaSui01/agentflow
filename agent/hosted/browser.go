package hosted

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/browser"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// BrowserAutomationTool exposes browser automation as a hosted tool.
type BrowserAutomationTool struct {
	tool   *browser.BrowserTool
	logger *zap.Logger
}

// NewBrowserAutomationTool creates a new browser automation hosted tool.
func NewBrowserAutomationTool(tool *browser.BrowserTool, logger *zap.Logger) *BrowserAutomationTool {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BrowserAutomationTool{
		tool:   tool,
		logger: logger.With(zap.String("tool", "browser_automation")),
	}
}

func (t *BrowserAutomationTool) Type() HostedToolType { return ToolTypeBrowser }
func (t *BrowserAutomationTool) Name() string         { return "browser_automation" }
func (t *BrowserAutomationTool) Description() string {
	return "Automate browser navigation, interaction, extraction and screenshots"
}

func (t *BrowserAutomationTool) Schema() types.ToolSchema {
	params, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session_id": map[string]any{
				"type":        "string",
				"description": "Browser session identifier",
			},
			"action": map[string]any{
				"type": "string",
				"enum": []string{
					"navigate", "click", "type", "screenshot", "extract", "wait", "state",
					"history", "close_session", "close_all",
				},
				"description": "Browser action to execute",
			},
			"selector": map[string]any{
				"type":        "string",
				"description": "CSS selector for click/type/extract/wait",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "Action payload (URL/text)",
			},
			"timeout_ms": map[string]any{
				"type":        "integer",
				"description": "Wait timeout in milliseconds",
			},
			"options": map[string]any{
				"type":        "object",
				"description": "Additional action options",
			},
		},
		"required": []string{"session_id", "action"},
	})
	if err != nil {
		params = []byte("{}")
	}
	return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: params}
}

type browserArgs struct {
	SessionID string            `json:"session_id"`
	Action    string            `json:"action"`
	Selector  string            `json:"selector,omitempty"`
	Value     string            `json:"value,omitempty"`
	TimeoutMS int               `json:"timeout_ms,omitempty"`
	Options   map[string]string `json:"options,omitempty"`
}

func (t *BrowserAutomationTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if t.tool == nil {
		return nil, fmt.Errorf("browser tool is not configured")
	}

	var req browserArgs
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.Action == "" {
		return nil, fmt.Errorf("action is required")
	}

	switch req.Action {
	case "close_session":
		if err := t.tool.CloseSession(req.SessionID); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"ok": true})
	case "close_all":
		if err := t.tool.CloseAll(); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"ok": true})
	}

	session, err := t.tool.GetOrCreateSession(req.SessionID)
	if err != nil {
		return nil, err
	}

	var result *browser.BrowserResult
	switch req.Action {
	case "navigate":
		result, err = session.Navigate(ctx, req.Value)
	case "click":
		result, err = session.Click(ctx, req.Selector)
	case "type":
		result, err = session.Type(ctx, req.Selector, req.Value)
	case "screenshot":
		result, err = session.Screenshot(ctx)
	case "extract":
		result, err = session.Extract(ctx, req.Selector)
	case "wait":
		timeout := 5 * time.Second
		if req.TimeoutMS > 0 {
			timeout = time.Duration(req.TimeoutMS) * time.Millisecond
		}
		result, err = session.Wait(ctx, req.Selector, timeout)
	case "state":
		state, stateErr := session.GetState(ctx)
		if stateErr != nil {
			return nil, stateErr
		}
		return json.Marshal(state)
	case "history":
		return json.Marshal(session.GetHistory())
	default:
		result, err = t.tool.ExecuteCommand(ctx, req.SessionID, browser.BrowserCommand{
			Action:   browser.Action(req.Action),
			Selector: req.Selector,
			Value:    req.Value,
			Options:  req.Options,
		})
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
