package bank

import (
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
)

// NewSberClientFromCredentials holds the observed minimum cookie pair in memory.
// Hosts must both be explicit or both absent (runtime discovery). SessionPath,
// when set, persists only rotated credentials, not extra protection cookies.
func NewSberClientFromCredentials(credentials sdkSession.SberCredentials, bundleOptions sdkSession.CredentialsBundleOptions, options ClientOptions) (*SberClient, error) {
	if (bundleOptions.APIBase == "") != (bundleOptions.WebBase == "") {
		return nil, &sdkErrs.MissingSession{Message: "API and web hosts must be paired"}
	}
	b, e := credentials.ToBundle(bundleOptions)
	if e != nil {
		return nil, e
	}
	c, e := NewSberClient(b, options)
	if e != nil {
		return nil, e
	}
	c.core().credentialsOnly = true
	return c, nil
}

// NewSberClientFromSessionFile loads only the explicitly named profile. Private
// files are mandatory unless SessionLoadOptions explicitly relaxes permissions.
func NewSberClientFromSessionFile(path string, options ClientOptions, load ...sdkSession.SessionLoadOptions) (*SberClient, error) {
	b, e := sdkSession.LoadSessionBundle(path, load...)
	if e != nil {
		return nil, e
	}
	options.SessionPath = path
	c, e := NewSberClient(b, options)
	if e != nil {
		return nil, e
	}
	c.core().credentialsOnly = len(b.Cookies) == 2 && (b.Cookies[0].Name == "UFS-SESSION" && b.Cookies[1].Name == "UFS-TOKEN" || b.Cookies[0].Name == "UFS-TOKEN" && b.Cookies[1].Name == "UFS-SESSION")
	return c, nil
}
