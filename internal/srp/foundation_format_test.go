package srp

import (
	"fmt"
	"strings"
	"testing"
)

func TestFoundationSRPConcurrentFormatting(t *testing.T) {
	client, err := NewWithPrivateHex(strings.Repeat("f", 62)+"43", "2", "1122334455667788")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 40; i++ {
			if _, err := client.Process("13579", "deadbeefcafebabe", "2f1338c99116b736cba5984a87fb932ccdae361395e11ebd2992cb145da5d90a"); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 1000; i++ {
		format := "%#f"
		if fmt.Sprintf(format, client) != "PinSRP(<redacted>)" {
			t.Fatal("concurrent diagnostic bypassed formatter")
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestFoundationSRPDiagnosticFormatting(t *testing.T) {
	client, err := NewWithPrivateHex(strings.Repeat("f", 62)+"43", "2", "1122334455667788")
	if err != nil {
		t.Fatal(err)
	}
	// A fresh value is constructed field-by-field: Client must not be copied
	// after use (it owns a mutex). Both Go method sets must redact diagnostics.
	value := any(Client{n: client.n, g: client.g, a: client.a, public: client.public, width: client.width, expected: []byte("synthetic-proof-secret")})
	for _, v := range []any{client, value} {
		for _, format := range []string{"%t", "%f", "%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%b", "%e", "%g", "%p", "%U", "%#t", "%#f", "%#x", "%#q", "%020.5f", "%+30v", "%#b", "%F", "%c", "%o", "%O", "%#U", "%#p"} {
			got := fmt.Sprintf(format, v)
			for _, secret := range []string{"1234605616436508552", "1122334455667788", "synthetic-proof-secret"} {
				if strings.Contains(got, secret) {
					t.Errorf("SRP %s diagnostic leaked synthetic private/proof state", format)
				}
			}
			if format != "%p" && format != "%#p" && got != "PinSRP(<redacted>)" {
				t.Errorf("SRP %s bypassed secret formatter", format)
			}
		}
		if got := fmt.Sprint(v); got != "PinSRP(<redacted>)" {
			t.Error("normal fmt bypassed redaction")
		}
	}
	// Formatting must not touch locking or secret state even while Process owns mu.
	diagnostic := "%t"
	client.mu.Lock()
	if got := fmt.Sprintf(diagnostic, client); got != "PinSRP(<redacted>)" {
		t.Error("locked-state formatting failed")
	}
	client.mu.Unlock()
	var zero Client
	if _, err := zero.Process("", "", ""); err != ErrInput {
		t.Error("zero Client invalid-input semantics changed")
	}
	if _, err := zero.Verify(""); err != ErrChallengeNotProcessed {
		t.Error("zero Client challenge semantics changed")
	}
	var nilClient *Client
	if got := fmt.Sprintf(diagnostic, nilClient); strings.Contains(got, "1234605616436508552") {
		t.Error("nil diagnostic leaked state")
	}
}
