package strictjson

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestValidateRejectsInvalidUTF8(t *testing.T) {
	raw := append([]byte(`{"id":"`), 0xff)
	raw = append(raw, []byte(`"}`)...)
	if err := Validate(raw); err == nil {
		t.Fatal("invalid UTF-8 was accepted")
	}
	if err := Validate([]byte(`{"amount":0.01000000000000000000000001}`)); err != nil {
		t.Fatal("valid JSON rejected")
	}
}

func TestValidateRejectsUnpairedEscapes(t *testing.T) {
	for _, raw := range []string{`"\ud800"`, `"\udfff"`, `"\ud800x"`, `"\ud800\u0061"`, `"\udfff\ud800"`, `{"id":"\ud800"}`} {
		if err := Validate([]byte(raw)); err == nil {
			t.Fatalf("unpaired surrogate accepted: %q", raw)
		}
	}
	for _, raw := range []string{`"\ud83d\ude00"`, `"\ufffd"`, `"\\ud800"`, `"a\"b"`, `"\\"`, `{"text":"雪😀"}`} {
		if err := Validate([]byte(raw)); err != nil {
			t.Fatalf("valid string rejected: %q", raw)
		}
	}
}

func TestValidateRejectsDecodedDuplicateProperties(t *testing.T) {
	for _, raw := range []string{`{"amount":true,"amount":"1.00"}`, `{"body":{"operations":[{"id":"known"}],"operations":[]}}`, `{"id":1,"\u0069d":2}`, `[{"a":1,"a":2}]`} {
		if err := Validate([]byte(raw)); err == nil {
			t.Fatalf("decoded duplicate property accepted: %q", raw)
		}
	}
	for _, raw := range []string{`[{"id":1},{"id":2}]`, `{"amount":"1.00","nested":{"amount":"2.00"}}`, `{"body":{"operations":[]}}`, `[]`, `null`, `1e-9999`} {
		if err := Validate([]byte(raw)); err != nil {
			t.Fatalf("valid JSON rejected: %q", raw)
		}
	}
}

func TestValidateStaticErrorsAndCompleteDocuments(t *testing.T) {
	for _, raw := range []string{"", `{"private":"synthetic-do-not-log"`, `{} {}`, `01`, `NaN`, `"\x41"`, `[1,]`, `{`, `[`} {
		if err := Validate([]byte(raw)); err != ErrInvalidJSON || strings.Contains(err.Error(), "synthetic") {
			t.Fatal("invalid document did not return static error")
		}
	}
	raw := []byte(`{"id":"\ud83d\ude00","amount":0.010000000000000000000000001}`)
	original := append([]byte{}, raw...)
	if err := Validate(raw); err != nil || !bytes.Equal(raw, original) {
		t.Fatal("validation changed the supplied document")
	}
}

func FuzzValidate(f *testing.F) {
	for _, raw := range [][]byte{[]byte(`{"id":"known","id":"replacement"}`), []byte(`"\ud800"`), []byte(`"\ud83d\ude00"`), []byte(`{"body":{"operations":[]}}`), []byte(`1e-9999`), {0xff}, []byte(`"\\ud800"`)} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 65536 {
			t.Skip()
		}
		original := append([]byte{}, raw...)
		err := Validate(raw)
		if !bytes.Equal(raw, original) {
			t.Fatal("validation mutated bytes")
		}
		if err != nil {
			if err != ErrInvalidJSON {
				t.Fatal("unexpected error")
			}
			return
		}
		if !utf8.Valid(raw) || !json.Valid(raw) {
			t.Fatal("accepted invalid text or document")
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var value any
		if err := dec.Decode(&value); err != nil {
			t.Fatal(err)
		}
		canonical, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(canonical); err != nil {
			t.Fatal("accepted value failed canonical round trip")
		}
	})
}

// Assert generated violations before permissive decoding can erase them.
func FuzzRejectCorruptions(f *testing.F) {
	f.Add([]byte("synthetic"), uint16(0))
	f.Add([]byte{}, uint16(0x7ff))
	f.Fuzz(func(t *testing.T, seed []byte, code uint16) {
		if len(seed) > 128 {
			seed = seed[:128]
		}
		key := "k" + hex.EncodeToString(seed)
		quoted, _ := json.Marshal(key)
		alias := `"\u006b` + key[1:] + `"`
		duplicate := []byte(`{"body":[{` + string(quoted) + `:0,` + alias + `:1}]}`)
		unpaired := []byte(fmt.Sprintf(`{"id":"\u%04x"}`, uint16(0xd800)|(code&0x7ff)))
		for _, raw := range [][]byte{duplicate, unpaired} {
			original := append([]byte{}, raw...)
			if err := Validate(raw); err != ErrInvalidJSON {
				t.Fatal("generated corruption accepted")
			}
			if !bytes.Equal(raw, original) {
				t.Fatal("rejected input mutated")
			}
		}
		paired := []byte(fmt.Sprintf(`{"id":"\u%04x\u%04x"}`, uint16(0xd800)|(code&0x3ff), uint16(0xdc00)|(code&0x3ff)))
		original := append([]byte{}, paired...)
		if err := Validate(paired); err != nil {
			t.Fatal("valid generated pair rejected")
		}
		if !bytes.Equal(paired, original) {
			t.Fatal("accepted input mutated")
		}
	})
}
