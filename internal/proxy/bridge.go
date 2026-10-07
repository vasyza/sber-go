package proxy

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vasyza/sber-go/internal/errs"
	"golang.org/x/net/netutil"
	socks "golang.org/x/net/proxy"
)

// Bridge supplies a loopback HTTP CONNECT endpoint for browsers that cannot
// authenticate to SOCKS5. It never terminates destination TLS. A random local
// token protects its listener; the remote credentials remain in its dialer.
type Bridge struct {
	ctx         context.Context
	cancel      context.CancelFunc
	server      *http.Server
	listener    net.Listener
	dialer      socks.ContextDialer
	local       Options
	timeout     time.Duration
	mu          sync.Mutex
	closed      bool
	connections map[net.Conn]struct{}
	workers     sync.WaitGroup
	done        chan struct{}
	once        sync.Once
}

func (b *Bridge) String() string               { return "ProxyBridge(<redacted>)" }
func (b *Bridge) Format(f fmt.State, _ rune)   { errs.FormatError(f, b.String()) }
func (b *Bridge) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }
func (b *Bridge) Options() Options             { return b.local }

func StartBridge(ctx context.Context, o Options, timeout time.Duration, capacity int) (*Bridge, error) {
	if ctx == nil || timeout <= 0 || timeout > 120*time.Second || capacity < 1 || capacity > 100 {
		return nil, invalid()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o, err := o.Normalize()
	if err != nil || !strings.HasPrefix(o.URL, "socks5://") {
		return nil, invalid()
	}
	var auth *socks.Auth
	if o.Username != "" {
		auth = &socks.Auth{User: o.Username, Password: o.Password}
	}
	dialer, err := socks.SOCKS5("tcp", strings.TrimPrefix(o.URL, "socks5://"), auth, &net.Dialer{Timeout: timeout})
	if err != nil {
		return nil, invalid()
	}
	contextDialer, ok := dialer.(socks.ContextDialer)
	if !ok {
		return nil, invalid()
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, &errs.TransportError{Code: "proxy_failed"}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, &errs.TransportError{Code: "proxy_failed"}
	}
	root, cancel := context.WithCancel(ctx)
	b := &Bridge{ctx: root, cancel: cancel, listener: netutil.LimitListener(listener, capacity), dialer: contextDialer,
		local: Options{URL: "http://" + listener.Addr().String(), Username: "sber-bridge", Password: hex.EncodeToString(token[:])}, timeout: timeout,
		connections: make(map[net.Conn]struct{}), done: make(chan struct{})}
	b.server = &http.Server{Handler: http.HandlerFunc(b.serve), ReadHeaderTimeout: timeout, IdleTimeout: timeout, WriteTimeout: timeout,
		MaxHeaderBytes: 8 * 1024, ErrorLog: log.New(io.Discard, "", 0), BaseContext: func(net.Listener) context.Context { return root },
		ConnState: func(c net.Conn, state http.ConnState) {
			if state == http.StateNew {
				b.track(c)
			} else if state == http.StateClosed {
				b.untrack(c)
			}
		}}
	go func() { defer close(b.done); _ = b.server.Serve(b.listener) }()
	context.AfterFunc(root, b.Close)
	return b, nil
}

// Close cancels pending SOCKS authentication, stops the listener, closes all
// active tunnels, and joins their workers. It is safe to call more than once.
func (b *Bridge) Close() {
	b.once.Do(func() {
		b.mu.Lock()
		b.closed = true
		connections := make([]net.Conn, 0, len(b.connections))
		for c := range b.connections {
			connections = append(connections, c)
		}
		b.mu.Unlock()
		b.cancel()
		_ = b.server.Close()
		_ = b.listener.Close()
		for _, c := range connections {
			_ = c.Close()
		}
		<-b.done
		b.workers.Wait()
	})
}

func (b *Bridge) track(c net.Conn) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		c.Close()
		return false
	}
	b.connections[c] = struct{}{}
	return true
}
func (b *Bridge) untrack(c net.Conn) { b.mu.Lock(); delete(b.connections, c); b.mu.Unlock() }

func (b *Bridge) serve(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	b.workers.Add(1)
	b.mu.Unlock()
	defer b.workers.Done()
	credentials := r.Clone(r.Context())
	credentials.Header = r.Header.Clone()
	credentials.Header.Set("Authorization", r.Header.Get("Proxy-Authorization"))
	username, password, ok := credentials.BasicAuth()
	if !ok || subtle.ConstantTimeCompare([]byte(username), []byte(b.local.Username)) != 1 || subtle.ConstantTimeCompare([]byte(password), []byte(b.local.Password)) != 1 {
		w.Header().Set("Proxy-Authenticate", `Basic realm="sber"`)
		w.WriteHeader(http.StatusProxyAuthRequired)
		return
	}
	if r.Method != http.MethodConnect || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, err := (Options{URL: "http://" + r.Host}).Normalize(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, b.timeout)
	defer cancel()
	upstream, err := b.dialer.DialContext(ctx, "tcp", r.Host)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if !b.track(upstream) {
		return
	}
	defer b.untrack(upstream)
	defer upstream.Close()
	client, buffered, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer b.untrack(client)
	defer client.Close()
	deadline, _ := ctx.Deadline()
	client.SetDeadline(deadline)
	upstream.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { client.Close(); upstream.Close() })
	defer stop()
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if err := buffered.Flush(); err != nil {
		return
	}
	done := make(chan struct{})
	go func() { defer close(done); io.Copy(upstream, buffered); upstream.Close() }()
	io.Copy(client, upstream)
	client.Close()
	upstream.Close()
	<-done
}
