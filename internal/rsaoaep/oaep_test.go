// Synthetic protocol interoperability tests. Keys are generated in memory only.
package rsaoaep

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"testing"
)

var keyOnce sync.Once
var testKey *rsa.PrivateKey
var keyError error

func fixtureKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	keyOnce.Do(func() { testKey, keyError = rsa.GenerateKey(rand.Reader, 2048) })
	if keyError != nil {
		t.Fatal(keyError)
	}
	return testKey
}

func TestOAEPFrontendSHA1InteroperatesPKCS1AndSPKI(t *testing.T) {
	key := fixtureKey(t)
	pkix, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	for name, der := range map[string][]byte{"pkcs1": x509.MarshalPKCS1PublicKey(&key.PublicKey), "spki": pkix} {
		t.Run(name, func(t *testing.T) {
			encoded := base64.StdEncoding.EncodeToString(der)
			cipher, err := Encrypt(encoded, "13579")
			if err != nil {
				t.Fatal(err)
			}
			ciphertext, err := base64.StdEncoding.DecodeString(cipher)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := rsa.DecryptOAEP(sha1.New(), nil, key, ciphertext, nil)
			if err != nil || string(plain) != "13579" {
				t.Fatalf("frontend OAEP SHA1/empty-label roundtrip failed: %v", err)
			}
			other, err := Encrypt(encoded, "13579")
			if err != nil || other == cipher {
				t.Fatal("OAEP encryption reused entropy")
			}
		})
	}
}

func TestOAEPInvalidKeyDoesNotExposePIN(t *testing.T) {
	pin := "synthetic-pin-do-not-log"
	_, err := Encrypt("synthetic-public-key", pin)
	if err == nil {
		t.Fatal("invalid key accepted")
	}
	if strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, err), pin) {
		t.Fatal("secret leaked in errors")
	}
}

func TestOAEPRejectsOversizedMessage(t *testing.T) {
	key := fixtureKey(t)
	_, err := Encrypt(base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey)), strings.Repeat("x", 215))
	if err == nil {
		t.Fatal("overlong OAEP plaintext accepted")
	}
}
