package persistence

import (
	"context"
	"encoding/json"
	"time"
)

// TaskStore定义了同步任务持久性的接口.
// 它在服务重启后为任务状态管理提供恢复支持.
type TaskStore interface {
	Store

	// 保存任务持续到存储( 创建或更新) 。
	SaveTask(ctx context.Context, task *AsyncTask) error

	// 通过 ID 获取任务
	GetTask(ctx context.Context, taskID string) (*AsyncTask, error)

	// ListTasks 检索匹配过滤标准的任务
	ListTasks(ctx context.Context, filter TaskFilter) ([]*AsyncTask, error)

	// 更新状态更新任务状态
	UpdateStatus(ctx context.Context, taskID string, status TaskStatus, result any, errMsg string) error

	// 更新进度更新任务进度
	UpdateProgress(ctx context.Context, taskID string, progress float64) error

	// 删除任务从商店中删除任务
	DeleteTask(ctx context.Context, taskID string) error

	// 获取可回收的任务检索重启后需要回收的任务
	// 这包括待决或运行状态中的任务
	GetRecoverableTasks(ctx context.Context) ([]*AsyncTask, error)

	// 清除完成/ 失败的任务超过指定期限
	Cleanup(ctx context.Context, olderThan time.Duration) (int, error)

	// Stats 返回关于任务存储的统计
	Stats(ctx context.Context) (*TaskStoreStats, error)
}

// 任务状态 :
type TaskStatus string

const (
	// 任务状态显示任务正在等待执行
	TaskStatusPending TaskStatus = "pending"

	// 任务状态运行显示任务正在执行中
	TaskStatusRunning TaskStatus = "running"

	// 任务状态完成后显示任务成功完成
	TaskStatusCompleted TaskStatus = "completed"

	// 任务状态失败 。
	TaskStatusFailed TaskStatus = "failed"

	// 任务状态已取消 。
	TaskStatusCancelled TaskStatus = "cancelled"

	// 任务状态超时显示任务超时
	TaskStatusTimeout TaskStatus = "timeout"
)

// IsTerminal 如果状态是终端状态, 返回为真
func (s TaskStatus) IsTerminal() bool {
	switch s {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled, TaskStatusTimeout:
		return true
	default:
		return false
	}
}

// 如果任务在重启后恢复, 可恢复返回为真
func (s TaskStatus) IsRecoverable() bool {
	switch s {
	case TaskStatusPending, TaskStatusRunning:
		return true
	default:
		return false
	}
}

// AsyncTask 代表一个持久的同步任务
type AsyncTask struct {
	// ID 是任务的唯一标识符
	ID string `json:"id"`

	// 会话ID 是此任务所属的会话
	SessionID string `json:"session_id,omitempty"`

	// AgentID 是执行此任务的代理
	AgentID string `json:"agent_id"`

	// 类型是任务类型
	Type string `json:"type"`

	// 状态是当前任务状态
	Status TaskStatus `json:"status"`

	// 输入包含任务输入数据
	Input map[string]any `json:"input,omitempty"`

	// 结果包含任务结果(完成后)
	Result any `json:"result,omitempty"`

	// 错误包含错误消息( 当失败时)
	Error string `json:"error,omitempty"`

	// 进展是任务进展(0-100)
	Progress float64 `json:"progress"`

	// 优先是任务优先(较高=更重要)
	Priority int `json:"priority"`

	// CreatedAt 是任务创建时
	CreatedAt time.Time `json:"created_at"`

	// 更新到上次更新任务时
	UpdatedAt time.Time `json:"updated_at"`

	// 开始是任务开始执行时
	StartedAt *time.Time `json:"started_at,omitempty"`

	// 任务完成时
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// 超时是任务超时的期限
	Timeout time.Duration `json:"timeout,omitempty"`

	// 重试请求是重试尝试的次数
	RetryCount int `json:"retry_count"`

	// MaxRetries 是允许的最大重试次数
	MaxRetries int `json:"max_retries"`

	// 元数据包含额外的任务元数据
	Metadata map[string]string `json:"metadata,omitempty"`

	// 父任务ID 是父任务ID( 用于子任务)
	ParentTaskID string `json:"parent_task_id,omitempty"`

	// 儿童任务ID是儿童任务ID
	ChildTaskIDs []string `json:"child_task_ids,omitempty"`
}

// JSON警长执行JSON。 元目录
func (t *AsyncTask) MarshalJSON() ([]byte, error) {
	type Alias AsyncTask
	return json.Marshal(&struct {
		*Alias
		Timeout string `json:"timeout,omitempty"`
	}{
		Alias:   (*Alias)(t),
		Timeout: t.Timeout.String(),
	})
}

