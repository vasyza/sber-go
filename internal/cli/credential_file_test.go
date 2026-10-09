package cli

import (
	"context"
	"errors"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/vasyza/sber-sdk/internal/ownerinput"
)

func TestCredentialFileLiteralSyntax(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want map[string]string
	}{
		{"empty", "# comment\n\n", map[string]string{}},
		{"plain", "SBER_PINCODE=01234\r\n", map[string]string{"SBER_PINCODE": "01234"}},
		{"export and comment", "export\tSBER_LOGIN = 'synthetic user # value' # comment\n", map[string]string{"SBER_LOGIN": "synthetic user # value"}},
		{"double quotes", `SBER_PASSWORD="synthetic \"quoted\" password\\value"`, map[string]string{"SBER_PASSWORD": `synthetic "quoted" password\value`}},
		{"unquoted comments", "SBER_PASSWORD=synthetic#value # comment\n", map[string]string{"SBER_PASSWORD": "synthetic#value"}},
		{"literal expressions", "SBER_PASSWORD='$(synthetic-command) ${SBER_LOGIN} `synthetic-command`'\n", map[string]string{"SBER_PASSWORD": "$(synthetic-command) ${SBER_LOGIN} `synthetic-command`"}},
		{"empty value", "SBER_LOGIN= # comment\n", map[string]string{"SBER_LOGIN": ""}},
		{"last assignment", "SBER_LOGIN=first\nSBER_LOGIN=last\n", map[string]string{"SBER_LOGIN": "last"}},
		{"unrelated variable", "OTHER_SETTING=synthetic\nSBER_PHONE=79000000001\n", map[string]string{"SBER_PHONE": "79000000001"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseCredentialFile([]byte(test.raw))
			if err != nil || !maps.Equal(got, test.want) {
				t.Fatal("credential file did not preserve the documented literal syntax")
			}
		})
	}
	for _, raw := range []string{
		"SBER_LOGIN", "1INVALID=value", "SBER_LOGIN='unterminated", `SBER_LOGIN="unterminated`,
		"SBER_LOGIN='value' trailing-data", "SBER_LOGIN=bad\x00value", "SBER_LOGIN=bad\xffvalue",
		`SBER_PASSWORD="line\nvalue"`, "SBER_PASSWORD='line\nvalue'", "SBER_PASSWORD=" + strings.Repeat("x", 4097),
	} {
		if _, err := parseCredentialFile([]byte(raw)); !errors.Is(err, errCredentialFile) {
			t.Fatal("invalid credential syntax was accepted")
		}
	}
}

func TestCredentialInputPrecedenceMissingValuesAndTerminalOnlyChallenges(t *testing.T) {
	flags := credentialTestSource(t, "file")
	t.Setenv("SBER_LOGIN", "synthetic-process-login")
	t.Setenv("SBER_PASSWORD", "") // An explicit empty override selects terminal input.
	t.Setenv("SBER_OTP", "synthetic-ignored-code")
	t.Setenv("SBER_CONFIRM", "CONFIRM")
	var prompts []ownerinput.Prompt
	a := Authentication{ReadTerminalSecret: func(_ context.Context, prompt ownerinput.Prompt) (string, error) {
		prompts = append(prompts, prompt)
		return "synthetic-terminal", nil
	}}
	if err := configureSecretInput(&a, flags[1]); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		prompt ownerinput.Prompt
		want   string
	}{
		{ownerinput.Login, "synthetic-process-login"},
		{ownerinput.Password, "synthetic-terminal"},
		{ownerinput.Phone, syntheticEnvironmentCredentials["SBER_PHONE"]},
		{ownerinput.CardNumber, syntheticEnvironmentCredentials["SBER_CARD_NUMBER"]},
		{ownerinput.PIN, "12345"}, {ownerinput.NewPIN, "12345"}, {ownerinput.ConfirmPIN, "12345"},
		{ownerinput.OTP, "synthetic-terminal"}, {ownerinput.ConfirmAction, "synthetic-terminal"},
	} {
		value, err := a.ReadSecret(context.Background(), test.prompt)
		if err != nil || value != test.want {
			t.Fatal("credential input lost precedence or terminal-only confirmation")
		}
	}
	if len(prompts) != 3 || prompts[0] != ownerinput.Password || prompts[1] != ownerinput.OTP || prompts[2] != ownerinput.ConfirmAction {
		t.Fatal("configured values prompted or confirmations consumed an environment credential")
	}
	if _, present := os.LookupEnv("SBER_PINCODE"); present {
		t.Fatal("credential file changed the process environment")
	}
	b := Authentication{ReadTerminalSecret: func(context.Context, ownerinput.Prompt) (string, error) { return "synthetic-next-terminal", nil }}
	if err := configureSecretInput(&b, ""); err != nil {
		t.Fatal(err)
	}
	if value, err := b.ReadSecret(context.Background(), ownerinput.PIN); err != nil || value != "synthetic-next-terminal" || b.configuredPIN {
		t.Fatal("credential file values persisted into the next invocation")
	}
}

func TestCredentialInputCancellationAndInvalidEnvironment(t *testing.T) {
	credentialTestSource(t, "environment")
	a := Authentication{ReadTerminalSecret: func(context.Context, ownerinput.Prompt) (string, error) {
		t.Fatal("configured or cancelled input requested a terminal")
		return "", nil
	}}
	if err := configureSecretInput(&a, ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if value, err := a.ReadSecret(ctx, ownerinput.Login); value != "" || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled input returned a credential")
	}
	for _, value := range []string{"123x5", "１２３４５", "1", "1234567890123"} {
		t.Setenv("SBER_PINCODE", value)
		if secret, err := a.ReadSecret(context.Background(), ownerinput.PIN); secret != "" || err == nil {
			t.Fatal("invalid configured PIN was accepted")
		}
	}
	t.Setenv("SBER_PASSWORD", "synthetic-private\nvalue")
	if secret, err := a.ReadSecret(context.Background(), ownerinput.Password); secret != "" || !errors.Is(err, errEnvironmentCredential) || strings.Contains(loginFailureMessage(err), "synthetic-private") {
		t.Fatal("invalid environment value was accepted or disclosed")
	}
}
