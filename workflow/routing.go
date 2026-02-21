package workflow

import (
	"context"
	"fmt"
	"sync"
)

// Router 路由器接口
// 根据输入决定使用哪个处理器
type Router interface {
	// Route 路由决策，返回路由键
	Route(ctx context.Context, input any) (string, error)
}

// RouterFunc 路由函数类型
type RouterFunc func(ctx context.Context, input any) (string, error)

// FuncRouter 函数路由器
type FuncRouter struct {
	fn RouterFunc
}

// NewFuncRouter 创建函数路由器
func NewFuncRouter(fn RouterFunc) *FuncRouter {
	return &FuncRouter{fn: fn}
}

func (r *FuncRouter) Route(ctx context.Context, input any) (string, error) {
	return r.fn(ctx, input)
}

// Handler 处理器接口
type Handler interface {
	Runnable
	// Name 返回处理器名称
	Name() string
}

// HandlerFunc 处理器函数类型
type HandlerFunc func(ctx context.Context, input any) (any, error)

// FuncHandler 函数处理器
type FuncHandler struct {
	name string
	fn   HandlerFunc
}

// NewFuncHandler 创建函数处理器
func NewFuncHandler(name string, fn HandlerFunc) *FuncHandler {
	return &FuncHandler{
		name: name,
		fn:   fn,
	}
}

func (h *FuncHandler) Execute(ctx context.Context, input any) (any, error) {
	return h.fn(ctx, input)
}

func (h *FuncHandler) Name() string {
	return h.name
}

// RoutingWorkflow 路由工作流
// 根据输入分类，将任务路由到专门的处理器
type RoutingWorkflow struct {
	name         string
	description  string
	router       Router
	handlers     map[string]Handler
	defaultRoute string
	// mu protects handlers map against concurrent RegisterHandler (write) and
	// Route/Execute (read) calls. Bug fix (P0): without this lock, concurrent
	// access to the handlers map causes a panic.
	mu sync.RWMutex
}

// NewRoutingWorkflow 创建路由工作流
func NewRoutingWorkflow(name, description string, router Router) *RoutingWorkflow {
	return &RoutingWorkflow{
		name:        name,
		description: description,
		router:      router,
		handlers:    make(map[string]Handler),
	}
}

// RegisterHandler 注册处理器
func (w *RoutingWorkflow) RegisterHandler(route string, handler Handler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[route] = handler
}

// SetDefaultRoute 设置默认路由
func (w *RoutingWorkflow) SetDefaultRoute(route string) {
	w.defaultRoute = route
}

// Execute 执行路由工作流
// 1. 使用路由器决定路由
// 2. 查找对应的处理器
// 3. 执行处理器
func (w *RoutingWorkflow) Execute(ctx context.Context, input any) (any, error) {
	// 1. 路由决策
	route, err := w.router.Route(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("routing failed: %w", err)
	}

	// 2. 查找处理器 — read lock protects concurrent access to handlers map
	w.mu.RLock()
	handler, ok := w.handlers[route]
	if !ok {
		// 尝试使用默认路由
		if w.defaultRoute != "" {
			handler, ok = w.handlers[w.defaultRoute]
			if !ok {
				w.mu.RUnlock()
				return nil, fmt.Errorf("no handler for route: %s (default route also not found)", route)
			}
		} else {
			w.mu.RUnlock()
			return nil, fmt.Errorf("no handler for route: %s", route)
		}
	}
	w.mu.RUnlock()

	// 3. 执行处理器
	result, err := handler.Execute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("handler %s failed: %w", handler.Name(), err)
	}

	return result, nil
}

func (w *RoutingWorkflow) Name() string {
	return w.name
}

func (w *RoutingWorkflow) Description() string {
	return w.description
}

// Routes 返回所有已注册的路由
func (w *RoutingWorkflow) Routes() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	routes := make([]string, 0, len(w.handlers))
	for route := range w.handlers {
		routes = append(routes, route)
	}
	return routes
}
