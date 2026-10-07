package sber

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
)

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
			input = &TransportError{Code: marker, cause: foreign}
		case 1:
			input = &APIRejected{Message: marker, Code: marker, Text: marker, UUID: marker}
		case 2:
			input = &MissingSession{Message: marker}
		case 3:
			input = NewParseError(marker)
		case 4:
			input = &MutationUncertain{Message: marker}
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
		var te *TransportError
		var rej *APIRejected
		var missing *MissingSession
		var parsed *ParseError
		var uncertain *MutationUncertain
		if errors.As(got, &te) && te.Code == marker || errors.As(got, &rej) && (rej.Message == marker || rej.Code == marker || rej.Text == marker || rej.UUID == marker) || errors.As(got, &missing) && missing.Message == marker || errors.As(got, &parsed) && parsed.Field() == marker || errors.As(got, &uncertain) && uncertain.Message == marker {
			t.Fatal("generated copied fields retain input")
		}
	})
}
