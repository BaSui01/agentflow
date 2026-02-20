// 包浏览器为AI代理提供浏览器自动化能力.
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 动作代表浏览器动作类型.
type Action string

const (
	ActionNavigate   Action = "navigate"
	ActionClick      Action = "click"
	ActionType       Action = "type"
	ActionScroll     Action = "scroll"
	ActionScreenshot Action = "screenshot"
	ActionExtract    Action = "extract"
	ActionWait       Action = "wait"
	ActionSelect     Action = "select"
	ActionHover      Action = "hover"
	ActionBack       Action = "back"
	ActionForward    Action = "forward"
	ActionRefresh    Action = "refresh"
)

// 浏览器Command 代表要在浏览器中执行的命令.
type BrowserCommand struct {
	Action   Action            `json:"action"`
	Selector string            `json:"selector,omitempty"` // CSS selector or XPath
	Value    string            `json:"value,omitempty"`    // For type, navigate actions
	Options  map[string]string `json:"options,omitempty"`
}

// 浏览器Result代表了浏览器命令的结果.
type BrowserResult struct {
	Success    bool            `json:"success"`
	Action     Action          `json:"action"`
	Data       json.RawMessage `json:"data,omitempty"`
	Screenshot []byte          `json:"screenshot,omitempty"`
	Error      string          `json:"error,omitempty"`
	Duration   time.Duration   `json:"duration"`
	URL        string          `json:"url,omitempty"`
	Title      string          `json:"title,omitempty"`
}

// PageState 代表浏览器页面的当前状态.
type PageState struct {
	URL          string            `json:"url"`
	Title        string            `json:"title"`
	Content      string            `json:"content,omitempty"`  // Simplified DOM or text content
	Elements     []PageElement     `json:"elements,omitempty"` // Interactive elements
	Screenshot   []byte            `json:"screenshot,omitempty"`
	Cookies      map[string]string `json:"cookies,omitempty"`
	LocalStorage map[string]string `json:"local_storage,omitempty"`
}

// PageElement代表了页面上的互动元素.
type PageElement struct {
	ID          string            `json:"id,omitempty"`
	Tag         string            `json:"tag"`
	Text        string            `json:"text,omitempty"`
	Selector    string            `json:"selector"`
	Type        string            `json:"type,omitempty"` // button, input, link, etc.
	Visible     bool              `json:"visible"`
	Attrs       map[string]string `json:"attrs,omitempty"`
	BoundingBox *BoundingBox      `json:"bounding_box,omitempty"`
}

// BboundingBox代表元素位置和大小.
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// 浏览器Config配置了浏览器自动化.
type BrowserConfig struct {
	Headless          bool          `json:"headless"`
	Timeout           time.Duration `json:"timeout"`
	ViewportWidth     int           `json:"viewport_width"`
	ViewportHeight    int           `json:"viewport_height"`
	UserAgent         string        `json:"user_agent,omitempty"`
	ProxyURL          string        `json:"proxy_url,omitempty"`
	ScreenshotOnError bool          `json:"screenshot_on_error"`
}

// 默认浏览器 Config 返回合理的默认值 。
func DefaultBrowserConfig() BrowserConfig {
	return BrowserConfig{
		Headless:          true,
		Timeout:           30 * time.Second,
		ViewportWidth:     1920,
		ViewportHeight:    1080,
		ScreenshotOnError: true,
	}
}

// 浏览器定义了浏览器自动化的界面.
type Browser interface {
	// 执行运行浏览器命令 。
	Execute(ctx context.Context, cmd BrowserCommand) (*BrowserResult, error)
	// GetState 返回当前页面状态 。
	GetState(ctx context.Context) (*PageState, error)
	// 关闭浏览器 。
	Close() error
}

// 浏览器Session管理一个浏览器自动化会话.
type BrowserSession struct {
	id      string
	config  BrowserConfig
	browser Browser
	history []BrowserCommand
	mu      sync.RWMutex
	logger  *zap.Logger
}

// 新浏览器会话创建一个新的浏览器会话 。
func NewBrowserSession(id string, browser Browser, config BrowserConfig, logger *zap.Logger) *BrowserSession {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BrowserSession{
		id:      id,
		config:  config,
		browser: browser,
		history: make([]BrowserCommand, 0),
		logger:  logger,
	}
}

