// Compatibility facade; implementation lives in internal/transport.
package sber

import (
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

type HeaderOverrides = sdkTransport.HeaderOverrides
type RequestOptions = sdkTransport.RequestOptions
type Response = sdkTransport.Response
type TransportOptions = sdkTransport.TransportOptions
type HTTPTransport = sdkTransport.HTTPTransport

func NewHTTPTransport(b SessionBundle, o TransportOptions) (*HTTPTransport, error) {
	return sdkTransport.NewHTTPTransport(b, o)
}

func NewAuthenticationTransport(b SessionBundle, o TransportOptions) (*HTTPTransport, error) {
	return sdkTransport.NewAuthenticationTransport(b, o)
}

type Transport = sdkTransport.Transport

const PublicBootstrapURL = sdkTransport.PublicBootstrapURL

type BrowserBootstrapResult = sdkTransport.BrowserBootstrapResult
type BrowserBootstrapProvider = sdkTransport.BrowserBootstrapProvider
type BrowserBootstrapFunc = sdkTransport.BrowserBootstrapFunc

func NewBrowserBootstrapResult(r BrowserBootstrapResult) (BrowserBootstrapResult, error) {
	return sdkTransport.NewBrowserBootstrapResult(r)
}

func IsBrowserCheck(html string) bool { return sdkTransport.IsBrowserCheck(html) }
