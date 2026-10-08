package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sber "github.com/vasyza/sber-go"
)

type qrImageTransport struct {
	testing *testing.T
	jar     *sber.CookieJar
}

func (tr *qrImageTransport) Get(_ context.Context, target string, _ sber.RequestOptions) (*sber.Response, error) {
	if target != sber.PublicBootstrapURL {
		tr.testing.Fatal("unexpected fixture GET")
	}
	html := `<script>window.config={baseApiUrl:"CSAFront",processId:"synthetic",authTypeByCookie:"start",srpConfig:{enabled:true,N:"` + strings.Repeat("f", 512) + `",g:"2"},pinConfig:{enabled:true,hasPin:false},validation:{pin:{length:5}}};</script>`
	return &sber.Response{StatusCode: 200, Content: []byte(html)}, nil
}
func (tr *qrImageTransport) Post(_ context.Context, target string, _ map[string]any, _ sber.RequestOptions) (*sber.Response, error) {
	if target != sber.AppOrigin+"/CSAFront/uapi/v2/identify" {
		tr.testing.Fatal("unexpected fixture POST")
	}
	raw, err := json.Marshal(map[string]any{"operation": map[string]any{"guid": "synthetic-guid"}, "identifier": map[string]any{"type": "qr", "data": map[string]any{"value": "synthetic-qr-challenge", "qr_format": "content", "qr_size": 190}}})
	return &sber.Response{StatusCode: 200, Content: raw}, err
}
func (tr *qrImageTransport) PostForm(context.Context, string, map[string]string, sber.RequestOptions) (*sber.Response, error) {
	tr.testing.Fatal("unexpected fixture form")
	return nil, nil
}
func (tr *qrImageTransport) CookieJar() *sber.CookieJar { return tr.jar }
func (*qrImageTransport) Close() error                  { return nil }
func syntheticQRImageChallenge(t *testing.T) sber.QRCode {
	t.Helper()
	jar, err := sber.NewCookieJar(nil)
	if err != nil {
		t.Fatal(err)
	}
	d := "synthetic-device"
	auth, err := sber.NewQRAuth(sber.AuthOptions{Deviceprint: &d, Transport: &qrImageTransport{testing: t, jar: jar}})
	if err != nil {
		t.Fatal(err)
	}
	defer auth.Close()
	challenge, err := auth.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return challenge
}
func TestQRImageIsPrivateValidPNGAndCannotReplaceAFile(t *testing.T) {
	c := syntheticQRImageChallenge(t)
	path := filepath.Join(testPrivateDir(t), "qr.png")
	var output bytes.Buffer
	if err := displayLoginQR(path, c, &output); err != nil {
		t.Fatal(err)
	}
	state, err := os.Stat(path)
	if err != nil || state.Mode().Perm() != 0600 {
		t.Fatal("QR file is not private")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil || decoded.Bounds().Dx() != 384 || decoded.Bounds().Dy() != 384 {
		t.Fatal("QR output is not a valid PNG")
	}
	if strings.Contains(output.String(), c.Content()) {
		t.Fatal("QR content appeared in text diagnostics")
	}
	if err = displayLoginQR(path, c, &output); err == nil {
		t.Fatal("QR output replaced an existing file")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("existing QR output changed")
	}
}
func TestQRImageRejectsSymbolicLink(t *testing.T) {
	c := syntheticQRImageChallenge(t)
	dir := testPrivateDir(t)
	target := filepath.Join(dir, "owner-file")
	if err := os.WriteFile(target, []byte("synthetic-owner-file"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "qr.png")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symbolic links unavailable")
	}
	var output bytes.Buffer
	if err := displayLoginQR(link, c, &output); err == nil {
		t.Fatal("QR output followed a symbolic link")
	}
	raw, err := os.ReadFile(target)
	if err != nil || string(raw) != "synthetic-owner-file" {
		t.Fatal("QR output changed link target")
	}
}
