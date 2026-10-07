package sber_test

import (
	"encoding/json"
	"testing"

	sber "github.com/vasyza/sber-go"
)

type cycle4ZeroStruct struct{}

func (cycle4ZeroStruct) IsZero() bool                 { return true }
func (cycle4ZeroStruct) MarshalJSON() ([]byte, error) { return []byte(`"<zero-struct-retained>"`), nil }

type cycle4EmptyCustomSlice []string

var cycle4EmptySliceCalls int

func (cycle4EmptyCustomSlice) MarshalJSON() ([]byte, error) {
	cycle4EmptySliceCalls++
	return []byte(`"<not-called-for-empty>"`), nil
}

func TestModelsReviewCycle4OmitEmptyEncoderSemantics(t *testing.T) {
	zero := 0
	input := struct {
		Cards      []*sber.BankCard             `json:"cards,omitempty"`
		Accounts   map[string]*sber.BankAccount `json:"accounts,omitempty"`
		KeptEmpty  []any                        `json:"kept_empty"`
		KeptNil    []any                        `json:"kept_nil"`
		ZeroStruct struct {
			N int `json:"n"`
		} `json:"zero_struct,omitempty"`
		ZeroArray      [1]int                 `json:"zero_array,omitempty"`
		EmptyArray     [0]int                 `json:"empty_array,omitempty"`
		ZeroCustom     cycle4ZeroStruct       `json:"zero_custom,omitempty"`
		EmptyCustom    cycle4EmptyCustomSlice `json:"empty_custom,omitempty"`
		TypedNil       any                    `json:"typed_nil,omitempty"`
		EmptyInterface any                    `json:"empty_interface,omitempty"`
		Pointer        *int                   `json:"pointer,omitempty"`
		False          bool                   `json:"false,omitempty"`
		Zero           int                    `json:"zero,omitempty"`
		ZeroFloat      float64                `json:"zero_float,omitempty"`
		EmptyText      string                 `json:"empty_text,omitempty"`
		FakeOption     string                 `json:"fake_option,notomitempty"`
		FakeSuffix     string                 `json:"fake_suffix,omitempty_suffix"`
	}{Cards: []*sber.BankCard{}, Accounts: map[string]*sber.BankAccount{}, KeptEmpty: []any{}, EmptyCustom: cycle4EmptyCustomSlice{}, TypedNil: (*sber.BankAccount)(nil), Pointer: &zero}
	expected := map[string]any{
		"kept_empty": []any{}, "kept_nil": nil, "zero_struct": map[string]any{"n": 0}, "zero_array": []int{0},
		"zero_custom": "<zero-struct-retained>", "typed_nil": nil, "pointer": 0, "fake_option": "", "fake_suffix": "",
	}
	// The actual selected encoder, as well as legacy json.Marshal, agree.
	official, _ := json.Marshal(input)
	independentAssert(t, independentRaw(official), expected)
	independentAssert(t, independentFallback{Payload: input}, map[string]any{"payload": expected})
	cycle4EmptySliceCalls = 0
	independentAssert(t, input, expected)
	independentAssert(t, &input, expected)
	if cycle4EmptySliceCalls != 0 {
		t.Fatal("omitempty called a custom serializer on an empty slice")
	}
}

func TestModelsReviewCycle4OmitEmptyNilEmptyNonemptyContainers(t *testing.T) {
	a := sber.NewBankAccount(sber.Account{ID: independentID, Balance: independentMoney(t, independentAmount, "RUB")}, nil, nil)
	for _, tc := range []struct {
		name     string
		cards    []*sber.BankCard
		accounts map[string]*sber.BankAccount
		want     any
	}{
		{"nil", nil, nil, map[string]any{"keep": []any{}}},
		{"explicit_empty", []*sber.BankCard{}, map[string]*sber.BankAccount{}, map[string]any{"keep": []any{}}},
		{"nonempty", []*sber.BankCard{nil}, map[string]*sber.BankAccount{"a": a}, map[string]any{"cards": []any{nil}, "accounts": map[string]any{"a": a.Snapshot()}, "keep": []any{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := struct {
				Cards    []*sber.BankCard             `json:"cards,omitempty"`
				Accounts map[string]*sber.BankAccount `json:"accounts,omitempty"`
				Keep     []any                        `json:"keep"`
			}{tc.cards, tc.accounts, []any{}}
			independentAssert(t, input, tc.want)
			independentAssert(t, independentFallback{Payload: input}, map[string]any{"payload": tc.want})
		})
	}
}
