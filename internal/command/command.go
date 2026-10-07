// Package command configures Cobra help and private command diagnostics.
package command

import (
	"context"
	"errors"
	"io"

	"github.com/spf13/cobra"
)

var (
	ErrArguments = errors.New("The command arguments are not valid.")
	ErrOutput    = errors.New("The command cannot write the help text.")
)

const helpTemplate = `{{with .Long}}{{.}}{{else}}{{.Short}}{{end}}

Use:
  {{.UseLine}}
{{if .HasAvailableSubCommands}}
Commands:
{{range .Commands}}{{if .IsAvailableCommand}}  {{rpad .Name .NamePadding}} {{.Short}}
{{end}}{{end}}{{end}}{{if .HasExample}}
Examples:
{{.Example}}
{{end}}{{if .HasAvailableLocalFlags}}
Options:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}`

// Execute uses a fresh command tree supplied by the caller. It suppresses
// Cobra diagnostics because they can include argument values and private paths.
// The caller maps errors to static messages and its existing exit codes.
func Execute(ctx context.Context, root *cobra.Command, args []string, output io.Writer) error {
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.DisableSuggestions = true
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpTemplate(helpTemplate)
	root.SetErr(io.Discard)
	writer := &helpWriter{output: output}
	root.SetOut(writer)
	// A nil argument slice makes Cobra read process arguments. Always supply
	// a non-nil slice so an embedded caller controls the complete invocation.
	root.SetArgs(append([]string{}, args...))
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		// Cobra's hidden completion handler can write raw parser errors directly
		// to os.Stderr. Disable it before its handler can run.
		if cmd.Hidden {
			return ErrArguments
		}
		return nil
	}
	help := &cobra.Command{
		Use:   "help [COMMAND]",
		Short: "Show command help.",
		RunE: func(_ *cobra.Command, args []string) error {
			target, rest, err := root.Find(args)
			if err != nil || len(rest) != 0 || target.Hidden {
				return ErrArguments
			}
			return target.Help()
		},
	}
	root.SetHelpCommand(help)
	root.AddCommand(help)
	configureHelp(root)
	err := root.ExecuteContext(ctx)
	if writer.failed {
		return ErrOutput
	}
	return err
}

func configureHelp(cmd *cobra.Command) {
	cmd.DisableFlagsInUseLine = true
	cmd.Flags().BoolP("help", "h", false, "Show command help.")
	for _, child := range cmd.Commands() {
		configureHelp(child)
	}
}

type helpWriter struct {
	output io.Writer
	failed bool
}

func (w *helpWriter) Write(data []byte) (int, error) {
	n, err := w.output.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.failed = true
	}
	return n, err
}
