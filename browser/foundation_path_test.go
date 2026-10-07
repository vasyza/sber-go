package browser

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	sber "github.com/vasyza/sber-go"
)

func TestFoundationPublicPathTraversal(t *testing.T) {
	const traversal = "/TSPD/%2e%2e%2fuoh-bh/v1/operations/list"
	wire := make(chan string, 1)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wire <- r.RequestURI
		fmt.Fprint(w, path.Clean(r.URL.Path))
	}))
	defer origin.Close()
	// Only localhost is contacted. This is an ordinary server's routing
	// normalization, not an assertion about any bank's implementation.
	res, err := origin.Client().Get(origin.URL + traversal)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if got := <-wire; got != traversal {
		t.Fatalf("wire fixture changed: %q", got)
	}
	if string(normalized) != "/uoh-bh/v1/operations/list" {
		t.Fatalf("wire normalization fixture invalid: %q", normalized)
	}
	for _, p := range []string{
		traversal, "/TSPD/../uoh-bh/v1/operations/list", "/TSPD/%2e%2e/uoh-bh/v1/operations/list",
		"/TSPD/%252e%252e%252fuoh-bh/v1/operations/list", "/TSPD/%25252e%25252e%25252fuoh-bh/v1/operations/list",
		"/TSPD/foo%2fbar", "/TSPD/foo%5cbar", "/TSPD/%255c..%255cauthMainJson.do",
		"/TSPD/./synthetic", "/TSPD//synthetic", "/CSAFront/../private.js",
	} {
		for _, resource := range []string{"fetch", "document", "script"} {
			if publicBootstrapRequest(sber.AppOrigin+p, "GET", resource, sber.PublicBootstrapURL, false) {
				t.Errorf("ambiguous/traversing path allowed: %s (%s)", p, resource)
			}
		}
	}
	for _, p := range []string{"/TSPD/synthetic", "/TSPD/synthetic.js?cache=1", "/CSAFront/synthetic.css", "/CSAFront/synthetic%20asset.js", "/CSAFront/%73ynthetic.js"} {
		if !publicBootstrapRequest(sber.AppOrigin+p, "GET", "script", sber.PublicBootstrapURL, false) {
			t.Errorf("ordinary public path rejected: %s", p)
		}
	}
	if !publicBootstrapRequest(sber.PublicBootstrapURL, "GET", "document", sber.PublicBootstrapURL, true) {
		t.Error("ordinary main bootstrap rejected")
	}
}
