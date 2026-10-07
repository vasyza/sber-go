package mcpwire

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vasyza/sber-go/internal/strictjson"
)

var ErrCancelled = errors.New("MCP service cancelled")
var ErrClientResponse = errors.New("client responses are not permitted on stdio")

// Serve delegates protocol lifecycle, dispatch, correlation, and cancellation
// to the official SDK. Ownership of closable streams transfers to Serve;
// blocking streams must be closable and callbacks must honor cancellation.
func (server *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	if ctx == nil || input == nil || output == nil {
		return ErrConfiguration
	}
	sdk, err := server.sdkServer()
	if err != nil {
		return err
	}
	reader := &streamReader{source: input}
	reader.scanner = bufio.NewScanner(guardedReader{source: input})
	reader.scanner.Buffer(make([]byte, 4096), MaxFrameBytes+2)
	reader.scanner.Split(splitFrame)
	writer := &streamWriter{source: output}
	defer reader.Close()
	defer writer.Close()
	// SDK Close drains handlers before closing I/O. Close owned streams first
	// when the caller cancels, so a blocked read/write cannot prevent that drain.
	stop, watcherDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			reader.Close()
			writer.Close()
		case <-stop:
		}
	}()
	err = sdk.Run(ctx, &safeTransport{maxInFlight: server.maxInFlight, IOTransport: mcp.IOTransport{
		Reader: reader, Writer: writer, MaxLineLength: -1, // streamReader bounds complete frames.
	}})
	close(stop)
	<-watcherDone
	if ctx.Err() != nil {
		return ErrCancelled
	}
	// Run has closed the connection and joined all active handlers.
	if writer.closeErr != nil {
		return writer.closeErr
	}
	if reader.closeErr != nil {
		return reader.closeErr
	}
	if writer.failed.Load() {
		return ErrOutput
	}
	if err != nil {
		for _, known := range []error{ErrOutput, ErrFrameTooLarge, ErrTruncatedFrame, ErrClientResponse} {
			if errors.Is(err, known) {
				return known
			}
		}
		return ErrInput
	}
	return nil
}

// Frame guards enforce local byte/Unicode limits without implementing MCP or
// JSON-RPC. Malformed framing/JSON terminates the transport with a static error.
type streamReader struct {
	source   io.Reader
	scanner  *bufio.Scanner
	pending  []byte
	closeErr error
	once     sync.Once
}

