package sber

import "testing"

func TestClientCycle4ErrorNodeBudgetIncludesTypedNilChildren(t *testing.T) {
	children := make([]error, 128)
	for i := range children {
		children[i] = (*APIRejected)(nil)
	}
	got := clientSafeError(&client4ManyErrors{children})
	tree, ok := got.(interface{ Unwrap() []error })
	if !ok || len(tree.Unwrap()) > 64 {
		t.Error("typed nil children escaped the client error graph budget")
	}
}

type client4ManyErrors struct{ children []error }

func (*client4ManyErrors) Error() string     { return "synthetic broad error graph" }
func (e *client4ManyErrors) Unwrap() []error { return e.children }
