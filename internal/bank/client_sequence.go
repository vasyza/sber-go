package bank

import (
	"context"
	"sync"
	"sync/atomic"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

// MutationSequence holds the operation gate for the WHOLE workflow against
// reads, PIN renewal, other writes and transport cleanup. Only the supplied
// sender may be used inside it. Guarded APIs (PostRead/Mutate/Close/WarmUp) must
// not be called recursively from the callback. ExportSession is safe there.
// The capability is invalidated at callback return, including panic. A failed
// send poisons the sequence: ignoring its error never enables another POST.
func (c *SberClient) MutationSequence(ctx context.Context, callback func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error {
	if !c.core().options.AllowMutations {
		return &sdkErrs.TransportError{Code: "mutation_disabled"}
	}
	if callback == nil {
		return &sdkErrs.TransportError{Code: "invalid_mutation_sequence"}
	}
	ctx, done, e := c.enter(ctx)
	if e != nil {
		return e
	}
	defer done()
	var sendMu sync.Mutex
	var active atomic.Bool
	active.Store(true)
	var failed error
	defer func() {
		active.Store(false)
		// Wait for an in-flight sender before returning from the sequence.
		sendMu.Lock()
		defer sendMu.Unlock()
	}()
	send := func(requestCtx context.Context, path string, payload map[string]any, query map[string]string, pageID string, workflow bool) (map[string]any, error) {
		sendMu.Lock()
		defer sendMu.Unlock()
		if !active.Load() {
			return nil, &sdkErrs.TransportError{Code: "mutation_sequence_closed"}
		}
		if failed != nil {
			return nil, failed
		}
		if requestCtx == nil {
			failed = &sdkErrs.TransportError{Code: "invalid_context"}
			return nil, failed
		}
		child, cancel := context.WithCancel(requestCtx)
		stop := context.AfterFunc(ctx, cancel)
		defer stop()
		defer cancel()
		// AfterFunc is asynchronous; an already-canceled outer scope must never send.
		if ctx.Err() != nil {
			cancel()
		}
		data, err := c.mutateOnce(child, path, payload, query, pageID, workflow)
		err = clientDiagnostic(err)
		if err != nil {
			failed = err
		}
		return data, err
	}
	callbackError := callback(send)
	active.Store(false)
	sendMu.Lock()
	defer sendMu.Unlock()
	// A callback cannot overwrite an established send outcome, including its
	// uncertainty and cancellation identity. Do not retain unrelated private
	// callback errors: the recorded, already-sanitized sender failure dominates.
	if failed != nil {
		return failed
	}
	if callbackError != nil {
		return clientSafeError(callbackError)
	}
	return c.check(ctx)
}
