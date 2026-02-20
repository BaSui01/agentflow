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

// FileWatcher watches configuration files for changes
type FileWatcher struct {
	mu sync.RWMutex

	// 配置
	paths         []string
	debounceDelay time.Duration

	// 状态
	running   bool
	stopChan  chan struct{}
	eventChan chan FileEvent

	// 回调
	callbacks []func(event FileEvent)

	// 记录器
	logger *zap.Logger

	// 轮询回退的最后修改时间
	lastModTimes map[string]time.Time
}

// FileEvent represents a file change event
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

// FileOp represents file operation types
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

// String returns the string representation of FileOp
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

// WatcherOption configures the FileWatcher
type WatcherOption func(*FileWatcher)

// WithDebounceDelay sets the debounce delay for file events
func WithDebounceDelay(d time.Duration) WatcherOption {
	return func(w *FileWatcher) {
		w.debounceDelay = d
	}
}

// WithWatcherLogger sets the logger for the watcher
func WithWatcherLogger(logger *zap.Logger) WatcherOption {
	return func(w *FileWatcher) {
		w.logger = logger
	}
}

// --- 文件监听器实现 ---

// NewFileWatcher creates a new file watcher
func NewFileWatcher(paths []string, opts ...WatcherOption) (*FileWatcher, error) {
	w := &FileWatcher{
		paths:         paths,
		debounceDelay: 100 * time.Millisecond,
		stopChan:      make(chan struct{}),
		eventChan:     make(chan FileEvent, 100),
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

// OnChange registers a callback for file change events
func (w *FileWatcher) OnChange(callback func(FileEvent)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, callback)
}

// Start begins watching for file changes
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

// Stop stops the file watcher
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

// pollLoop polls files for changes (fallback for systems without fsnotify)
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

// checkFiles checks all watched files for modifications
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

// dispatchLoop dispatches events to callbacks with debouncing
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
			debounceTimer = time.AfterFunc(w.debounceDelay, func() {
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
			})
		}
	}
}

// AddPath adds a new path to watch
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

// RemovePath removes a path from watching
func (w *FileWatcher) RemovePath(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	absPath, _ := filepath.Abs(path)

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

// Paths returns the list of watched paths
func (w *FileWatcher) Paths() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	paths := make([]string, len(w.paths))
	copy(paths, w.paths)
	return paths
}

// IsRunning returns whether the watcher is running
func (w *FileWatcher) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}
