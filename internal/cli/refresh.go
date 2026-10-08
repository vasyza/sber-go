package cli

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	sber "github.com/vasyza/sber-go"
)

func runRefresh(ctx context.Context, args *commandArguments, output, diagnostics io.Writer, dependencies *Authentication) int {
	profile, ca := args.profile, args.ca
	if _, err := sber.LoadSessionBundle(profile); err != nil {
		return fail(diagnostics, 3, "The command cannot open the private profile.\nThe command did not start login.")
	}
	options, err := nativeAuthOptions(ca, args.selectedProxy)
	if err != nil {
		return fail(diagnostics, 3, "The command cannot prepare authentication.\nThe saved profile did not change.")
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
	if err := configureSecretInput(&a, args.envFile); err != nil {
		return fail(diagnostics, 3, credentialFailureMessage(err)+"\nThe saved profile did not change.")
	}
	bundle, err := authenticateRemembered(ctx, a.ReadSecret, func() (PINAuthenticator, error) {
		return a.NewPIN(profile, options)
	})
	if err != nil {
		if ctx.Err() != nil {
			return 130
		}
		message := strings.TrimSuffix(loginFailureMessage(err), "\nThe command did not publish the profile.")
		return fail(diagnostics, 3, message+"\nThe saved profile did not change.")
	}
	if ctx.Err() != nil {
		return 130
	}
	if err := bundle.Save(profile); err != nil {
		return fail(diagnostics, 3, "The command cannot confirm publication of the restored profile.\nRead the profile metadata before you try again.")
	}
	if err := json.NewEncoder(output).Encode(map[string]any{"session_refreshed": true, "bank_authorization_checked": true, "mutations_enabled": false}); err != nil {
		return fail(diagnostics, 3, "The command restored the profile.\nThe command cannot write the output.")
	}
	return 0
}