func (r *streamReader) Read(p []byte) (int, error) {
	if len(r.pending) == 0 {
		if !r.scanner.Scan() {
			if err := r.scanner.Err(); err != nil {
				if err != ErrFrameTooLarge && err != ErrTruncatedFrame {
					err = ErrInput
				}
				return 0, err
			}
			return 0, io.EOF
		}
		frame := r.scanner.Bytes()
		if strictjson.Validate(frame) != nil || !object(frame) {
			return 0, ErrInput
		}
		var envelope map[string]json.RawMessage
		_ = json.Unmarshal(frame, &envelope)
		if _, present := envelope["result"]; present {
			return 0, ErrClientResponse
		}
		if _, present := envelope["error"]; present {
			return 0, ErrClientResponse
		}
		if id, present := envelope["id"]; present && !validWireID(id) {
			return 0, ErrInput
		}
		// The SDK decoder rejects whitespace buffered after a JSON value.
		// Trimming framing whitespace preserves every original argument byte.
		r.pending = append(append([]byte(nil), bytes.TrimSpace(frame)...), '\n')
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *streamReader) Close() error {
	r.once.Do(func() {
		if closer, ok := r.source.(io.Closer); ok {
			r.closeErr = closeIO(closer, ErrInput)
		}
	})
	return r.closeErr
}

type streamWriter struct {
	source   io.Writer
	closeErr error
	once     sync.Once
	failed   atomic.Bool
}

func (w *streamWriter) Write(p []byte) (int, error) {
	n, err := writeFrame(w.source, p)
	if err != nil || n != len(p) {
		w.failed.Store(true)
		return n, ErrOutput
	}
	return n, nil
}

func (w *streamWriter) Close() error {
	w.once.Do(func() {
		if closer, ok := w.source.(io.Closer); ok {
			w.closeErr = closeIO(closer, ErrOutput)
		}
	})
	return w.closeErr
}

type safeTransport struct {
	mcp.IOTransport
	maxInFlight int
}

func (t *safeTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	conn, err := t.IOTransport.Connect(ctx)
	if err != nil {
		return nil, ErrInput
	}
	return &safeConnection{Connection: conn, active: make(map[jsonrpc.ID]requestState), maxInFlight: t.maxInFlight}, nil
}

// The stdio binding forbids replies after explicit cancellation. The SDK
// cancels callback contexts; this guard suppresses their eventual wire reply.
type safeConnection struct {
	mcp.Connection
	mu                sync.Mutex
	writeMu           sync.Mutex
	active            map[jsonrpc.ID]requestState
	maxInFlight       int
	toolsPending      int
	legacyInitialized bool
}

type requestState struct {
	cancelled  bool
	tool       bool
	initialize bool
}

func (c *safeConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	for {
		msg, err := c.Connection.Read(ctx)
		if err != nil {
			if err != io.EOF {
				c.Connection.Close() // Interrupt writes before SDK drains handlers.
			}
			return nil, err
		}
		req, ok := msg.(*jsonrpc.Request)
		if !ok {
			return nil, ErrClientResponse
		}
		if !req.IsCall() {
			if !strings.HasPrefix(req.Method, "notifications/") {
				continue // Methods requiring a response cannot be notifications.
			}
			if req.Method == "notifications/cancelled" {
				id, valid := cancellationID(req.Params)
				if !valid {
					continue
				}
				c.mu.Lock()
				if state, exists := c.active[id]; exists {
					state.cancelled = true
					c.active[id] = state
				}
				c.mu.Unlock()
			}
			return req, nil
		}
		c.mu.Lock()
		_, duplicate := c.active[req.ID]
		// Bound queued control responses as well as tools. One extra request
		// permits a capacity rejection/control response without an unbounded
		// SDK handler queue when the peer stops reading output.
		full := len(c.active) >= c.maxInFlight+1
		excessTool := req.Method == "tools/call" && c.toolsPending >= c.maxInFlight
		waited := duplicate || full || excessTool
		if waited {
			c.mu.Unlock()
			// A peer can receive the final response bytes before Write returns.
			// Wait for response cleanup before deciding whether ID reuse or a
			// new tool exceeds the budget; blocked output backpressures input.
			c.writeMu.Lock()
			c.mu.Lock()
			_, duplicate = c.active[req.ID]
			full = len(c.active) >= c.maxInFlight+1
			excessTool = req.Method == "tools/call" && c.toolsPending >= c.maxInFlight
		}
		legacyInitialized := c.legacyInitialized
		if !duplicate && !full {
			state := requestState{tool: req.Method == "tools/call" && !excessTool, initialize: req.Method == "initialize"}
			c.active[req.ID] = state
			if state.tool {
				c.toolsPending++
			}
		}
		c.mu.Unlock()
		if waited {
			c.writeMu.Unlock()
		}
		if duplicate || full {
			c.Connection.Close()
			return nil, ErrInput // Never misattribute a duplicate active ID.
		}
		if excessTool {
			if err := c.Write(ctx, &jsonrpc.Response{ID: req.ID, Error: protocolError(-32603)}); err != nil {
				return nil, err
			}
			continue
		}
		if err := validateRequestBoundary(req); err != nil {
			if err := c.Write(ctx, &jsonrpc.Response{ID: req.ID, Error: err}); err != nil {
				return nil, err
			}
			continue
		}
		if (req.Method == "tools/list" || req.Method == "tools/call") && !requestHasModernMeta(req) && !legacyInitialized {
			if err := c.Write(ctx, &jsonrpc.Response{ID: req.ID, Error: protocolError(-32602)}); err != nil {
				return nil, err
			}
			continue
		}
		return req, nil
	}
}

func (c *safeConnection) Write(ctx context.Context, msg jsonrpc.Message) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if response, ok := msg.(*jsonrpc.Response); ok {
		c.mu.Lock()
		state := c.active[response.ID]
		if state.initialize && response.Error == nil {
			var result struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if json.Unmarshal(response.Result, &result) == nil && result.ProtocolVersion == LegacyVersion {
				c.legacyInitialized = true
			}
		}
		c.mu.Unlock()
		// Retain correlation and admission until the response is emitted or
		// suppressed. Cleanup precedes writeMu release, so immediate ID reuse
		// cannot race with retirement of the prior request.
		defer func() {
			c.mu.Lock()
			delete(c.active, response.ID)
			if state.tool {
				c.toolsPending--
			}
			c.mu.Unlock()
		}()
		if state.cancelled {
			return nil
		}
		copy := *response
		if response.Error != nil {
			var wireError *jsonrpc.Error
			failure := jsonrpc.Error{Code: -32603, Message: staticMessage(-32603)}
			if errors.As(response.Error, &wireError) {
				failure = *wireError
				failure.Message = staticMessage(failure.Code)
				if failure.Code != mcp.CodeUnsupportedProtocolVersion {
					failure.Data = nil
				}
			}
			copy.Error = &failure
		}
		msg = &copy
	}
	frame, err := jsonrpc.EncodeMessage(msg)
	if err != nil || len(frame) > MaxFrameBytes || strictjson.Validate(frame) != nil {
		response, ok := msg.(*jsonrpc.Response)
		if !ok {
			return ErrOutput
		}
		msg = &jsonrpc.Response{ID: response.ID, Error: &jsonrpc.Error{Code: -32603, Message: staticMessage(-32603)}}
	}
	return c.Connection.Write(ctx, msg)
}

