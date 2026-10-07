//go:build live

package integration_test

import (
	"context"
	"flag"
	"path/filepath"
	"testing"
	"time"

	sber "github.com/vasyza/sber-go"
)

var (
	liveEnabled = flag.Bool("sber-live", false, "allow an owner-requested authenticated bank read")
	liveProfile = flag.String("sber-profile", "", "explicit absolute private profile path; never credentials")
	liveCA      = flag.String("sber-ca-bundle", "", "explicit trusted PEM certificate bundle")
)

// TestLiveReadOnly is deliberately outside the default synthetic suite. The
// owner completes terminal login first. No credentials, responses, identifiers,
// balances or cookie values are printed or copied into repository fixtures.
func TestLiveReadOnly(t *testing.T) {
	if !*liveEnabled {
		t.Skip("requires explicit -sber-live after owner login")
	}
	if !filepath.IsAbs(*liveProfile) {
		t.Fatal("requires -sber-profile with an absolute private path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := sber.NewSberClientFromSessionFile(*liveProfile, sber.ClientOptions{
		AllowMutations: false,
		TransportOptions: sber.TransportOptions{
			CABundle: *liveCA,
			Timeout:  30 * time.Second,
			Retry:    0,
		},
	})
	if err != nil {
		t.Fatal("cannot open the selected private session")
	}
	t.Cleanup(func() {
		if client.Close() != nil {
			t.Error("session cleanup failed")
		}
	})
	if client.WarmUp(ctx, true) != nil {
		t.Fatal("authenticated session check failed")
	}
	t.Log("authenticated session check passed")
	products, err := client.Products().Get(ctx, false)
	if err != nil {
		t.Fatal("authenticated products read failed")
	}
	t.Logf("products read passed: accounts=%d cards=%d", len(products.Accounts), len(products.Cards))
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal("cannot determine the history date window")
	}
	now := time.Now().In(location)
	page, err := client.Operations().Page(ctx, sber.OperationsPageOptions{
		Limit: 5,
		From:  now.AddDate(0, 0, -7).Format(time.DateOnly),
		To:    now.Format(time.DateOnly),
	})
	if err != nil {
		t.Fatal("authenticated history page read failed")
	}
	if len(page.Operations) > 5 {
		t.Fatal("history page exceeded the requested limit")
	}
	t.Logf("history page read passed: operations=%d has_next=%t; full history coverage remains unknown", len(page.Operations), page.NextOffset != nil)
}
