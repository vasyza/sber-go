package cli

import (
	"context"
	"errors"
	"os"
	"strings"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/enrollment"
	"github.com/vasyza/sber-go/internal/ownerinput"
)

var errCredentialFile = errors.New("credential file is not safe or valid")
var errEnvironmentCredential = errors.New("configured credential is not valid")

type terminalCredentialError struct{ prompt ownerinput.Prompt }

func (*terminalCredentialError) Error() string     { return "credential requires terminal input" }
func (*terminalCredentialError) Is(err error) bool { return err == ownerinput.ErrTerminal }
func (e *terminalCredentialError) Unwrap() error {
	if e.prompt == ownerinput.OTP {
		// The SDK copies known error types at its redaction boundary. Preserve
		// the SMS challenge classification when read renewal crosses it.
		return &sber.PinOTPRequired{}
	}
	return ownerinput.ErrTerminal
}

// Credential files are scoped to one invocation. They never modify the process
// environment or the saved profile. Environment values take precedence, even
// when an explicit empty value selects terminal input instead of a file value.
type credentialInput struct {
	values   map[string]string
	terminal func(context.Context, ownerinput.Prompt) (string, error)
}

func configureSecretInput(a *Authentication, path string) error {
	if a.ReadSecret != nil {
		return nil
	}
	input := credentialInput{terminal: a.ReadTerminalSecret}
	if input.terminal == nil {
		input.terminal = ownerSecret
	}
	if path != "" {
		raw, err := enrollment.ReadPrivateFile(path, 64*1024)
		if err != nil || raw == nil {
			return errCredentialFile
		}
		defer clear(raw)
		input.values, err = parseCredentialFile(raw)
		if err != nil {
			return err
		}
	}
	a.configuredPIN = input.value("SBER_PINCODE") != ""
	a.ReadSecret = input.read
	return nil
}

func (input credentialInput) value(name string) string {
	if value, present := os.LookupEnv(name); present {
		return value
	}
	return input.values[name]
}

func credentialVariable(prompt ownerinput.Prompt) string {
	switch prompt {
	case ownerinput.Login:
		return "SBER_LOGIN"
	case ownerinput.Password:
		return "SBER_PASSWORD"
	case ownerinput.PIN, ownerinput.NewPIN, ownerinput.ConfirmPIN:
		return "SBER_PINCODE"
	case ownerinput.Phone:
		return "SBER_PHONE"
	case ownerinput.CardNumber:
		return "SBER_CARD_NUMBER"
	}
	return ""
}

func (input credentialInput) read(ctx context.Context, prompt ownerinput.Prompt) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if name := credentialVariable(prompt); name != "" {
		if value := input.value(name); value != "" {
			if len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") {
				return "", errEnvironmentCredential
			}
			if name == "SBER_PINCODE" && (len(value) < 4 || len(value) > 12 || !pinDigits(value, len(value))) {
				return "", &sber.PinAuthError{Code: "invalid_pin"}
			}
			return value, nil
		}
	}
	value, err := input.terminal(ctx, prompt)
	if errors.Is(err, ownerinput.ErrTerminal) {
		return "", &terminalCredentialError{prompt: prompt}
	}
	return value, err
}

func credentialFailureMessage(err error) string {
	switch {
	case errors.Is(err, errCredentialFile):
		return "The command cannot read the private credential file.\nCheck --env-file PATH and private file permissions."
	case errors.Is(err, errEnvironmentCredential):
		return "The configured authentication value is not valid.\nCheck the environment or credential file."
	}
	var terminal *terminalCredentialError
	if errors.As(err, &terminal) {
		if terminal.prompt == ownerinput.OTP {
			return "The bank requires an SMS code.\nRun this command in a terminal to enter the code."
		}
		if name := credentialVariable(terminal.prompt); name != "" {
			return "The command requires an authentication value.\nSet " + name + " or run this command in a terminal."
		}
	}
	var otp *sber.PinOTPRequired
	if errors.As(err, &otp) {
		return "The bank requires an SMS code.\nRun this command in a terminal to enter the code."
	}
	return ""
}
