package sber

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func clientPrivatePath(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if e := os.Chmod(d, 0700); e != nil {
		t.Fatal(e)
	}
	return filepath.Join(d, "synthetic-profile.json")
}
func TestClientFactoriesKeepCAAndNeverContactBankDuringConstruction(t *testing.T) {
	creds := SberCredentials{"session-fixture", "token-fixture"}
	b := clientFixture(t, "factory")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	var transports []*clientFakeTransport
	o := ClientOptions{TransportOptions: TransportOptions{CABundle: "explicit-synthetic-CA", Timeout: 123}, TransportFactory: func(got SessionBundle, options TransportOptions) (Transport, error) {
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
	c, e = NewSberClientFromCredentials(creds, CredentialsBundleOptions{APIBase: b.APIBase, WebBase: b.WebBase, Deviceprint: b.Deviceprint}, o)
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
	if _, e = NewSberClientFromCredentials(creds, CredentialsBundleOptions{APIBase: b.APIBase}, o); e == nil {
		t.Fatal("unpaired hosts accepted")
	}
	if _, e = NewSberClientFromCredentials(SberCredentials{}, CredentialsBundleOptions{}, o); e == nil {
		t.Fatal("missing credentials accepted")
	}
	if e = os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	var insecure *InsecureSessionFile
	if _, e = NewSberClientFromSessionFile(path, o); !errors.As(e, &insecure) {
		t.Fatal("nonprivate profile accepted")
	}
	c, e = NewSberClientFromSessionFile(path, o, SessionLoadOptions{AllowNonPrivate: true})
	if e != nil {
		t.Fatal(e)
	}
	c.Close()
	o.TransportOptions.Retry = 1
	if _, e = NewSberClient(b, o); e == nil {
		t.Fatal("retry enabled")
	}
	// The real DEFAULT transport is constructed and closed but no Get/Post is called.
	c, e = NewSberClientFromCredentials(creds, CredentialsBundleOptions{}, ClientOptions{})
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
	if _, e = c.PostRead(ctx, "/uoh-bh/v1/operations/list", nil); !errors.Is(e, ErrClosed) {
		t.Fatal("closed lifetime reused")
	}
}
