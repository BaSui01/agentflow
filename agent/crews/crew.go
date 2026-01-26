// Package crews provides role-based agent teams with autonomous negotiation.
// Implements CrewAI-style role definitions and collaborative task execution.
package crews

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Role defines an agent's role in a crew.
type Role struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Goal            string   `json:"goal"`
	Backstory       string   `json:"backstory,omitempty"`
	Skills          []string `json:"skills"`
	Tools           []string `json:"tools,omitempty"`
	AllowDelegation bool     `json:"allow_delegation"`
}

// CrewMember represents an agent in a crew.
type CrewMember struct {
	ID       string         `json:"id"`
	Role     Role           `json:"role"`
	Agent    CrewAgent      `json:"-"`
	Status   MemberStatus   `json:"status"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// MemberStatus represents a crew member's status.
type MemberStatus string

const (
	MemberStatusIdle    MemberStatus = "idle"
	MemberStatusWorking MemberStatus = "working"
	MemberStatusWaiting MemberStatus = "waiting"
)

// CrewAgent interface for agents in a crew.
type CrewAgent interface {
	ID() string
	Execute(ctx context.Context, task CrewTask) (*TaskResult, error)
	Negotiate(ctx context.Context, proposal Proposal) (*NegotiationResult, error)
}

// CrewTask represents a task for the crew.
type CrewTask struct {
	ID           string         `json:"id"`
	Description  string         `json:"description"`
	Expected     string         `json:"expected_output"`
	Context      string         `json:"context,omitempty"`
	AssignedTo   string         `json:"assigned_to,omitempty"`
	Dependencies []string       `json:"dependencies,omitempty"`
	Priority     int            `json:"priority"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// TaskResult represents the result of a task.
type TaskResult struct {
	TaskID   string `json:"task_id"`
	Output   any    `json:"output"`
	Error    string `json:"error,omitempty"`
	Duration int64  `json:"duration_ms"`
}

// Proposal represents a negotiation proposal.
type Proposal struct {
	Type       ProposalType   `json:"type"`
	FromMember string         `json:"from_member"`
	ToMember   string         `json:"to_member,omitempty"`
	Task       *CrewTask      `json:"task,omitempty"`
	Message    string         `json:"message"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ProposalType defines types of proposals.
type ProposalType string

const (
	ProposalTypeDelegate ProposalType = "delegate"
	ProposalTypeAssist   ProposalType = "assist"
	ProposalTypeInform   ProposalType = "inform"
	ProposalTypeRequest  ProposalType = "request"
)

// NegotiationResult represents the result of a negotiation.
type NegotiationResult struct {
	Accepted bool      `json:"accepted"`
	Response string    `json:"response"`
	Counter  *Proposal `json:"counter_proposal,omitempty"`
}

// Crew represents a team of agents working together.
type Crew struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Members     map[string]*CrewMember `json:"members"`
	Tasks       []*CrewTask            `json:"tasks"`
	Process     ProcessType            `json:"process"`
	Verbose     bool                   `json:"verbose"`
	logger      *zap.Logger
	mu          sync.RWMutex
}

// ProcessType defines how tasks are processed.
type ProcessType string

const (
	ProcessSequential   ProcessType = "sequential"
	ProcessHierarchical ProcessType = "hierarchical"
	ProcessConsensus    ProcessType = "consensus"
)

// CrewConfig configures a crew.
type CrewConfig struct {
	Name        string
	Description string
	Process     ProcessType
	Verbose     bool
}

// NewCrew creates a new crew.
func NewCrew(config CrewConfig, logger *zap.Logger) *Crew {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Crew{
		ID:          generateCrewID(),
		Name:        config.Name,
		Description: config.Description,
		Members:     make(map[string]*CrewMember),
		Tasks:       make([]*CrewTask, 0),
		Process:     config.Process,
		Verbose:     config.Verbose,
		logger:      logger.With(zap.String("component", "crew"), zap.String("crew", config.Name)),
	}
}

// AddMember adds a member to the crew.
func (c *Crew) AddMember(agent CrewAgent, role Role) *CrewMember {
	c.mu.Lock()
	defer c.mu.Unlock()

	member := &CrewMember{
		ID:     agent.ID(),
		Role:   role,
		Agent:  agent,
		Status: MemberStatusIdle,
	}
	c.Members[member.ID] = member
	c.logger.Info("added crew member", zap.String("id", member.ID), zap.String("role", role.Name))
	return member
}

// AddTask adds a task to the crew.
func (c *Crew) AddTask(task CrewTask) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if task.ID == "" {
		task.ID = fmt.Sprintf("task_%d", len(c.Tasks)+1)
	}
	c.Tasks = append(c.Tasks, &task)
}

// Execute executes all tasks with the crew.
func (c *Crew) Execute(ctx context.Context) (*CrewResult, error) {
	c.logger.Info("starting crew execution", zap.Int("tasks", len(c.Tasks)))
	start := time.Now()

	result := &CrewResult{
		CrewID:      c.ID,
		TaskResults: make(map[string]*TaskResult),
		StartTime:   start,
	}

	switch c.Process {
	case ProcessSequential:
		if err := c.executeSequential(ctx, result); err != nil {
			return result, err
		}
	case ProcessHierarchical:
		if err := c.executeHierarchical(ctx, result); err != nil {
			return result, err
		}
	case ProcessConsensus:
		if err := c.executeConsensus(ctx, result); err != nil {
			return result, err
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(start)
	c.logger.Info("crew execution completed", zap.Duration("duration", result.Duration))
	return result, nil
}

func (c *Crew) executeSequential(ctx context.Context, result *CrewResult) error {
	for _, task := range c.Tasks {
		member := c.findBestMember(task)
		if member == nil {
			return fmt.Errorf("no member found for task: %s", task.ID)
		}

		member.Status = MemberStatusWorking
		taskResult, err := member.Agent.Execute(ctx, *task)
		member.Status = MemberStatusIdle

		if err != nil {
			taskResult = &TaskResult{TaskID: task.ID, Error: err.Error()}
		}
		result.TaskResults[task.ID] = taskResult
	}
	return nil
}

func (c *Crew) executeHierarchical(ctx context.Context, result *CrewResult) error {
	// Find manager (highest priority member)
	var manager *CrewMember
	for _, m := range c.Members {
		if manager == nil || m.Role.AllowDelegation {
			manager = m
		}
	}

	if manager == nil {
		return fmt.Errorf("no manager found")
	}

	// Manager delegates tasks
	for _, task := range c.Tasks {
		delegatee := c.findBestMember(task)
		if delegatee == nil || delegatee.ID == manager.ID {
			delegatee = manager
		}

		// Negotiate delegation
		if delegatee.ID != manager.ID {
			proposal := Proposal{
				Type:       ProposalTypeDelegate,
				FromMember: manager.ID,
				ToMember:   delegatee.ID,
				Task:       task,
				Message:    fmt.Sprintf("Please handle task: %s", task.Description),
			}
			negResult, _ := delegatee.Agent.Negotiate(ctx, proposal)
			if negResult != nil && !negResult.Accepted {
				delegatee = manager
			}
		}

		delegatee.Status = MemberStatusWorking
		taskResult, err := delegatee.Agent.Execute(ctx, *task)
		delegatee.Status = MemberStatusIdle

		if err != nil {
			taskResult = &TaskResult{TaskID: task.ID, Error: err.Error()}
		}
		result.TaskResults[task.ID] = taskResult
	}
	return nil
}

func (c *Crew) executeConsensus(ctx context.Context, result *CrewResult) error {
	for _, task := range c.Tasks {
		// All members vote on who should handle the task
		votes := make(map[string]int)
		for _, member := range c.Members {
			proposal := Proposal{
				Type:    ProposalTypeRequest,
				Task:    task,
				Message: "Who should handle this task?",
			}
			negResult, _ := member.Agent.Negotiate(ctx, proposal)
			if negResult != nil && negResult.Response != "" {
				votes[negResult.Response]++
			}
		}

		// Find winner
		var winner *CrewMember
		maxVotes := 0
		for memberID, count := range votes {
			if count > maxVotes {
				maxVotes = count
				winner = c.Members[memberID]
			}
		}

		if winner == nil {
			winner = c.findBestMember(task)
		}

		if winner != nil {
			winner.Status = MemberStatusWorking
			taskResult, err := winner.Agent.Execute(ctx, *task)
			winner.Status = MemberStatusIdle
			if err != nil {
				taskResult = &TaskResult{TaskID: task.ID, Error: err.Error()}
			}
			result.TaskResults[task.ID] = taskResult
		}
	}
	return nil
}

func (c *Crew) findBestMember(task *CrewTask) *CrewMember {
	if task.AssignedTo != "" {
		if member, ok := c.Members[task.AssignedTo]; ok {
			return member
		}
	}

	// Find by skills match
	for _, member := range c.Members {
		if member.Status == MemberStatusIdle {
			return member
		}
	}
	return nil
}

// CrewResult contains the results of crew execution.
type CrewResult struct {
	CrewID      string                 `json:"crew_id"`
	TaskResults map[string]*TaskResult `json:"task_results"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time"`
	Duration    time.Duration          `json:"duration"`
}

func generateCrewID() string {
	return fmt.Sprintf("crew_%d", time.Now().UnixNano())
}
