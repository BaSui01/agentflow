// 配置文件变更监听器实现。
//
// 基于文件系统事件与轮询兜底机制触发配置重载回调。
package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// --- 文件监听器类型定义 ---

// FileWatcher 监视配置文件的更改
type FileWatcher struct {
	mu sync.RWMutex

	// 配置
	paths         []string
	debounceDelay time.Duration

	// 状态
	running    bool
	stopChan   chan struct{}
	eventChan  chan FileEvent
	dispatchCh chan string // 通知 dispatchLoop 执行 dispatch 的 channel

	// 回调
	callbacks []func(event FileEvent)

	// 记录器
	logger *zap.Logger

	// 轮询回退的最后修改时间
	lastModTimes map[string]time.Time
}

// FileEvent 代表文件更改事件
type FileEvent struct {
	// Path是改变的文件路径
	Path string `json:"path"`

	// op 是操作类型
	Op FileOp `json:"op"`

	// 时间戳是事件发生的时间
	Timestamp time.Time `json:"timestamp"`

	// 检测过程中如有错误
	Error error `json:"error,omitempty"`
}

// FileOp 代表文件操作类型
type FileOp int

const (
	// FileOpCreate 表示文件已创建
	FileOpCreate FileOp = iota
	// FileOpWrite 指示文件已被修改
	FileOpWrite
	// FileOpRemove 表示文件已被删除
	FileOpRemove
	// FileOpRename 表示文件已重命名
	FileOpRename
	// FileOpChmod 表示文件权限已更改
	FileOpChmod
)

// String 返回 FileOp 的字符串表示形式
func (op FileOp) String() string {
	switch op {
	case FileOpCreate:
		return "CREATE"
	case FileOpWrite:
		return "WRITE"
	case FileOpRemove:
		return "REMOVE"
	case FileOpRename:
		return "RENAME"
	case FileOpChmod:
		return "CHMOD"
	default:
		return "UNKNOWN"
	}
}

// --- 文件监听器选项 ---

// WatcherOption 配置文件观察器
type WatcherOption func(*FileWatcher)

// WithDebounceDelay 设置文件事件的去抖延迟
func WithDebounceDelay(d time.Duration) WatcherOption {
	return func(w *FileWatcher) {
		w.debounceDelay = d
	}
}

// WithWatcherLogger 设置观察者的记录器
func WithWatcherLogger(logger *zap.Logger) WatcherOption {
	return func(w *FileWatcher) {
		w.logger = logger
	}
}

// --- 文件监听器实现 ---

// NewFileWatcher 创建一个新的文件观察器
func NewFileWatcher(paths []string, opts ...WatcherOption) (*FileWatcher, error) {
	w := &FileWatcher{
		paths:         paths,
		debounceDelay: 100 * time.Millisecond,
		stopChan:      make(chan struct{}),
		eventChan:     make(chan FileEvent, 100),
		dispatchCh:    make(chan string, 100),
		callbacks:     make([]func(FileEvent), 0),
		lastModTimes:  make(map[string]time.Time),
		logger:        zap.NewNop(),
	}

	for _, opt := range opts {
		opt(w)
	}

	// 验证路径是否存在
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				w.logger.Warn("Config file does not exist, will watch for creation",
					zap.String("path", path))
			} else {
				return nil, fmt.Errorf("failed to stat path %s: %w", path, err)
			}
		}
	}

	return w, nil
}

// OnChange 注册文件更改事件的回调
func (w *FileWatcher) OnChange(callback func(FileEvent)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, callback)
}

// Start 开始监视文件更改
func (w *FileWatcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return fmt.Errorf("watcher already running")
	}
	w.running = true
	w.mu.Unlock()

	// 初始化上次修改时间
	for _, path := range w.paths {
		if info, err := os.Stat(path); err == nil {
			w.lastModTimes[path] = info.ModTime()
		}
	}

	// 开始轮询goroutine（跨平台后备）
	go w.pollLoop(ctx)

	// 启动事件调度程序
	go w.dispatchLoop(ctx)

	w.logger.Info("File watcher started",
		zap.Strings("paths", w.paths),
		zap.Duration("debounce_delay", w.debounceDelay))

	return nil
}

