package sber_test

import (
	"testing"

	sber "github.com/vasyza/sber-go"
)

type cycle4Slice []any

func TestModelsReviewCycle4FiniteBackingViews(t *testing.T) {
	a := sber.NewBankAccount(sber.Account{ID: independentID, Balance: independentMoney(t, independentAmount, "RUB")}, nil, nil)
	prefix := make([]any, 2, 4)
	prefix[0] = a
	prefix[1] = prefix[:1:1]
	empty := make([]any, 2, 4)
	empty[0] = empty[:0:0]
	empty[1] = a
	shifted := make([]any, 3)
	shifted[0] = shifted[1:2]
	shifted[1] = a
	shifted[2] = shifted[1:2]
	named := make(cycle4Slice, 2)
	named[0] = a
	named[1] = named[:1]
	w := a.Snapshot()
	for _, tc := range []struct {
		name        string
		input, want any
	}{
		{"prefix_cap_limited", prefix, []any{w, []any{w}}},
		{"empty_cap_limited", empty, []any{[]any{}, w}},
		{"shifted_shared_siblings", shifted, []any{[]any{w}, w, []any{w}}},
		{"named_prefix", named, []any{w, []any{w}}},
		{"prefix_pointer", &prefix, []any{w, []any{w}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			independentAssert(t, tc.input, tc.want)
			independentAssert(t, independentFallback{Payload: tc.input}, map[string]any{"payload": tc.want})
		})
	}
	if prefix[0] != a || empty[1] != a || a.ID() != independentID {
		t.Fatal("view traversal mutated native input")
	}
}

func TestModelsReviewCycle4ActualCyclesRemainRejected(t *testing.T) {
	self := make([]any, 1)
	self[0] = self
	named := make(cycle4Slice, 1)
	named[0] = named
	// Unequal active lengths can still eventually repeat a real view.
	prefix := make([]any, 2)
	prefix[0] = prefix[:1]
	prefix[1] = 42
	capacity := make([]any, 1, 4)
	capacity[0] = capacity[:1:1]
	pairA := make([]any, 1)
	pairB := make([]any, 1)
	pairA[0] = pairB
	pairB[0] = pairA
	m := map[string]any{}
	m["self"] = m
	o := sber.OperationDetail{UOHID: independentID}
	o.Fields = []sber.OperationDetailField{{Value: &o}}
	for _, tc := range []struct {
		name string
		in   any
	}{
		{"self", self}, {"named", named}, {"nested_prefix_cycle", prefix}, {"capacity_not_identity", capacity},
		{"mutual", pairA}, {"map", m}, {"native_union", &o},
	} {
		t.Run(tc.name, func(t *testing.T) { independentReject(t, tc.in) })
	}
	independentAssert(t, []any{[]any{}, []any{}, nil}, []any{[]any{}, []any{}, nil})
}
