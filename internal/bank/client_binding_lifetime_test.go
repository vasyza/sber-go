package bank

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientBindingEveryConstructorRetainsOneBundleWithoutExtraEffects(t *testing.T) {
	for _, allow := range []bool{false, true} {
		for _, name := range []string{"bundle", "credentials", "session-file", "ready-pin-profile", "synthetic-files"} {
			t.Run(name+map[bool]string{false: "-read-only", true: "-opt-in"}[allow], func(t *testing.T) {
				bundle := clientFixture(t, "binding-constructor")
				path := clientPrivatePath(t)
				if err := bundle.Save(path); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var builds, renewals, providers atomic.Int32
				var tr *clientFakeTransport
				options := ClientOptions{AllowMutations: allow, TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					builds.Add(1)
					tr = clientFake(t, b)
					return tr, nil
				}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
					renewals.Add(1)
					return bundle, nil
				}}
				var c *SberClient
				switch name {
				case "bundle":
					c, err = NewSberClient(bundle, options)
				case "credentials":
					c, err = NewSberClientFromCredentials(sdkSession.SberCredentials{UFSSession: "synthetic-constructor-session", UFSToken: "synthetic-constructor-token"}, sdkSession.CredentialsBundleOptions{APIBase: bundle.APIBase, WebBase: bundle.WebBase, Deviceprint: bundle.Deviceprint}, options)
				case "session-file":
					c, err = NewSberClientFromSessionFile(path, options)
				case "ready-pin-profile":
					c, err = NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { providers.Add(1); return "synthetic-pin", nil }, options)
				case "synthetic-files":
					// Generated here; no real HAR/session source is ever read.
					har := clientPrivatePath(t)
					raw := `{"log":{"entries":[{"request":{"method":"GET","url":"https://web2.online.sberbank.ru/main","headers":[]},"response":{"headers":[],"content":{}}},{"request":{"method":"POST","url":"https://web-node2.online.sberbank.ru/main-screen/rest/v2/m1/web/section/meta","headers":[],"cookies":[{"name":"UFS-SESSION","value":"synthetic-import-cookie","domain":"online.sberbank.ru","path":"/","secure":true}]},"response":{"headers":[],"content":{}}}]}}`
					if err = os.WriteFile(har, []byte(raw), 0600); err != nil {
						t.Fatal(err)
					}
					c, err = NewSberClientFromFiles(har, "", options)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				if !clientBindingOwnsRequester(c, c.Products().requester) || !clientBindingOwnsRequester(c, c.Operations().requester) || !clientBindingOwnsRequester(c, c.Accounts().requester) || !clientBindingOwnsRequester(c, c.Cards().requester) || !clientBindingOwnsRequester(c, c.Transfers().requester) || !clientBindingOwnsRequester(c, c.Analytics().requester) || !clientBindingOwnsRequester(c, c.Session().requester) {
					t.Fatal("constructor detached resource ownership")
				}
				if c.Accounts().binding != c.Cards().binding || c.Accounts().binding.transfers != c.Transfers() || c.Accounts().binding.operations != c.Operations() || c.Accounts().binding.cards != c.Cards() {
					t.Fatal("constructor created separate shortcut workflow owners")
				}
				if c.core().options.AllowMutations != allow || c.Cards().options.AllowMutations != allow || c.Transfers().options.AllowMutations != allow || c.Accounts().binding.options.AllowMutations != allow {
					t.Fatal("constructor mutation policy differs")
				}
				if builds.Load() != 1 || renewals.Load() != 0 || providers.Load() != 0 {
					t.Fatal("binding added transport/auth effects")
				}
				if n, _, _ := tr.counts(); n != 0 {
					t.Fatal("constructor made network request")
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("resource initialization changed persistence")
				}
			})
		}
	}
}

func TestClientBindingConcurrentGettersAndReadsKeepOneIssuer(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourcePortfolioResponse())
	other, _ := clientBindingSyntheticClient(t, resourcePortfolioResponse())
	if c.Transfers() == other.Transfers() || c.Products() == other.Products() || c.Accounts().binding == other.Accounts().binding {
		t.Fatal("global client interning")
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.Products() != c.Products() || c.Operations() != c.Operations() || c.Accounts() != c.Accounts() || c.Cards() != c.Cards() || c.Transfers() != c.Transfers() || c.Analytics() != c.Analytics() || c.Session() != c.Session() {
				t.Error("getter cache unstable")
			}
			p, err := c.Portfolio(context.Background(), false)
			if err != nil {
				t.Error(err)
				return
			}
			if p.Transfers() != c.Transfers() || p.Cards()[0].bankBinding() != c.Accounts().binding || p.Cards()[0].Account().Cards()[0] != p.Cards()[0] {
				t.Error("concurrent snapshot detached")
			}
			copy := p.Raw()
			copy.Cards[0].Name = "caller-edit"
		}()
	}
	wg.Wait()
	if n, _, _ := tr.counts(); n != 24 {
		t.Fatal("concurrent reads added/dropped requests")
	}
}

func TestClientBindingHistoryCalendarDoesNotUseMonotonicWarmupClock(t *testing.T) {
	bundle := clientFixture(t, "binding-clock")
	tr := clientFake(t, bundle)
	var monotonicCalls atomic.Int32
	tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientBindingResponse(t, resourceOperationsResponse()), nil
	}
	c, err := NewSberClient(bundle, ClientOptions{Transport: tr, Monotonic: func() time.Time { monotonicCalls.Add(1); return time.Unix(1, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	before := time.Now().In(domainMoscow).AddDate(0, 0, -120).Truncate(time.Second)
	if _, err := c.Operations().Page(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := time.Now().In(domainMoscow).AddDate(0, 0, -120)
	calls := tr.snapshotCalls()
	from, err := time.ParseInLocation("02.01.2006T15:04:05", calls[0].body["from"].(string), domainMoscow)
	if err != nil || from.Before(before) || from.After(after) || monotonicCalls.Load() != 0 {
		t.Fatal("history used monotonic/debounce clock instead of resource calendar default")
	}
	if !reflect.DeepEqual(calls[0].body["paginationSize"], 31) {
		t.Fatal("history default changed")
	}
	if err := c.Session().WarmUp(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := c.Session().WarmUp(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if monotonicCalls.Load() == 0 {
		t.Fatal("warmup no longer uses client monotonic clock")
	}
	if n, _, _ := tr.counts(); n != 2 {
		t.Fatal("history warmed session or debounce ignored")
	}
}
