package core

import "fmt"

// ErrInvalidTransition indicates an invalid lifecycle transition.
type ErrInvalidTransition struct {
	From State
	To   State
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid state transition: %s -> %s", e.From, e.To)
}
