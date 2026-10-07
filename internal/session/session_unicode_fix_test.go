package session

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFoundationSessionEscapedSurrogates(t *testing.T) {
	b, err := NewSessionBundle(syntheticBundle())
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := json.Marshal(b.secretPayload())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeSessionBundle(baseline); err != nil {
		t.Fatalf("invalid base fixture: %v", err)
	}
	for _, field := range []string{"synthetic-session-secret", "synthetic-agent-secret", "synthetic-device-secret", "synthetic-antifraud-secret", "2026-01-01T00:00:00+00:00"} {
		for _, escaped := range []string{`\ud800`, `\udfff`, `\ud800x`, `\ud800\ud800`, `\udc00\ud800`, `\ud83d\u0041`} {
			raw := bytes.Replace(baseline, []byte(field), []byte(escaped), 1)
			if !json.Valid(raw) {
				t.Fatal("invalid JSON fixture, not a surrogate repro")
			}
			if _, err := DecodeSessionBundle(raw); err == nil {
				t.Errorf("escaped unpaired surrogate silently rewritten in %s", field)
			}
		}
	}
	for _, raw := range []string{`{"\ud800":"value"}`, `{"unknown":{"key":"\ud800"}}`, `{"nested":[{"key":"\udfff"}]}`} {
		if _, err := decodeUniqueJSON([]byte(raw)); err == nil {
			t.Errorf("private decoder rewrote surrogate in key/nested/unknown data: %s", raw)
		}
	}
	for _, tt := range []struct{ escaped, exact string }{
		{`\ud83d\ude00`, "😀"}, {`\\ud800`, `\ud800`}, {`\\uDFFF`, `\uDFFF`}, {`\ufffd`, "�"},
	} {
		raw := bytes.Replace(baseline, []byte("synthetic-session-secret"), []byte(tt.escaped), 1)
		decoded, err := DecodeSessionBundle(raw)
		if err != nil || len(decoded.Cookies) != 1 || decoded.Cookies[0].Value != tt.exact {
			t.Errorf("valid/literal Unicode did not remain exact: %s (%v)", tt.escaped, err)
		}
	}
	for _, raw := range []string{`{"same":1,"same":2}`, `{"\u0073ame":1,"same":2}`, strings.Repeat("[", 130) + "0" + strings.Repeat("]", 130)} {
		if _, err := decodeUniqueJSON([]byte(raw)); err == nil {
			t.Error("existing duplicate/depth guard weakened")
		}
	}
}
