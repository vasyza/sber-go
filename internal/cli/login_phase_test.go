package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/ownerinput"
)

type failedPINEnrollment struct{ syntheticPrimary }

func (*failedPINEnrollment) CreatePIN(context.Context, string) (sber.SessionBundle, error) {
	return sber.SessionBundle{}, errors.New("synthetic-private-remote-body")
}

func TestPrimaryFailureReportsLocalPhaseWithoutRemoteMetadata(t *testing.T) {
	var output, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"login", "--profile", filepath.Join(testPrivateDir(t), "new.json")}, &output, &diagnostics, Options{Authentication: &Authentication{
		NewPrimary: func() (PrimaryAuthenticator, error) { return &failedPINEnrollment{}, nil },
		ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) { return "synthetic-private-input", nil },
	}})
	if code != 3 || output.Len() != 0 || !strings.Contains(diagnostics.String(), "stage=pin-enrollment") || strings.Contains(diagnostics.String(), "synthetic-private") {
		t.Fatal("PIN enrollment failure lost its local phase or exposed remote data")
	}
}
