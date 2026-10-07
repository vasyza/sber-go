package sber

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestClientBindingUncertainWorkflowCannotResetThroughNewSnapshots(t *testing.T) {
	for stage := 0; stage < 5; stage++ {
		for _, failure := range []string{"transport", "canceled-after-send", "invalid-response"} {
			t.Run(fmt.Sprintf("stage-%d-%s", stage, failure), func(t *testing.T) {
				workflow := clientBindingSuccessfulWorkflowSteps()
				switch failure {
				case "transport":
					workflow[stage].Err = &TransportError{Code: "synthetic-send-failed"}
				case "canceled-after-send":
					workflow[stage].Err = context.Canceled
				case "invalid-response":
					workflow[stage].Response = map[string]any{"success": true, "body": map[string]any{"result": "unrecognized"}}
				}
				steps := []resourceStep{{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}}
				steps = append(steps, workflow[:stage+1]...)
				steps = append(steps, resourceStep{Call: resourceProductsCall(true), Response: resourcePortfolioResponse()}, resourceStep{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()})
				c, tr := clientBindingScript(t, true, steps)
				initial := clientBindingPortfolio(t, c, false)
				draft, err := c.Transfers().Start(context.Background())
				var prepared PreparedTransfer
				if stage > 0 {
					if err != nil {
						t.Fatal(err)
					}
					prepared, err = initial.Transfers().Prepare(context.Background(), draft, "transactionAccount:1001", "card:4004", resourceAmount(t, "10.50"))
				}
				if stage > 1 {
					if err != nil {
						t.Fatal(err)
					}
					_, err = c.Transfers().Confirm(context.Background(), prepared)
				}
				var uncertain *MutationUncertain
				if !errors.As(err, &uncertain) {
					t.Fatalf("attempted workflow failure not uncertain: %v", err)
				}
				cards, err := c.Cards().List(context.Background(), true)
				if err != nil {
					t.Fatal(err)
				}
				later := clientBindingPortfolio(t, c, false)
				if cards[0].bankBinding().transfers != initial.Transfers() || later.Transfers() != initial.Transfers() {
					t.Fatal("new read reset uncertain workflow issuer")
				}
				switch stage {
				case 0:
					_, err = later.Transfers().Start(context.Background())
				case 1:
					_, err = later.Transfers().Prepare(context.Background(), draft, "transactionAccount:1001", "card:4004", resourceAmount(t, "10.50"))
				default:
					_, err = later.Transfers().Confirm(context.Background(), prepared)
				}
				if err == nil {
					t.Fatal("new snapshot bypassed one-shot failed attempt")
				}
				if n, _, _ := tr.counts(); n != len(steps) {
					t.Fatal("uncertain mutation retried or later read added hidden workflow")
				}
			})
		}
	}
}

func TestClientBindingConcurrentConfirmAcrossSnapshotsIsOneShot(t *testing.T) {
	steps := []resourceStep{{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}, {Call: resourceProductsCall(true), Response: resourcePortfolioResponse()}}
	steps = append(steps, clientBindingSuccessfulWorkflowSteps()...)
	c, tr := clientBindingScript(t, true, steps)
	first, later := clientBindingPortfolio(t, c, false), clientBindingPortfolio(t, c, true)
	draft, err := c.Transfers().Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := later.Transfers().Prepare(context.Background(), draft, "transactionAccount:1001", "card:4004", resourceAmount(t, "10.50"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			issuer := c.Transfers()
			if i%2 == 0 {
				issuer = first.Transfers()
			} else {
				issuer = later.Transfers()
			}
			if _, err := issuer.Confirm(context.Background(), prepared); err == nil {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("concurrent aliases confirmed more/less than once")
	}
	if n, _, _ := tr.counts(); n != len(steps) {
		t.Fatal("concurrent aliases sent repeated confirmation")
	}
}

func TestClientBindingConcurrentPrepareAcrossSnapshotsIsOneShot(t *testing.T) {
	steps := []resourceStep{{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}, {Call: resourceProductsCall(true), Response: resourcePortfolioResponse()}}
	steps = append(steps, clientBindingSuccessfulWorkflowSteps()[:2]...)
	c, tr := clientBindingScript(t, true, steps)
	first, later := clientBindingPortfolio(t, c, false), clientBindingPortfolio(t, c, true)
	draft, err := c.Transfers().Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			issuer := first.Transfers()
			if i%2 == 0 {
				issuer = later.Transfers()
			}
			if _, err := issuer.Prepare(context.Background(), draft, "transactionAccount:1001", "card:4004", resourceAmount(t, "10.50")); err == nil {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("concurrent aliases prepared more/less than once")
	}
	if n, _, _ := tr.counts(); n != len(steps) {
		t.Fatal("concurrent aliases sent repeated preparation")
	}
}
