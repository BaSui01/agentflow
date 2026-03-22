package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/types"

	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
)

// Tool names for planner tools.
const (
	ToolCreatePlan    = "create_plan"
	ToolUpdatePlan    = "update_plan"
	ToolGetPlanStatus = "get_plan_status"
)

// CreatePlanToolSchema returns the tool schema for creating a plan.
func CreatePlanToolSchema() types.ToolSchema {
	return types.ToolSchema{
		Name:        ToolCreatePlan,
		Description: "Create an execution plan with tasks that can be assigned to team members. Each task can have dependencies on other tasks.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"title": {
					"type": "string",
					"description": "Title of the plan"
				},
				"tasks": {
					"type": "array",
					"description": "List of tasks in the plan",
					"items": {
						"type": "object",
						"properties": {
							"id": {
								"type": "string",
								"description": "Unique task identifier"
							},
							"parent_id": {
								"type": "string",
								"description": "Parent task ID for recursive decomposition"
							},
							"title": {
								"type": "string",
								"description": "Short title of the task"
							},
							"description": {
								"type": "string",
								"description": "Detailed description of what the task should accomplish"
							},
							"assign_to": {
								"type": "string",
								"description": "Role or ID of the agent to assign this task to"
							},
							"dependencies": {
								"type": "array",
								"items": {"type": "string"},
								"description": "IDs of tasks that must complete before this task"
							},
							"priority": {
								"type": "integer",
								"description": "Task priority (higher = more important)"
							}
						},
						"required": ["id", "title", "description"]
					}
				}
			},
			"required": ["title", "tasks"]
		}`),
	}
}

// UpdatePlanToolSchema returns the tool schema for updating a plan.
func UpdatePlanToolSchema() types.ToolSchema {
	return types.ToolSchema{
		Name:        ToolUpdatePlan,
		Description: "Update task statuses in an existing plan. Use this to mark tasks as completed, failed, or skipped.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"plan_id": {
					"type": "string",
					"description": "ID of the plan to update"
				},
				"task_updates": {
					"type": "array",
					"description": "List of task status updates",
					"items": {
						"type": "object",
						"properties": {
							"task_id": {
								"type": "string",
								"description": "ID of the task to update"
							},
							"status": {
								"type": "string",
								"enum": ["pending", "blocked", "ready", "running", "completed", "failed", "skipped"],
								"description": "New status for the task"
							},
							"error": {
								"type": "string",
								"description": "Error message if the task failed"
							}
						},
						"required": ["task_id"]
					}
				}
			},
			"required": ["plan_id", "task_updates"]
		}`),
	}
}

// GetPlanStatusToolSchema returns the tool schema for querying plan status.
func GetPlanStatusToolSchema() types.ToolSchema {
	return types.ToolSchema{
		Name:        ToolGetPlanStatus,
		Description: "Get the current status of a plan and all its tasks, including completion counts and per-task details.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"plan_id": {
					"type": "string",
					"description": "ID of the plan to query"
				}
			},
			"required": ["plan_id"]
		}`),
	}
}

// GetPlannerToolSchemas returns all planner tool schemas.
func GetPlannerToolSchemas() []types.ToolSchema {
	return []types.ToolSchema{
		CreatePlanToolSchema(),
		UpdatePlanToolSchema(),
		GetPlanStatusToolSchema(),
	}
}

// PlannerToolHandler handles tool calls for planner operations.
type PlannerToolHandler struct {
	planner *TaskPlanner
}

// NewPlannerToolHandler creates a new PlannerToolHandler.
func NewPlannerToolHandler(planner *TaskPlanner) *PlannerToolHandler {
	return &PlannerToolHandler{planner: planner}
}

// CanHandle returns true if the tool name is a planner tool.
func (h *PlannerToolHandler) CanHandle(name string) bool {
	switch name {
	case ToolCreatePlan, ToolUpdatePlan, ToolGetPlanStatus:
		return true
	}
	return false
}

// Handle dispatches a tool call to the appropriate handler method.
func (h *PlannerToolHandler) Handle(ctx context.Context, call types.ToolCall) llmtools.ToolResult {
	start := time.Now()
	result := llmtools.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
	}

	var err error
	var data any

	switch call.Name {
	case ToolCreatePlan:
		data, err = h.handleCreatePlan(ctx, call.Arguments)
	case ToolUpdatePlan:
		data, err = h.handleUpdatePlan(ctx, call.Arguments)
	case ToolGetPlanStatus:
		data, err = h.handleGetPlanStatus(ctx, call.Arguments)
	default:
		err = fmt.Errorf("unknown planner tool: %s", call.Name)
	}

	result.Duration = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		return result
	}

	resultJSON, marshalErr := json.Marshal(data)
	if marshalErr != nil {
		result.Error = fmt.Sprintf("failed to marshal result: %s", marshalErr.Error())
		return result
	}
	result.Result = resultJSON
	return result
}

// createPlanResponse is the response for create_plan.
type createPlanResponse struct {
	PlanID string     `json:"plan_id"`
	Title  string     `json:"title"`
	Status PlanStatus `json:"status"`
	Tasks  int        `json:"tasks"`
}

func (h *PlannerToolHandler) handleCreatePlan(ctx context.Context, args json.RawMessage) (*createPlanResponse, error) {
	var createArgs CreatePlanArgs
	if err := json.Unmarshal(args, &createArgs); err != nil {
		return nil, fmt.Errorf("invalid create_plan arguments: %w", err)
	}

	plan, err := h.planner.CreatePlan(ctx, createArgs)
	if err != nil {
		return nil, err
	}

	return &createPlanResponse{
		PlanID: plan.ID,
		Title:  plan.Title,
		Status: plan.Status,
		Tasks:  len(plan.Tasks),
	}, nil
}

// updatePlanResponse is the response for update_plan.
type updatePlanResponse struct {
	PlanID  string     `json:"plan_id"`
	Status  PlanStatus `json:"status"`
	Updated int        `json:"updated"`
}

func (h *PlannerToolHandler) handleUpdatePlan(ctx context.Context, args json.RawMessage) (*updatePlanResponse, error) {
	var updateArgs UpdatePlanArgs
	if err := json.Unmarshal(args, &updateArgs); err != nil {
		return nil, fmt.Errorf("invalid update_plan arguments: %w", err)
	}

	if err := h.planner.UpdatePlan(ctx, updateArgs); err != nil {
		return nil, err
	}

	plan, ok := h.planner.GetPlan(updateArgs.PlanID)
	if !ok {
		return nil, fmt.Errorf("plan not found after update: %s", updateArgs.PlanID)
	}

	return &updatePlanResponse{
		PlanID:  plan.ID,
		Status:  plan.Status,
		Updated: len(updateArgs.TaskUpdates),
	}, nil
}

func (h *PlannerToolHandler) handleGetPlanStatus(ctx context.Context, args json.RawMessage) (*PlanStatusReport, error) {
	var statusArgs struct {
		PlanID string `json:"plan_id"`
	}
	if err := json.Unmarshal(args, &statusArgs); err != nil {
		return nil, fmt.Errorf("invalid get_plan_status arguments: %w", err)
	}

	return h.planner.GetPlanStatus(ctx, statusArgs.PlanID)
}
