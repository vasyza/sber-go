package proxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestParseProxyFormats(t *testing.T) {
	for _, tc := range []struct{ input, address, username, password string }{
		{"127.0.0.1:3128", "http://127.0.0.1:3128", "", ""},
		{"localhost:3128:synthetic-user:synthetic-password", "http://localhost:3128", "synthetic-user", "synthetic-password"},
		{"localhost:3128:u:p://with:colons", "http://localhost:3128", "u", "p://with:colons"},
		{"http://127.0.0.1:3128:u:p", "http://127.0.0.1:3128", "u", "p"},
		{"https://localhost:443:u:p:with:colons", "https://localhost:443", "u", "p:with:colons"},
		{"socks5://127.0.0.1:1080:u:p@/?#%", "socks5://127.0.0.1:1080", "u", "p@/?#%"},
		{"socks5://[::1]:1080:u:p", "socks5://[::1]:1080", "u", "p"},
	} {
		t.Run(tc.address, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err != nil || got.URL != tc.address || got.Username != tc.username || got.Password != tc.password {
				t.Fatal("proxy format was not parsed")
			}
		})
	}
}

func TestInvalidProxyAndSecretRedaction(t *testing.T) {
	for _, input := range []string{"", "localhost", "localhost:0", "localhost:65536", "localhost:-1", "ftp://localhost:21", "http://localhost:1/path", "http://localhost:1?query", "http://u:p@localhost:1", "localhost:1:user", "localhost:1::secret", "localhost:1:u:p\n", "::1:1080:u:p"} {
		if _, err := Parse(input); err == nil || strings.Contains(err.Error(), input) && input != "" {
			t.Fatal("invalid proxy accepted or echoed")
		}
	}
	o := Options{URL: "http://synthetic-private-host:3128", Username: "synthetic-user", Password: "synthetic-password"}
	raw, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{string(raw), fmt.Sprintf("%v", o), fmt.Sprintf("%+v", o), fmt.Sprintf("%#v", o)} {
		if strings.Contains(output, "synthetic") {
			t.Fatal("proxy settings leaked")
		}
	}
}
