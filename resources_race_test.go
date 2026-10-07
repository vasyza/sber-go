package sber

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestResourceConcurrentPrepareIsOneShot(t *testing.T) {
	a, r, d := resourcePreparedAPI(t, resourceStep{Call: resourcePrepareCall(resourceFixtureSource, resourceFixtureDestination, "1", "RUB", ""), Response: resourceWorkflowResponse("SUCCESS", "me2meCreate", "summary", "", nil)})
	var wg sync.WaitGroup
	var successes atomic.Int64
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Prepare(context.Background(), d, resourceFixtureSource, resourceFixtureDestination, resourceAmount(t, "1")); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 || len(r.calls) != 2 {
		t.Fatal("concurrent prepare replay")
	}
	r.done()
}
func TestResourceConcurrentConfirmIsOneShot(t *testing.T) {
	var steps []resourceStep
	for i, p := range resourceConfirmResponses() {
		steps = append(steps, resourceStep{Call: resourceConfirmCall(i), Response: p})
	}
	a, r, p := resourceReadyTransfer(t, steps...)
	var wg sync.WaitGroup
	var successes atomic.Int64
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Confirm(context.Background(), p); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 || len(r.calls) != 5 || r.sequences != 1 {
		t.Fatal("concurrent confirm replay")
	}
	r.done()
}
func TestResourceConfirmSequenceDoesNotReacquireMutationOrInterleaveRename(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var steps []resourceStep
	for i, p := range resourceConfirmResponses() {
		s := resourceStep{Call: resourceConfirmCall(i), Response: p}
		if i == 0 {
			s.Before = func(context.Context) { close(entered); <-release }
		}
		steps = append(steps, s)
	}
	steps = append(steps, resourceStep{Call: resourceRenameCall("mutation", 12345, "Valid"), Response: map[string]any{"success": true}})
	a, r, p := resourceReadyTransfer(t, steps...)
	confirmed := make(chan error, 1)
	go func() { _, err := a.Confirm(context.Background(), p); confirmed <- err }()
	<-entered
	cards := NewCardsAPI(r, ResourceOptions{AllowMutations: true})
	renamed := make(chan error, 1)
	renameStarted := make(chan struct{})
	go func() { close(renameStarted); renamed <- cards.Rename(context.Background(), 12345, "Valid") }()
	<-renameStarted
	select {
	case err := <-renamed:
		t.Fatalf("rename interleaved: %v", err)
	default:
	}
	close(release)
	if err := <-confirmed; err != nil {
		t.Fatal(err)
	}
	if err := <-renamed; err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 6 || r.calls[5].Path != "/ufs-productdetail/rest/v1/changeProductName" {
		t.Fatal("sequence not contiguous")
	}
	r.done()
}
func TestResourceCanceledRenameAfterSenderBeginsCannotReplay(t *testing.T) {
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceRenameCall("mutation", 12345, "Valid"), Before: func(ctx context.Context) { close(entered); <-ctx.Done() }, Err: context.Canceled}}}
	cards := NewCardsAPI(r, ResourceOptions{AllowMutations: true})
	done := make(chan error, 1)
	go func() { done <- cards.Rename(ctx, 12345, "Valid") }()
	<-entered
	cancel()
	err := <-done
	var unknown *MutationUncertain
	if !errors.As(err, &unknown) {
		t.Fatal("cancel not uncertain")
	}
	if err = cards.Rename(context.Background(), 12345, "Valid"); !errors.As(err, &unknown) || len(r.calls) != 1 {
		t.Fatal("canceled rename repeated")
	}
	r.done()
}
func TestEntityConcurrentSnapshotAccessPreservesIdentityAndMoney(t *testing.T) {
	p := NewBankPortfolio(entityFixtureProducts(t), &resourceScript{t: t})
	var wg sync.WaitGroup
	for n := 0; n < 24; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				raw := p.Raw()
				raw.Accounts[0].Balance.Currency = "USD"
				c := p.Cards()[0]
				v := c.Snapshot()
				v.Balance.Currency = "USD"
				if c.Account() != p.Accounts()[0] || c.Account().Balance().Currency != "RUB" {
					t.Error("snapshot race")
				}
				_ = fmt.Sprint(p)
			}
		}()
	}
	wg.Wait()
}
