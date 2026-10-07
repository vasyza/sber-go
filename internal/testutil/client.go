// Package testutil provides synthetic, network-free application test fixtures.
package testutil

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	sber "github.com/vasyza/sber-go"
)

type Request struct {
	Path string
	Body map[string]any
}
type Client struct {
	Read     func(context.Context, string, map[string]any) (map[string]any, error)
	mu       sync.Mutex
	requests []Request
	Closes   atomic.Int32
	Warmups  atomic.Int32
}

func (c *Client) PostRead(ctx context.Context, path string, body map[string]any) (map[string]any, error) {
	c.mu.Lock()
	c.requests = append(c.requests, Request{path, body})
	c.mu.Unlock()
	if c.Read != nil {
		return c.Read(ctx, path, body)
	}
	if path == sber.ProductsPath {
		return map[string]any{"success": true, "body": map[string]any{"sections": map[string]any{"technicalSection": map[string]any{"sectionProductData": map[string]any{"ctaccounts": map[string]any{"data": []any{map[string]any{"id": "synthetic-account", "name": "card 4111111111111111", "balance": map[string]any{"amount": "9007199254740993.10", "currencyCode": "RUB"}}}}}}}}}, nil
	}
	return map[string]any{"success": true, "body": map[string]any{"operations": []any{}}}, nil
}
func (c *Client) Requests() []Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Request(nil), c.requests...)
}
func (*Client) Mutate(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error) {
	return nil, errors.New("synthetic mutations disabled")
}
func (*Client) MutationSequence(context.Context, func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error {
	return errors.New("synthetic mutations disabled")
}
func (*Client) ExportSession() (sber.SessionBundle, error) {
	return sber.NewSessionBundle(sber.SessionBundle{APIBase: sber.AppOrigin, WebBase: sber.AppOrigin, Cookies: []sber.CookieRecord{{Name: "fixture", Value: "synthetic-private-cookie", Domain: "online.sberbank.ru", Path: "/", Secure: true}}})
}
func (*Client) ExportCredentials() (sber.SberCredentials, error) {
	return sber.SberCredentials{}, errors.New("no synthetic credentials")
}
func (c *Client) WarmUp(context.Context, bool) error { c.Warmups.Add(1); return nil }
func (c *Client) Close() error                       { c.Closes.Add(1); return nil }
