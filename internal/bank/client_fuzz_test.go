package bank

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
	"reflect"
	"strconv"
	"strings"
	"testing"
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

// Test-only generated negatives for the exercised copy boundary, not a source
// oracle or an authentication runtime. No network or secret-bearing input.
func FuzzClientCycle4ErrorBoundary(f *testing.F) {
	for i, text := range []string{"plain", "wrapped", "joined", "context", "utf8", "\xff"} {
		f.Add(text, uint8(i))
	}
	f.Fuzz(func(t *testing.T, text string, kind uint8) {
		if len(text) > 512 {
			return
		}
		marker := "cycle4-private-canary-" + hex.EncodeToString([]byte(text))
		foreign := &client4PrivateCause{marker}
		var input error
		switch kind % 6 {
		case 0:
			input = sdkErrs.NewTransportError(marker, foreign)
		case 1:
			input = &sdkErrs.APIRejected{Message: marker, Code: marker, Text: marker, UUID: marker}
		case 2:
			input = &sdkErrs.MissingSession{Message: marker}
		case 3:
			input = NewParseError(marker)
		case 4:
			input = &sdkErrs.MutationUncertain{Message: marker}
		default:
			input = &client4ForgedClassification{text: marker}
		}
		cause := context.Canceled
		if kind&0x80 != 0 {
			cause = context.DeadlineExceeded
		}
		input = &client4WrappedError{inner: errors.Join(input, foreign, cause), text: marker}
		got := clientSafeError(input)
		if !errors.Is(got, cause) {
			t.Fatal("generated context identity lost")
		}
		var leaked *client4PrivateCause
		var outer *client4WrappedError
		if errors.Is(got, foreign) || errors.As(got, &leaked) || errors.As(got, &outer) {
			t.Fatal("generated foreign object retained")
		}
		badVerb := "%w"
		for _, out := range []string{got.Error(), fmt.Errorf("%w", got).Error(), fmt.Sprintf(badVerb, got), fmt.Sprintf("%#v", got)} {
			if strings.Contains(out, marker) {
				t.Fatal("generated diagnostic leaked")
			}
		}
		var te *sdkErrs.TransportError
		var rej *sdkErrs.APIRejected
		var missing *sdkErrs.MissingSession
		var parsed *ParseError
		var uncertain *sdkErrs.MutationUncertain
		if errors.As(got, &te) && te.Code == marker || errors.As(got, &rej) && (rej.Message == marker || rej.Code == marker || rej.Text == marker || rej.UUID == marker) || errors.As(got, &missing) && missing.Message == marker || errors.As(got, &parsed) && parsed.Field() == marker || errors.As(got, &uncertain) && uncertain.Message == marker {
			t.Fatal("generated copied fields retain input")
		}
	})
}
