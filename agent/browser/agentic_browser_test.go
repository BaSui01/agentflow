package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Mock implementations ---

type mockBrowserDriver struct {
	navigateFunc   func(ctx context.Context, url string) error
	screenshotFunc func(ctx context.Context) (*Screenshot, error)
	clickFunc      func(ctx context.Context, x, y int) error
	typeFunc       func(ctx context.Context, text string) error
	scrollFunc     func(ctx context.Context, dx, dy int) error
	getURLFunc     func(ctx context.Context) (string, error)
	closeFunc      func() error
}

func (m *mockBrowserDriver) Navigate(ctx context.Context, url string) error {
	if m.navigateFunc != nil {
		return m.navigateFunc(ctx, url)
	}
	return nil
}

func (m *mockBrowserDriver) Screenshot(ctx context.Context) (*Screenshot, error) {
	if m.screenshotFunc != nil {
		return m.screenshotFunc(ctx)
	}
	return &Screenshot{Data: []byte("fake"), Width: 100, Height: 100, Timestamp: time.Now()}, nil
}

func (m *mockBrowserDriver) Click(ctx context.Context, x, y int) error {
	if m.clickFunc != nil {
		return m.clickFunc(ctx, x, y)
	}
	return nil
}

func (m *mockBrowserDriver) Type(ctx context.Context, text string) error {
	if m.typeFunc != nil {
		return m.typeFunc(ctx, text)
	}
	return nil
}

func (m *mockBrowserDriver) Scroll(ctx context.Context, dx, dy int) error {
	if m.scrollFunc != nil {
		return m.scrollFunc(ctx, dx, dy)
	}
	return nil
}

func (m *mockBrowserDriver) GetURL(ctx context.Context) (string, error) {
	if m.getURLFunc != nil {
		return m.getURLFunc(ctx)
	}
	return "http://example.com", nil
}

func (m *mockBrowserDriver) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

type mockVisionModel struct {
	analyzeFunc     func(ctx context.Context, screenshot *Screenshot) (*VisionAnalysis, error)
	planActionsFunc func(ctx context.Context, goal string, analysis *VisionAnalysis) ([]AgenticAction, error)
}

func (m *mockVisionModel) Analyze(ctx context.Context, screenshot *Screenshot) (*VisionAnalysis, error) {
	if m.analyzeFunc != nil {
		return m.analyzeFunc(ctx, screenshot)
	}
	return &VisionAnalysis{
		PageTitle:   "Test Page",
		PageType:    "other",
		Description: "A test page",
	}, nil
}

func (m *mockVisionModel) PlanActions(ctx context.Context, goal string, analysis *VisionAnalysis) ([]AgenticAction, error) {
	if m.planActionsFunc != nil {
		return m.planActionsFunc(ctx, goal, analysis)
	}
	return nil, nil
}

// --- AgenticBrowser tests ---

func TestNewAgenticBrowser(t *testing.T) {
	driver := &mockBrowserDriver{}
	vision := &mockVisionModel{}
	config := DefaultAgenticBrowserConfig()

	ab := NewAgenticBrowser(driver, vision, config, nil)
	assert.NotNil(t, ab)
	assert.Empty(t, ab.GetHistory())
}

func TestAgenticBrowser_ExecuteTask_GoalAchieved(t *testing.T) {
	driver := &mockBrowserDriver{}
	vision := &mockVisionModel{
		analyzeFunc: func(ctx context.Context, s *Screenshot) (*VisionAnalysis, error) {
			return &VisionAnalysis{Suggestions: []string{"goal_achieved"}}, nil
		},
	}
	config := DefaultAgenticBrowserConfig()
	config.Timeout = 5 * time.Second
	config.ActionDelay = 0
	config.ScreenshotDelay = 0

	ab := NewAgenticBrowser(driver, vision, config, zap.NewNop())
	task := BrowserTask{ID: "t1", Goal: "test goal"}

	result, err := ab.ExecuteTask(context.Background(), task)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "t1", result.TaskID)
}

