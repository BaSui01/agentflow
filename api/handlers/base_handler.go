package handlers

import (
	"reflect"
	"sync"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ServiceAccessor is the interface that any service type must implement
// to be used with BaseHandler. In practice, all service types are concrete
// types (including nil), so this is a marker interface.
type ServiceAccessor interface{}

// BaseHandler provides the common hot-reloadable service holder pattern
// used by all API handlers. It eliminates the repeated mu+service+logger
// boilerplate across 13+ handler types.
//
// Usage: embed BaseHandler[S] into your handler struct and delegate
// UpdateService / currentService to the embedded field.
type BaseHandler[S any] struct {
	mu      sync.RWMutex
	service S
	logger  *zap.Logger
}

// NewBaseHandler creates a new BaseHandler with the given service and logger.
func NewBaseHandler[S any](service S, logger *zap.Logger) BaseHandler[S] {
	return BaseHandler[S]{
		service: service,
		logger:  logger,
	}
}

// UpdateService swaps the handler's service in place so existing HTTP
// route bindings keep using the latest service after hot reload.
func (h *BaseHandler[S]) UpdateService(service S) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.service = service
}

// currentService returns the currently held service instance.
func (h *BaseHandler[S]) currentService() S {
	if h == nil {
		var zero S
		return zero
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.service
}

func serviceIsNil(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// currentServiceOrUnavailable returns the current service or a typed service-unavailable error.
func (h *BaseHandler[S]) currentServiceOrUnavailable(component string) (S, *types.Error) {
	service := h.currentService()
	if serviceIsNil(any(service)) {
		var zero S
		return zero, serviceUnavailableError(component)
	}
	return service, nil
}

// Logger returns the handler's logger.
func (h *BaseHandler[S]) Logger() *zap.Logger {
	if h == nil {
		return nil
	}
	return h.logger
}

// SetLogger sets the handler's logger.
func (h *BaseHandler[S]) SetLogger(logger *zap.Logger) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.logger = logger
}
