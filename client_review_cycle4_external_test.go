package sber_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/vasyza/sber-go"
)

// Embedding the exported interface legitimately promotes its private marker.
// This external package cannot declare sdk.sberError; that is not needed.
type i3ForeignError struct {
	sdk.SberError
	diagnostic string
}

func (e *i3ForeignError) Error() string { return e.diagnostic }

var _ sdk.SberError = (*i3ForeignError)(nil)

type i3ExternalTransport struct {
	jar     *sdk.CookieJar
	err     error
	posts   atomic.Int32
	expired bool
}

func (t *i3ExternalTransport) Get(context.Context, string, sdk.RequestOptions) (*sdk.Response, error) {
	return nil, t.err
}
func (t *i3ExternalTransport) Post(context.Context, string, map[string]any, sdk.RequestOptions) (*sdk.Response, error) {
	t.posts.Add(1)
	if t.expired {
		return &sdk.Response{StatusCode: 401, Headers: http.Header{}, Content: []byte(`{}`)}, nil
	}
	return nil, t.err
}
func (*i3ExternalTransport) PostForm(context.Context, string, map[string]string, sdk.RequestOptions) (*sdk.Response, error) {
	panic("credential POST forbidden")
}
func (t *i3ExternalTransport) CookieJar() *sdk.CookieJar { return t.jar }
func (*i3ExternalTransport) Close() error                { return nil }
func i3ExternalSetup(t *testing.T) (sdk.SessionBundle, *i3ExternalTransport) {
	t.Helper()
	dp, af := "synthetic-device", "synthetic-antifraud"
	b, e := (sdk.SberCredentials{UFSSession: "synthetic-session", UFSToken: "synthetic-token"}).ToBundle(sdk.CredentialsBundleOptions{APIBase: "https://web-node3.online.sberbank.ru", WebBase: "https://web3.online.sberbank.ru", Deviceprint: &dp, AntifraudDeviceprint: &af})
	if e != nil {
		t.Fatal(e)
	}
	j, e := sdk.NewCookieJar(b.Cookies)
	if e != nil {
		t.Fatal(e)
	}
	return b, &i3ExternalTransport{jar: j}
}
func i3AssertErrorSanitized(t *testing.T, e error, marker string, raw *i3ForeignError) {
	t.Helper()
	if e == nil {
		t.Fatal("injected failure was swallowed")
	}
	texts := []string{e.Error(), fmt.Sprintf("%v", e), fmt.Sprintf("%+v", e), fmt.Sprintf("%#v", e), fmt.Errorf("%w", e).Error()}
	data, je := json.Marshal(e)
	if je == nil {
		texts = append(texts, string(data))
	}
	leaked := false
	for _, text := range texts {
		if strings.Contains(text, marker) {
			leaked = true
		}
	}
	if leaked {
		t.Error("client returned synthetic private diagnostics from foreign SDK-marked wrapper")
	}
	var retained *i3ForeignError
	if errors.As(e, &retained) || errors.Is(e, raw) {
		t.Error("client retained foreign wrapper instead of copying allowlisted safe SDK classification")
	}
}
func TestIndependentClient3ExternalSDKMarkerDoesNotBypassSanitization(t *testing.T) {
	for _, route := range []string{"read", "mutation", "callback", "factory", "renewal"} {
		for _, kind := range []string{"plain-control", "interface-embedded-sdk"} {
			t.Run(route+"/"+kind, func(t *testing.T) {
				b, tr := i3ExternalSetup(t)
				marker := strings.Join([]string{"independent", "synthetic", "private", "foreign", "error"}, "-")
				raw := &i3ForeignError{SberError: &sdk.TransportError{Code: "synthetic"}, diagnostic: marker}
				var injected error = raw
				if kind == "plain-control" {
					injected = errors.New(marker)
				}
				tr.err = injected
				if raw.Error() != marker {
					t.Fatal("foreign error control lost diagnostic")
				}
				var c *sdk.SberClient
				var e error
				if route == "factory" {
					_, e = sdk.NewSberClient(b, sdk.ClientOptions{TransportFactory: func(sdk.SessionBundle, sdk.TransportOptions) (sdk.Transport, error) { return nil, injected }})
				} else if route == "renewal" {
					p := filepath.Join(t.TempDir(), "synthetic-profile.json")
					if e = b.Save(p); e != nil {
						t.Fatal(e)
					}
					tr.expired = true
					c, e = sdk.NewSberClientFromPINProfile(context.Background(), p, func(context.Context) (string, error) { t.Error("unexpected PIN use"); return "", nil }, sdk.ClientOptions{Transport: tr, Renewal: func(context.Context, sdk.SessionBundle, sdk.PINProvider, sdk.AuthOptions) (sdk.SessionBundle, error) {
						return sdk.SessionBundle{}, injected
					}})
					if e != nil {
						t.Fatal(e)
					}
					defer c.Close()
					_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
				} else {
					c, e = sdk.NewSberClient(b, sdk.ClientOptions{Transport: tr, AllowMutations: true})
					if e != nil {
						t.Fatal(e)
					}
					defer c.Close()
					switch route {
					case "read":
						_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
					case "mutation":
						_, e = c.Mutate(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", true)
					case "callback":
						e = c.MutationSequence(context.Background(), func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
							return injected
						})
					}
				}
				i3AssertErrorSanitized(t, e, marker, raw)
				want := int32(1)
				if route == "factory" || route == "callback" {
					want = 0
				}
				if tr.posts.Load() != want {
					t.Fatal("injected diagnostic probe replayed request")
				}
			})
		}
	}
}

var _ = os.FileMode(0600)
