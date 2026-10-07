package bank

import (
	"context"
	"errors"
	"reflect"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

func TestClientCycle3OptionsRetainExistingExportedFieldLayout(t *testing.T) {
	typ := reflect.TypeOf(ClientOptions{})
	expected := []string{"Transport", "TransportFactory", "TransportOptions", "AllowMutations", "SessionPath", "Monotonic", "Renewal", "AuthOptions", "BrowserBootstrap", "BrowserBootstrapTimeout"}
	if typ.NumField() != len(expected) {
		t.Fatal("ClientOptions gained a field, breaking prior positional literals")
	}
	for i, name := range expected {
		field := typ.Field(i)
		if field.Name != name || field.PkgPath != "" {
			t.Fatal("ClientOptions public layout changed")
		}
	}
}

func TestClientCycle3OpaqueCopiesShareCloseAndCachedWorkflowLifetime(t *testing.T) {
	b := clientFixture(t, "cycle3-copy-lifetime")
	tr := clientFake(t, b)
	c, err := NewSberClient(b, ClientOptions{Transport: tr})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	copy := reflect.ValueOf(c).Elem().Interface().(SberClient)
	if copy.Products() != c.Products() || copy.Transfers() != c.Transfers() {
		t.Fatal("handle copy rebuilt Resources/workflow issuer")
	}
	if err = copy.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = c.ExportSession(); !errors.Is(err, sdkErrs.ErrClosed) {
		t.Fatal("copy close did not close the original lifetime")
	}
	if _, err = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); !errors.Is(err, sdkErrs.ErrClosed) {
		t.Fatal("copy close resurrected original")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if n, closes, _ := tr.counts(); n != 0 || closes != 1 {
		t.Fatal("copy close duplicated owner cleanup")
	}
}
