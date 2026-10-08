package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/testutil"
	"github.com/vasyza/sber-go/mcp"
)

// Every CLI test has a synthetic user configuration directory. Default lookup
// must never inspect the machine owner's bank profile or proxy credentials.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sber-cli-test-user-")
	if err != nil {
		os.Exit(2)
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		os.Exit(2)
	}
	if os.Chmod(dir, 0700) != nil || os.Setenv("HOME", dir) != nil || os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "configuration")) != nil {
		os.Exit(2)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func configTestOptions(t *testing.T) Options {
	t.Helper()
	path := filepath.Join(testPrivateDir(t), "settings", "config.json")
	return Options{DefaultConfigPath: func() (string, error) { return path, nil }, DefaultProfilePath: func() (string, error) {
		t.Fatal("config resolved a bank profile")
		return "", nil
	}, OpenClient: func(string) (mcp.Client, error) { t.Fatal("config opened a bank client"); return nil, nil }}
}

func TestConfigWithoutSubcommandShowsHelpWithoutReadingPrivateState(t *testing.T) {
	o := configTestOptions(t)
	o.DefaultConfigPath = func() (string, error) {
		t.Fatal("config help read private proxy settings")
		return "", nil
	}
	var output, diagnostics bytes.Buffer
	if code := RunWithOptions(context.Background(), []string{"config"}, &output, &diagnostics, o); code != 0 || diagnostics.Len() != 0 {
		t.Fatalf("config help failed with code %d", code)
	}
	for _, text := range []string{"sber config COMMAND", "Commands:", "get", "list", "set", "unset"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("config help omitted %q", text)
		}
	}
	output.Reset()
	diagnostics.Reset()
	if code := RunWithOptions(context.Background(), []string{"config", "synthetic-private-argument"}, &output, &diagnostics, o); code != 2 || output.Len() != 0 || strings.Contains(diagnostics.String(), "synthetic-private") {
		t.Fatal("unknown config command did not fail safely")
	}
}

func TestCLIConfigProxyLifecycleAndRedaction(t *testing.T) {
	o := configTestOptions(t)
	run := func(args ...string) (int, string, string) {
		t.Helper()
		var output, diagnostics bytes.Buffer
		code := RunWithOptions(context.Background(), append([]string{"config"}, args...), &output, &diagnostics, o)
		if strings.Contains(output.String()+diagnostics.String(), "synthetic-secret") {
			t.Fatal("proxy credential leaked")
		}
		return code, output.String(), diagnostics.String()
	}
	if code, output, diagnostics := run("list"); code != 0 || output != "" || diagnostics != "" {
		t.Fatal("empty configuration list failed")
	}
	path, _ := o.DefaultConfigPath()
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("config list created state")
	}
	if code, output, diagnostics := run("set", "proxy", "socks5://127.0.0.1:1080:synthetic-secret-user:synthetic-secret-password"); code != 0 || output != "" || diagnostics != "" {
		t.Fatal("config set failed")
	}
	if code, output, diagnostics := run("get", "proxy"); code != 0 || output != "socks5://127.0.0.1:1080\n" || diagnostics != "" {
		t.Fatal("config get failed")
	}
	if code, output, diagnostics := run("list"); code != 0 || output != "proxy=socks5://127.0.0.1:1080\n" || diagnostics != "" {
		t.Fatal("config list failed")
	}
	if code, _, _ := run("set", "proxy", "127.0.0.1:3128"); code != 0 {
		t.Fatal("proxy replacement failed")
	}
	saved, err := loadProxySettings(path)
	if err != nil || saved.URL != "http://127.0.0.1:3128" || saved.Username != "" || saved.Password != "" {
		t.Fatal("replacement retained old credentials")
	}
	if code, _, _ := run("set", "proxy", "ftp://synthetic-secret:21"); code != 2 {
		t.Fatal("invalid proxy accepted")
	}
	if saved, err := loadProxySettings(path); err != nil || saved.URL != "http://127.0.0.1:3128" {
		t.Fatal("invalid set changed settings")
	}
	if code, _, _ := run("unset", "proxy"); code != 0 {
		t.Fatal("config unset failed")
	}
	if saved, err := loadProxySettings(path); err != nil || saved != (sber.ProxyOptions{}) {
		t.Fatal("unset retained proxy credentials")
	}
	if code, _, diagnostics := run("get", "proxy"); code != 1 || diagnostics != "The proxy is not set.\n" {
		t.Fatal("missing proxy result changed")
	}
}

