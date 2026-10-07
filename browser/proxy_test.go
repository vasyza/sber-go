package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	sber "github.com/vasyza/sber-go"
)

func TestFirefoxExplicitProxyOptionsAndBridgeLifecycle(t *testing.T) {
	for _, tc := range []struct {
		scheme        string
		authenticated bool
	}{{"http", false}, {"http", true}, {"https", true}, {"socks5", false}, {"socks5", true}} {
		t.Run(fmt.Sprintf("%s/auth=%t", tc.scheme, tc.authenticated), func(t *testing.T) {
			provider, _, _ := fakeProvider(t)
			provider.options.Proxy = sber.ProxyOptions{URL: tc.scheme + "://127.0.0.1:1080"}
			if tc.authenticated {
				provider.options.Proxy.Username, provider.options.Proxy.Password = "synthetic-user", "synthetic-password"
			}
			options, closeProxy, err := provider.browserLaunchOptions(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer closeProxy()
			if options.Proxy == nil || options.IgnoreHttpsErrors == nil || *options.IgnoreHttpsErrors || options.HttpCredentials != nil {
				t.Fatal("proxy weakened browser TLS or used origin credentials")
			}
			if _, exists := options.FirefoxUserPrefs["network.proxy.type"]; exists {
				t.Fatal("direct-only preference conflicts with explicit proxy")
			}
			if tc.scheme == "socks5" && tc.authenticated {
				if !strings.HasPrefix(options.Proxy.Server, "http://127.0.0.1:") || options.Proxy.Username == nil || *options.Proxy.Username == "synthetic-user" || options.Proxy.Password == nil || *options.Proxy.Password == "synthetic-password" {
					t.Fatal("authenticated SOCKS5 was not adapted")
				}
				closeProxy()
				if conn, err := net.DialTimeout("tcp", strings.TrimPrefix(options.Proxy.Server, "http://"), 100*time.Millisecond); err == nil {
					conn.Close()
					t.Fatal("browser proxy cleanup left its adapter open")
				}
			} else if options.Proxy.Server != provider.options.Proxy.URL || tc.authenticated && (options.Proxy.Username == nil || *options.Proxy.Username != "synthetic-user" || options.Proxy.Password == nil || *options.Proxy.Password != "synthetic-password") {
				t.Fatal("browser proxy setting changed")
			}
			raw, err := json.Marshal(provider.options)
			if err != nil {
				t.Fatal(err)
			}
			for _, output := range []string{string(raw), fmt.Sprintf("%+v", provider.options), fmt.Sprintf("%#v", provider)} {
				if strings.Contains(output, "synthetic-user") || strings.Contains(output, "synthetic-password") {
					t.Fatal("browser proxy credentials leaked")
				}
			}
		})
	}
}

func TestFirefoxProxyConstructorRejectsInvalidSettingsBeforeLaunch(t *testing.T) {
	if _, err := NewFirefoxBootstrap(FirefoxOptions{Proxy: sber.ProxyOptions{URL: "ftp://localhost:21", Password: "synthetic-secret"}}); err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatal("invalid browser proxy accepted or echoed")
	}
}
