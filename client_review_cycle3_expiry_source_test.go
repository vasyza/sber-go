package sber

import (
	"encoding/json"
	"math"
	"testing"
)

func TestClientCycle3BooleanTimestampRetainsSourceExpiredSemantics(t *testing.T) {
	for _, value := range []bool{false, true} {
		t.Run(map[bool]string{false: "false", true: "true"}[value], func(t *testing.T) {
			builds := 0
			c, err := NewSberClientFromFiles(clientCycle3HAR(t, map[string]any{"expires": value}, false), "", ClientOptions{TransportFactory: func(b SessionBundle, _ TransportOptions) (Transport, error) { builds++; return clientFake(t, b), nil }})
			if c != nil {
				c.Close()
			}
			if err == nil || c != nil || builds != 0 {
				t.Fatal("source numeric boolean expiry became a live session cookie")
			}
		})
	}
}

func TestClientCycle3FutureNumericOverflowKeepsExplicitBoundedLifetime(t *testing.T) {
	for _, number := range []string{"1e30", "9223372036854775808", "999999999999999999999999999999999999999999999999999999"} {
		t.Run(number, func(t *testing.T) {
			c, err := NewSberClientFromFiles(clientCycle3HAR(t, map[string]any{"expires": json.Number(number)}, false), "", ClientOptions{TransportFactory: func(b SessionBundle, _ TransportOptions) (Transport, error) { return clientFake(t, b), nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			b, err := c.ExportSession()
			if err != nil || b.Cookies[0].Expires == nil || *b.Cookies[0].Expires != math.MaxInt64 {
				t.Fatal("explicit future lifetime erased or narrowed")
			}
		})
	}
}
