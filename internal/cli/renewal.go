package cli

import (
	"context"
	"os"

	"golang.org/x/term"

	sber "github.com/vasyza/sber-sdk"
	"github.com/vasyza/sber-sdk/internal/ownerinput"
	"github.com/vasyza/sber-sdk/mcp"
)

func openReadClient(ctx context.Context, path string, options sber.ClientOptions, provider sber.PINProvider) (mcp.Client, error) {
	if provider != nil {
		bundle, err := sber.LoadSessionBundle(path)
		if err != nil {
			return nil, err
		}
		if bundle.Deviceprint != nil {
			return sber.NewSberClientFromPINProfile(ctx, path, provider, options)
		}
	}
	return sber.NewSberClientFromSessionFile(path, options)
}

func readClientOptions(command string, args *commandArguments, dependencies *Authentication) (sber.ClientOptions, sber.PINProvider, error) {
	options := sber.ClientOptions{TransportOptions: sber.TransportOptions{CABundle: args.ca, Timeout: args.timeout, Proxy: args.selectedProxy}}
	if isMutationCommand(command) {
		options.AllowMutations = args.execute
		return options, nil, nil
	}
	a := Authentication{}
	if dependencies != nil {
		a = *dependencies
	}
	if args.noRenew || command == "mcp" || command == "export-session" || command == "inspect-credentials" {
		return options, nil, nil
	}
	canRead := term.IsTerminal(int(os.Stdin.Fd())) || a.ReadSecret != nil
	if err := configureSecretInput(&a, args.envFile); err != nil {
		return options, nil, err
	}
	if !canRead && !a.configuredPIN {
		return options, nil, nil
	}
	if a.NewPINFromBundle == nil {
		a.NewPINFromBundle = func(bundle sber.SessionBundle, options sber.AuthOptions) (PINAuthenticator, error) {
			return sber.NewPINAuth(bundle, options)
		}
	}
	authOptions, err := nativeAuthOptions(args.ca, args.selectedProxy)
	if err != nil {
		return options, nil, err
	}
	options.AuthOptions = authOptions
	provider := sber.PINProvider(func(ctx context.Context) (string, error) { return a.ReadSecret(ctx, ownerinput.PIN) })
	options.Renewal = func(ctx context.Context, current sber.SessionBundle, pin sber.PINProvider, authOptions sber.AuthOptions) (sber.SessionBundle, error) {
		read := func(ctx context.Context, prompt ownerinput.Prompt) (string, error) {
			if prompt == ownerinput.PIN {
				return pin(ctx)
			}
			return a.ReadSecret(ctx, prompt)
		}
		return authenticateRemembered(ctx, read, func() (PINAuthenticator, error) { return a.NewPINFromBundle(current, authOptions) })
	}
	return options, provider, nil
}
