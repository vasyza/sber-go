// Package rsaoaep implements the audited frontend's RSA-OAEP wire flavor:
// SHA-1, MGF1/SHA-1 and an empty label. The hash is protocol compatibility,
// not a recommendation for designing a new cryptographic protocol.
package rsaoaep

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"strings"
)

var (
	ErrPublicKey  = errors.New("invalid RSA public key")
	ErrEncryption = errors.New("RSA-OAEP encryption failed")
)

// Encrypt accepts Base64 DER PKCS#1 or SubjectPublicKeyInfo, not an owner
// private key. Failures deliberately contain neither plaintext nor key input.
func Encrypt(publicDERBody, plaintext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(publicDERBody), ""))
	if err != nil || len(raw) == 0 {
		return "", ErrPublicKey
	}
	key, err := x509.ParsePKCS1PublicKey(raw)
	if err != nil {
		parsed, parseError := x509.ParsePKIXPublicKey(raw)
		if parseError != nil {
			return "", ErrPublicKey
		}
		var ok bool
		key, ok = parsed.(*rsa.PublicKey)
		if !ok {
			return "", ErrPublicKey
		}
	}
	if key.N == nil || key.N.BitLen() < 2048 || key.E < 3 || key.E%2 == 0 {
		return "", ErrPublicKey
	}
	cipher, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, key, []byte(plaintext), nil)
	if err != nil {
		return "", ErrEncryption
	}
	return base64.StdEncoding.EncodeToString(cipher), nil
}
