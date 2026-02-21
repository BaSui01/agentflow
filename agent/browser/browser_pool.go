package browser

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// BrowserPool 浏览器实例池
type BrowserPool struct {
	config    BrowserConfig
	pool      chan *ChromeDPBrowser
	active    map[*ChromeDPBrowser]bool
	maxSize   int
	minIdle   int
	logger    *zap.Logger
	mu        sync.Mutex
	closeOnce sync.Once
	closed    bool
}

// BrowserPoolConfig 浏览器池配置
type BrowserPoolConfig struct {
	MaxSize       int           `json:"max_size"`
	MinIdle       int           `json:"min_idle"`
	MaxIdleTime   time.Duration `json:"max_idle_time"`
	BrowserConfig BrowserConfig `json:"browser_config"`
}

// DefaultBrowserPoolConfig 默认池配置
func DefaultBrowserPoolConfig() BrowserPoolConfig {
	return BrowserPoolConfig{
		MaxSize:       5,
		MinIdle:       1,
		MaxIdleTime:   5 * time.Minute,
		BrowserConfig: DefaultBrowserConfig(),
	}
}

// NewBrowserPool 创建浏览器池
func NewBrowserPool(config BrowserPoolConfig, logger *zap.Logger) (*BrowserPool, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	pool := &BrowserPool{
		config:  config.BrowserConfig,
		pool:    make(chan *ChromeDPBrowser, config.MaxSize),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: config.MaxSize,
		minIdle: config.MinIdle,
		logger:  logger.With(zap.String("component", "browser_pool")),
	}

	// 预创建最小空闲实例
	for i := 0; i < config.MinIdle; i++ {
		browser, err := NewChromeDPBrowser(config.BrowserConfig, logger)
		if err != nil {
			pool.Close() // 清理已创建的
			return nil, fmt.Errorf("failed to pre-create browser %d: %w", i, err)
		}
		pool.pool <- browser
	}

	logger.Info("browser pool created",
		zap.Int("max_size", config.MaxSize),
		zap.Int("min_idle", config.MinIdle),
		zap.Int("pre_created", config.MinIdle))

	return pool, nil
}

// Acquire 获取一个浏览器实例
func (p *BrowserPool) Acquire(ctx context.Context) (*ChromeDPBrowser, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("browser pool is closed")
	}
	p.mu.Unlock()

	// 尝试从池中获取
	select {
	case browser := <-p.pool:
		p.mu.Lock()
		p.active[browser] = true
		p.mu.Unlock()
		p.logger.Debug("acquired browser from pool")
		return browser, nil
	default:
	}

	// 池为空，检查是否可以创建新实例
	p.mu.Lock()
	totalCount := len(p.active) + len(p.pool)
	if totalCount >= p.maxSize {
		p.mu.Unlock()
		// 等待可用实例
		p.logger.Debug("pool exhausted, waiting for available browser")
		select {
		case browser := <-p.pool:
			p.mu.Lock()
			p.active[browser] = true
			p.mu.Unlock()
			return browser, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	p.mu.Unlock()

	// 创建新实例
	browser, err := NewChromeDPBrowser(p.config, p.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create browser: %w", err)
	}

	p.mu.Lock()
	p.active[browser] = true
	p.mu.Unlock()

	p.logger.Debug("created new browser instance")
	return browser, nil
}

// Release 归还浏览器实例
func (p *BrowserPool) Release(browser *ChromeDPBrowser) {
	p.mu.Lock()
	delete(p.active, browser)

	if p.closed {
		p.mu.Unlock()
		_ = browser.Close()
		return
	}

	// 在锁保护范围内尝试放回池中，防止 Close() 在解锁后关闭 channel 导致 panic
	select {
	case p.pool <- browser:
		p.mu.Unlock()
		p.logger.Debug("browser returned to pool")
	default:
		p.mu.Unlock()
		// 池满，关闭多余实例
		_ = browser.Close()
		p.logger.Debug("pool full, closing excess browser")
	}
}

// Close 关闭浏览器池
func (p *BrowserPool) Close() error {
	p.mu.Lock()
	p.closed = true
	// 关闭所有活跃实例
	for browser := range p.active {
		_ = browser.Close()
	}
	p.active = make(map[*ChromeDPBrowser]bool)
	// 在锁内关闭 channel，确保 Release 不会向已关闭的 channel 发送
	p.closeOnce.Do(func() { close(p.pool) })
	p.mu.Unlock()

	// 排空池中的实例
	for browser := range p.pool {
		_ = browser.Close()
	}

	p.logger.Info("browser pool closed")
	return nil
}

// Stats 返回池统计信息
func (p *BrowserPool) Stats() (idle, active, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	idle = len(p.pool)
	active = len(p.active)
	total = idle + active
	return
}

// ChromeDPBrowserFactory 实现 BrowserFactory 接口
type ChromeDPBrowserFactory struct {
	logger *zap.Logger
}

// NewChromeDPBrowserFactory 创建工厂
func NewChromeDPBrowserFactory(logger *zap.Logger) *ChromeDPBrowserFactory {
	return &ChromeDPBrowserFactory{logger: logger}
}

// Create 创建浏览器实例
func (f *ChromeDPBrowserFactory) Create(config BrowserConfig) (Browser, error) {
	return NewChromeDPBrowser(config, f.logger)
}
