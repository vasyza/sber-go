// Package testproxy supplies bounded localhost proxies for synthetic tests.
package testproxy

import (
	"bufio"
	"crypto/tls"
	"encoding/binary"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type Stats struct {
	Connections atomic.Int32
	Authorized  atomic.Int32
}

func tunnel(a net.Conn, reader io.Reader, b net.Conn) {
	defer a.Close()
	defer b.Close()
	if a.SetDeadline(time.Now().Add(5*time.Second)) != nil || b.SetDeadline(time.Now().Add(5*time.Second)) != nil {
		return
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(b, reader); _ = b.Close(); close(done) }()
	_, _ = io.Copy(a, b)
	_ = a.Close()
	<-done
}

// HTTP accepts only CONNECT to the specified synthetic destination. HTTPS
// mode uses httptest's local certificate; no remote destination can be dialed.
func HTTP(t testing.TB, secure bool, username, password, destination string) (*httptest.Server, *Stats) {
	return httpProxy(t, secure, nil, username, password, destination)
}

func HTTPWithTLS(t testing.TB, config *tls.Config, username, password, destination string) (*httptest.Server, *Stats) {
	return httpProxy(t, true, config, username, password, destination)
}

func httpProxy(t testing.TB, secure bool, config *tls.Config, username, password, destination string) (*httptest.Server, *Stats) {
	t.Helper()
	stats := new(Stats)
	var workers sync.WaitGroup
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stats.Connections.Add(1)
		if username != "" {
			clone := r.Clone(r.Context())
			clone.Header = r.Header.Clone()
			clone.Header.Set("Authorization", r.Header.Get("Proxy-Authorization"))
			u, p, ok := clone.BasicAuth()
			if !ok || u != username || p != password {
				w.Header().Set("Proxy-Authenticate", `Basic realm="synthetic"`)
				w.WriteHeader(http.StatusProxyAuthRequired)
				return
			}
		}
		stats.Authorized.Add(1)
		if r.Method != http.MethodConnect || r.Host != destination {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		upstream, err := net.DialTimeout("tcp", destination, time.Second)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			_ = upstream.Close()
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		client, buffered, err := hijacker.Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		workers.Add(1)
		defer workers.Done()
		if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			_ = client.Close()
			_ = upstream.Close()
			return
		}
		if err := buffered.Flush(); err != nil {
			_ = client.Close()
			_ = upstream.Close()
			return
		}
		tunnel(client, buffered, upstream)
	})
	s := httptest.NewUnstartedServer(handler)
	if config != nil {
		s.TLS = config.Clone()
	}
	s.Config.ErrorLog = log.New(io.Discard, "", 0)
	if secure {
		s.StartTLS()
	} else {
		s.Start()
	}
	t.Cleanup(func() { s.Close(); workers.Wait() })
	return s, stats
}

// SOCKS5 implements only the test handshake and one exact local destination.
func SOCKS5(t testing.TB, username, password, destination string) (string, *Stats) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stats := new(Stats)
	var workers sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			workers.Add(1)
			go func() { defer workers.Done(); serveSOCKS(client, stats, username, password, destination) }()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done; workers.Wait() })
	return listener.Addr().String(), stats
}

func serveSOCKS(client net.Conn, stats *Stats, username, password, destination string) {
	defer client.Close()
	if client.SetDeadline(time.Now().Add(5*time.Second)) != nil {
		return
	}
	stats.Connections.Add(1)
	reader := bufio.NewReader(client)
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil || header[0] != 5 {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(reader, methods); err != nil {
		return
	}
	want := byte(0)
	if username != "" {
		want = 2
	}
	accepted := false
	for _, m := range methods {
		if m == want {
			accepted = true
		}
	}
	if !accepted {
		_, _ = client.Write([]byte{5, 255})
		return
	}
	if _, err := client.Write([]byte{5, want}); err != nil {
		return
	}
	if want == 2 {
		if _, err := io.ReadFull(reader, header); err != nil || header[0] != 1 {
			return
		}
		u := make([]byte, int(header[1]))
		if _, err := io.ReadFull(reader, u); err != nil {
			return
		}
		n, err := reader.ReadByte()
		if err != nil {
			return
		}
		p := make([]byte, int(n))
		if _, err := io.ReadFull(reader, p); err != nil {
			return
		}
		if string(u) != username || string(p) != password {
			_, _ = client.Write([]byte{1, 1})
			return
		}
		if _, err := client.Write([]byte{1, 0}); err != nil {
			return
		}
	}
	stats.Authorized.Add(1)
	request := make([]byte, 4)
	if _, err := io.ReadFull(reader, request); err != nil || request[0] != 5 || request[1] != 1 {
		return
	}
	var host string
	switch request[3] {
	case 1:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(reader, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 4:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(reader, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 3:
		n, err := reader.ReadByte()
		if err != nil {
			return
		}
		name := make([]byte, int(n))
		if _, err := io.ReadFull(reader, name); err != nil {
			return
		}
		host = string(name)
	default:
		return
	}
	port := make([]byte, 2)
	if _, err := io.ReadFull(reader, port); err != nil {
		return
	}
	address := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port))))
	if address != destination {
		_, _ = client.Write([]byte{5, 2, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	upstream, err := net.DialTimeout("tcp", destination, time.Second)
	if err != nil {
		_, _ = client.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	if _, err := client.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 1}); err != nil {
		_ = upstream.Close()
		return
	}
	tunnel(client, reader, upstream)
}
