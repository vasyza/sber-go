package cli

import (
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
	sber "github.com/vasyza/sber-go"
)

type commandDefinition struct{ name, description string }

var commands = []commandDefinition{
	{"login", "Make a private profile with hidden terminal input."},
	{"refresh-session", "Restore the selected session with hidden PIN input."},
	{"status", "Get file metadata without a bank request."},
	{"inspect-session", "Read profile metadata with secret values removed."},
	{"products", "Read accounts and cards."},
	{"accounts", "Read account data."},
	{"cards", "Read card data."},
	{"portfolio", "Read accounts, cards, and their relationships."},
	{"card-info", "Read data for specified cards."},
	{"card-limits", "Read limits for one card."},
	{"operations", "Read history and its coverage metadata."},
	{"operations-page", "Read one page of history."},
	{"operation-details", "Read data for one operation."},
	{"analytics", "Read income or expenditure totals."},
	{"check-session", "Send one session check request to the bank."},
	{"export-session", "Write a private session copy to a new file."},
	{"inspect-credentials", "Read credential metadata with secret values removed."},
	{"card-rename", "Show a plan for one card name change."},
	{"transfer-own", "Show a plan for one transfer between your accounts."},
	{"mcp", "Supply read tools through MCP standard input and output."},
}

func findCommand(name string) (commandDefinition, bool) {
	for _, command := range commands {
		if command.name == name {
			return command, true
		}
	}
	return commandDefinition{}, false
}

type cardIDs []string

func (v *cardIDs) String() string        { return strings.Join(*v, ",") }
func (v *cardIDs) Set(text string) error { *v = append(*v, text); return nil }
func (v *cardIDs) Type() string          { return "ID" }

type commandArguments struct {
	name, source, amount, currency, purpose                               string
	execute                                                               bool
	profile, ca, resource, from, to, operationID, incomeType, destination string
	remembered                                                            string
	browser                                                               loginBrowserSelection
	force, noRenew                                                        bool
	betweenOwn, openBanking, showCategories, showProducts                 bool
	limit, pages, offset                                                  int
	cards                                                                 cardIDs
	timeout                                                               time.Duration
}

