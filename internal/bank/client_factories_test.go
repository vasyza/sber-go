package bank

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func clientPrivatePath(t *testing.T) string {
	t.Helper()
	d := testPrivateDir(t)
	if e := os.Chmod(d, 0700); e != nil {
		t.Fatal(e)
	}
	return filepath.Join(d, "synthetic-profile.json")
}
func TestClientFactoriesKeepCAAndNeverContactBankDuringConstruction(t *testing.T) {
	creds := sdkSession.SberCredentials{UFSSession: "session-fixture", UFSToken: "token-fixture"}
	b := clientFixture(t, "factory")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	var transports []*clientFakeTransport
	o := ClientOptions{TransportOptions: sdkTransport.TransportOptions{CABundle: "explicit-synthetic-CA", Timeout: 123}, TransportFactory: func(got sdkSession.SessionBundle, options sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		if options.CABundle != "explicit-synthetic-CA" || options.AllowUnready || options.Retry != 0 {
			t.Error("factory option mismatch")
		}
		tr := clientFake(t, got)
		transports = append(transports, tr)
		return tr, nil
	}}
	c, e := NewSberClient(b, o)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c, e = NewSberClientFromCredentials(creds, sdkSession.CredentialsBundleOptions{APIBase: b.APIBase, WebBase: b.WebBase, Deviceprint: b.Deviceprint}, o)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c, e = NewSberClientFromSessionFile(path, o)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	for _, tr := range transports {
		calls, closes, _ := tr.counts()
		if calls != 0 || closes != 1 {
			t.Fatal("constructor performed I/O")
		}
	}
	if len(transports) != 3 {
		t.Fatal("not all constructors used factory")
	}
	if _, e = NewSberClientFromCredentials(creds, sdkSession.CredentialsBundleOptions{APIBase: b.APIBase}, o); e == nil {
		t.Fatal("unpaired hosts accepted")
	}
	if _, e = NewSberClientFromCredentials(sdkSession.SberCredentials{}, sdkSession.CredentialsBundleOptions{}, o); e == nil {
		t.Fatal("missing credentials accepted")
	}
	if e = os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	var insecure *sdkErrs.InsecureSessionFile
	if _, e = NewSberClientFromSessionFile(path, o); !errors.As(e, &insecure) {
		t.Fatal("nonprivate profile accepted")
	}
	c, e = NewSberClientFromSessionFile(path, o, sdkSession.SessionLoadOptions{AllowNonPrivate: true})
	if e != nil {
		t.Fatal(e)
	}
	c.Close()
	o.TransportOptions.Retry = 1
	if _, e = NewSberClient(b, o); e == nil {
		t.Fatal("retry enabled")
	}
	// The real DEFAULT transport is constructed and closed but no Get/Post is called.
	c, e = NewSberClientFromCredentials(creds, sdkSession.CredentialsBundleOptions{}, ClientOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.ExportCredentials(); e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Existing closed lifetime must not become usable via a new operation.
	if _, e = c.PostRead(ctx, "/uoh-bh/v1/operations/list", nil); !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("closed lifetime reused")
	}
}
