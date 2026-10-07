package enrollment

import "context"

// Callback errors and panic values may contain owner data. Never wrap, format,
// inspect, or propagate them. These guards cover synchronous callbacks only.
func prepareCandidate(ctx context.Context, prepare Prepare) (writer CandidateWriter, err error) {
	defer func() {
		if recover() != nil {
			writer = nil
			err = ErrPrepare
		}
	}()
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	writer, e := prepare(ctx)
	if canceled := ctx.Err(); canceled != nil {
		return nil, canceled
	}
	if e != nil || writer == nil {
		return nil, ErrPrepare
	}
	return writer, nil
}
func writeCandidate(ctx context.Context, path string, writer CandidateWriter) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrWrite
		}
	}()
	if e := ctx.Err(); e != nil {
		return e
	}
	e := writer(ctx, path)
	if canceled := ctx.Err(); canceled != nil {
		return canceled
	}
	if e != nil {
		return ErrWrite
	}
	return nil
}
