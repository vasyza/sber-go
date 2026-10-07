package sber_test

import (
	"reflect"
	"testing"

	sber "github.com/vasyza/sber-go"
)

func TestModelsReviewCycle4ForeignPropertyNames(t *testing.T) {
	money := independentMoney(t, independentAmount, independentID)
	input := struct {
		Account sber.Account `json:"4111 1111 1111 1111"`
		Money   *sber.Money  `json:"financial"`
		Forged  string       `json:"forged" sber:"literal"`
	}{Account: sber.Account{ID: independentID, Name: independentPAN, Balance: money}, Money: money, Forged: independentID}
	expected := map[string]any{
		"•••• 1111": sber.Account{ID: independentID, Name: independentMasked, Balance: money},
		"financial": money, "forged": "•••• 1111",
	}
	for _, tc := range []struct {
		name     string
		in, want any
	}{
		{"value", input, expected}, {"pointer", &input, expected},
		{"stream", independentStream{Payload: input}, map[string]any{"~reached/0": expected, "foreign": nil}},
		{"fallback", independentFallback{Payload: input}, map[string]any{"payload": expected}},
	} {
		t.Run(tc.name, func(t *testing.T) { independentAssert(t, tc.in, tc.want) })
	}
	if input.Account.ID != independentID || input.Money.Amount.String() != independentAmount || input.Forged != independentID {
		t.Fatal("property-name policy mutated values")
	}
}

func TestModelsReviewCycle4PropertyNameCollisions(t *testing.T) {
	for _, input := range []any{
		struct {
			A int `json:"4111 1111 1111 1111"`
			B int `json:"4111-1111-1111-1111"`
		}{1, 2},
		struct {
			A int `json:"4111111111111111"`
			B int `json:"•••• 1111"`
		}{1, 2},
		reflect.ValueOf(struct{ A, B int }{1, 2}).Convert(reflect.StructOf([]reflect.StructField{
			{Name: "A", Type: reflect.TypeFor[int](), Tag: `json:"same"`},
			{Name: "B", Type: reflect.TypeFor[int](), Tag: `json:"same"`},
		})).Interface(),
	} {
		independentReject(t, input)
	}
	// Raw name integrity precedes tag unquoting and display masking.
	typ := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: reflect.TypeFor[string](), Tag: reflect.StructTag("json:\"original-invalid\xff\"")}})
	independentReject(t, reflect.New(typ).Elem().Interface())
	valid := struct {
		Value string `json:"literal � 😀"`
	}{"valid � 😀"}
	independentAssert(t, valid, map[string]any{"literal � 😀": "valid � 😀"})
	noPAN := struct {
		Value string `json:"4111111111111112"`
	}{"not-a-pan"}
	independentAssert(t, noPAN, map[string]any{"4111111111111112": "not-a-pan"})
}
