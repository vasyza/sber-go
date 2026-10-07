// Package cli is the owner-operated native command boundary. Current commands
// are offline; no default owner-state discovery or bank request occurs.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/spf13/cobra"
	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/command"
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
		return fail(diagnostics, 2, "The command context or output is not valid.")
	}
	code := 0
	root := &cobra.Command{
		Use:   "sber COMMAND",
		Short: "Check a local profile without a bank request.",
		Long:  "Check a local profile without a bank request.\nUse --profile to give the profile file path.",
		RunE: func(*cobra.Command, []string) error {
			return command.ErrArguments
		},
	}
	root.AddCommand(
		profileCommand("status", "Check the profile file properties.",
			"Check the profile file properties.\nThe command does not read the file contents.\nThe command does not make a profile.", output, diagnostics, &code),
		profileCommand("inspect-session", "Show the profile properties.",
			"Show the properties of a private profile.\nThe command reads the profile.\nThe output does not contain cookie values, deviceprints, or tokens.", output, diagnostics, &code),
	)
	if err := command.Execute(ctx, root, args, output); err != nil {
		if errors.Is(err, command.ErrOutput) {
			return fail(diagnostics, 3, "The command cannot write the output.")
		}
		return fail(diagnostics, 2, "The command arguments are not valid.\nUse sber --help for command help.")
	}
	return code
}

func profileCommand(name, short, long string, output, diagnostics io.Writer, code *int) *cobra.Command {
	var profile string
	cmd := &cobra.Command{
		Use:   name + " --profile PATH",
		Short: short,
		Long:  long,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil || profile == "" {
				return command.ErrArguments
			}
			return nil
		},
		Run: func(cmd *cobra.Command, _ []string) {
			*code = runOffline(cmd.Context(), name, profile, output, diagnostics)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Use the profile file at `PATH`.")
	_ = cmd.MarkFlagRequired("profile")
	return cmd
}

func runOffline(ctx context.Context, name, profile string, output, diagnostics io.Writer) int {
	if ctx.Err() != nil {
		return fail(diagnostics, 2, "The command context or output is not valid.")
	}
	facts := map[string]any{"command": name, "bank_authorization_checked": false}
	if name == "inspect-session" {
		bundle, err := sber.LoadSessionBundle(profile)
		if err != nil {
			return fail(diagnostics, 3, "The command cannot read the private profile.")
		}
		facts["metadata"] = bundle.Redacted()
	} else {
		exists, err := enrollment.SafeProfileExists(profile)
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
