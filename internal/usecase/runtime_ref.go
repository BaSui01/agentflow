package usecase

import "sync/atomic"

// RuntimeRef provides a tiny application-layer holder for hot-reloadable runtime state.
// Phase-1 only introduces the shared contract; concrete runtime slices migrate to it later.
type RuntimeRef[T any] interface {
	Load() T
	Store(value T)
}

// AtomicRuntimeRef is the default RuntimeRef backed by atomic.Pointer.
type AtomicRuntimeRef[T any] struct {
	ptr atomic.Pointer[T]
}

func NewAtomicRuntimeRef[T any](initial T) *AtomicRuntimeRef[T] {
	ref := &AtomicRuntimeRef[T]{}
	ref.Store(initial)
	return ref
}

func (r *AtomicRuntimeRef[T]) Load() T {
	var zero T
	if r == nil {
		return zero
	}
	if ptr := r.ptr.Load(); ptr != nil {
		return *ptr
	}
	return zero
}

func (r *AtomicRuntimeRef[T]) Store(value T) {
	if r == nil {
		return
	}
	v := new(T)
	*v = value
	r.ptr.Store(v)
}
