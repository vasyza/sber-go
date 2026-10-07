package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/mcp"
)

const usage = `Usage: sber COMMAND --profile PATH [options]

Commands:
  login             Create a private profile using hidden owner input (Linux/macOS)
  status            Inspect private profile metadata without reading it
  inspect-session   Read redacted profile metadata offline
  products          Read accounts and cards
  accounts          Read account snapshots
  cards             Read card snapshots
  operations        Read paginated history with completeness metadata
  operations-page   Read one history page
  check-session     Check the selected session with one warm-up request
  mcp               Serve read APIs over MCP stdio

Options: --ca-bundle PATH (explicit trusted PEM bundle),
         --force-update, --resource ID, --from DATE, --to DATE,
         --limit 30, --max-pages 100, --offset 0
Login:   --remembered-profile PATH (PIN login into a new profile),
         --browser-profile PATH --playwright-driver PATH --firefox-executable PATH
         (explicit public browser initialization; all paths absolute)
Secret values are never accepted as command arguments or environment variables.
`

// Options provides application dependencies. No client is constructed for an
// offline command or until the complete live command has passed validation.
type Options struct {
	Input          io.Reader
	OpenClient     func(string) (mcp.Client, error)
	Authentication *Authentication
}

func Run(ctx context.Context, args []string, output, diagnostics io.Writer) int {
	return RunWithOptions(ctx, args, output, diagnostics, Options{})
}

func RunWithOptions(ctx context.Context, args []string, output, diagnostics io.Writer, o Options) int {
	if ctx == nil || ctx.Err() != nil || output == nil {
		return fail(diagnostics, 2, "invalid command context")
	}
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		if _, err := io.WriteString(output, usage); err != nil {
			return fail(diagnostics, 3, "output failed")
		}
		return 0
	}
	if len(args) > 0 && (args[0] == "status" || args[0] == "inspect-session") {
		return runOffline(ctx, args, output, diagnostics)
	}
	if len(args) > 0 && args[0] == "login" {
		return runLogin(ctx, args[1:], output, diagnostics, o.Authentication)
	}
	if len(args) == 0 {
		return fail(diagnostics, 2, usage)
	}
	command := args[0]
	switch command {
	case "products", "accounts", "cards", "operations", "operations-page", "check-session", "mcp":
	default:
		return fail(diagnostics, 2, usage)
	}
	flags := flag.NewFlagSet("sber", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "", "explicit private session path")
	caBundle := flags.String("ca-bundle", "", "explicit trusted PEM certificate bundle")
	force := flags.Bool("force-update", false, "force products refresh")
	resource := flags.String("resource", "", "history resource")
	from := flags.String("from", "", "inclusive start")
	to := flags.String("to", "", "inclusive end")
	limit := flags.Int("limit", 30, "page size")
	pages := flags.Int("max-pages", 100, "page cap")
	offset := flags.Int("offset", 0, "page offset")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || *profile == "" || *limit < 1 || *limit > 100 || *pages < 1 || *pages > 10000 || *offset < 0 {
		return fail(diagnostics, 2, "invalid command arguments; use sber --help")
	}
	if _, err := sber.NewSourceTimeFilter(*from, *to); err != nil {
		return fail(diagnostics, 2, "invalid history dates")
	}
	if o.OpenClient == nil {
		o.OpenClient = func(path string) (mcp.Client, error) {
			return sber.NewSberClientFromSessionFile(path, sber.ClientOptions{TransportOptions: sber.TransportOptions{CABundle: *caBundle}})
		}
	}
	client, err := o.OpenClient(*profile)
	if err != nil {
		return fail(diagnostics, 3, "cannot open private session")
	}
	if client == nil {
		return fail(diagnostics, 3, "cannot open private session")
	}
	code := execute(ctx, command, client, output, diagnostics, o, *force, *resource, *from, *to, *limit, *pages, *offset)
	if err := client.Close(); err != nil && code == 0 {
		return fail(diagnostics, 3, "session cleanup failed")
	}
	return code
}

func execute(ctx context.Context, command string, client mcp.Client, output, diagnostics io.Writer, o Options, force bool, resource, from, to string, limit, pages, offset int) int {
	if command == "mcp" {
		server, err := mcp.New(mcp.Options{Client: client, ProfileExists: true})
		if err != nil {
			return fail(diagnostics, 3, "cannot start MCP server")
		}
		input := o.Input
		if input == nil {
			input = os.Stdin
		}
		if err := server.Serve(ctx, input, output); err != nil {
			if ctx.Err() != nil {
				return 130
			}
			return fail(diagnostics, 3, "MCP transport failed")
		}
		return 0
	}
	resources := sber.NewResources(client)
	var value any
	var err error
	switch command {
	case "products":
		value, err = resources.Products.Get(ctx, force)
	case "accounts":
		value, err = resources.Accounts.List(ctx, force)
	case "cards":
		value, err = resources.Cards.List(ctx, force)
	case "operations":
		value, err = resources.Operations.Collect(ctx, sber.OperationsQuery{Resource: resource, From: from, To: to, Limit: limit, MaxPages: pages})
	case "operations-page":
		value, err = resources.Operations.Page(ctx, sber.OperationsPageOptions{Resource: resource, From: from, To: to, Limit: limit, Offset: offset})
	case "check-session":
		err = client.WarmUp(ctx, true)
		value = map[string]any{"bank_authorization_checked": err == nil}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return 130
		}
		if message := transportFailureMessage(err, "bank"); message != "" {
			return fail(diagnostics, 3, message+"; no complete result")
		}
		return fail(diagnostics, 3, "bank request failed; no complete result")
	}
	raw, err := sber.ExportJSON(value)
	if err != nil {
		return fail(diagnostics, 3, "cannot serialize result")
	}
	raw = append(raw, '\n')
	if n, err := output.Write(raw); err != nil || n != len(raw) {
		return fail(diagnostics, 3, "output failed")
	}
	return 0
}
