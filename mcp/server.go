// Package mcp connects the SDK's read APIs to a local MCP stdio server.
// A caller supplies one explicit session; tool arguments cannot select files,
// inject credentials, authenticate, or enable financial mutations.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"sync"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/mcptools"
	"github.com/vasyza/sber-go/internal/mcpwire"
)

// Client is owned by Server until Close. SDK clients satisfy this interface.
type Client interface {
	sber.BusinessRequester
	Close() error
}
type Options struct {
	Client        Client
	ProfileExists bool
}
type Server struct {
	client        Client
	resources     *sber.Resources
	wire          *mcpwire.Server
	profileExists bool
	mu            sync.Mutex
	closed        bool
}

var ErrConfiguration = errors.New("sber: MCP client required")
var ErrArguments = errors.New("sber: invalid MCP arguments")

func New(o Options) (*Server, error) {
	if o.Client == nil || (reflect.ValueOf(o.Client).Kind() == reflect.Pointer && reflect.ValueOf(o.Client).IsNil()) {
		return nil, ErrConfiguration
	}
	server := &Server{client: o.Client, resources: sber.NewResources(o.Client), profileExists: o.ProfileExists}
	supported := map[string]bool{"sber_setup_status": true, "sber_session_info": true, "sber_session_close": true, "sber_products": true, "sber_operations": true, "sber_operations_page": true}
	descriptions := map[string]string{
		"sber_setup_status":    "Inspect the explicitly selected local session without contacting the bank.",
		"sber_session_info":    "Return redacted session metadata; check_live explicitly checks authorization with a warm-up request.",
		"sber_session_close":   "Close the selected session and disable further reads in this server.",
		"sber_products":        "Read account and card snapshots with exact decimal amounts and masked display text.",
		"sber_operations":      "Read paginated operations with explicit coverage metadata; pagination exhaustion does not prove complete bank history.",
		"sber_operations_page": "Read one operations page with its next offset.",
	}
	var tools []mcpwire.Tool
	for _, definition := range mcptools.Catalog(false) {
		if !supported[definition.Name] {
			continue
		}
		name := definition.Name
		tools = append(tools, mcpwire.Tool{Name: name, Description: descriptions[name], InputSchema: definition.InputSchema, ReadOnly: name != "sber_session_close", LocalControl: name == "sber_session_close",
			Validate: func(ctx context.Context, raw json.RawMessage) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := mcptools.ValidateArguments(name, raw, false); err != nil {
					return err
				}
				_, err := server.arguments(raw)
				return err
			},
			Handle: func(ctx context.Context, raw json.RawMessage) (mcpwire.ToolResult, error) {
				return server.call(ctx, name, raw)
			},
		})
	}
	wire, err := mcpwire.New(mcpwire.Options{Info: mcpwire.Implementation{Name: "sber-go", Version: "dev"}, Tools: tools})
	if err != nil {
		return nil, err
	}
	server.wire = wire
	return server, nil
}

// Serve transfers stream ownership to the wire engine. Use Close to release
// the session after Serve returns, including cancellation and output failure.
func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	if ctx == nil || input == nil || output == nil {
		return ErrArguments
	}
	return s.wire.Serve(ctx, input, output)
}
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return s.client.Close()
}

func (s *Server) arguments(raw json.RawMessage) (map[string]any, error) {
	args, err := sber.DecodeJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrArguments
	}
	if value, exists := args["session_id"]; exists && value != nil {
		if selected, ok := value.(string); !ok || selected != "current" {
			return nil, ErrArguments
		}
	}
	if value, exists := args["profile"]; exists && value != nil {
		if selected, ok := value.(string); !ok || selected != "default" {
			return nil, ErrArguments
		}
	}
	return args, nil
}
func text(args map[string]any, name string) string  { v, _ := args[name].(string); return v }
func boolean(args map[string]any, name string) bool { v, _ := args[name].(bool); return v }
func integer(args map[string]any, name string, fallback, min, max int) (int, error) {
	value, present := args[name]
	if !present {
		return fallback, nil
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, ErrArguments
	}
	f, err := number.Float64()
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) || f < float64(min) || f > float64(max) || math.Trunc(f) != f {
		return 0, ErrArguments
	}
	return int(f), nil
}
func result(value any) (mcpwire.ToolResult, error) {
	raw, err := sber.ExportJSON(value)
	if err != nil {
		return mcpwire.ToolResult{}, err
	}
	return mcpwire.ToolResult{Content: []mcpwire.TextContent{{Text: string(raw)}}, StructuredContent: raw}, nil
}
func (s *Server) call(ctx context.Context, name string, raw json.RawMessage) (mcpwire.ToolResult, error) {
	args, err := s.arguments(raw)
	if err != nil {
		return mcpwire.ToolResult{}, err
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed && name != "sber_session_close" && name != "sber_setup_status" {
		return mcpwire.ToolResult{}, sber.ErrClosed
	}
	switch name {
	case "sber_setup_status":
		return result(map[string]any{"profile_exists": s.profileExists, "session_available": !closed, "bank_authorization_checked": false, "mutations_enabled": false})
	case "sber_session_close":
		if err := s.Close(); err != nil {
			return mcpwire.ToolResult{}, err
		}
		return result(map[string]any{"closed": true})
	case "sber_session_info":
		checked := boolean(args, "check_live")
		if checked {
			if err := s.client.WarmUp(ctx, true); err != nil {
				return mcpwire.ToolResult{}, err
			}
		}
		bundle, err := s.client.ExportSession()
		if err != nil {
			return mcpwire.ToolResult{}, err
		}
		return result(map[string]any{"metadata": bundle.Redacted(), "bank_authorization_checked": checked})
	case "sber_products":
		products, err := s.resources.Products.Get(ctx, boolean(args, "force_update"))
		if err != nil {
			return mcpwire.ToolResult{}, err
		}
		return result(products)
	case "sber_operations", "sber_operations_page":
		limit, err := integer(args, "limit", 30, 1, 100)
		if err != nil {
			return mcpwire.ToolResult{}, err
		}
		if name == "sber_operations_page" {
			offset, err := integer(args, "offset", 0, 0, 1000000000)
			if err != nil {
				return mcpwire.ToolResult{}, err
			}
			page, err := s.resources.Operations.Page(ctx, sber.OperationsPageOptions{Resource: text(args, "resource"), From: text(args, "from_date"), To: text(args, "to_date"), Offset: offset, Limit: limit})
			if err != nil {
				return mcpwire.ToolResult{}, err
			}
			return result(page)
		}
		pages, err := integer(args, "max_pages", 3, 1, 10000)
		if err != nil {
			return mcpwire.ToolResult{}, err
		}
		collection, err := s.resources.Operations.Collect(ctx, sber.OperationsQuery{Resource: text(args, "resource"), From: text(args, "from_date"), To: text(args, "to_date"), Limit: limit, MaxPages: pages})
		if err != nil {
			return mcpwire.ToolResult{}, err
		}
		return result(map[string]any{"operations": collection.Operations, "metadata": collection.Metadata})
	default:
		return mcpwire.ToolResult{}, ErrArguments
	}
}
