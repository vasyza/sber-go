package sber

import ("context"; "errors"; "sync"; "sync/atomic")

type independentRequester struct {
	mu        sync.Mutex
	sequence  sync.Mutex
	calls     []resourceCall
	replies   []map[string]any
	failures  []error
	allowed   bool
	closed    bool
	attempts  int
	sequences int
	hook      func(int)
}

var independentClosed = errors.New("synthetic requester closed")

func (r *independentRequester) send(ctx context.Context, c resourceCall) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.attempts++
	if r.closed {
		r.mu.Unlock()
		return nil, independentClosed
	}
	if c.Kind != "read" && !r.allowed {
		r.mu.Unlock()
		return nil, ErrResourceMutationsDisabled
	}
	r.calls = append(r.calls, c)
	n := len(r.calls)
	var p map[string]any
	var err error
	if len(r.replies) > 0 {
		p = r.replies[0]
		r.replies = r.replies[1:]
	}
	if len(r.failures) > 0 {
		err = r.failures[0]
		r.failures = r.failures[1:]
	}
	hook := r.hook
	r.mu.Unlock()
	if hook != nil {
		hook(n)
	}
	return p, err
}
func (r *independentRequester) PostRead(ctx context.Context, p string, b map[string]any) (map[string]any, error) {
	return r.send(ctx, resourceCall{Kind: "read", Path: p, Payload: b})
}
func (r *independentRequester) Mutate(ctx context.Context, p string, b map[string]any, q map[string]string, id string, w bool) (map[string]any, error) {
	r.sequence.Lock()
	defer r.sequence.Unlock()
	return r.send(ctx, resourceCall{Kind: "mutation", Path: p, Payload: b, Query: q, PageID: id, Workflow: w})
}
func (r *independentRequester) MutationSequence(ctx context.Context, f func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error {
	r.sequence.Lock()
	defer r.sequence.Unlock()
	r.mu.Lock()
	r.sequences++
	closed, allowed := r.closed, r.allowed
	r.mu.Unlock()
	if closed {
		return independentClosed
	}
	if !allowed {
		return ErrResourceMutationsDisabled
	}
	var active atomic.Bool
	active.Store(true)
	defer active.Store(false)
	return f(func(ctx context.Context, p string, b map[string]any, q map[string]string, id string, w bool) (map[string]any, error) {
		if !active.Load() {
			return nil, errors.New("synthetic sequence inactive")
		}
		return r.send(ctx, resourceCall{Kind: "sequence", Path: p, Payload: b, Query: q, PageID: id, Workflow: w})
	})
}
func (r *independentRequester) ExportSession() (SessionBundle, error) {
	return SessionBundle{}, independentClosed
}
func (r *independentRequester) ExportCredentials() (SberCredentials, error) {
	return SberCredentials{}, independentClosed
}
func (r *independentRequester) WarmUp(context.Context, bool) error { return independentClosed }
func (r *independentRequester) Close()                             { r.mu.Lock(); r.closed = true; r.mu.Unlock() }
