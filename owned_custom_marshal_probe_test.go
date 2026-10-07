package sber

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Synthetic, native-only integrity regression. No bank or credentials.
// Original invalid UTF-8 must be rejected before an owned custom serializer
// hands repaired bytes to the generic walker; distinct IDs must not merge.
func TestIndependentOwnedCustomMarshalRejectsNativeIdentityRepair(t *testing.T) {
	for _, kind := range []string{"account", "card", "portfolio"} {
		t.Run(kind, func(t *testing.T) {
			var native any
			var raw any
			var explicit func() ([]byte, error)
			switch kind {
			case "account":
				v := NewBankAccount(Account{ID: "synthetic_A\xff", Kind: "ctaccount"}, &independentRequester{}, nil)
				native = v
				raw = v.Snapshot()
				explicit = v.ExportJSON
			case "card":
				v := NewBankCard(Card{ID: "synthetic_A\xff"}, &independentRequester{}, nil)
				native = v
				raw = v.Snapshot()
				explicit = v.ExportJSON
			case "portfolio":
				v := NewBankPortfolio(Products{Accounts: []Account{{ID: "synthetic_parent", Kind: "ctaccount"}}, Cards: []Card{{ID: "synthetic_A\xff"}, {ID: "synthetic_A\xfe"}}}, &independentRequester{})
				native = v
				raw = v.Raw()
				explicit = v.ExportJSON
			}
			if _, err := explicit(); err == nil {
				t.Fatal("explicit export control did not reject")
			}
			direct, directErr := native.(json.Marshaler).MarshalJSON()
			data, err := json.Marshal(native)
			t.Logf("owned MarshalJSON kind=%s direct=%s directErr=%v returned=%s error=%v raw=%#v", kind, direct, directErr, data, err, raw)
			if err == nil || directErr == nil {
				t.Fatalf("owned custom serializer silently repaired distinct invalid UTF-8 IDs to U+FFFD: %s", data)
			}
		})
	}
}
func TestIndependentOwnedCustomMarshalValidReplacementCharacterControl(t *testing.T) {
	raw := Card{ID: "synthetic_A�"}
	v := NewBankCard(raw, &independentRequester{}, nil)
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Card
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, raw) || v.ID() != raw.ID {
		t.Fatal("valid literal replacement character changed")
	}
}
