// Copyright (c) 2026 sber-mcp contributors. MIT; see LICENSE and NOTICE.
package bank

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

// ClientOptions configures one owned client. SessionPath is an explicit private
// persistence destination; options never relax the bank session's privileges.
type ClientTransportFactory func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error)

// ClientRenewal is an injected one-attempt PIN flow. It must honor context and
// never retry/replay credential POSTs. The bundle is a detached live snapshot.
type ClientRenewal func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error)
type ClientOptions struct {
	Transport        sdkTransport.Transport
	TransportFactory ClientTransportFactory
	TransportOptions sdkTransport.TransportOptions
	// AllowMutations is application policy, not bank-enforced read-only scope.
	AllowMutations          bool
	SessionPath             string
	Monotonic               func() time.Time
	Renewal                 ClientRenewal
	AuthOptions             sdkAuth.AuthOptions
	BrowserBootstrap        sdkTransport.BrowserBootstrapProvider
	BrowserBootstrapTimeout time.Duration
}

func (ClientOptions) String() string               { return "ClientOptions(<redacted>)" }
func (o ClientOptions) Format(f fmt.State, v rune) { sdkErrs.FormatError(f, o.String()) }
func (ClientOptions) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

// SberClient owns a rotating native transport. ExportSession/ExportCredentials
// are explicit sensitive reads; ordinary formatting and JSON stay redacted.
// Copies share the same private lifetime and locks; no lock or secret is copied.
// The second pointer is a terminal fmt boundary even when redactors are bypassed.
type SberClient struct{ state **clientState }

func (c SberClient) core() *clientState { return *c.state }

type clientState struct {
	mu                    sync.Mutex
	gate                  chan struct{}
	root                  context.Context
	cancel                context.CancelFunc
	closeMu               sync.Mutex
	bundle                sdkSession.SessionBundle
	transport             sdkTransport.Transport
	apiResolved           bool
	closed, closeComplete bool
	sessionPath           string
	credentialsOnly       bool
	options               ClientOptions
	defaultPINRenewal     bool
	lastWarmup            *time.Time
	pinProvider           sdkAuth.PINProvider
	reauthEpoch           uint64
	currentOwned          *clientOwnedTransport
	owned                 []*clientOwnedTransport
	// Terminal pointer storage avoids traversing the client/resource cycle in fmt.
	resources **Resources
}

func NewSberClient(bundle sdkSession.SessionBundle, o ClientOptions) (*SberClient, error) {
	return clientNewSberClient(bundle, o, nil)
}

func clientNewSberClient(bundle sdkSession.SessionBundle, o ClientOptions, initial *clientInitialOwners) (*SberClient, error) {
	defaultPIN := o.Renewal == nil
	o, e := clientNormalizeOptions(o)
	if e != nil {
		return nil, e
	}
	b, e := bundle.Clone()
	if e != nil {
		return nil, e
	}
	if _, e = b.ToSeed(true); e != nil {
		return nil, e
	}
	tr := o.Transport
	if tr == nil {
		factoryBundle, _ := b.Clone()
		tr, e = o.TransportFactory(factoryBundle, o.TransportOptions)
	}
	var owned *clientOwnedTransport
	var owners []*clientOwnedTransport
	if clientTransportPresent(tr) {
		if initial != nil {
			var conflict error
			owned, owners, conflict = initial.adopt(tr)
			if conflict != nil {
				// A borrowed or previously closed alias is not acquired/discarded.
				return nil, conflict
			}
		} else {
			owned = clientNewOwnedTransport(tr)
			owners = []*clientOwnedTransport{owned}
		}
	}
	if e != nil {
		return nil, clientDiscardOwnedConstruction(owned, clientSafeError(e))
	}
	if !clientTransportPresent(tr) || tr.CookieJar() == nil {
		return nil, clientDiscardOwnedConstruction(owned, &sdkErrs.MissingSession{Message: "transport cookie jar required"})
	}
	root, cancel := context.WithCancel(context.Background())
	o.Transport = nil
	state := &clientState{bundle: b, transport: tr, apiResolved: b.APIBase != sdkSession.AppOrigin, sessionPath: o.SessionPath, options: o, defaultPINRenewal: defaultPIN, root: root, cancel: cancel, gate: make(chan struct{}, 1), currentOwned: owned, owned: owners}
	c := &SberClient{state: &state}
	requester := &clientResourceRequester{client: &c}
	resources := NewResources(requester, ResourceOptions{AllowMutations: o.AllowMutations})
	c.core().resources = &resources
	return c, nil
}
func (c SberClient) String() string               { return "SberClient(<redacted>)" }
func (c SberClient) GoString() string             { return c.String() }
func (c SberClient) Format(f fmt.State, v rune)   { sdkErrs.FormatError(f, c.String()) }
func (c SberClient) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }
func (c *SberClient) ExportSession() (sdkSession.SessionBundle, error) {
	c.core().mu.Lock()
	defer c.core().mu.Unlock()
	if c.core().closed {
		return sdkSession.SessionBundle{}, sdkErrs.ErrClosed
	}
	return c.core().bundle.WithCookieJar(c.core().transport.CookieJar())
}
func (c *SberClient) ExportCredentials() (sdkSession.SberCredentials, error) {
	b, e := c.ExportSession()
	if e != nil {
		return sdkSession.SberCredentials{}, e
	}
	return sdkSession.CredentialsFromBundle(b)
}
func (c *SberClient) Close() error {
	c.core().mu.Lock()
	c.core().closed = true
	c.core().pinProvider = nil
	c.core().cancel()
	c.core().mu.Unlock()
	c.core().closeMu.Lock()
	defer c.core().closeMu.Unlock()
	c.core().gate <- struct{}{}
	defer func() { <-c.core().gate }()
	c.core().mu.Lock()
	complete := c.core().closeComplete
	owned := append([]*clientOwnedTransport(nil), c.core().owned...)
	c.core().mu.Unlock()
	if complete {
		return nil
	}
	failed := false
	for i := len(owned) - 1; i >= 0; i-- {
		if e := owned[i].close(); e != nil {
			failed = true
		}
	}
	if failed {
		return &sdkErrs.TransportError{Code: "close_failed"}
	}
	c.core().mu.Lock()
	c.core().closeComplete = true
	c.core().mu.Unlock()
	return nil
}
