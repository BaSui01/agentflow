// Package browser provides browser automation capabilities for AI agents.
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Action represents a browser action type.
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

// BrowserCommand represents a command to execute in the browser.
type BrowserCommand struct {
	Action   Action            `json:"action"`
	Selector string            `json:"selector,omitempty"` // CSS selector or XPath
	Value    string            `json:"value,omitempty"`    // For type, navigate actions
	Options  map[string]string `json:"options,omitempty"`
}

// BrowserResult represents the result of a browser command.
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

// PageState represents the current state of a browser page.
type PageState struct {
	URL          string            `json:"url"`
	Title        string            `json:"title"`
	Content      string            `json:"content,omitempty"`  // Simplified DOM or text content
	Elements     []PageElement     `json:"elements,omitempty"` // Interactive elements
	Screenshot   []byte            `json:"screenshot,omitempty"`
	Cookies      map[string]string `json:"cookies,omitempty"`
	LocalStorage map[string]string `json:"local_storage,omitempty"`
}

// PageElement represents an interactive element on the page.
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

// BoundingBox represents element position and size.
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// BrowserConfig configures the browser automation.
type BrowserConfig struct {
	Headless          bool          `json:"headless"`
	Timeout           time.Duration `json:"timeout"`
	ViewportWidth     int           `json:"viewport_width"`
	ViewportHeight    int           `json:"viewport_height"`
	UserAgent         string        `json:"user_agent,omitempty"`
	ProxyURL          string        `json:"proxy_url,omitempty"`
	ScreenshotOnError bool          `json:"screenshot_on_error"`
}

// DefaultBrowserConfig returns sensible defaults.
func DefaultBrowserConfig() BrowserConfig {
	return BrowserConfig{
		Headless:          true,
		Timeout:           30 * time.Second,
		ViewportWidth:     1920,
		ViewportHeight:    1080,
		ScreenshotOnError: true,
	}
}

// Browser defines the interface for browser automation.
type Browser interface {
	// Execute runs a browser command.
	Execute(ctx context.Context, cmd BrowserCommand) (*BrowserResult, error)
	// GetState returns the current page state.
	GetState(ctx context.Context) (*PageState, error)
	// Close closes the browser.
	Close() error
}

// BrowserSession manages a browser automation session.
type BrowserSession struct {
	id      string
	config  BrowserConfig
	browser Browser
	history []BrowserCommand
	mu      sync.RWMutex
	logger  *zap.Logger
}

// NewBrowserSession creates a new browser session.
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

// Execute runs a command and records it in history.
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

// GetHistory returns the command history.
func (s *BrowserSession) GetHistory() []BrowserCommand {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]BrowserCommand{}, s.history...)
}

// Navigate navigates to a URL.
func (s *BrowserSession) Navigate(ctx context.Context, url string) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action: ActionNavigate,
		Value:  url,
	})
}

// Click clicks on an element.
func (s *BrowserSession) Click(ctx context.Context, selector string) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action:   ActionClick,
		Selector: selector,
	})
}

// Type types text into an element.
func (s *BrowserSession) Type(ctx context.Context, selector, text string) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action:   ActionType,
		Selector: selector,
		Value:    text,
	})
}

// Screenshot takes a screenshot.
func (s *BrowserSession) Screenshot(ctx context.Context) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action: ActionScreenshot,
	})
}

// Extract extracts content from the page.
func (s *BrowserSession) Extract(ctx context.Context, selector string) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action:   ActionExtract,
		Selector: selector,
	})
}

// Wait waits for an element to appear.
func (s *BrowserSession) Wait(ctx context.Context, selector string, timeout time.Duration) (*BrowserResult, error) {
	return s.Execute(ctx, BrowserCommand{
		Action:   ActionWait,
		Selector: selector,
		Options: map[string]string{
			"timeout": timeout.String(),
		},
	})
}

// Close closes the session.
func (s *BrowserSession) Close() error {
	return s.browser.Close()
}

// BrowserTool wraps browser automation as an agent tool.
type BrowserTool struct {
	sessions map[string]*BrowserSession
	factory  BrowserFactory
	config   BrowserConfig
	mu       sync.RWMutex
	logger   *zap.Logger
}

// BrowserFactory creates browser instances.
type BrowserFactory interface {
	Create(config BrowserConfig) (Browser, error)
}

// NewBrowserTool creates a new browser tool.
func NewBrowserTool(factory BrowserFactory, config BrowserConfig, logger *zap.Logger) *BrowserTool {
	return &BrowserTool{
		sessions: make(map[string]*BrowserSession),
		factory:  factory,
		config:   config,
		logger:   logger,
	}
}

// GetOrCreateSession gets or creates a browser session.
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

// CloseSession closes a specific session.
func (t *BrowserTool) CloseSession(sessionID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if session, ok := t.sessions[sessionID]; ok {
		delete(t.sessions, sessionID)
		return session.Close()
	}
	return nil
}

// CloseAll closes all sessions.
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

// ExecuteCommand executes a browser command in a session.
func (t *BrowserTool) ExecuteCommand(ctx context.Context, sessionID string, cmd BrowserCommand) (*BrowserResult, error) {
	session, err := t.GetOrCreateSession(sessionID)
	if err != nil {
		return nil, err
	}
	return session.Execute(ctx, cmd)
}
