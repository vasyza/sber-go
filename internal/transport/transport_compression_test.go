package transport

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

func TestHTTPNativeCompressionAndDecodedSizeBound(t *testing.T) {
	const content = "synthetic native compression content"
	for _, enc := range []string{"gzip", "deflate", "br", "zstd"} {
		t.Run(enc, func(t *testing.T) {
			var raw bytes.Buffer
			var writer io.WriteCloser
			switch enc {
			case "gzip":
				writer = gzip.NewWriter(&raw)
			case "deflate":
				writer = zlib.NewWriter(&raw)
			case "br":
				writer = brotli.NewWriter(&raw)
			case "zstd":
				w, err := zstd.NewWriter(&raw)
				if err != nil {
					t.Fatal(err)
				}
				writer = w
			}
			if _, err := io.WriteString(writer, content); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", enc)
				w.Write(raw.Bytes())
			}))
			defer s.Close()
			tr, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, s), AllowUnready: true})
			if err != nil {
				t.Fatal(err)
			}
			defer tr.Close()
			response, err := tr.Get(context.Background(), s.URL, RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd"})
			if err != nil || response.Text() != content {
				t.Fatalf("native %s decoder not usable: %v", enc, err)
			}
			small, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, s), AllowUnready: true, MaxResponseBytes: 4})
			if err != nil {
				t.Fatal(err)
			}
			defer small.Close()
			if _, err := small.Get(context.Background(), s.URL, RequestOptions{}); safeTransportCode(err) != "response_too_large" {
				t.Fatal("decoded size bound failed")
			}
		})
	}
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		io.WriteString(w, strings.Repeat("invalid", 3))
	}))
	defer s.Close()
	tr, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, s), AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if _, err := tr.Get(context.Background(), s.URL, RequestOptions{}); safeTransportCode(err) != "invalid_encoding" {
		t.Fatal("invalid compressed response accepted")
	}
}
