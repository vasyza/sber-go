package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	sber "github.com/vasyza/sber-go"
	cliCommand "github.com/vasyza/sber-go/internal/command"
	"github.com/vasyza/sber-go/internal/enrollment"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	"github.com/vasyza/sber-go/mcp"
)

// Options supplies application dependencies. No client is constructed until
// the complete live command has passed validation.
type Options struct {
	Input io.Reader
	// DefaultProfilePath resolves the user profile only when --profile is absent.
	// Initial argument validation and help never call it.
	DefaultProfilePath    func() (string, error)
	OpenClient            func(string) (mcp.Client, error)
	OpenClientWithOptions func(context.Context, string, sber.ClientOptions, sber.PINProvider) (mcp.Client, error)
	Authentication        *Authentication
}

func Run(ctx context.Context, args []string, output, diagnostics io.Writer) int {
	return RunWithOptions(ctx, args, output, diagnostics, Options{})
}

func RunWithOptions(ctx context.Context, args []string, output, diagnostics io.Writer, o Options) int {
	if ctx == nil || ctx.Err() != nil || output == nil {
		return fail(diagnostics, 2, "The command context or output is not valid.")
	}
	code := 0
	root := &cobra.Command{
		Use:   "sber COMMAND",
		Short: "Read bank data or manage a private profile.",
		Long: "Use the saved profile for the current user.\n" +
			"Use --profile PATH to select a different profile.\n" +
			"Enter secret values only at hidden terminal prompts.\n" +
			"The bank root CA is part of the application.",
		RunE: func(*cobra.Command, []string) error { return cliCommand.ErrArguments },
	}
	for _, definition := range commands {
		flags, a := commandFlags(definition.name)
		child := &cobra.Command{
			Use:   definition.name + " [options]",
			Short: definition.description,
			Args: func(cmd *cobra.Command, args []string) error {
				if len(args) != 0 || cmd.Flags().Changed("profile") && a.profile == "" || !validateCommand(definition.name, a) {
					return cliCommand.ErrArguments
				}
				return nil
			},
			RunE: func(cmd *cobra.Command, _ []string) error {
				if err := selectProfile(a, o.DefaultProfilePath); err != nil {
					code = fail(diagnostics, 3, "The command cannot select the default profile.\nUse --profile PATH to select a profile.")
					return nil
				}
				// Resolve the source before checking an export destination against it.
				if !validateCommand(definition.name, a) {
					return cliCommand.ErrArguments
				}
				code = runValidated(cmd.Context(), definition.name, a, output, diagnostics, o)
				return nil
			},
		}
		child.Flags().AddFlagSet(flags)
		root.AddCommand(child)
	}
	if err := cliCommand.Execute(ctx, root, args, output); err != nil {
		if errors.Is(err, cliCommand.ErrOutput) {
			return fail(diagnostics, 3, "The command cannot write the output.")
		}
		return fail(diagnostics, 2, "The command arguments are not valid.\nUse sber --help for command help.")
	}
	return code
}

func runValidated(ctx context.Context, command string, a *commandArguments, output, diagnostics io.Writer, o Options) int {
	if a.defaultProfile && command != "login" && command != "status" && !(isMutationCommand(command) && !a.execute) {
		exists, err := enrollment.SafeProfileExists(a.profile)
		if err != nil {
			return fail(diagnostics, 3, "The profile file properties are not safe.")
		}
		if !exists {
			return fail(diagnostics, 3, "The CLI has no saved profile.\nUse sber login to make a profile.")
		}
	}
	switch command {
	case "status", "inspect-session":
		return runOffline(command, a, output, diagnostics)
	case "login":
		return runLogin(ctx, a, output, diagnostics, o.Authentication)
	case "refresh-session":
		return runRefresh(ctx, a, output, diagnostics, o.Authentication)
	}
	if isMutationCommand(command) {
		if !a.execute {
			return writeResult(output, diagnostics, mutationPlan(command, a))
		}
		if code := confirmAction(ctx, diagnostics, o.Authentication, mutationPlan(command, a)); code != 0 {
			return code
		}
	}
	if o.OpenClient == nil {
		options, provider, err := readClientOptions(command, a, o.Authentication)
		if err != nil {
			return fail(diagnostics, 3, "The command cannot prepare authentication.")
		}
		open := o.OpenClientWithOptions
		if open == nil {
			open = openReadClient
		}
		o.OpenClient = func(path string) (mcp.Client, error) { return open(ctx, path, options, provider) }
	}
	client, err := o.OpenClient(a.profile)
	if err != nil || client == nil {
		if err != nil && ctx.Err() != nil {
			return 130
		}
		if message := transportFailureMessage(err, "bank"); message != "" {
			return fail(diagnostics, 3, message)
		}
		return fail(diagnostics, 3, "The command cannot open the private session.")
	}
	var result bytes.Buffer
	stream := io.Writer(&result)
	if command == "mcp" {
		stream = output
	}
	code := execute(ctx, command, client, stream, diagnostics, o, a)
	if err := client.Close(); err != nil && code == 0 {
		return fail(diagnostics, 3, "The command cannot close the session.")
	}
	if code == 0 && command != "mcp" {
		if n, err := output.Write(result.Bytes()); err != nil || n != result.Len() {
			return fail(diagnostics, 3, "The command cannot write the output.")
		}
	}
	return code
}

