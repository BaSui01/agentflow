package browser

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"go.uber.org/zap"
)

// ChromeDPDriver 基于 chromedp 的 BrowserDriver 实现
type ChromeDPDriver struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	ctx         context.Context
	cancel      context.CancelFunc
	config      BrowserConfig
	logger      *zap.Logger
	mu          sync.Mutex
}

// ChromeDPDriverOption 配置选项
type ChromeDPDriverOption func(*ChromeDPDriver)

// NewChromeDPDriver 创建 chromedp 驱动
func NewChromeDPDriver(config BrowserConfig, logger *zap.Logger) (*ChromeDPDriver, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", config.Headless),
		chromedp.WindowSize(config.ViewportWidth, config.ViewportHeight),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	if config.UserAgent != "" {
		opts = append(opts, chromedp.UserAgent(config.UserAgent))
	}
	if config.ProxyURL != "" {
		opts = append(opts, chromedp.ProxyServer(config.ProxyURL))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(func(format string, args ...any) {
			logger.Debug(fmt.Sprintf(format, args...))
		}),
	)

	// 设置超时
	if config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
	}

	driver := &ChromeDPDriver{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		ctx:         ctx,
		cancel:      cancel,
		config:      config,
		logger:      logger.With(zap.String("component", "chromedp_driver")),
	}

	// 启动浏览器
	if err := chromedp.Run(ctx); err != nil {
		allocCancel()
		cancel()
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	logger.Info("chromedp browser started",
		zap.Bool("headless", config.Headless),
		zap.Int("viewport_w", config.ViewportWidth),
		zap.Int("viewport_h", config.ViewportHeight))

	return driver, nil
}

// Navigate 导航到 URL
func (d *ChromeDPDriver) Navigate(ctx context.Context, url string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Debug("navigating", zap.String("url", url))
	return chromedp.Run(d.ctx, chromedp.Navigate(url))
}

// Screenshot 截取页面截图
func (d *ChromeDPDriver) Screenshot(ctx context.Context) (*Screenshot, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var buf []byte
	if err := chromedp.Run(d.ctx, chromedp.FullScreenshot(&buf, 90)); err != nil {
		return nil, fmt.Errorf("screenshot failed: %w", err)
	}

	var currentURL string
	if err := chromedp.Run(d.ctx, chromedp.Location(&currentURL)); err != nil {
		currentURL = "unknown"
	}

	return &Screenshot{
		Data:      buf,
		Width:     d.config.ViewportWidth,
		Height:    d.config.ViewportHeight,
		Timestamp: time.Now(),
		URL:       currentURL,
	}, nil
}

// Click 点击指定坐标
func (d *ChromeDPDriver) Click(ctx context.Context, x, y int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Debug("clicking", zap.Int("x", x), zap.Int("y", y))
	return chromedp.Run(d.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(
				input.MousePressed,
				float64(x), float64(y),
			).WithButton(input.Left).WithClickCount(1).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(
				input.MouseReleased,
				float64(x), float64(y),
			).WithButton(input.Left).WithClickCount(1).Do(ctx)
		}),
	)
}

// Type 输入文本
func (d *ChromeDPDriver) Type(ctx context.Context, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Debug("typing", zap.String("text", text))
	return chromedp.Run(d.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			for _, ch := range text {
				if err := input.DispatchKeyEvent(input.KeyChar).
					WithText(string(ch)).Do(ctx); err != nil {
					return err
				}
			}
			return nil
		}),
	)
}

// Scroll 滚动页面
func (d *ChromeDPDriver) Scroll(ctx context.Context, deltaX, deltaY int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Debug("scrolling", zap.Int("deltaX", deltaX), zap.Int("deltaY", deltaY))
	return chromedp.Run(d.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseWheel, 0, 0).
				WithDeltaX(float64(deltaX)).
				WithDeltaY(float64(deltaY)).Do(ctx)
		}),
	)
}

// GetURL 获取当前 URL
func (d *ChromeDPDriver) GetURL(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var url string
	if err := chromedp.Run(d.ctx, chromedp.Location(&url)); err != nil {
		return "", fmt.Errorf("failed to get URL: %w", err)
	}
	return url, nil
}

// Close 关闭浏览器
func (d *ChromeDPDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Info("closing chromedp browser")
	d.cancel()
	d.allocCancel()
	return nil
}

