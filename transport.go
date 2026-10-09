// Public transport API; implementation lives in internal/transport.
package sber

import (
	sdkTransport "github.com/vasyza/sber-sdk/internal/transport"
)

type HeaderOverrides = sdkTransport.HeaderOverrides
type RequestOptions = sdkTransport.RequestOptions
type Response = sdkTransport.Response
type TransportOptions = sdkTransport.TransportOptions
type ProxyOptions = sdkTransport.ProxyOptions
type HTTPTransport = sdkTransport.HTTPTransport

func NewHTTPTransport(b SessionBundle, o TransportOptions) (*HTTPTransport, error) {
	return sdkTransport.NewHTTPTransport(b, o)
}

func NewAuthenticationTransport(b SessionBundle, o TransportOptions) (*HTTPTransport, error) {
	return sdkTransport.NewAuthenticationTransport(b, o)
}

type Transport = sdkTransport.Transport

const PublicBootstrapURL = sdkTransport.PublicBootstrapURL

func IsBrowserCheck(html string) bool { return sdkTransport.IsBrowserCheck(html) }
