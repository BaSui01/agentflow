package execution

import "context"

// Func is the core execution function signature.
type Func[In any, Out any] func(ctx context.Context, input In) (Out, error)

// Middleware wraps a Func, adding pre/post processing.
type Middleware[In any, Out any] func(ctx context.Context, input In, next Func[In, Out]) (Out, error)

// Pipeline chains middlewares around a core Func.
type Pipeline[In any, Out any] struct {
	middlewares []Middleware[In, Out]
	core        Func[In, Out]
}

func NewPipeline[In any, Out any](core Func[In, Out]) *Pipeline[In, Out] {
	return &Pipeline[In, Out]{core: core}
}

func (p *Pipeline[In, Out]) Use(mws ...Middleware[In, Out]) {
	p.middlewares = append(p.middlewares, mws...)
}

func (p *Pipeline[In, Out]) Execute(ctx context.Context, input In) (Out, error) {
	fn := p.core
	for i := len(p.middlewares) - 1; i >= 0; i-- {
		mw := p.middlewares[i]
		next := fn
		fn = func(ctx context.Context, input In) (Out, error) {
			return mw(ctx, input, next)
		}
	}
	return fn(ctx, input)
}