// 执行命令并记录在历史上.
func (s *BrowserSession) Execute(ctx context.Context, cmd BrowserCommand) (*BrowserResult, error) {
	s.mu.Lock()
	s.history = append(s.history, cmd)
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()

	s.logger.Debug("executing browser command",
		zap.String("action", string(cmd.Action)),
		zap.String("selector", cmd.Selector))

	result, err := s.browser.Execute(ctx, cmd)
	if err != nil {
		s.logger.Error("browser command failed",
			zap.String("action", string(cmd.Action)),
			zap.Error(err))
		return nil, err
	}

	return result, nil
}

// GetHistory返回命令历史.
func (s *BrowserSession) GetHistory() []BrowserCommand {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]BrowserCommand{}, s.history...)
}

// 导航导航到 URL 。
func (s *BrowserSession) Navigate(ctx context.Context, url string) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action: ActionNavigate,
		Value:  url,
	})
}

// 点击元素。
func (s *BrowserSession) Click(ctx context.Context, selector string) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action:   ActionClick,
		Selector: selector,
	})
}

// 将文本类型输入元素。
func (s *BrowserSession) Type(ctx context.Context, selector, text string) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action:   ActionType,
		Selector: selector,
		Value:    text,
	})
}

// 截图取出截图.
func (s *BrowserSession) Screenshot(ctx context.Context) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action: ActionScreenshot,
	})
}

// 提取页面中的内容。
func (s *BrowserSession) Extract(ctx context.Context, selector string) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action:   ActionExtract,
		Selector: selector,
	})
}

// 等待元素出现。
func (s *BrowserSession) Wait(ctx context.Context, selector string, timeout time.Duration) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action:   ActionWait,
		Selector: selector,
		Options: map[string]string{
			"timeout": timeout.String(),
		},
	})
}

// 关闭会话 。
func (s *BrowserSession) Close() error {
	return s.browser.Close()
}

// 浏览器工具将浏览器自动化包成代理工具.
type BrowserTool struct {
	sessions map[string]*BrowserSession
	factory  BrowserFactory
	config   BrowserConfig
	mu       sync.RWMutex
	logger   *zap.Logger
}

// 浏览器 Factory 创建浏览器实例 。
type BrowserFactory interface {
	Create(config BrowserConfig) (Browser, error)
}

// NewBrowserTooll创建了一个新的浏览器工具.
func NewBrowserTool(factory BrowserFactory, config BrowserConfig, logger *zap.Logger) *BrowserTool {
	return &BrowserTool{
		sessions: make(map[string]*BrowserSession),
		factory:  factory,
		config:   config,
		logger:   logger,
	}
}

// Get OrCreate Session 获取或创建浏览器会话 。
func (t *BrowserTool) GetOrCreateSession(sessionID string) (*BrowserSession, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if session, ok := t.sessions[sessionID]; ok {
		return session, nil
	}

	browser, err := t.factory.Create(t.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create browser: %w", err)
	}

	session := NewBrowserSession(sessionID, browser, t.config, t.logger)
	t.sessions[sessionID] = session
	return session, nil
}

// 闭会结束某届特定会议。
func (t *BrowserTool) CloseSession(sessionID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if session, ok := t.sessions[sessionID]; ok {
		delete(t.sessions, sessionID)
		return session.Close()
	}
	return nil
}

// 关闭全部会话 。
func (t *BrowserTool) CloseAll() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var lastErr error
	for id, session := range t.sessions {
		if err := session.Close(); err != nil {
			lastErr = err
			t.logger.Error("failed to close session", zap.String("id", id), zap.Error(err))
		}
		delete(t.sessions, id)
	}
	return lastErr
}

// 执行Command在会话中执行浏览器命令.
func (t *BrowserTool) ExecuteCommand(ctx context.Context, sessionID string, cmd BrowserCommand) (*BrowserResult, error) {
	session, err := t.GetOrCreateSession(sessionID)
	if err != nil {
		return nil, err
	}
	return session.Execute(ctx, cmd)
}
