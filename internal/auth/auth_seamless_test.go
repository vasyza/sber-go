package auth

import (
	"context"
	"testing"

	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestAuthSeamlessRedirectCarriesProcessIdentity(t *testing.T) {
	const processID = "synthetic-redirect-process"
	script := newAuthScript(t, authStep{
		method: "POST", target: sdkSession.AppOrigin + "/finish",
		response: authJSON(500, map[string]any{}),
		inspect: func(body map[string]any, _ map[string]string, options sdkTransport.RequestOptions) {
			if body != nil {
				t.Error("seamless redirect must keep an empty request body")
			}
			if authHeader(options, "Process-Id") != processID {
				t.Error("seamless redirect lost the originating authentication process")
			}
			if authHeader(options, "X-Seamless-Web") != "true" {
				t.Error("seamless redirect lost its request mode")
			}
		},
	})
	auth, err := NewPrimaryAuth(AuthOptions{Deviceprint: stringPointer("synthetic-device"), Transport: script})
	if err != nil {
		t.Fatal(err)
	}
	defer auth.Close()
	config := sdkSession.NewFrontendConfig("", processID, 0, "", "", true, true)
	_, err = auth.finishRedirect(context.Background(), config, "/finish")
	authErrorCode(t, err, "redirect_failed")
	if script.calls != 1 {
		t.Fatal("failed redirect was automatically retried")
	}
}

func stringPointer(value string) *string { return &value }
