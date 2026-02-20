// 软件包浏览器为代理浏览器提供了Vision-Action Loop.
package browser

import (
	"context"
	"encoding/base64"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AgenticAction代表了代理浏览器的浏览器动作.
type AgenticAction struct {
	Type     Action         `json:"type"`
	Selector string         `json:"selector,omitempty"`
	Value    string         `json:"value,omitempty"`
	X        int            `json:"x,omitempty"`
	Y        int            `json:"y,omitempty"`
	Duration time.Duration  `json:"duration,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// 屏幕截图代表了浏览器截图.
type Screenshot struct {
	Data      []byte    `json:"data"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Timestamp time.Time `json:"timestamp"`
	URL       string    `json:"url"`
}

// 元素代表被检测到的UI元素.
type Element struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"` // button, input, link, text, image
	Text       string  `json:"text,omitempty"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Clickable  bool    `json:"clickable"`
	Confidence float64 `json:"confidence"`
}

// VisionModel分析截图.
type VisionModel interface {
	Analyze(ctx context.Context, screenshot *Screenshot) (*VisionAnalysis, error)
	PlanActions(ctx context.Context, goal string, analysis *VisionAnalysis) ([]AgenticAction, error)
}

// 远景分析是远景模型分析的结果。
type VisionAnalysis struct {
	Elements    []Element `json:"elements"`
	PageTitle   string    `json:"page_title"`
	PageType    string    `json:"page_type"`
	Description string    `json:"description"`
	Suggestions []string  `json:"suggestions,omitempty"`
}

// 浏览器Driver接口用于浏览器控制.
type BrowserDriver interface {
	Navigate(ctx context.Context, url string) error
	Screenshot(ctx context.Context) (*Screenshot, error)
	Click(ctx context.Context, x, y int) error
	Type(ctx context.Context, text string) error
	Scroll(ctx context.Context, deltaX, deltaY int) error
	GetURL(ctx context.Context) (string, error)
	Close() error
}

// Agentic Browser提供Vision-Action Loop浏览器自动化.
type AgenticBrowser struct {
	driver  BrowserDriver
	vision  VisionModel
	config  AgenticBrowserConfig
	history []ActionRecord
	logger  *zap.Logger
	mu      sync.Mutex
}

// 代理浏览器Config配置代理浏览器.
type AgenticBrowserConfig struct {
	MaxActions      int           `json:"max_actions"`
	ActionDelay     time.Duration `json:"action_delay"`
	ScreenshotDelay time.Duration `json:"screenshot_delay"`
	Timeout         time.Duration `json:"timeout"`
	RetryOnFailure  bool          `json:"retry_on_failure"`
	MaxRetries      int           `json:"max_retries"`
}

// 默认代理浏览器 Config 返回默认配置 。
func DefaultAgenticBrowserConfig() AgenticBrowserConfig {
	return AgenticBrowserConfig{
		MaxActions:      50,
		ActionDelay:     500 * time.Millisecond,
		ScreenshotDelay: 200 * time.Millisecond,
		Timeout:         5 * time.Minute,
		RetryOnFailure:  true,
		MaxRetries:      3,
	}
}

// 动作记录记录已执行动作 。
type ActionRecord struct {
	Action     AgenticAction `json:"action"`
	Screenshot *Screenshot   `json:"screenshot,omitempty"`
	Success    bool          `json:"success"`
	Error      string        `json:"error,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}

// 新代理浏览器创建了新的代理浏览器.
func NewAgenticBrowser(driver BrowserDriver, vision VisionModel, config AgenticBrowserConfig, logger *zap.Logger) *AgenticBrowser {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgenticBrowser{
		driver:  driver,
		vision:  vision,
		config:  config,
		history: make([]ActionRecord, 0),
		logger:  logger.With(zap.String("component", "agentic_browser")),
	}
}

// 执行任务使用 Vision-Action Loop执行任务.
func (b *AgenticBrowser) ExecuteTask(ctx context.Context, task BrowserTask) (*TaskResult, error) {
	ctx, cancel := context.WithTimeout(ctx, b.config.Timeout)
	defer cancel()

	result := &TaskResult{
		TaskID:    task.ID,
		StartTime: time.Now(),
		Actions:   make([]ActionRecord, 0),
	}

	b.logger.Info("starting browser task", zap.String("task_id", task.ID), zap.String("goal", task.Goal))

	// 如果提供则启动 URL 导航
	if task.StartURL != "" {
		if err := b.driver.Navigate(ctx, task.StartURL); err != nil {
			result.Error = err.Error()
			return result, err
		}
		time.Sleep(b.config.ScreenshotDelay)
	}

	// 愿景-行动循环
	for i := 0; i < b.config.MaxActions; i++ {
		select {
		case <-ctx.Done():
			result.Error = "timeout"
			return result, ctx.Err()
		default:
		}

		// 抓取截图
		screenshot, err := b.driver.Screenshot(ctx)
		if err != nil {
			b.logger.Error("screenshot failed", zap.Error(err))
			continue
		}

		// 用视觉模型分析
		analysis, err := b.vision.Analyze(ctx, screenshot)
		if err != nil {
			b.logger.Error("vision analysis failed", zap.Error(err))
			continue
		}

		// 检查目标是否实现
		if b.isGoalAchieved(task.Goal, analysis) {
			result.Success = true
			break
		}

		// 规划下一步行动
		actions, err := b.vision.PlanActions(ctx, task.Goal, analysis)
		if err != nil {
			b.logger.Error("action planning failed", zap.Error(err))
			continue
		}

		if len(actions) == 0 {
			b.logger.Warn("no actions planned")
			break
		}

		// 执行第一个动作
		action := actions[0]
		record := b.executeAction(ctx, action, screenshot)
		result.Actions = append(result.Actions, record)

		b.mu.Lock()
		b.history = append(b.history, record)
		b.mu.Unlock()

		if !record.Success && !b.config.RetryOnFailure {
			result.Error = record.Error
			break
		}

		time.Sleep(b.config.ActionDelay)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	b.logger.Info("browser task completed",
		zap.String("task_id", task.ID),
		zap.Bool("success", result.Success),
		zap.Int("actions", len(result.Actions)),
	)

	return result, nil
}

func (b *AgenticBrowser) executeAction(ctx context.Context, action AgenticAction, screenshot *Screenshot) ActionRecord {
	record := ActionRecord{
		Action:     action,
		Screenshot: screenshot,
		Timestamp:  time.Now(),
	}

	var err error
	switch action.Type {
	case ActionClick:
		err = b.driver.Click(ctx, action.X, action.Y)
	case ActionType:
		err = b.driver.Type(ctx, action.Value)
	case ActionScroll:
		err = b.driver.Scroll(ctx, action.X, action.Y)
	case ActionNavigate:
		err = b.driver.Navigate(ctx, action.Value)
	case ActionWait:
		time.Sleep(action.Duration)
	}

	if err != nil {
		record.Error = err.Error()
		record.Success = false
	} else {
		record.Success = true
	}

	return record
}

func (b *AgenticBrowser) isGoalAchieved(goal string, analysis *VisionAnalysis) bool {
	// 简单的heuristic - 可以用 LLM 增强
	for _, suggestion := range analysis.Suggestions {
		if suggestion == "goal_achieved" {
			return true
		}
	}
	return false
}

// 浏览器Task代表浏览器自动化任务.
type BrowserTask struct {
	ID           string         `json:"id"`
	Goal         string         `json:"goal"`
	StartURL     string         `json:"start_url,omitempty"`
	Instructions []string       `json:"instructions,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// TaskResult代表了浏览器任务的结果.
type TaskResult struct {
	TaskID    string         `json:"task_id"`
	Success   bool           `json:"success"`
	Actions   []ActionRecord `json:"actions"`
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time"`
	Duration  time.Duration  `json:"duration"`
	Error     string         `json:"error,omitempty"`
}

// GetHistory返回动作历史.
func (b *AgenticBrowser) GetHistory() []ActionRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]ActionRecord{}, b.history...)
}

// 屏幕截图 ToBase64将截图转换为 Base64.
func ScreenshotToBase64(s *Screenshot) string {
	return base64.StdEncoding.EncodeToString(s.Data)
}