func TestAgenticBrowser_ExecuteTask_WithStartURL(t *testing.T) {
	navigated := false
	driver := &mockBrowserDriver{
		navigateFunc: func(ctx context.Context, url string) error {
			navigated = true
			assert.Equal(t, "http://example.com", url)
			return nil
		},
	}
	vision := &mockVisionModel{
		analyzeFunc: func(ctx context.Context, s *Screenshot) (*VisionAnalysis, error) {
			return &VisionAnalysis{Suggestions: []string{"goal_achieved"}}, nil
		},
	}
	config := DefaultAgenticBrowserConfig()
	config.Timeout = 5 * time.Second
	config.ActionDelay = 0
	config.ScreenshotDelay = 0

	ab := NewAgenticBrowser(driver, vision, config, zap.NewNop())
	task := BrowserTask{ID: "t1", Goal: "test", StartURL: "http://example.com"}

	_, err := ab.ExecuteTask(context.Background(), task)
	require.NoError(t, err)
	assert.True(t, navigated)
}

func TestAgenticBrowser_ExecuteTask_NoActionsPlanned(t *testing.T) {
	driver := &mockBrowserDriver{}
	vision := &mockVisionModel{
		planActionsFunc: func(ctx context.Context, goal string, analysis *VisionAnalysis) ([]AgenticAction, error) {
			return nil, nil // No actions
		},
	}
	config := DefaultAgenticBrowserConfig()
	config.Timeout = 5 * time.Second
	config.ActionDelay = 0
	config.ScreenshotDelay = 0

	ab := NewAgenticBrowser(driver, vision, config, zap.NewNop())
	task := BrowserTask{ID: "t1", Goal: "test"}

	result, err := ab.ExecuteTask(context.Background(), task)
	require.NoError(t, err)
	assert.False(t, result.Success)
}

func TestAgenticBrowser_ExecuteTask_ExecutesActions(t *testing.T) {
	clickCount := 0
	driver := &mockBrowserDriver{
		clickFunc: func(ctx context.Context, x, y int) error {
			clickCount++
			return nil
		},
	}

	callCount := 0
	vision := &mockVisionModel{
		planActionsFunc: func(ctx context.Context, goal string, analysis *VisionAnalysis) ([]AgenticAction, error) {
			callCount++
			if callCount > 2 {
				return nil, nil // Stop after 2 iterations
			}
			return []AgenticAction{{Type: ActionClick, X: 100, Y: 200}}, nil
		},
	}
	config := DefaultAgenticBrowserConfig()
	config.Timeout = 5 * time.Second
	config.ActionDelay = 0
	config.ScreenshotDelay = 0

	ab := NewAgenticBrowser(driver, vision, config, zap.NewNop())
	task := BrowserTask{ID: "t1", Goal: "click stuff"}

	result, err := ab.ExecuteTask(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, 2, clickCount)
	assert.Len(t, result.Actions, 2)
	assert.True(t, result.Actions[0].Success)
}

func TestAgenticBrowser_ExecuteAction_AllTypes(t *testing.T) {
	driver := &mockBrowserDriver{}
	vision := &mockVisionModel{}
	config := DefaultAgenticBrowserConfig()
	ab := NewAgenticBrowser(driver, vision, config, zap.NewNop())

	screenshot := &Screenshot{Data: []byte("fake")}

	tests := []struct {
		name   string
		action AgenticAction
	}{
		{"click", AgenticAction{Type: ActionClick, X: 10, Y: 20}},
		{"type", AgenticAction{Type: ActionType, Value: "hello"}},
		{"scroll", AgenticAction{Type: ActionScroll, X: 0, Y: 100}},
		{"navigate", AgenticAction{Type: ActionNavigate, Value: "http://example.com"}},
		{"wait", AgenticAction{Type: ActionWait, Duration: time.Millisecond}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := ab.executeAction(context.Background(), tt.action, screenshot)
			assert.True(t, record.Success)
		})
	}
}

