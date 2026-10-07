package sber

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestClientPersistsOnlySuccessfulRotationAndPreservesProfileScope(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(map[bool]string{false: "minimum", true: "full"}[full], func(t *testing.T) {
			b := clientFixture(t, "old")
			b.AntifraudDeviceprint = ptrString("version%3Dfixture")
			if full {
				b.Cookies = append(b.Cookies, CookieRecord{Name: "remembered", Value: "synthetic-remembered", Domain: AuthCookieDomain, Path: "/CSAFront", Secure: true, HTTPOnly: true, HostOnly: true, SameSite: ptrString("Strict")})
			}
			path := clientPrivatePath(t)
			if e := b.Save(path); e != nil {
				t.Fatal(e)
			}
			tr := clientFake(t, b)
			mode := "success"
			tr.post = func(_ context.Context, _ string, _ map[string]any, _ RequestOptions) (*Response, error) {
				u := mustClientURL(t, AppOrigin+"/")
				if e := tr.jar.ApplySetCookie(u, []string{"UFS-SESSION=session-" + mode + "; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly; SameSite=Lax", "UFS-TOKEN=token-" + mode + "; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly", "protection=synthetic-protection; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly; SameSite=None"}); e != nil {
					t.Fatal(e)
				}
				switch mode {
				case "expired":
					return clientResponse(401, `{}`), nil
				case "rejected":
					return clientResponse(200, `{"success":false}`), nil
				case "network":
					return nil, errors.New("raw sensitive URL must not escape")
				}
				return clientResponse(200, `{"success":true}`), nil
			}
			c, e := NewSberClientFromSessionFile(path, ClientOptions{Transport: tr})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operation/details", nil); e != nil {
				t.Fatal(e)
			}
			saved, e := LoadSessionBundle(path)
			if e != nil {
				t.Fatal(e)
			}
			creds, e := CredentialsFromBundle(saved)
			if e != nil || creds.UFSSession != "session-success" {
				t.Fatal("rotation was not saved")
			}
			expected := 2
			if full {
				expected = 4
			}
			if len(saved.Cookies) != expected || *saved.Deviceprint != *b.Deviceprint || *saved.AntifraudDeviceprint != *b.AntifraudDeviceprint {
				t.Fatal("wrong persistence scope")
			}
			if full && saved.Cookies[2].Path != "/CSAFront" {
				t.Fatal("remembered identity lost")
			}
			info, e := os.Stat(path)
			if e != nil || info.Mode().Perm() != 0600 {
				t.Fatal("profile not private")
			}
			before, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			for _, next := range []string{"expired", "rejected", "network"} {
				mode = next
				if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operation/details", nil); e == nil {
					t.Fatal("failed response accepted")
				}
				after, e := os.ReadFile(path)
				if e != nil || !bytes.Equal(before, after) {
					t.Fatal("failed response persisted")
				}
			}
			// The live rotating jar is still available explicitly, including extra cookies
			// that credential-only persistence deliberately excludes.
			exported, e := c.ExportSession()
			if e != nil || len(exported.Cookies) < 3 {
				t.Fatal("live cookies lost")
			}
		})
	}
}
func TestClientReadPersistenceFailureIsReturnedWithoutReplay(t *testing.T) {
	b := clientFixture(t, "persist")
	tr := clientFake(t, b)
	path := clientPrivatePath(t)
	if e := os.Chmod(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr, SessionPath: path})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.PostRead(context.Background(), "/pfpv_alf_mb/v1.00/alf/amounts", nil)
	var insecure *InsecureSessionFile
	if !errors.As(e, &insecure) {
		t.Fatal("persistence failure hidden")
	}
	calls, _, _ := tr.counts()
	if calls != 1 {
		t.Fatal("business read replayed for persistence")
	}
}
