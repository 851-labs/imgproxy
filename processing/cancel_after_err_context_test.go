package processing

import (
	"context"
	"time"
)

type cancelAfterErrContext struct {
	cancelOnErrCall int
	errCallCount    int
	deadline        func() (time.Time, bool)
	done            <-chan struct{}
	value           func(any) any
}

func newCancelAfterErrContext(ctx context.Context, cancelOnErrCall int) *cancelAfterErrContext {
	return &cancelAfterErrContext{
		cancelOnErrCall: cancelOnErrCall,
		deadline: func() (time.Time, bool) {
			return ctx.Deadline()
		},
		done: ctx.Done(),
		value: func(key any) any {
			return ctx.Value(key)
		},
	}
}

func (c *cancelAfterErrContext) Deadline() (deadline time.Time, ok bool) {
	return c.deadline()
}

func (c *cancelAfterErrContext) Done() <-chan struct{} {
	return c.done
}

func (c *cancelAfterErrContext) Err() error {
	c.errCallCount++
	if c.errCallCount >= c.cancelOnErrCall {
		return context.Canceled
	}

	return nil
}

func (c *cancelAfterErrContext) Value(key any) any {
	return c.value(key)
}
