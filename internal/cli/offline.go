// Package cli is the owner-operated native command boundary. Current commands
// are offline; no default owner-state discovery or bank request occurs.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"io"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/enrollment"
)

func fail(output io.Writer, code int, message string) int {
	if output != nil {
		_, _ = io.WriteString(output, message+"\n")
	}
	return code
}

// Run executes an explicitly selected offline command. Secret argument/env
// loaders are deliberately absent; a profile path is not opened by status.
func Run(ctx context.Context, args []string, output, diagnostics io.Writer) int {
	if ctx == nil || ctx.Err() != nil || output == nil {
		return fail(diagnostics, 2, "invalid command context")
	}
	if len(args) == 0 || (args[0] != "status" && args[0] != "inspect-session") {
		return fail(diagnostics, 2, "usage: sber {status|inspect-session} --profile PATH")
	}
	flags := flag.NewFlagSet("sber", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "", "explicit profile path")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || *profile == "" {
		return fail(diagnostics, 2, "usage: sber {status|inspect-session} --profile PATH")
	}
	facts := map[string]any{"command": args[0], "bank_authorization_checked": false}
	if args[0] == "inspect-session" {
		bundle, err := sber.LoadSessionBundle(*profile)
		if err != nil {
			return fail(diagnostics, 3, "cannot inspect private profile")
		}
		facts["metadata"] = bundle.Redacted()
	} else {
		exists, err := enrollment.SafeProfileExists(*profile)
		if err != nil {
			return fail(diagnostics, 3, "unsafe profile metadata")
		}
		facts["profile_exists"] = exists
	}
	if err := json.NewEncoder(output).Encode(facts); err != nil {
		return fail(diagnostics, 3, "output failed")
	}
	return 0
}
