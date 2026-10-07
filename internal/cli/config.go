package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	sber "github.com/vasyza/sber-go"
	cliCommand "github.com/vasyza/sber-go/internal/command"
	"github.com/vasyza/sber-go/internal/enrollment"
	sdkProxy "github.com/vasyza/sber-go/internal/proxy"
	"github.com/vasyza/sber-go/internal/strictjson"
)

// Disk records are the single explicit serialization boundary for credentials.
// Public SDK options retain their redacted fmt and JSON representations.
type proxyRecord struct {
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}
type settingsRecord struct {
	Version int          `json:"version"`
	Proxy   *proxyRecord `json:"proxy,omitempty"`
}

func defaultConfigPath() (string, error) {
	profile, err := defaultProfilePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(profile), "config.json"), nil
}

func resolveConfigPath(resolve func() (string, error)) (string, error) {
	if resolve == nil {
		resolve = defaultConfigPath
	}
	path, err := resolve()
	if err != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 {
		return "", enrollment.ErrUnsafe
	}
	return path, nil
}

func loadProxySettings(path string) (sber.ProxyOptions, error) {
	raw, err := enrollment.ReadPrivateFile(path, 16*1024)
	if err != nil || raw == nil {
		return sber.ProxyOptions{}, err
	}
	if strictjson.Validate(raw) != nil {
		return sber.ProxyOptions{}, enrollment.ErrUnsafe
	}
	var record settingsRecord
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || record.Version != 1 || decoder.Decode(new(any)) != io.EOF {
		return sber.ProxyOptions{}, enrollment.ErrUnsafe
	}
	if record.Proxy == nil {
		return sber.ProxyOptions{}, nil
	}
	o, err := (sber.ProxyOptions{URL: record.Proxy.URL, Username: record.Proxy.Username, Password: record.Proxy.Password}).Normalize()
	if err != nil || o.URL == "" {
		return sber.ProxyOptions{}, enrollment.ErrUnsafe
	}
	return o, nil
}

func saveProxySettings(ctx context.Context, path string, proxy sber.ProxyOptions) error {
	record := settingsRecord{Version: 1}
	if proxy.URL != "" {
		record.Proxy = &proxyRecord{URL: proxy.URL, Username: proxy.Username, Password: proxy.Password}
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return enrollment.ErrWrite
	}
	return enrollment.ReplacePrivateFile(ctx, path, append(raw, '\n'))
}

func validateConfigCommand(command string, args []string) bool {
	switch command {
	case "set":
		if len(args) != 2 || args[0] != "proxy" {
			return false
		}
		_, err := sdkProxy.Parse(args[1])
		return err == nil
	case "get", "unset":
		return len(args) == 1 && args[0] == "proxy"
	case "list":
		return len(args) == 0
	}
	return false
}

func configCommand(output, diagnostics io.Writer, o Options, code *int) *cobra.Command {
	command := &cobra.Command{Use: "config COMMAND", Short: "Read or change the saved CLI settings.", RunE: func(*cobra.Command, []string) error { return cliCommand.ErrArguments }}
	for _, definition := range []commandDefinition{{"set", "Save a proxy address and its optional login values."}, {"get", "Read the proxy address with login values removed."}, {"list", "Read the saved settings with login values removed."}, {"unset", "Remove the saved proxy and its login values."}} {
		usage := definition.name
		if definition.name == "set" {
			usage += " proxy ADDRESS"
		} else if definition.name != "list" {
			usage += " proxy"
		}
		child := &cobra.Command{Use: usage, Short: definition.description,
			Args: func(_ *cobra.Command, args []string) error {
				if !validateConfigCommand(definition.name, args) {
					return cliCommand.ErrArguments
				}
				return nil
			},
			RunE: func(cmd *cobra.Command, args []string) error {
				*code = runConfig(cmd.Context(), definition.name, args, output, diagnostics, o.DefaultConfigPath)
				return nil
			},
		}
		command.AddCommand(child)
	}
	return command
}

func runConfig(ctx context.Context, command string, args []string, output, diagnostics io.Writer, resolve func() (string, error)) int {
	path, err := resolveConfigPath(resolve)
	if err != nil {
		return fail(diagnostics, 3, "The command cannot select the CLI settings file.")
	}
	proxy := sber.ProxyOptions{}
	if command == "set" {
		proxy, err = sdkProxy.Parse(args[1])
		if err != nil {
			return fail(diagnostics, 2, "The proxy address is not valid.")
		}
	}
	if command == "set" || command == "unset" {
		err = saveProxySettings(ctx, path, proxy)
		if err == nil {
			return 0
		}
		if ctx.Err() != nil {
			return 130
		}
		if errors.Is(err, enrollment.ErrBusy) {
			return fail(diagnostics, 3, "Another command is changing the CLI settings.")
		}
		return fail(diagnostics, 3, "The command cannot confirm the change to the CLI settings.\nCheck the settings file properties.")
	}
	proxy, err = loadProxySettings(path)
	if err != nil {
		return fail(diagnostics, 3, "The command cannot read the CLI settings.\nCheck the settings file properties.")
	}
	if proxy.URL == "" {
		if command == "get" {
			return fail(diagnostics, 1, "The proxy is not set.")
		}
		return 0
	}
	text := proxy.URL + "\n"
	if command == "list" {
		text = "proxy=" + text
	}
	if n, err := io.WriteString(output, text); err != nil || n != len(text) {
		return fail(diagnostics, 3, "The command cannot write the output.")
	}
	return 0
}

func selectProxy(args *commandArguments, resolve func() (string, error)) error {
	if args.noProxy {
		args.selectedProxy = sber.ProxyOptions{}
		return nil
	}
	if args.proxy != "" {
		proxy, err := sdkProxy.Parse(args.proxy)
		if err != nil {
			return err
		}
		args.selectedProxy = proxy
		return nil
	}
	path, err := resolveConfigPath(resolve)
	if err != nil {
		return err
	}
	args.selectedProxy, err = loadProxySettings(path)
	return err
}
