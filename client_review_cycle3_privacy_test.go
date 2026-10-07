package sber

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func TestClientCycle3OpaqueValueDiagnosticsPreserveExplicitExports(t *testing.T) {
	b := clientFixture(t, "cycle3-private-cookie-canary")
	b.Browser.Headers = []BrowserHeader{{Name: "user-agent", Value: "cycle3-private-header-canary"}}
	b.AntifraudDeviceprint = ptrString("cycle3-private-antifraud-canary")
	tr := clientFake(t, b)
	c, err := NewSberClient(b, ClientOptions{Transport: tr, SessionPath: "cycle3-private-path-canary"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	value := reflect.ValueOf(c).Elem().Interface()
	private := struct{ client any }{value}
	cases := []struct {
		name  string
		value any
	}{
		{"pointer", c}, {"value", value}, {"private-wrapper", private}, {"private-wrapper-pointer", &private},
		{"slice", []any{value, c}}, {"map", map[string]any{"client": value}},
		{"reflect-value", reflect.ValueOf(value)}, {"private-reflect-field", reflect.ValueOf(private).Field(0)},
	}
	formats := []string{"%v", "%+v", "%#v", "%d", "%p", "%#p", "%w", "%#w", "%x", "%J", "%[0]w", "%w %w"}
	markers := []string{"session-cycle3-private-cookie-canary", "token-cycle3-private-cookie-canary", "cycle3-private-header-canary", "cycle3-private-path-canary", "cycle3-private-antifraud-canary"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, format := range formats {
				t.Run(format, func(t *testing.T) {
					var buf bytes.Buffer
					log.New(&buf, "", 0).Printf(format, tc.value)
					err := fmt.Errorf(format, tc.value)
					texts := []string{fmt.Sprintf(format, tc.value), buf.String(), err.Error(), fmt.Sprintf("%#v", err)}
					buf.Reset()
					slog.New(slog.NewTextHandler(&buf, nil)).Info("synthetic", slog.Any("client", tc.value))
					texts = append(texts, buf.String())
					for _, text := range texts {
						for _, marker := range markers {
							if strings.Contains(text, marker) {
								t.Fatal("ordinary diagnostic exposed private client state")
							}
						}
					}
				})
			}
		})
	}
	// Explicit exports must retain, not zero or mask, real synthetic state.
	session, err := c.ExportSession()
	if err != nil || session.Browser.Headers[0].Value != b.Browser.Headers[0].Value || *session.AntifraudDeviceprint != *b.AntifraudDeviceprint {
		t.Fatal("explicit session state lost")
	}
	credentials, err := c.ExportCredentials()
	if err != nil || credentials.UFSSession != "session-cycle3-private-cookie-canary" || credentials.UFSToken != "token-cycle3-private-cookie-canary" {
		t.Fatal("explicit credentials lost")
	}
	for _, v := range []any{c, value, []any{value, c}, struct{ Client any }{value}} {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if strings.Contains(string(raw), marker) {
				t.Fatal("ordinary JSON exposed state")
			}
		}
	}
}