// --- ChromeDPBrowser 实现 Browser 接口（browser.go 中定义的） ---

// ChromeDPBrowser 实现 Browser 接口
type ChromeDPBrowser struct {
	driver *ChromeDPDriver
	config BrowserConfig
	logger *zap.Logger
}

// NewChromeDPBrowser 创建 ChromeDPBrowser
func NewChromeDPBrowser(config BrowserConfig, logger *zap.Logger) (*ChromeDPBrowser, error) {
	driver, err := NewChromeDPDriver(config, logger)
	if err != nil {
		return nil, err
	}
	return &ChromeDPBrowser{
		driver: driver,
		config: config,
		logger: logger,
	}, nil
}

// Execute 执行浏览器命令
func (b *ChromeDPBrowser) Execute(ctx context.Context, cmd BrowserCommand) (*BrowserResult, error) {
	start := time.Now()
	result := &BrowserResult{
		Action: cmd.Action,
	}

	var err error
	switch cmd.Action {
	case ActionNavigate:
		err = b.driver.Navigate(ctx, cmd.Value)
	case ActionClick:
		if cmd.Selector != "" {
			err = b.clickBySelector(ctx, cmd.Selector)
		}
	case ActionType:
		if cmd.Selector != "" {
			err = b.typeBySelector(ctx, cmd.Selector, cmd.Value)
		} else {
			err = b.driver.Type(ctx, cmd.Value)
		}
	case ActionScreenshot:
		screenshot, sErr := b.driver.Screenshot(ctx)
		if sErr != nil {
			err = sErr
		} else {
			result.Screenshot = screenshot.Data
		}
	case ActionScroll:
		err = b.driver.Scroll(ctx, 0, 300) // 默认向下滚动
	case ActionWait:
		if cmd.Selector != "" {
			err = chromedp.Run(b.driver.ctx,
				chromedp.WaitVisible(cmd.Selector, chromedp.ByQuery))
		}
	case ActionExtract:
		var text string
		err = chromedp.Run(b.driver.ctx,
			chromedp.Text(cmd.Selector, &text, chromedp.ByQuery))
		if err == nil {
			result.Data = []byte(fmt.Sprintf(`{"text":%q}`, text))
		}
	case ActionBack:
		err = chromedp.Run(b.driver.ctx, chromedp.NavigateBack())
	case ActionForward:
		err = chromedp.Run(b.driver.ctx, chromedp.NavigateForward())
	case ActionRefresh:
		err = chromedp.Run(b.driver.ctx, chromedp.Reload())
	default:
		err = fmt.Errorf("unsupported action: %s", cmd.Action)
	}

	result.Duration = time.Since(start)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		if b.config.ScreenshotOnError {
			if ss, ssErr := b.driver.Screenshot(ctx); ssErr == nil {
				result.Screenshot = ss.Data
			}
		}
		return result, err
	}

	result.Success = true
	if url, urlErr := b.driver.GetURL(ctx); urlErr == nil {
		result.URL = url
	}

	return result, nil
}

func (b *ChromeDPBrowser) clickBySelector(ctx context.Context, selector string) error {
	return chromedp.Run(b.driver.ctx, chromedp.Click(selector, chromedp.ByQuery))
}

func (b *ChromeDPBrowser) typeBySelector(ctx context.Context, selector, text string) error {
	return chromedp.Run(b.driver.ctx,
		chromedp.Clear(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, text, chromedp.ByQuery),
	)
}

// GetState 获取页面状态
func (b *ChromeDPBrowser) GetState(ctx context.Context) (*PageState, error) {
	state := &PageState{}

	// 获取 URL
	if url, err := b.driver.GetURL(ctx); err == nil {
		state.URL = url
	}

	// 获取 Title
	var title string
	if err := chromedp.Run(b.driver.ctx, chromedp.Title(&title)); err == nil {
		state.Title = title
	}

	// 获取页面 HTML 内容
	var content string
	if err := chromedp.Run(b.driver.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			node, err := dom.GetDocument().Do(ctx)
			if err != nil {
				return err
			}
			content, err = dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
			return err
		}),
	); err == nil {
		// 截断过长内容
		if len(content) > 10000 {
			content = content[:10000] + "..."
		}
		state.Content = content
	}

	return state, nil
}

// Close 关闭浏览器
func (b *ChromeDPBrowser) Close() error {
	return b.driver.Close()
}