func TestAgenticBrowser_ExecuteAction_Error(t *testing.T) {
	driver := &mockBrowserDriver{
		clickFunc: func(ctx context.Context, x, y int) error {
			return fmt.Errorf("click failed")
		},
	}
	vision := &mockVisionModel{}
	config := DefaultAgenticBrowserConfig()
	ab := NewAgenticBrowser(driver, vision, config, zap.NewNop())

	record := ab.executeAction(context.Background(), AgenticAction{Type: ActionClick}, nil)
	assert.False(t, record.Success)
	assert.Equal(t, "click failed", record.Error)
}

func TestAgenticBrowser_GetHistory(t *testing.T) {
	driver := &mockBrowserDriver{}
	vision := &mockVisionModel{}
	config := DefaultAgenticBrowserConfig()
	ab := NewAgenticBrowser(driver, vision, config, zap.NewNop())

	// History should be a copy
	history := ab.GetHistory()
	assert.Empty(t, history)
}

func TestAgenticBrowser_IsGoalAchieved(t *testing.T) {
	ab := &AgenticBrowser{}

	assert.True(t, ab.isGoalAchieved("test", &VisionAnalysis{Suggestions: []string{"goal_achieved"}}))
	assert.False(t, ab.isGoalAchieved("test", &VisionAnalysis{Suggestions: []string{"try_again"}}))
	assert.False(t, ab.isGoalAchieved("test", &VisionAnalysis{}))
}

// --- ScreenshotToBase64 ---

func TestScreenshotToBase64(t *testing.T) {
	s := &Screenshot{Data: []byte("hello")}
	b64 := ScreenshotToBase64(s)
	assert.NotEmpty(t, b64)
	assert.Equal(t, "aGVsbG8=", b64)
}

// --- DefaultConfigs ---

func TestDefaultAgenticBrowserConfig(t *testing.T) {
	config := DefaultAgenticBrowserConfig()
	assert.Equal(t, 50, config.MaxActions)
	assert.Greater(t, config.Timeout, time.Duration(0))
}

func TestDefaultBrowserConfig(t *testing.T) {
	config := DefaultBrowserConfig()
	assert.True(t, config.Headless)
	assert.Equal(t, 1920, config.ViewportWidth)
}

func TestDefaultBrowserPoolConfig(t *testing.T) {
	config := DefaultBrowserPoolConfig()
	assert.Equal(t, 5, config.MaxSize)
	assert.Equal(t, 1, config.MinIdle)
}

// --- BrowserSession tests ---

type mockBrowser struct {
	executeFunc func(ctx context.Context, cmd BrowserCommand) (*BrowserResult, error)
	closeFunc   func() error
}

func (m *mockBrowser) Execute(ctx context.Context, cmd BrowserCommand) (*BrowserResult, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, cmd)
	}
	return &BrowserResult{Success: true, Action: cmd.Action}, nil
}

func (m *mockBrowser) GetState(ctx context.Context) (*PageState, error) {
	return &PageState{URL: "http://example.com", Title: "Test"}, nil
}

func (m *mockBrowser) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestBrowserSession_Execute(t *testing.T) {
	browser := &mockBrowser{}
	config := DefaultBrowserConfig()
	session := NewBrowserSession("s1", browser, config, zap.NewNop())

	result, err := session.Execute(context.Background(), BrowserCommand{Action: ActionNavigate, Value: "http://example.com"})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, session.GetHistory(), 1)
}

