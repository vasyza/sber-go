package sber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientFailedConstructorReturnsRetryableCleanupOwnership(t *testing.T) {
	for _, mode := range []string{"factory-error", "missing-jar", "canceled-pin"} {
		t.Run(mode, func(t *testing.T) {
			b := clientFixture(t, "construct-cleanup")
			tr := clientFake(t, b)
			var attempts atomic.Int32
			tr.close = func() error {
				if attempts.Add(1) == 1 {
					return errors.New("synthetic secret close failure")
				}
				return nil
			}
			var c *SberClient
			var e error
			switch mode {
			case "factory-error":
				c, e = NewSberClient(b, ClientOptions{TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) {
					return tr, errors.New("synthetic secret factory failure")
				}})
			case "missing-jar":
				tr.jar = nil
				c, e = NewSberClient(b, ClientOptions{Transport: tr})
			case "canceled-pin":
				path := clientPrivatePath(t)
				if err := b.Save(path); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c, e = NewSberClientFromPINProfile(ctx, path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { cancel(); return tr, nil }})
			}
			if c != nil || e == nil {
				t.Fatal("failed constructor returned ready client")
			}
			var cleanup interface{ Close() error }
			if !errors.As(e, &cleanup) {
				t.Fatal("failed cleanup ownership discarded")
			}
			if mode == "canceled-pin" && !errors.Is(e, context.Canceled) {
				t.Fatal("cleanup error hid cancellation")
			}
			for _, text := range []string{e.Error(), fmt.Sprintf("%#v", e)} {
				if strings.Contains(text, "synthetic secret") {
					t.Fatal("unsafe cleanup diagnostic")
				}
			}
			data, err := json.Marshal(e)
			if err != nil || strings.Contains(string(data), "synthetic secret") {
				t.Fatal("unsafe cleanup JSON")
			}
			if e = cleanup.Close(); e != nil {
				t.Fatal(e)
			}
			if e = cleanup.Close(); e != nil {
				t.Fatal(e)
			}
			if attempts.Load() != 2 {
				t.Fatal("cleanup retry lost or repeated")
			}
		})
	}
}
