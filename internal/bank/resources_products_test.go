package bank

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	sdkSession "github.com/vasyza/sber-go/internal/session"
)

// resourceScript is an offline requester, not a bank/client-core substitute.
// All responses/identifiers in this file are explicitly synthetic.
type resourceCall struct {
	Kind, Path string
	Payload    map[string]any
	Query      map[string]string
	PageID     string
	Workflow   bool
}
type resourceStep struct {
	Call     resourceCall
	Response map[string]any
	Err      error
	Before   func(context.Context)
}
type resourceScript struct {
	t           *testing.T
	mu          sync.Mutex
	sequence    sync.Mutex
	steps       []resourceStep
	calls       []resourceCall
	warm        []bool
	bundle      sdkSession.SessionBundle
	credentials sdkSession.SberCredentials
	exportErr   error
	warmErr     error
	sequences   int
}

func resourceMap(text string) map[string]any {
	p, err := DecodeJSON(strings.NewReader(text))
	if err != nil {
		panic(err)
	}
	return p
}
func (r *resourceScript) send(ctx context.Context, c resourceCall) (map[string]any, error) {
	r.mu.Lock()
	r.calls = append(r.calls, c)
	if len(r.steps) == 0 {
		r.mu.Unlock()
		r.t.Errorf("unexpected synthetic call: %s %s", c.Kind, c.Path)
		return nil, errors.New("script exhausted")
	}
	step := r.steps[0]
	r.steps = r.steps[1:]
	r.mu.Unlock()
	if !reflect.DeepEqual(c, step.Call) {
		r.t.Errorf("exact request mismatch:\n got %#v\nwant %#v", c, step.Call)
	}
	if step.Before != nil {
		step.Before(ctx)
	}
	return step.Response, step.Err
}
func (r *resourceScript) PostRead(ctx context.Context, p string, b map[string]any) (map[string]any, error) {
	return r.send(ctx, resourceCall{Kind: "read", Path: p, Payload: b})
}
func (r *resourceScript) Mutate(ctx context.Context, p string, b map[string]any, q map[string]string, id string, w bool) (map[string]any, error) {
	r.sequence.Lock()
	defer r.sequence.Unlock()
	return r.send(ctx, resourceCall{Kind: "mutation", Path: p, Payload: b, Query: q, PageID: id, Workflow: w})
}
func (r *resourceScript) MutationSequence(ctx context.Context, f func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error {
	r.sequence.Lock()
	defer r.sequence.Unlock()
	r.mu.Lock()
	r.sequences++
	r.mu.Unlock()
	active := true
	defer func() { active = false }()
	return f(func(ctx context.Context, p string, b map[string]any, q map[string]string, id string, w bool) (map[string]any, error) {
		if !active {
			return nil, errors.New("sequence ended")
		}
		return r.send(ctx, resourceCall{Kind: "sequence", Path: p, Payload: b, Query: q, PageID: id, Workflow: w})
	})
}
func (r *resourceScript) ExportSession() (sdkSession.SessionBundle, error) {
	return r.bundle, r.exportErr
}
func (r *resourceScript) ExportCredentials() (sdkSession.SberCredentials, error) {
	return r.credentials, r.exportErr
}
func (r *resourceScript) WarmUp(ctx context.Context, force bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warm = append(r.warm, force)
	return r.warmErr
}
func (r *resourceScript) done() {
	r.t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.steps) != 0 {
		r.t.Fatalf("unconsumed synthetic steps: %d", len(r.steps))
	}
}

func TestResourceProductsGetExactBooleanPayload(t *testing.T) {
	response := resourceMap(`{"success":true,"body":{"sections":{"technicalSection":{"sectionProductData":{"ctaccounts":{"data":[{"id":1001,"name":"Fixture current","number":"0001","balance":{"amount":"10.50","currencyCode":"RUB"}}]},"accounts":{"data":[{"id":2002,"name":"Fixture saving"}]},"cardsInWallet":{"data":[{"id":4004,"name":"Fixture card","number":"0004","availableLimit":{"amount":"2.50","currencyCode":"RUB"}}]}}}}}}`)
	for _, force := range []bool{false, true} {
		r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/main-screen/rest/v2/m1/web/section/meta", Payload: map[string]any{"withData": true, "forceUpdate": force}}, Response: response}}}
		got, err := NewProductsAPI(r).Get(context.Background(), force)
		if err != nil || len(got.Accounts) != 2 || len(got.Cards) != 1 || got.Accounts[0].Balance.Amount.String() != "10.50" || got.Accounts[1].Kind != "account" {
			t.Fatalf("typed products: %#v %v", got, err)
		}
		r.done()
	}
}