// UnmarshalJSON 执行json。 解马沙勒
func (t *AsyncTask) UnmarshalJSON(data []byte) error {
	type Alias AsyncTask
	aux := &struct {
		*Alias
		Timeout string `json:"timeout,omitempty"`
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if aux.Timeout != "" {
		duration, err := time.ParseDuration(aux.Timeout)
		if err != nil {
			return err
		}
		t.Timeout = duration
	}
	return nil
}

// 如果任务处于终端状态, Is Terminal 返回为真
func (t *AsyncTask) IsTerminal() bool {
	return t.Status.IsTerminal()
}

// 如果任务在重启后恢复, 可恢复返回为真
func (t *AsyncTask) IsRecoverable() bool {
	return t.Status.IsRecoverable()
}

// 持续时间返回任务持续时间( 如果仍在运行, 则返回任务持续时间)
func (t *AsyncTask) Duration() time.Duration {
	if t.StartedAt == nil {
		return 0
	}
	if t.CompletedAt != nil {
		return t.CompletedAt.Sub(*t.StartedAt)
	}
	return time.Since(*t.StartedAt)
}

// IsTimedOut 如果任务超过超时返回真
func (t *AsyncTask) IsTimedOut() bool {
	if t.Timeout == 0 || t.StartedAt == nil {
		return false
	}
	return time.Since(*t.StartedAt) > t.Timeout
}

// 如果任务要重试, 重试是否返回为真
func (t *AsyncTask) ShouldRetry() bool {
	if t.Status != TaskStatusFailed {
		return false
	}
	return t.RetryCount < t.MaxRetries
}

// 任务 过滤器定义过滤任务的标准
type TaskFilter struct {
	// 按会话排列的会话ID过滤器
	SessionID string `json:"session_id,omitempty"`

	// 通过代理代理ID过滤器
	AgentID string `json:"agent_id,omitempty"`

	// 按任务类型分类的过滤器
	Type string `json:"type,omitempty"`

	// 按状态进行状态过滤( 可以是多个)
	Status []TaskStatus `json:"status,omitempty"`

	// 父任务过滤器
	ParentTaskID string `json:"parent_task_id,omitempty"`

	// 在此次过滤后创建任务
	CreatedAfter *time.Time `json:"created_after,omitempty"`

	// 在此之前创建过滤器任务
	CreatedBefore *time.Time `json:"created_before,omitempty"`

	// 限制是返回的最大任务数
	Limit int `json:"limit,omitempty"`

	// 偏移为要跳过的任务数
	Offset int `json:"offset,omitempty"`

	// 顺序By 指定排序顺序
	OrderBy string `json:"order_by,omitempty"`

	// 命令代斯克指定了降序
	OrderDesc bool `json:"order_desc,omitempty"`
}

// TaskStats 包含关于任务存储的统计数据
type TaskStoreStats struct {
	// 任务总数是存储中的任务总数
	TotalTasks int64 `json:"total_tasks"`

	// 待决 任务是待决任务的数量
	PendingTasks int64 `json:"pending_tasks"`

	// 运行 任务是运行中的任务数
	RunningTasks int64 `json:"running_tasks"`

	// 已完成 任务是已完成任务的数量
	CompletedTasks int64 `json:"completed_tasks"`

	// 失败的任务是失败的任务数
	FailedTasks int64 `json:"failed_tasks"`

	// 已取消 任务是已取消的任务的数量
	CancelledTasks int64 `json:"cancelled_tasks"`

	// 状态计数为每个状态的任务数
	StatusCounts map[TaskStatus]int64 `json:"status_counts"`

	// AgentCounts 是每个代理的任务数
	AgentCounts map[string]int64 `json:"agent_counts"`

	// 平均完成任务时间
	AverageCompletionTime time.Duration `json:"average_completion_time"`

	// 最老的Ping-Age是最老的待决任务年龄
	OldestPendingAge time.Duration `json:"oldest_pending_age"`
}

// TaskEvent 代表任务生命周期中的事件
type TaskEvent struct {
	// TaskID 是此事件所属的任务
	TaskID string `json:"task_id"`

	// 类型是事件类型
	Type TaskEventType `json:"type"`

	// 旧状态是上一个状态( 对于状态变化事件)
	OldStatus TaskStatus `json:"old_status,omitempty"`

	// 新状态是新状态(用于改变状态事件)
	NewStatus TaskStatus `json:"new_status,omitempty"`

	// 进步是进步价值(对于进步活动)
	Progress float64 `json:"progress,omitempty"`

	// 信件是可选事件消息
	Message string `json:"message,omitempty"`

	// 时间戳是事件发生时
	Timestamp time.Time `json:"timestamp"`
}

// TaskEventType 代表任务事件的类型
type TaskEventType string

const (
	// 任务EventCreated 表示创建了任务
	TaskEventCreated TaskEventType = "created"

	// 任务启动( T) 表示任务已经开始执行
	TaskEventStarted TaskEventType = "started"

	// 任务进度显示任务进度已更新
	TaskEventProgress TaskEventType = "progress"

	// 任务Event 已完成 。
	TaskEventCompleted TaskEventType = "completed"

	// 任务Event 失败表示任务失败
	TaskEventFailed TaskEventType = "failed"

	// 任务已取消
	TaskEventCancelled TaskEventType = "cancelled"

	// 任务EventRetry 显示任务正在重试
	TaskEventRetry TaskEventType = "retry"

	// 任务Event 已恢复 。
	TaskEventRecovered TaskEventType = "recovered"
)
