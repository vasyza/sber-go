package bank

import (
	"context"
	"sync"

	sdkAuth "github.com/vasyza/sber-sdk/internal/auth"
	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
	sdkTransport "github.com/vasyza/sber-sdk/internal/transport"
)

// Normalize direct and factory injection behind the same ownership gate. The
// explicit transport is used only for auth's first construction; subsequent
// authentication processes still use the caller's original factory.
func clientGuardAuthOptions(o sdkAuth.AuthOptions, claim func(sdkTransport.Transport) (*clientOwnedTransport, error)) sdkAuth.AuthOptions {
	injected, factory := o.Transport, o.TransportFactory
	var mu sync.Mutex
	first := true
	o.Transport = nil
	o.TransportFactory = func(b sdkSession.SessionBundle, to sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		mu.Lock()
		tr := sdkTransport.Transport(nil)
		if first {
			tr = injected
			first = false
		}
		mu.Unlock()
		var err error
		if tr == nil {
			tr, err = factory(b, to)
		}
		if !clientTransportPresent(tr) {
			if err != nil {
				return nil, clientSafeError(err)
			}
			return nil, &sdkErrs.MissingSession{Message: "auth transport cookie jar required"}
		}
		// Validate before acquiring cleanup ownership, even when a factory
		// returned both a transport and an error. Never close a borrowed alias.
		owner, ownershipError := claim(tr)
		if ownershipError != nil {
			return nil, ownershipError
		}
		if err == nil && tr.CookieJar() == nil {
			err = &sdkErrs.MissingSession{Message: "auth transport cookie jar required"}
		}
		if err != nil {
			primary := clientSafeError(err)
			if owner.close() != nil {
				return nil, &ClientCleanupError{cause: primary, cleanup: owner.close}
			}
			return nil, primary
		}
		return &clientAuthTransport{owner: &owner}, nil
	}
	return o
}

// Auth and client cleanup share this one closing owner. Auth owns this private
// forwarding handle, not a second independently closing reference to tr. The
// terminal pointer also prevents fallback diagnostics from traversing tr.
type clientAuthTransport struct{ owner **clientOwnedTransport }

func (t *clientAuthTransport) Get(c context.Context, u string, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	return (*t.owner).transport.Get(c, u, o)
}
func (t *clientAuthTransport) Post(c context.Context, u string, p map[string]any, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	return (*t.owner).transport.Post(c, u, p, o)
}
func (t *clientAuthTransport) PostForm(c context.Context, u string, p map[string]string, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	return (*t.owner).transport.PostForm(c, u, p, o)
}
func (t *clientAuthTransport) CookieJar() *sdkSession.CookieJar {
	return (*t.owner).transport.CookieJar()
}
func (t *clientAuthTransport) Close() error { return (*t.owner).close() }
func (t *clientAuthTransport) ClientTransportOwner() sdkTransport.Transport {
	return (*t.owner).identity
}

func (c *SberClient) claimAuthTransport(tr sdkTransport.Transport) (*clientOwnedTransport, error) {
	owner := clientNewOwnedTransport(tr)
	c.core().mu.Lock()
	defer c.core().mu.Unlock()
	if clientOwnershipConflict(owner, c.core().owned) {
		return nil, &sdkErrs.TransportError{Code: "reused_transport"}
	}
	c.core().owned = append(c.core().owned, owner)
	return owner, nil
}

// A constructor-local ledger survives auth Close and is consumed by first
// business adoption. Reserved injections are borrowed, never cleanup owners.
// The final adoption seals the auth capability and transfers actual acquired
// owners to the client, including closed auth generations for later alias checks.
type clientInitialOwners struct {
	mu       sync.Mutex
	reserved []*clientOwnedTransport
	owned    []*clientOwnedTransport
	sealed   bool
}

func clientNewInitialOwners(reserved sdkTransport.Transport) *clientInitialOwners {
	s := &clientInitialOwners{}
	if clientTransportPresent(reserved) {
		s.reserved = []*clientOwnedTransport{clientNewOwnedTransport(reserved)}
	}
	return s
}
func (s *clientInitialOwners) claim(tr sdkTransport.Transport) (*clientOwnedTransport, error) {
	owner := clientNewOwnedTransport(tr)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sealed || clientOwnershipConflict(owner, s.reserved) || clientOwnershipConflict(owner, s.owned) {
		return nil, &sdkErrs.TransportError{Code: "reused_transport"}
	}
	s.owned = append(s.owned, owner)
	return owner, nil
}
func (s *clientInitialOwners) adopt(tr sdkTransport.Transport) (*clientOwnedTransport, []*clientOwnedTransport, error) {
	owner := clientNewOwnedTransport(tr)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sealed || clientOwnershipConflict(owner, s.reserved) || clientOwnershipConflict(owner, s.owned) {
		return nil, nil, &sdkErrs.TransportError{Code: "reused_transport"}
	}
	s.sealed = true
	s.owned = append(s.owned, owner)
	return owner, append([]*clientOwnedTransport(nil), s.owned...), nil
}

func clientOwnershipConflict(candidate *clientOwnedTransport, owners []*clientOwnedTransport) bool {
	if candidate.identity == nil {
		return true
	}
	for _, owner := range owners {
		if owner.identity == nil || owner.identity == candidate.identity || clientSameTransport(owner.transport, candidate.transport) {
			return true
		}
	}
	return false
}
