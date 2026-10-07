// Package srp implements the ESA frontend's SHA-512 PIN proof flavor.
// Protocol port from ex3lite/sber-mcp (MIT); this is not generic RFC SRP.
package srp

import (
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
)

var (
	ErrInput                 = errors.New("invalid SRP input")
	ErrChallengeNotProcessed = errors.New("SRP challenge not processed")
	ErrEntropy               = errors.New("SRP cryptographic entropy unavailable")
)

// Client keeps ephemeral proof state; instances must not be copied.
type Client struct {
	clientFormatter
	mu              sync.Mutex
	n, g, a, public *big.Int
	width           int
	expected        []byte
}

// New obtains the private exponent from the operating system's CSPRNG.
func New(nHex, gHex string) (*Client, error) {
	return NewWithReader(nHex, gHex, rand.Reader)
}

// NewWithReader follows the frontend's entropy-width rule; only tests replace
// the reader. Reader failures never fall back to deterministic exponent state.
func NewWithReader(nHex, gHex string, reader io.Reader) (*Client, error) {
	n, err := hexInteger(nHex)
	if err != nil {
		return nil, err
	}
	g, err := hexInteger(gHex)
	if err != nil {
		return nil, err
	}
	if n.Cmp(big.NewInt(3)) <= 0 || g.Cmp(big.NewInt(1)) <= 0 || g.Cmp(n) >= 0 {
		return nil, ErrInput
	}
	byteLength := ((n.BitLen() + 7) / 8) / 8
	if byteLength == 0 {
		return nil, ErrInput
	}
	if reader == nil {
		return nil, ErrEntropy
	}
	bytes := make([]byte, byteLength)
	for {
		if _, err := io.ReadFull(reader, bytes); err != nil {
			return nil, ErrEntropy
		}
		a := new(big.Int).SetBytes(bytes)
		if a.Sign() != 0 {
			return NewWithPrivateHex(nHex, gHex, a.Text(16))
		}
	}
}

// NewWithPrivateHex is for deterministic protocol-vector verification.
// Live callers must use the separately provided cryptographic-entropy constructor.
func NewWithPrivateHex(nHex, gHex, aHex string) (*Client, error) {
	n, err := hexInteger(nHex)
	if err != nil {
		return nil, err
	}
	g, err := hexInteger(gHex)
	if err != nil {
		return nil, err
	}
	a, err := hexInteger(aHex)
	if err != nil {
		return nil, err
	}
	if n.Cmp(big.NewInt(3)) <= 0 || g.Cmp(big.NewInt(1)) <= 0 || g.Cmp(n) >= 0 || a.Sign() == 0 {
		return nil, ErrInput
	}
	public := new(big.Int).Exp(g, a, n)
	if public.Sign() == 0 {
		return nil, ErrInput
	}
	return &Client{n: n, g: g, a: a, public: public, width: (n.BitLen() + 7) / 8}, nil
}

func (c *Client) PublicHex() string { return c.public.Text(16) }

func (c *Client) String() string   { return "PinSRP(<redacted>)" }
func (c *Client) GoString() string { return c.String() }

// Promote a stateless formatter into both Client method sets. Unlike a Client
// value receiver this neither copies the mutex nor reads mutable proof state.
type clientFormatter struct{}

func (clientFormatter) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("PinSRP(<redacted>)")) }

// Process computes M1 and remembers the server M2 expected for this challenge.
func (c *Client) Process(pin, saltHex, serverHex string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expected = nil
	if pin == "" {
		return "", ErrInput
	}
	salt, err := hexInteger(saltHex)
	if err != nil {
		return "", err
	}
	server, err := hexInteger(serverHex)
	if err != nil {
		return "", err
	}
	if server.Sign() <= 0 || server.Cmp(c.n) >= 0 {
		return "", ErrInput
	}
	k := digestInteger(c.pad(c.n), c.pad(c.g))
	x := digestInteger(integerBytes(salt), []byte(pin))
	u := digestInteger(c.pad(c.public), c.pad(server))
	if u.Sign() == 0 {
		return "", ErrInput
	}
	gx := new(big.Int).Exp(c.g, x, c.n)
	base := new(big.Int).Sub(server, new(big.Int).Mul(k, gx))
	exponent := new(big.Int).Add(c.a, new(big.Int).Mul(u, x))
	shared := new(big.Int).Exp(base, exponent, c.n)
	if shared.Sign() == 0 {
		return "", ErrInput
	}
	sessionHash := digest(c.pad(shared))
	nHash, gHash := digest(integerBytes(c.n)), digest(integerBytes(c.g))
	mixed := make([]byte, sha512.Size)
	for i := range mixed {
		mixed[i] = nHash[i] ^ gHash[i]
	}
	m1 := new(big.Int).SetBytes(digest(mixed, integerBytes(salt), c.pad(c.public), c.pad(server), sessionHash))
	c.expected = digest(c.pad(c.public), integerBytes(m1), sessionHash)
	return m1.Text(16), nil
}

// Verify compares the padded server proof without data-dependent byte comparison.
func (c *Client) Verify(proofHex string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.expected == nil {
		return false, ErrChallengeNotProcessed
	}
	proof, err := hexInteger(proofHex)
	if err != nil || proof.BitLen() > sha512.Size*8 {
		return false, nil
	}
	supplied := make([]byte, sha512.Size)
	proof.FillBytes(supplied)
	return subtle.ConstantTimeCompare(c.expected, supplied) == 1, nil
}

func hexInteger(text string) (*big.Int, error) {
	if text == "" {
		return nil, ErrInput
	}
	for _, r := range text {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return nil, ErrInput
		}
	}
	v, ok := new(big.Int).SetString(text, 16)
	if !ok {
		return nil, ErrInput
	}
	return v, nil
}

func integerBytes(v *big.Int) []byte {
	b := v.Bytes()
	if len(b) == 0 {
		return []byte{0}
	}
	return b
}

func (c *Client) pad(v *big.Int) []byte {
	raw := integerBytes(v)
	if len(raw) >= c.width {
		return raw
	}
	out := make([]byte, c.width)
	copy(out[c.width-len(raw):], raw)
	return out
}

func digest(parts ...[]byte) []byte {
	h := sha512.New()
	for _, part := range parts {
		_, _ = h.Write(part)
	}
	return h.Sum(nil)
}

func digestInteger(parts ...[]byte) *big.Int { return new(big.Int).SetBytes(digest(parts...)) }