func TestBrowserSession_Navigate(t *testing.T) {
	browser := &mockBrowser{}
	session := NewBrowserSession("s1", browser, DefaultBrowserConfig(), zap.NewNop())

	result, err := session.Navigate(context.Background(), "http://example.com")
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestBrowserSession_Click(t *testing.T) {
	browser := &mockBrowser{}
	session := NewBrowserSession("s1", browser, DefaultBrowserConfig(), zap.NewNop())

	result, err := session.Click(context.Background(), "#btn")
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestBrowserSession_Type(t *testing.T) {
	browser := &mockBrowser{}
	session := NewBrowserSession("s1", browser, DefaultBrowserConfig(), zap.NewNop())

	result, err := session.Type(context.Background(), "#input", "hello")
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestBrowserSession_Screenshot(t *testing.T) {
	browser := &mockBrowser{}
	session := NewBrowserSession("s1", browser, DefaultBrowserConfig(), zap.NewNop())

	result, err := session.Screenshot(context.Background())
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestBrowserSession_Extract(t *testing.T) {
	browser := &mockBrowser{}
	session := NewBrowserSession("s1", browser, DefaultBrowserConfig(), zap.NewNop())

	result, err := session.Extract(context.Background(), ".content")
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestBrowserSession_Wait(t *testing.T) {
	browser := &mockBrowser{}
	session := NewBrowserSession("s1", browser, DefaultBrowserConfig(), zap.NewNop())

	result, err := session.Wait(context.Background(), ".loading", time.Second)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestBrowserSession_Close(t *testing.T) {
	closed := false
	browser := &mockBrowser{closeFunc: func() error { closed = true; return nil }}
	session := NewBrowserSession("s1", browser, DefaultBrowserConfig(), zap.NewNop())

	require.NoError(t, session.Close())
	assert.True(t, closed)
}

func TestBrowserSession_GetHistory_ReturnsCopy(t *testing.T) {
	browser := &mockBrowser{}
	session := NewBrowserSession("s1", browser, DefaultBrowserConfig(), zap.NewNop())
	_, err := session.Navigate(context.Background(), "http://example.com")
	require.NoError(t, err)

	h1 := session.GetHistory()
	h2 := session.GetHistory()
	assert.Len(t, h1, 1)
	assert.Len(t, h2, 1)
	// Modifying one should not affect the other
	_ = append(h1, BrowserCommand{})
	assert.Len(t, session.GetHistory(), 1)
}

// --- BrowserTool tests ---

type mockBrowserFactory struct {
	createFunc func(config BrowserConfig) (Browser, error)
}

func (f *mockBrowserFactory) Create(config BrowserConfig) (Browser, error) {
	if f.createFunc != nil {
		return f.createFunc(config)
	}
	return &mockBrowser{}, nil
}

func TestBrowserTool_GetOrCreateSession(t *testing.T) {
	factory := &mockBrowserFactory{}
	tool := NewBrowserTool(factory, DefaultBrowserConfig(), zap.NewNop())

	s1, err := tool.GetOrCreateSession("sess1")
	require.NoError(t, err)
	assert.NotNil(t, s1)

	// Same session ID should return same session
	s2, err := tool.GetOrCreateSession("sess1")
	require.NoError(t, err)
	assert.Equal(t, s1, s2)
}

func TestBrowserTool_CloseSession(t *testing.T) {
	factory := &mockBrowserFactory{}
	tool := NewBrowserTool(factory, DefaultBrowserConfig(), zap.NewNop())

	_, err := tool.GetOrCreateSession("sess1")
	require.NoError(t, err)
	require.NoError(t, tool.CloseSession("sess1"))

	// Closing non-existent session should not error
	require.NoError(t, tool.CloseSession("nonexistent"))
}

func TestBrowserTool_CloseAll(t *testing.T) {
	factory := &mockBrowserFactory{}
	tool := NewBrowserTool(factory, DefaultBrowserConfig(), zap.NewNop())

	_, err := tool.GetOrCreateSession("s1")
	require.NoError(t, err)
	_, err = tool.GetOrCreateSession("s2")
	require.NoError(t, err)

	require.NoError(t, tool.CloseAll())
}

func TestBrowserTool_ExecuteCommand(t *testing.T) {
	factory := &mockBrowserFactory{}
	tool := NewBrowserTool(factory, DefaultBrowserConfig(), zap.NewNop())

	result, err := tool.ExecuteCommand(context.Background(), "s1", BrowserCommand{Action: ActionNavigate, Value: "http://example.com"})
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestBrowserTool_ExecuteCommand_FactoryError(t *testing.T) {
	factory := &mockBrowserFactory{
		createFunc: func(config BrowserConfig) (Browser, error) {
			return nil, fmt.Errorf("browser creation failed")
		},
	}
	tool := NewBrowserTool(factory, DefaultBrowserConfig(), zap.NewNop())

	_, err := tool.ExecuteCommand(context.Background(), "s1", BrowserCommand{Action: ActionNavigate})
	assert.Error(t, err)
}

// --- LLMVisionAdapter tests ---

type mockLLMVisionProvider struct {
	analyzeFunc func(ctx context.Context, imageBase64 string, prompt string) (string, error)
}

func (m *mockLLMVisionProvider) AnalyzeImage(ctx context.Context, imageBase64 string, prompt string) (string, error) {
	if m.analyzeFunc != nil {
		return m.analyzeFunc(ctx, imageBase64, prompt)
	}
	return "", nil
}

func TestLLMVisionAdapter_Analyze_Success(t *testing.T) {
	provider := &mockLLMVisionProvider{
		analyzeFunc: func(ctx context.Context, img string, prompt string) (string, error) {
			return `{"elements":[],"page_title":"Test","page_type":"other","description":"A page"}`, nil
		},
	}
	adapter := NewLLMVisionAdapter(provider, zap.NewNop())

	analysis, err := adapter.Analyze(context.Background(), &Screenshot{Data: []byte("img")})
	require.NoError(t, err)
	assert.Equal(t, "Test", analysis.PageTitle)
	assert.Equal(t, "other", analysis.PageType)
}

func TestLLMVisionAdapter_Analyze_EmptyScreenshot(t *testing.T) {
	adapter := NewLLMVisionAdapter(nil, nil)
	_, err := adapter.Analyze(context.Background(), nil)
	assert.Error(t, err)

	_, err = adapter.Analyze(context.Background(), &Screenshot{})
	assert.Error(t, err)
}

func TestLLMVisionAdapter_Analyze_InvalidJSON(t *testing.T) {
	provider := &mockLLMVisionProvider{
		analyzeFunc: func(ctx context.Context, img string, prompt string) (string, error) {
			return "not json, just a description", nil
		},
	}
	adapter := NewLLMVisionAdapter(provider, zap.NewNop())

	analysis, err := adapter.Analyze(context.Background(), &Screenshot{Data: []byte("img"), URL: "http://test.com"})
	require.NoError(t, err)
	assert.Equal(t, "not json, just a description", analysis.Description)
	assert.Equal(t, "http://test.com", analysis.PageTitle)
}

func TestLLMVisionAdapter_PlanActions_Success(t *testing.T) {
	provider := &mockLLMVisionProvider{
		analyzeFunc: func(ctx context.Context, img string, prompt string) (string, error) {
			return `[{"type":"click","x":100,"y":200}]`, nil
		},
	}
	adapter := NewLLMVisionAdapter(provider, zap.NewNop())

	actions, err := adapter.PlanActions(context.Background(), "click button", &VisionAnalysis{})
	require.NoError(t, err)
	assert.Len(t, actions, 1)
	assert.Equal(t, ActionClick, actions[0].Type)
}

func TestLLMVisionAdapter_PlanActions_Error(t *testing.T) {
	provider := &mockLLMVisionProvider{
		analyzeFunc: func(ctx context.Context, img string, prompt string) (string, error) {
			return "", fmt.Errorf("LLM error")
		},
	}
	adapter := NewLLMVisionAdapter(provider, zap.NewNop())

	_, err := adapter.PlanActions(context.Background(), "goal", &VisionAnalysis{})
	assert.Error(t, err)
}

func TestLLMVisionAdapter_PlanActions_InvalidJSON(t *testing.T) {
	provider := &mockLLMVisionProvider{
		analyzeFunc: func(ctx context.Context, img string, prompt string) (string, error) {
			return "not json", nil
		},
	}
	adapter := NewLLMVisionAdapter(provider, zap.NewNop())

	_, err := adapter.PlanActions(context.Background(), "goal", &VisionAnalysis{})
	assert.Error(t, err)
}

// Ensure unused import is used
var _ = json.Marshal