// Stop 停止文件观察器
func (w *FileWatcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}

	close(w.stopChan)
	w.running = false

	w.logger.Info("File watcher stopped")
	return nil
}

// pollLoop 轮询文件是否有更改（没有 fsnotify 的系统的回退）
func (w *FileWatcher) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.checkFiles()
		}
	}
}

// checkFiles 检查所有监视的文件是否有修改
func (w *FileWatcher) checkFiles() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, path := range w.paths {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				// 检查文件之前是否被跟踪（已删除）
				if _, existed := w.lastModTimes[path]; existed {
					delete(w.lastModTimes, path)
					w.eventChan <- FileEvent{
						Path:      path,
						Op:        FileOpRemove,
						Timestamp: time.Now(),
					}
				}
			}
			continue
		}

		lastMod, existed := w.lastModTimes[path]
		if !existed {
			// 新文件已创建
			w.lastModTimes[path] = info.ModTime()
			w.eventChan <- FileEvent{
				Path:      path,
				Op:        FileOpCreate,
				Timestamp: time.Now(),
			}
		} else if info.ModTime().After(lastMod) {
			// 文件已修改
			w.lastModTimes[path] = info.ModTime()
			w.eventChan <- FileEvent{
				Path:      path,
				Op:        FileOpWrite,
				Timestamp: time.Now(),
			}
		}
	}
}

// dispatchLoop 将事件分派到具有去抖动功能的回调
func (w *FileWatcher) dispatchLoop(ctx context.Context) {
	var (
		pendingEvents = make(map[string]FileEvent)
		debounceTimer *time.Timer
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopChan:
			return
		case event := <-w.eventChan:
			// 存储事件（覆盖相同路径的先前事件）
			pendingEvents[event.Path] = event

			// 重置防抖定时器
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			// 回调通过 dispatchCh 通知主循环，而非直接操作 pendingEvents
			debounceTimer = time.AfterFunc(w.debounceDelay, func() {
				// 发送一个空字符串作为 "flush" 信号
				select {
				case w.dispatchCh <- "":
				default:
				}
			})
		case <-w.dispatchCh:
			// 在主循环中安全地读写 pendingEvents
			w.mu.RLock()
			callbacks := make([]func(FileEvent), len(w.callbacks))
			copy(callbacks, w.callbacks)
			w.mu.RUnlock()

			// 调度所有待处理事件
			for path, evt := range pendingEvents {
				w.logger.Debug("Dispatching file event",
					zap.String("path", path),
					zap.String("op", evt.Op.String()))

				for _, cb := range callbacks {
					cb(evt)
				}
			}

			// 清除待处理事件
			pendingEvents = make(map[string]FileEvent)
		}
	}
}

// AddPath 添加新的观看路径
func (w *FileWatcher) AddPath(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 检查是否已经观看
	for _, p := range w.paths {
		if p == path {
			return nil
		}
	}

	// 解析为绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	w.paths = append(w.paths, absPath)

	// 如果文件存在则初始化修改时间
	if info, err := os.Stat(absPath); err == nil {
		w.lastModTimes[absPath] = info.ModTime()
	}

	w.logger.Info("Added path to watcher", zap.String("path", absPath))
	return nil
}

// RemovePath 从观看中删除一条路径
func (w *FileWatcher) RemovePath(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	for i, p := range w.paths {
		if p == absPath {
			w.paths = append(w.paths[:i], w.paths[i+1:]...)
			delete(w.lastModTimes, absPath)
			w.logger.Info("Removed path from watcher", zap.String("path", absPath))
			return nil
		}
	}

	return fmt.Errorf("path not found: %s", path)
}

// Paths 返回监视路径的列表
func (w *FileWatcher) Paths() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	paths := make([]string, len(w.paths))
	copy(paths, w.paths)
	return paths
}

// IsRunning 返回观察者是否正在运行
func (w *FileWatcher) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}
