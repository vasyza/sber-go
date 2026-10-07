package sber

import "context"

// SessionAPI exposes explicit session lifecycle calls; product/history reads
// never implicitly invoke WarmUp or export/save credentials.
type SessionAPI struct{ requester BusinessRequester }

func NewSessionAPI(requester BusinessRequester) *SessionAPI { return &SessionAPI{requester} }

// Export returns a defensive session snapshot. Only an explicitly supplied
// path invokes the existing private persistence contract.
func (a *SessionAPI) Export(path ...string) (SessionBundle, error) {
	if len(path) > 1 || (len(path) == 1 && path[0] == "") {
		return SessionBundle{}, NewParseError("path")
	}
	b, err := a.requester.ExportSession()
	if err != nil {
		return SessionBundle{}, err
	}
	b, err = b.Clone()
	if err != nil {
		return SessionBundle{}, err
	}
	if len(path) == 1 {
		if err = b.Save(path[0]); err != nil {
			return SessionBundle{}, err
		}
	}
	return b, nil
}
func (a *SessionAPI) Credentials() (SberCredentials, error) { return a.requester.ExportCredentials() }
func (a *SessionAPI) WarmUp(ctx context.Context, force ...bool) error {
	if len(force) > 1 {
		return NewParseError("force")
	}
	f := true
	if len(force) == 1 {
		f = force[0]
	}
	return a.requester.WarmUp(ctx, f)
}