func TestCLIProxyOverridesAndValidationBeforeState(t *testing.T) {
	o := configTestOptions(t)
	var output, diagnostics bytes.Buffer
	if code := RunWithOptions(context.Background(), []string{"config", "set", "proxy", "https://localhost:8443:u:p"}, &output, &diagnostics, o); code != 0 {
		t.Fatal("config set failed")
	}
	o.OpenClient = nil
	var expectedTestProxy sber.ProxyOptions
	o.OpenClientWithOptions = func(_ context.Context, _ string, options sber.ClientOptions, _ sber.PINProvider) (mcp.Client, error) {
		if options.TransportOptions.Proxy != expectedTestProxy {
			t.Fatal("command used the wrong proxy")
		}
		return &testutil.Client{}, nil
	}
	for _, tc := range []struct {
		flags []string
		want  sber.ProxyOptions
	}{
		{nil, sber.ProxyOptions{URL: "https://localhost:8443", Username: "u", Password: "p"}},
		{[]string{"--proxy", "socks5://127.0.0.1:1080:x:y"}, sber.ProxyOptions{URL: "socks5://127.0.0.1:1080", Username: "x", Password: "y"}},
		{[]string{"--no-proxy"}, sber.ProxyOptions{}},
	} {
		expectedTestProxy = tc.want
		output.Reset()
		diagnostics.Reset()
		args := append([]string{"products", "--profile", "synthetic-selected", "--no-renew"}, tc.flags...)
		if code := RunWithOptions(context.Background(), args, &output, &diagnostics, o); code != 0 {
			t.Fatalf("proxy command failed: %d %s", code, diagnostics.String())
		}
	}
	o.DefaultConfigPath = func() (string, error) {
		t.Fatal("help, explicit proxy, or invalid arguments resolved settings")
		return "", nil
	}
	for _, args := range [][]string{
		{"config", "set", "proxy", "127.0.0.1:0:synthetic-secret:p"}, {"config", "set", "proxy_auth", "true"},
		{"products", "--proxy", "bad:secret"}, {"products", "--proxy="}, {"products", "--proxy", "localhost:1", "--no-proxy"},
	} {
		output.Reset()
		diagnostics.Reset()
		if code := RunWithOptions(context.Background(), args, &output, &diagnostics, o); code != 2 || strings.Contains(diagnostics.String(), "synthetic-secret") {
			t.Fatal("invalid proxy arguments were not rejected safely")
		}
	}
	for _, args := range [][]string{{"config", "set", "--help"}, {"products", "--help"}} {
		output.Reset()
		diagnostics.Reset()
		if code := RunWithOptions(context.Background(), args, &output, &diagnostics, o); code != 0 {
			t.Fatal("help failed")
		}
	}
	for _, flags := range [][]string{{"--no-proxy"}, {"--proxy", "localhost:3128"}} {
		expectedTestProxy = sber.ProxyOptions{}
		if flags[0] == "--proxy" {
			expectedTestProxy.URL = "http://localhost:3128"
		}
		output.Reset()
		diagnostics.Reset()
		if code := RunWithOptions(context.Background(), append([]string{"products", "--profile", "synthetic-selected", "--no-renew"}, flags...), &output, &diagnostics, o); code != 0 {
			t.Fatal("explicit proxy override failed")
		}
	}
}

func TestCLIUnsafeProxySettingsFailBeforeClientOrOwnerInput(t *testing.T) {
	o := configTestOptions(t)
	path, _ := o.DefaultConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"proxy":{"url":"http://localhost:3128","password":"synthetic-private-secret"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"config", "list"}, {"config", "get", "proxy"}, {"products", "--profile", "synthetic-selected"}, {"login", "--profile", "synthetic-selected"}} {
		var output, diagnostics bytes.Buffer
		if code := RunWithOptions(context.Background(), args, &output, &diagnostics, o); code != 3 || output.Len() != 0 || strings.Contains(diagnostics.String(), "synthetic-private-secret") {
			t.Fatal("unsafe settings were not rejected safely")
		}
	}
	// Offline metadata does not consult the network settings.
	profile := filepath.Join(testPrivateDir(t), "absent", "profile.json")
	var output, diagnostics bytes.Buffer
	if code := RunWithOptions(context.Background(), []string{"status", "--profile", profile}, &output, &diagnostics, o); code != 0 {
		t.Fatal("unsafe proxy prevented offline metadata")
	}
}

func TestCLIProxySettingsRequireOneCompleteStrictRecord(t *testing.T) {
	o := configTestOptions(t)
	path, _ := o.DefaultConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"version":1,"version":1}`, `{"version":1,"proxy":{"url":"http://localhost:3128","url":"http://localhost:9999"}}`,
		`{"version":1,"unknown":"synthetic-secret"}`, `{"version":2}`, `{"version":1} {"version":1}`, `{"version":1,"proxy":{"url":"http://localhost:3128","password":"\ud800"}}`,
	} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadProxySettings(path); err == nil || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("invalid settings record accepted or echoed")
		}
	}
}
