package sber

import (
	"context"
	"errors"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func clientBindingRenameCall(name string) resourceCall {
	call := resourceRenameCall("mutation", 4004, name)
	call.PageID = "/app/cards/details/4004"
	return call
}

func TestClientBindingOptInRenameIsFunctionalAndDoesNotEditSnapshots(t *testing.T) {
	c, tr := clientBindingScript(t, true, []resourceStep{
		{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()},
		{Call: clientBindingRenameCall("Fixture alias"), Response: map[string]any{"success": true}},
		{Call: clientBindingRenameCall("Fixture direct"), Response: map[string]any{"success": true}},
	})
	p := clientBindingPortfolio(t, c, false)
	if err := p.Cards()[0].Rename(context.Background(), "Fixture alias"); err != nil {
		t.Fatal(err)
	}
	if err := c.Cards().Rename(context.Background(), 4004, "Fixture direct"); err != nil {
		t.Fatal(err)
	}
	if p.Cards()[0].Name() != "Fixture card" {
		t.Fatal("rename edited prior financial snapshot")
	}
	if n, _, _ := tr.counts(); n != 3 {
		t.Fatal("rename silently retried or read products")
	}
}

func TestClientBindingUncertainRenameCannotResetThroughShortcutReads(t *testing.T) {
	c, tr := clientBindingScript(t, true, []resourceStep{
		{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()},
		{Call: clientBindingRenameCall("Fixture alias"), Err: &TransportError{}},
		{Call: resourceProductsCall(true), Response: resourcePortfolioResponse()},
	})
	p := clientBindingPortfolio(t, c, false)
	var uncertain *MutationUncertain
	if err := p.Cards()[0].Rename(context.Background(), "Fixture alias"); !errors.As(err, &uncertain) {
		t.Fatal("entity rename not uncertain")
	}
	cards, err := c.Cards().List(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := cards[0].Rename(context.Background(), "Fixture retry"); !errors.As(err, &uncertain) {
		t.Fatal("new card snapshot reset rename guard")
	}
	if err := c.Cards().Rename(context.Background(), 4004, "Fixture retry"); !errors.As(err, &uncertain) {
		t.Fatal("direct API reset rename guard")
	}
	if n, _, _ := tr.counts(); n != 3 {
		t.Fatal("uncertain rename replayed")
	}
}

func TestClientBindingConfirmSequenceExcludesQueuedProductRead(t *testing.T) {
	steps := []resourceStep{{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}}
	steps = append(steps, clientBindingSuccessfulWorkflowSteps()...)
	steps = append(steps, resourceStep{Call: resourceProductsCall(true), Response: resourcePortfolioResponse()})
	c, tr := clientBindingScript(t, true, steps)
	originalPost := tr.post
	entered, release := make(chan struct{}), make(chan struct{})
	tr.post = func(ctx context.Context, target string, body map[string]any, options RequestOptions) (*Response, error) {
		u, _ := url.Parse(target)
		if u.Query().Get("name") == "summaryNext" {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return originalPost(ctx, target, body, options)
	}
	p := clientBindingPortfolio(t, c, false)
	draft, err := c.Transfers().Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := p.Transfers().Prepare(context.Background(), draft, "transactionAccount:1001", "card:4004", resourceAmount(t, "10.50"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	confirmed := make(chan error, 1)
	go func() { _, err := c.Transfers().Confirm(ctx, prepared); confirmed <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("confirmation did not start")
	}
	queuedContext, queued := clientQueue()
	read := make(chan error, 1)
	go func() { _, err := c.Portfolio(queuedContext, true); read <- err }()
	select {
	case <-queued:
	case <-ctx.Done():
		t.Fatal("product read did not queue")
	}
	close(release)
	if err := <-confirmed; err != nil {
		t.Fatal(err)
	}
	if err := <-read; err != nil {
		t.Fatal(err)
	}
	if n, _, _ := tr.counts(); n != len(steps) {
		t.Fatal("bound sequence interleaved/replayed")
	}
}

func TestClientBindingExpiredMutationNeverCallsPINRenewal(t *testing.T) {
	bundle := clientFixture(t, "binding-mutation-pin")
	bundle.AntifraudDeviceprint = ptrString("synthetic-observed-antifraud")
	path := clientPrivatePath(t)
	if err := bundle.Save(path); err != nil {
		t.Fatal(err)
	}
	tr := clientFake(t, bundle)
	tr.post = func(_ context.Context, target string, _ map[string]any, _ RequestOptions) (*Response, error) {
		u, _ := url.Parse(target)
		if u.Path == ProductsPath {
			return clientBindingResponse(t, resourcePortfolioResponse()), nil
		}
		if u.Path != Me2MeWorkflowPath {
			t.Error("unexpected mutation endpoint")
		}
		return clientResponse(401, `{}`), nil
	}
	var renewals, providers atomic.Int32
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { providers.Add(1); return "synthetic-pin", nil }, ClientOptions{Transport: tr, AllowMutations: true, Renewal: func(context.Context, SessionBundle, PINProvider, AuthOptions) (SessionBundle, error) {
		renewals.Add(1)
		return bundle, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p := clientBindingPortfolio(t, c, false)
	var uncertain *MutationUncertain
	if _, err := p.Accounts()[0].TransferTo(context.Background(), p.Cards()[0], resourceAmount(t, "10.50")); !errors.As(err, &uncertain) {
		t.Fatal("expired mutation accepted or lost uncertainty")
	}
	later := clientBindingPortfolio(t, c, true)
	if _, err := later.Transfers().Start(context.Background()); !errors.As(err, &uncertain) {
		t.Fatal("new snapshot reset expired one-shot workflow")
	}
	if renewals.Load() != 0 || providers.Load() != 0 {
		t.Fatal("resource mutation triggered credential login")
	}
	if n, _, _ := tr.counts(); n != 3 {
		t.Fatal("mutation was retried or implicitly warmed")
	}
}
