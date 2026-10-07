package bank

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientCycle3AuthGuardGatesEveryFreshOrAliasedGeneration(t *testing.T) {
	b := clientFixture(t, "cycle3-dynamic-auth")
	old, auth, next := clientFake(t, b), clientFake(t, b), clientFake(t, b)
	c, err := NewSberClient(b, ClientOptions{Transport: old})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	calls := 0
	guarded := clientGuardAuthOptions(sdkAuth.AuthOptions{Transport: clientCycle3ValueOwner{auth, []byte{1}}, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		calls++
		if calls == 1 {
			return old, errors.New("synthetic borrowed factory failure")
		}
		return clientCycle3ValueOwner{next, []byte{2}}, nil
	}}, c.claimAuthTransport)
	first, err := guarded.TransportFactory(b, sdkTransport.TransportOptions{})
	if err != nil || first == nil || first.CookieJar() != auth.jar || first.(ClientTransportOwner).ClientTransportOwner() != auth {
		t.Fatal("fresh explicit value auth owner not supported")
	}
	// This also exercises transparently delegated generic POST, without auth
	// credentials, endpoint discovery, financial sends, or network transport.
	payload := map[string]any{"synthetic": "delegation"}
	if _, err = first.Post(context.Background(), "https://example.invalid/synthetic", payload, sdkTransport.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	got := auth.snapshotCalls()
	if len(got) != 1 || !reflect.DeepEqual(got[0].body, payload) {
		t.Fatal("auth adapter altered generic POST")
	}
	if rejected, err := guarded.TransportFactory(b, sdkTransport.TransportOptions{}); rejected != nil || err == nil {
		t.Fatal("later auth generation took business owner")
	}
	if _, closes, _ := old.counts(); closes != 0 {
		t.Fatal("factory error closed borrowed business owner")
	}
	second, err := guarded.TransportFactory(b, sdkTransport.TransportOptions{})
	if err != nil || second.CookieJar() != next.jar || calls != 2 {
		t.Fatal("direct injection reused instead of moving to original factory")
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tr := range []*clientFakeTransport{old, auth, next} {
		if _, closes, _ := tr.counts(); closes != 1 {
			t.Fatal("auth/client cleanup duplicated a closing owner")
		}
	}
}

func TestClientCycle3ConcurrentAuthClaimsAcquireAnAliasOnlyOnce(t *testing.T) {
	b := clientFixture(t, "cycle3-parallel-auth")
	old, candidate := clientFake(t, b), clientFake(t, b)
	c, err := NewSberClient(b, ClientOptions{Transport: old})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	start := make(chan struct{})
	results := make(chan error, 16)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := c.claimAuthTransport(clientCycle3ValueOwner{candidate, []byte{byte(i)}})
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatal("concurrent aliases acquired multiple closing owners")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, closes, _ := candidate.counts(); closes != 1 {
		t.Fatal("concurrent claimed owner close duplicated")
	}
}