// Numeric IDs outside this exact range would be rounded by the SDK's float64
// ID decoder. Reject them before decoding; clients can use string IDs instead.
const MaxIDBytes = 256
const maxExactID = 1<<53 - 1

func validWireID(raw json.RawMessage) bool {
	if len(raw) == 0 || len(raw) > MaxIDBytes {
		return false
	}
	if _, ok := stringValue(raw); ok {
		return true
	}
	n, err := strconv.ParseInt(string(raw), 10, 64)
	return err == nil && n >= -maxExactID && n <= maxExactID
}

func cancellationID(params json.RawMessage) (jsonrpc.ID, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(params, &fields) != nil || !validWireID(fields["requestId"]) {
		return jsonrpc.ID{}, false
	}
	if reason, present := fields["reason"]; present {
		if _, ok := stringValue(reason); !ok {
			return jsonrpc.ID{}, false
		}
	}
	var value any
	_ = json.Unmarshal(fields["requestId"], &value)
	id, err := jsonrpc.MakeID(value)
	return id, err == nil
}

// Preserve the application's closed tool-call contract and prevent incomplete
// modern metadata from falling back to a prior legacy handshake in the SDK.
func validateRequestBoundary(req *jsonrpc.Request) error {
	var fields map[string]json.RawMessage
	if len(req.Params) != 0 && (json.Unmarshal(req.Params, &fields) != nil || fields == nil) {
		return protocolError(-32602)
	}
	if req.Method == "tools/call" {
		for key := range fields {
			if key != "_meta" && key != "name" && key != "arguments" {
				return protocolError(-32602)
			}
		}
	}
	if req.Method == "server/discover" {
		for key := range fields {
			if key != "_meta" {
				return protocolError(-32602)
			}
		}
	}
	var meta map[string]json.RawMessage
	if raw, present := fields["_meta"]; present {
		if json.Unmarshal(raw, &meta) != nil || meta == nil {
			return protocolError(-32602)
		}
	}
	if info, present := meta[mcp.MetaKeyClientInfo]; present && !validImplementation(info) {
		return protocolError(-32602)
	}
	modern := req.Method == "server/discover"
	for key := range meta {
		if strings.EqualFold(key, mcp.MetaKeyProtocolVersion) || strings.EqualFold(key, mcp.MetaKeyClientCapabilities) {
			modern = true
		}
	}
	if !modern {
		if req.Method == "initialize" {
			if _, valid := stringValue(fields["protocolVersion"]); !valid || !object(fields["capabilities"]) || !validImplementation(fields["clientInfo"]) {
				return protocolError(-32602)
			}
		}
		return nil
	}
	version, ok := stringValue(meta[mcp.MetaKeyProtocolVersion])
	if !ok || !object(meta[mcp.MetaKeyClientCapabilities]) {
		return protocolError(-32602)
	}
	if version < CurrentVersion {
		data, _ := json.Marshal(mcp.UnsupportedProtocolVersionData{Supported: []string{CurrentVersion, LegacyVersion}, Requested: version})
		return &jsonrpc.Error{Code: mcp.CodeUnsupportedProtocolVersion, Message: staticMessage(mcp.CodeUnsupportedProtocolVersion), Data: data}
	}
	return nil
}

func requestHasModernMeta(req *jsonrpc.Request) bool {
	if req.Method == "server/discover" {
		return true
	}
	var fields, meta map[string]json.RawMessage
	_ = json.Unmarshal(req.Params, &fields)
	_ = json.Unmarshal(fields["_meta"], &meta)
	for key := range meta {
		if strings.EqualFold(key, mcp.MetaKeyProtocolVersion) || strings.EqualFold(key, mcp.MetaKeyClientCapabilities) {
			return true
		}
	}
	return false
}
