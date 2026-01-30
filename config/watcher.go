// =============================================================================
// AgentFlow Configuration File Watcher
// =============================================================================
// Watches configuration files for changes and triggers reload callbacks.
// Uses fsnotify for cross-platform file system notifications.
// =============================================================================
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

// =============================================================================
// File Watcher Types
// =============================================================================

// FileWatcher watches configuration files for changes
type FileWatcher struct {
	mu sync.RWMutex

	// Configuration
	paths         []string
	debounceDelay time.Duration

	// State
	running   bool
	stopChan  chan struct{}
	eventChan chan FileEvent

	// Callbacks
	callbacks []func(event FileEvent)

	// Logger
	logger *zap.Logger

	// Last modification times for polling fallback
	lastModTimes map[string]time.Time
}

// FileEvent represents a file change event
type FileEvent struct {
	// Path is the file path that changed
	Path string `json:"path"`

	// Op is the operation type
	Op FileOp `json:"op"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Error if any occurred during detection
	Error error `json:"error,omitempty"`
}

// FileOp represents file operation types
type FileOp int

const (
	// FileOpCreate indicates a file was created
	FileOpCreate FileOp = iota
	// FileOpWrite indicates a file was modified
	FileOpWrite
	// FileOpRemove indicates a file was removed
	FileOpRemove
	// FileOpRename indicates a file was renamed
	FileOpRename
	// FileOpChmod indicates file permissions changed
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

// =============================================================================
// File Watcher Options
// =============================================================================

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

// =============================================================================
// File Watcher Implementation
// =============================================================================

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

	// Validate paths exist
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

	// Initialize last modification times
	for _, path := range w.paths {
		if info, err := os.Stat(path); err == nil {
			w.lastModTimes[path] = info.ModTime()
		}
	}

	// Start polling goroutine (cross-platform fallback)
	go w.pollLoop(ctx)

	// Start event dispatcher
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
				// Check if file was previously tracked (removed)
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
			// New file created
			w.lastModTimes[path] = info.ModTime()
			w.eventChan <- FileEvent{
				Path:      path,
				Op:        FileOpCreate,
				Timestamp: time.Now(),
			}
		} else if info.ModTime().After(lastMod) {
			// File modified
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
			// Store event (overwrites previous for same path)
			pendingEvents[event.Path] = event

			// Reset debounce timer
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(w.debounceDelay, func() {
				w.mu.RLock()
				callbacks := make([]func(FileEvent), len(w.callbacks))
				copy(callbacks, w.callbacks)
				w.mu.RUnlock()

				// Dispatch all pending events
				for path, evt := range pendingEvents {
					w.logger.Debug("Dispatching file event",
						zap.String("path", path),
						zap.String("op", evt.Op.String()))

					for _, cb := range callbacks {
						cb(evt)
					}
				}

				// Clear pending events
				pendingEvents = make(map[string]FileEvent)
			})
		}
	}
}

// AddPath adds a new path to watch
func (w *FileWatcher) AddPath(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if already watching
	for _, p := range w.paths {
		if p == path {
			return nil
		}
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	w.paths = append(w.paths, absPath)

	// Initialize modification time if file exists
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
