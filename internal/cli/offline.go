// Package cli is the owner-operated native command boundary. Each command uses
// --profile or the user default. Secrets are read only from a local terminal.
package cli

import (
	"encoding/json"
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

// runOffline executes an explicitly selected offline command. Secret argument/env
// loaders are deliberately absent; a profile path is not opened by status.
func runOffline(command string, args *commandArguments, output, diagnostics io.Writer) int {
	facts := map[string]any{"command": command, "bank_authorization_checked": false}
	if command == "inspect-session" {
		bundle, err := sber.LoadSessionBundle(args.profile)
		if err != nil {
			return fail(diagnostics, 3, "The command cannot read the private profile.")
		}
		facts["metadata"] = bundle.Redacted()
	} else {
		exists, err := enrollment.SafeProfileExists(args.profile)
		if err != nil {
			return fail(diagnostics, 3, "The profile file properties are not safe.")
		}
		facts["profile_exists"] = exists
	}
	if err := json.NewEncoder(output).Encode(facts); err != nil {
		return fail(diagnostics, 3, "The command cannot write the output.")
	}
	return 0
}
