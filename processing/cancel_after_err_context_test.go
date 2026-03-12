package processing

import "context"

type cancelAfterErrContext struct {
	context.Context
	cancelOnErrCall int
	errCallCount    int
}

func newCancelAfterErrContext(ctx context.Context, cancelOnErrCall int) *cancelAfterErrContext {
	return &cancelAfterErrContext{
		Context:         ctx,
		cancelOnErrCall: cancelOnErrCall,
	}
}

func (c *cancelAfterErrContext) Err() error {
	c.errCallCount++
	if c.errCallCount >= c.cancelOnErrCall {
		return context.Canceled
	}

	return nil
}
