package rentalcli

import (
	"context"
	"errors"
	"io"

	"github.com/spf13/cobra"
	"github.com/vasyza/sber-go/internal/command"
)

// RunCommand parses the native command arguments before it reads the ledger.
// Run remains the offline preview entry point for callers that supply only I/O.
func RunCommand(ctx context.Context, args []string, input io.Reader, output, diagnostics io.Writer) int {
	if ctx == nil || ctx.Err() != nil || input == nil || output == nil {
		return fail(diagnostics, 2, "The command context, input, or output is not valid.")
	}
	code := 0
	root := &cobra.Command{
		Use:   "rental-check",
		Short: "Show an offline rental ledger preview.",
		Long: "Read a rental ledger from standard input.\nWrite the offline preview as JSON.\n" +
			"The command does not send reminders or check bank access.",
		Example: "  rental-check < explicit-ledger.json",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			if cmd.Context().Err() != nil {
				code = fail(diagnostics, 2, "The command context, input, or output is not valid.")
				return
			}
			code = Run(input, output, diagnostics)
		},
	}
	if err := command.Execute(ctx, root, args, output); err != nil {
		if errors.Is(err, command.ErrOutput) {
			return fail(diagnostics, 5, "The command cannot write the help text.")
		}
		return fail(diagnostics, 2, "The command arguments are not valid.\nUse rental-check --help for command help.")
	}
	return code
}
