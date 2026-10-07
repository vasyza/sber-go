package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/testutil"
	"github.com/vasyza/sber-go/mcp"
)

func TestBankErrorClassificationsExcludeRemoteDetails(t *testing.T) {
	for _, tt := range []struct {
		err     error
		message string
	}{
		{&sber.APIRejected{Code: "synthetic-private", Text: "synthetic-private", UUID: "synthetic-private"}, "bank rejected the request"},
		{&sber.APIError{StatusCode: 500, Message: "synthetic-private"}, "HTTP 500"},
		{sber.NewParseError("synthetic-private"), "bank response format is not supported"},
	} {
		var output, diagnostics bytes.Buffer
		client := &testutil.Client{Read: func(context.Context, string, map[string]any) (map[string]any, error) { return nil, tt.err }}
		code := RunWithOptions(context.Background(), []string{"products", "--profile", "synthetic-selected"}, &output, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { return client, nil }})
		if code != 3 || output.Len() != 0 || !strings.Contains(diagnostics.String(), tt.message) || strings.Contains(diagnostics.String(), "synthetic-private") {
			t.Fatal("bank failure lost its safe classification")
		}
	}
}
