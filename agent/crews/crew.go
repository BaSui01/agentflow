package crews

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 角色定义了代理人在船员中的角色.
type Role struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Goal            string   `json:"goal"`
	Backstory       string   `json:"backstory,omitempty"`
	Skills          []string `json:"skills"`
	Tools           []string `json:"tools,omitempty"`
	AllowDelegation bool     `json:"allow_delegation"`
}

// 船员代表一个船员的特工
type CrewMember struct {
	ID       string         `json:"id"`
	Role     Role           `json:"role"`
	Agent    CrewAgent      `json:"-"`
	Status   MemberStatus   `json:"status"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// 成员地位代表船员地位。
type MemberStatus string

const (
	MemberStatusIdle    MemberStatus = "idle"
	MemberStatusWorking MemberStatus = "working"
	MemberStatusWaiting MemberStatus = "waiting"
)

// 机组特工的机组接口
type CrewAgent interface {
	ID() string
	Execute(ctx context.Context, task CrewTask) (*TaskResult, error)
	Negotiate(ctx context.Context, proposal Proposal) (*NegotiationResult, error)
}

// 船员任务代表着船员的任务.
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

// TaskResult代表着一项任务的结果.
type TaskResult struct {
	TaskID   string `json:"task_id"`
	Output   any    `json:"output"`
	Error    string `json:"error,omitempty"`
	Duration int64  `json:"duration_ms"`
}

// 提案是谈判提案。
type Proposal struct {
	Type       ProposalType   `json:"type"`
	FromMember string         `json:"from_member"`
	ToMember   string         `json:"to_member,omitempty"`
	Task       *CrewTask      `json:"task,omitempty"`
	Message    string         `json:"message"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// 提案Type定义了提案的类型.
type ProposalType string

const (
	ProposalTypeDelegate ProposalType = "delegate"
	ProposalTypeAssist   ProposalType = "assist"
	ProposalTypeInform   ProposalType = "inform"
	ProposalTypeRequest  ProposalType = "request"
)

// 谈判成果是谈判的结果。
type NegotiationResult struct {
	Accepted bool      `json:"accepted"`
	Response string    `json:"response"`
	Counter  *Proposal `json:"counter_proposal,omitempty"`
}

// 船员代表一组特工一起工作.
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

// ProcessType定义任务处理方式.
type ProcessType string

const (
	ProcessSequential   ProcessType = "sequential"
	ProcessHierarchical ProcessType = "hierarchical"
	ProcessConsensus    ProcessType = "consensus"
)

// CrewConfig配置一个船员.
type CrewConfig struct {
	Name        string
	Description string
	Process     ProcessType
	Verbose     bool
}

// NewCrew创建了新的团队.
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

// 添加"成员"为机组增加一名成员.
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

// 添加任务给船员添加了任务.
func (c *Crew) AddTask(task CrewTask) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if task.ID == "" {
		task.ID = fmt.Sprintf("task_%d", len(c.Tasks)+1)
	}
	c.Tasks = append(c.Tasks, &task)
}

// 执行与船员一起执行所有任务.
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
	// 查找管理器( 最优先的成员)
	var manager *CrewMember
	for _, m := range c.Members {
		if manager == nil || m.Role.AllowDelegation {
			manager = m
		}
	}

	if manager == nil {
		return fmt.Errorf("no manager found")
	}

	// 经理代表的任务
	for _, task := range c.Tasks {
		delegatee := c.findBestMember(task)
		if delegatee == nil || delegatee.ID == manager.ID {
			delegatee = manager
		}

		// 谈判代表团
		if delegatee.ID != manager.ID {
			proposal := Proposal{
				Type:       ProposalTypeDelegate,
				FromMember: manager.ID,
				ToMember:   delegatee.ID,
				Task:       task,
				Message:    fmt.Sprintf("Please handle task: %s", task.Description),
			}
			// BUG-5 FIX: 正确处理 negotiate 错误，记录日志并回退到 manager 执行
			negResult, negErr := delegatee.Agent.Negotiate(ctx, proposal)
			if negErr != nil {
				c.logger.Warn("negotiation failed, falling back to manager",
					zap.String("delegatee", delegatee.ID),
					zap.String("task", task.ID),
					zap.Error(negErr))
				delegatee = manager
			} else if negResult != nil && !negResult.Accepted {
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
		// 所有成员投票表决谁应承担这项任务
		votes := make(map[string]int)
		for _, member := range c.Members {
			proposal := Proposal{
				Type:    ProposalTypeRequest,
				Task:    task,
				Message: "Who should handle this task?",
			}
			// BUG-5 FIX: 正确处理 negotiate 错误，记录日志并跳过该成员的投票
			negResult, negErr := member.Agent.Negotiate(ctx, proposal)
			if negErr != nil {
				c.logger.Warn("consensus negotiation failed",
					zap.String("member", member.ID),
					zap.String("task", task.ID),
					zap.Error(negErr))
				continue
			}
			if negResult != nil && negResult.Response != "" {
				votes[negResult.Response]++
			}
		}

		// 找到赢家
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

	// 通过技能匹配查找
	for _, member := range c.Members {
		if member.Status == MemberStatusIdle {
			return member
		}
	}
	return nil
}

// CrewResult载有船员行刑的结果.
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
