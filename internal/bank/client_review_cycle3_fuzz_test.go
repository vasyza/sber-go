package bank

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

// Test-only strengthening of the already RED/GREEN expiry and opacity gates.
// All generated imports and transports remain synthetic/private/in-process.
func FuzzClientCycle3ExpiredNumericHARNeverResurrects(f *testing.F) {
	for _, value := range []string{"-1", "-1e30", "-9223372036854775809", "-1e999", "-0.99", "false", "true", "0", "0.0", "-999999999999999999999999999999999999999999"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, token string) {
		if len(token) > 512 {
			return
		}
		decoded, err := DecodeJSON(bytes.NewReader([]byte(`{"expiry":` + token + `}`)))
		if err != nil {
			return
		}
		value := decoded["expiry"]
		switch v := value.(type) {
		case json.Number:
			n, _ := strconv.ParseFloat(string(v), 64)
			if n > 0 {
				return
			}
		case bool:
		default:
			return
		}
		builds := 0
		c, err := NewSberClientFromFiles(clientCycle3HAR(t, map[string]any{"expires": value}, false), "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
			builds++
			return clientFake(t, b), nil
		}})
		if c != nil {
			c.Close()
		}
		if c != nil || err == nil || builds != 0 {
			t.Fatal("negative/source boolean expiry became a usable client")
		}
	})
}

func FuzzClientCycle3OpaqueFallbackDiagnostics(f *testing.F) {
	for i, verb := range []byte{'v', 'd', 'p', 'w', 'x', 'J', 's', 'q', 't'} {
		f.Add(verb, uint8(i))
	}
	f.Fuzz(func(t *testing.T, verb uint8, wrapping uint8) {
		b := clientFixture(t, "cycle3-fuzz-private-cookie")
		b.Browser.Headers = []sdkSession.BrowserHeader{{Name: "user-agent", Value: "cycle3-fuzz-private-header"}}
		c, err := NewSberClient(b, ClientOptions{Transport: clientFake(t, b), SessionPath: "cycle3-fuzz-private-path"})
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		value := reflect.ValueOf(c).Elem().Interface()
		private := struct{ client any }{value}
		cases := []any{c, value, private, &private, []any{c, value}, map[string]any{"client": value}, reflect.ValueOf(value), reflect.ValueOf(private).Field(0)}
		v := cases[int(wrapping)%len(cases)]
		format := "%#" + string(rune(verb))
		out := fmt.Sprintf(format, v) + fmt.Errorf(format, v).Error()
		for _, marker := range []string{"session-cycle3-fuzz-private-cookie", "token-cycle3-fuzz-private-cookie", "cycle3-fuzz-private-header", "cycle3-fuzz-private-path"} {
			if strings.Contains(out, marker) {
				t.Fatal("formatter fallback exposed client state")
			}
		}
	})
}