func commandFlags(name string) (*pflag.FlagSet, *commandArguments) {
	a := &commandArguments{limit: 30, pages: 100, incomeType: "outcome", timeout: 30 * time.Second}
	f := pflag.NewFlagSet("sber "+name, pflag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&a.profile, "profile", "", "Select the private profile `PATH` (required).")
	if name == "status" || name == "inspect-session" {
		return f, a
	}
	f.StringVar(&a.ca, "ca-bundle", "", "Use a different PEM trust bundle `PATH`.")
	if name == "login" || name == "refresh-session" {
		if name == "login" {
			f.StringVar(&a.remembered, "remembered-profile", "", "Use an existing profile `PATH` for PIN login.")
		}
		f.StringVar(&a.browser.profile, "browser-profile", "", "Select the private Firefox profile `PATH`.")
		f.StringVar(&a.browser.driver, "playwright-driver", "", "Select the installed Playwright driver `PATH`.")
		f.StringVar(&a.browser.executable, "firefox-executable", "", "Select the installed Firefox executable `PATH`.")
		return f, a
	}
	f.DurationVar(&a.timeout, "timeout", 30*time.Second, "Set the maximum time for each request (1s to 120s).")
	if name != "mcp" && name != "export-session" && name != "inspect-credentials" && !isMutationCommand(name) {
		f.BoolVar(&a.noRenew, "no-renew", false, "Do not request a PIN if the session expires.")
		f.StringVar(&a.browser.profile, "browser-profile", "", "Select the private Firefox profile `PATH` for PIN login.")
		f.StringVar(&a.browser.driver, "playwright-driver", "", "Select the installed Playwright driver `PATH` for PIN login.")
		f.StringVar(&a.browser.executable, "firefox-executable", "", "Select the installed Firefox executable `PATH` for PIN login.")
	}
	switch name {
	case "card-rename":
		f.Var(&a.cards, "card-id", "Select one numeric card `ID` (required).")
		f.StringVar(&a.name, "name", "", "Set the new card name (required).")
		f.BoolVar(&a.execute, "execute", false, "Let the CLI send this change after terminal confirmation.")
	case "transfer-own":
		f.StringVar(&a.source, "source", "", "Select the source resource `ID` (required).")
		f.StringVar(&a.destination, "destination", "", "Select the destination resource `ID` (required).")
		f.StringVar(&a.amount, "amount", "", "Set a positive decimal amount with up to two decimal places (required).")
		f.StringVar(&a.currency, "currency", "RUB", "Use a currency code with three letters.")
		f.StringVar(&a.purpose, "purpose", "", "Set the transfer purpose (up to 210 characters).")
		f.BoolVar(&a.execute, "execute", false, "Let the CLI send this transfer after two terminal confirmations.")
	case "products", "accounts", "cards", "portfolio":
		f.BoolVar(&a.force, "force-update", false, "Get new product data.")
	case "operations", "operations-page":
		f.StringVar(&a.resource, "resource", "", "Filter history by resource ID.")
		f.StringVar(&a.from, "from", "", "Set the inclusive start `DATE` or timestamp.")
		f.StringVar(&a.to, "to", "", "Set the inclusive end `DATE` or timestamp.")
		f.IntVar(&a.limit, "limit", 30, "Set the page size (1 to 100).")
		if name == "operations" {
			f.IntVar(&a.pages, "max-pages", 100, "Set the maximum number of pages (1 to 10000).")
		} else {
			f.IntVar(&a.offset, "offset", 0, "Set the page offset (zero or more).")
		}
	case "operation-details":
		f.StringVar(&a.operationID, "operation-id", "", "Select the operation `ID` (required).")
	case "card-info", "card-limits":
		f.Var(&a.cards, "card-id", "Select a numeric card `ID` (required; repeat for card-info).")
	case "analytics":
		f.StringVar(&a.from, "from", "", "Set the inclusive start `DATE` or timestamp (required).")
		f.StringVar(&a.to, "to", "", "Set the inclusive end `DATE` or timestamp (required).")
		f.StringVar(&a.incomeType, "income-type", "outcome", "Select income or outcome.")
		f.BoolVar(&a.betweenOwn, "between-own", true, "Include transfers between your accounts.")
		f.BoolVar(&a.openBanking, "open-banking", false, "Include external bank data.")
		f.BoolVar(&a.showCategories, "show-categories", true, "Include category totals.")
		f.BoolVar(&a.showProducts, "show-products", true, "Include product totals.")
	case "export-session":
		f.StringVar(&a.destination, "destination", "", "Select a new absolute private file `PATH` (required).")
	}
	return f, a
}

var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var cardIDPattern = regexp.MustCompile(`^[0-9]{1,16}$`)
var historyResourcePattern = regexp.MustCompile(`^(?:card|ct-account|account):[A-Za-z0-9_-]{1,128}$`)

func validateCommand(name string, a *commandArguments) bool {
	if a.profile == "" || !a.browser.valid() || a.timeout < time.Second || a.timeout > 120*time.Second || a.limit < 1 || a.limit > 100 || a.pages < 1 || a.pages > 10000 || a.offset < 0 {
		return false
	}
	switch name {
	case "card-rename", "transfer-own":
		return validateMutation(name, a)
	case "operations", "operations-page":
		_, err := sber.NewSourceTimeFilter(a.from, a.to)
		return err == nil && (a.resource == "" || historyResourcePattern.MatchString(a.resource)) && a.offset <= int(^uint(0)>>1)-a.limit
	case "operation-details":
		return operationIDPattern.MatchString(a.operationID)
	case "card-info", "card-limits":
		if len(a.cards) == 0 || len(a.cards) > 100 || name == "card-limits" && len(a.cards) != 1 {
			return false
		}
		for _, id := range a.cards {
			n, err := strconv.ParseInt(id, 10, 64)
			if !cardIDPattern.MatchString(id) || err != nil || n <= 0 || n > 9007199254740991 {
				return false
			}
		}
	case "analytics":
		_, err := sber.NewTimeFilter(a.from, a.to)
		return a.from != "" && a.to != "" && err == nil && (a.incomeType == "income" || a.incomeType == "outcome")
	case "export-session":
		return filepath.IsAbs(a.destination) && filepath.Clean(a.destination) == a.destination && a.destination != a.profile
	}
	return true
}
