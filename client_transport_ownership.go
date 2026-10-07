package sber

import "reflect"

// ClientTransportOwner is an optional closing-ownership contract for injected
// wrappers, especially non-comparable value transports. The returned handle
// must be non-nil, comparable, stable for the wrapper's lifetime and identical
// for every alias of the same physical Close owner. Distinct closing owners
// must return distinct handles. The client compares it but never calls Close
// or performs I/O through this handle. Ordinary comparable transports need
// not implement this interface; their own value is their ownership identity.
//
// An unverifiable first business transport is supported as a standalone owner,
// but cannot participate in rotation/separate auth ownership. Rejected unsafe
// candidates are not acquired or closed: their factory retains responsibility.
// This contract does not prove arbitrary hidden physical aliasing.
type ClientTransportOwner interface {
	ClientTransportOwner() Transport
}

func clientClosingIdentity(tr Transport) Transport {
	if !clientTransportPresent(tr) {
		return nil
	}
	if owner, ok := tr.(ClientTransportOwner); ok {
		tr = owner.ClientTransportOwner()
	}
	if !clientTransportPresent(tr) || !reflect.ValueOf(tr).Comparable() {
		return nil
	}
	return tr
}

func clientNewOwnedTransport(tr Transport) *clientOwnedTransport {
	return &clientOwnedTransport{transport: tr, identity: clientClosingIdentity(tr)}
}

func clientSameTransport(a, b Transport) bool {
	return clientTransportPresent(a) && clientTransportPresent(b) &&
		reflect.ValueOf(a).Comparable() && reflect.ValueOf(b).Comparable() && a == b
}
