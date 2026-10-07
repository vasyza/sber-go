package cli

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	sber "github.com/vasyza/sber-go"
)

func runRefresh(ctx context.Context, args *commandArguments, output, diagnostics io.Writer, dependencies *Authentication) int {
	profile, ca, selection := args.profile, args.ca, args.browser
	if _, err := sber.LoadSessionBundle(profile); err != nil {
		return fail(diagnostics, 3, "cannot open private profile; no login attempted")
	}
	options, err := selection.authOptions(ca)
	if err != nil {
		return fail(diagnostics, 3, "cannot prepare authentication; saved profile retained")
	}
	a := Authentication{}
	if dependencies != nil {
		a = *dependencies
	}
	if a.NewPIN == nil {
		a.NewPIN = func(path string, options sber.AuthOptions) (PINAuthenticator, error) {
			return sber.NewPINAuthFromProfile(path, options)
		}
	}
	if a.ReadSecret == nil {
		a.ReadSecret = ownerSecret
	}
	bundle, err := authenticateRemembered(ctx, a.ReadSecret, func() (PINAuthenticator, error) {
		return a.NewPIN(profile, options)
	})
	if err != nil {
		if ctx.Err() != nil {
			return 130
		}
		message := strings.ReplaceAll(loginFailureMessage(err), "; profile not published", "")
		return fail(diagnostics, 3, message+"; saved profile retained")
	}
	if ctx.Err() != nil {
		return 130
	}
	if err := bundle.Save(profile); err != nil {
		return fail(diagnostics, 3, "cannot confirm refreshed profile publication; inspect profile metadata before trying again")
	}
	if err := json.NewEncoder(output).Encode(map[string]any{"session_refreshed": true, "bank_authorization_checked": true, "mutations_enabled": false}); err != nil {
		return fail(diagnostics, 3, "profile refreshed; output failed")
	}
	return 0
}