func execute(ctx context.Context, command string, client mcp.Client, output, diagnostics io.Writer, o Options, a *commandArguments) int {
	if command == "mcp" {
		server, err := mcp.New(mcp.Options{Client: client, ProfileExists: true})
		if err != nil {
			return fail(diagnostics, 3, "The command cannot start the MCP server.")
		}
		input := o.Input
		if input == nil {
			input = os.Stdin
		}
		if err := server.Serve(ctx, input, output); err != nil {
			if ctx.Err() != nil {
				return 130
			}
			return fail(diagnostics, 3, "The MCP transport failed.")
		}
		return 0
	}
	resources := sber.NewResources(client, sber.ResourceOptions{AllowMutations: isMutationCommand(command) && a.execute})
	var value any
	var err error
	switch command {
	case "card-rename":
		err = resources.Cards.Rename(ctx, a.cards[0], a.name)
		value = map[string]any{"card_renamed": err == nil}
	case "transfer-own":
		var draft sber.TransferDraft
		draft, err = resources.Transfers.Start(ctx)
		if err == nil {
			amount, _ := sber.ParseDecimal(a.amount)
			var prepared sber.PreparedTransfer
			prepared, err = resources.Transfers.Prepare(ctx, draft, a.source, a.destination, amount, sber.TransferOptions{Currency: a.currency, PaymentPurpose: a.purpose})
			if err == nil {
				plan := mutationPlan(command, a)
				plan["transfer_prepared"] = true
				plan["bank_request_sent"] = true
				if code := confirmAction(ctx, diagnostics, o.Authentication, plan); code != 0 {
					return code
				}
				var result sber.TransferResult
				result, err = resources.Transfers.Confirm(ctx, prepared)
				value = map[string]any{"transfer_confirmed": err == nil, "document_id_present": result.DocumentID() != nil}
			}
		}
	case "products":
		value, err = resources.Products.Get(ctx, a.force)
	case "accounts":
		value, err = resources.Accounts.List(ctx, a.force)
	case "cards":
		value, err = resources.Cards.List(ctx, a.force)
	case "portfolio":
		value, err = resources.Portfolio(ctx, a.force)
	case "card-info":
		ids := make([]any, len(a.cards))
		for i, id := range a.cards {
			ids[i] = id
		}
		value, err = resources.Cards.Info(ctx, ids...)
	case "card-limits":
		value, err = resources.Cards.Limits(ctx, a.cards[0])
	case "operations":
		value, err = resources.Operations.Collect(ctx, sber.OperationsQuery{Resource: a.resource, From: a.from, To: a.to, Limit: a.limit, MaxPages: a.pages})
	case "operations-page":
		value, err = resources.Operations.Page(ctx, sber.OperationsPageOptions{Resource: a.resource, From: a.from, To: a.to, Limit: a.limit, Offset: a.offset})
	case "operation-details":
		value, err = resources.Operations.Details(ctx, a.operationID)
	case "analytics":
		value, err = resources.Analytics.Amounts(ctx, a.from, a.to, sber.AnalyticsOptions{IncomeType: a.incomeType, BetweenOwn: a.betweenOwn, OpenBanking: a.openBanking, ShowCategories: a.showCategories, ShowProducts: a.showProducts})
	case "check-session":
		err = resources.Session.WarmUp(ctx)
		value = map[string]any{"bank_authorization_checked": err == nil}
	case "export-session":
		var bundle sber.SessionBundle
		bundle, err = resources.Session.Export()
		if err == nil {
			err = enrollment.Enroll(ctx, a.destination, func(context.Context) (enrollment.CandidateWriter, error) {
				return func(ctx context.Context, path string) error {
					if err := ctx.Err(); err != nil {
						return err
					}
					return sdkSession.WriteEnrollmentCandidate(path, bundle)
				}, nil
			})
		}
		value = map[string]any{"session_exported": err == nil, "bank_authorization_checked": false}
	case "inspect-credentials":
		value, err = resources.Session.Credentials()
	}
	if err != nil {
		return requestFailure(ctx, diagnostics, err, a)
	}
	return writeResult(output, diagnostics, value)
}

func writeResult(output, diagnostics io.Writer, value any) int {
	raw, err := sber.ExportJSON(value)
	if err != nil {
		return fail(diagnostics, 3, "The command cannot prepare the JSON result.")
	}
	raw = append(raw, '\n')
	if n, err := output.Write(raw); err != nil || n != len(raw) {
		return fail(diagnostics, 3, "The command cannot write the output.")
	}
	return 0
}

func requestFailure(ctx context.Context, diagnostics io.Writer, err error, args *commandArguments) int {
	var uncertain *sber.MutationUncertain
	if errors.As(err, &uncertain) {
		return fail(diagnostics, 4, "The result of the operation is unknown.\nDo not repeat the operation.\nCheck the operation on the bank website.")
	}
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return 130
	}
	var expired *sber.AuthenticationExpired
	if errors.As(err, &expired) {
		command := "sber refresh-session"
		if !args.defaultProfile {
			command += " --profile PATH"
		}
		return fail(diagnostics, 3, "The session has expired.\nUse "+command+" to restore the session.\nNo complete result is available.")
	}
	var rejected *sber.APIRejected
	if errors.As(err, &rejected) {
		return fail(diagnostics, 3, "The bank rejected the request.\nNo complete result is available.")
	}
	var parse *sber.ParseError
	if errors.As(err, &parse) {
		return fail(diagnostics, 3, "The bank response format is not supported.\nNo complete result is available.")
	}
	var response *sber.APIError
	if errors.As(err, &response) && response.StatusCode >= 100 && response.StatusCode <= 599 {
		return fail(diagnostics, 3, fmt.Sprintf("The bank returned HTTP %d.\nNo complete result is available.", response.StatusCode))
	}
	if errors.Is(err, enrollment.ErrExists) {
		return fail(diagnostics, 3, "The destination file already exists.\nSelect a new path.")
	}
	if message := transportFailureMessage(err, "bank"); message != "" {
		return fail(diagnostics, 3, message+"\nNo complete result is available.")
	}
	return fail(diagnostics, 3, "The bank request failed.\nNo complete result is available.")
}
