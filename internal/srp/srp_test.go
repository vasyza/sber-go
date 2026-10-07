package srp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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

func TestFrontendSHA512ProofVector(t *testing.T) {
	client, err := NewWithPrivateHex(strings.Repeat("f", 62)+"43", "2", "1122334455667788")
	if err != nil {
		t.Fatal(err)
	}
	if got := client.PublicHex(); got != "8765227cda4b12387ec1e84f11c6aa8082172358d5fab8bb5dbd738aab39347f" {
		t.Fatalf("wrong public A: %s", got)
	}
	m1, err := client.Process("13579", "deadbeefcafebabe", "2f1338c99116b736cba5984a87fb932ccdae361395e11ebd2992cb145da5d90a")
	if err != nil {
		t.Fatal(err)
	}
	want := "77195883ffd3be2e3d92c26de691b997f1bf79877b59a5f9436233358ed301287dd861149a1b4a4d3e3f1f26855bde278603be950cc33abfe1f74eaf96e50909"
	if m1 != want {
		t.Fatalf("wrong frontend M1: %s", m1)
	}
	m2 := "42e99188f785607eee90fc6c64d41193f07874a72dc7801fdc4802cd9a74c422b3ff25cc34e058c21af9ee7effd7b943d3ddd5af3f379c4b7b0d6a3ebcb8724b"
	ok, err := client.Verify("000" + m2)
	if err != nil || !ok {
		t.Fatalf("valid frontend M2 rejected: %v", err)
	}
}

func TestProofIsInvalidatedByFailedChallenge(t *testing.T) {
	c, _ := NewWithPrivateHex(strings.Repeat("f", 62)+"43", "2", "1122334455667788")
	_, err := c.Process("13579", "deadbeefcafebabe", "2f1338c99116b736cba5984a87fb932ccdae361395e11ebd2992cb145da5d90a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Process("13579", "deadbeefcafebabe", "0"); err == nil {
		t.Fatal("invalid B accepted")
	}
	if ok, err := c.Verify("42e99188f785607eee90fc6c64d41193f07874a72dc7801fdc4802cd9a74c422b3ff25cc34e058c21af9ee7effd7b943d3ddd5af3f379c4b7b0d6a3ebcb8724b"); ok || !errors.Is(err, ErrChallengeNotProcessed) {
		t.Fatal("stale proof survives failed challenge")
	}
}

func TestFormattingRedactsEphemeralProofState(t *testing.T) {
	c, _ := NewWithPrivateHex(strings.Repeat("f", 62)+"43", "2", "1122334455667788")
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if got := fmt.Sprintf(format, c); got != "PinSRP(<redacted>)" {
			t.Fatalf("%s exposes state", format)
		}
	}
}

func TestConstructorUsesProvidedCryptographicEntropy(t *testing.T) {
	n := strings.Repeat("f", 62) + "43"
	reader := bytes.NewReader([]byte{0, 0, 0, 0, 0x11, 0x22, 0x33, 0x44})
	c, err := NewWithReader(n, "2", reader)
	if err != nil {
		t.Fatal(err)
	}
	fixed, _ := NewWithPrivateHex(n, "2", "11223344")
	if c.PublicHex() != fixed.PublicHex() || reader.Len() != 0 {
		t.Fatal("random private exponent differs from frontend byte rule")
	}
}
func TestConstructorRejectsUnavailableEntropy(t *testing.T) {
	if _, err := NewWithReader(strings.Repeat("f", 62)+"43", "2", bytes.NewReader(nil)); !errors.Is(err, ErrEntropy) {
		t.Fatal("entropy failure accepted")
	}
}
func TestConstructorRejectsGroupTooSmallForRandomExponent(t *testing.T) {
	if _, err := NewWithReader("ff", "2", bytes.NewReader([]byte{1})); !errors.Is(err, ErrInput) {
		t.Fatal("too-small random SRP group accepted")
	}
}
func TestConstructorUsesSecureDefault(t *testing.T) {
	c, err := New(strings.Repeat("f", 512), "2")
	if err != nil || c.PublicHex() == "0" {
		t.Fatal("secure default constructor unavailable")
	}
}

var _ io.Reader = (*bytes.Reader)(nil)
