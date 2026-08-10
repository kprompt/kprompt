package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/kprompt/kprompt/internal/pipeline"
)

// serverName / serverVersion identify this MCP server to clients.
const serverName = "kprompt"

// defaultProtocolVersion is used when the client does not advertise one.
const defaultProtocolVersion = "2025-06-18"

// maxLineBytes bounds a single newline-delimited JSON-RPC message (10 MiB).
const maxLineBytes = 10 << 20

// Server is a stdio MCP server exposing kprompt's read/plan-only tools.
type Server struct {
	in      io.Reader
	out     io.Writer
	version string

	tools   []toolDef
	byName  map[string]toolDef
	writeMu sync.Mutex

	// baseDeps seeds every pipeline run. Confirm and IsTerminal are always
	// overridden to guarantee no mutation is applied over MCP; tests may inject
	// a stub Provider / fake Client here.
	baseDeps pipeline.Deps
}

// toolDef is a registered MCP tool.
type toolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(ctx context.Context, args map[string]any) (string, error)
}

// NewServer builds a Server reading JSON-RPC from in and writing to out.
// version is stamped into the initialize serverInfo.
func NewServer(in io.Reader, out io.Writer, version string) *Server {
	return newServerWithDeps(in, out, version, pipeline.Deps{})
}

// newServerWithDeps is NewServer with injectable pipeline dependencies (tests).
func newServerWithDeps(in io.Reader, out io.Writer, version string, deps pipeline.Deps) *Server {
	s := &Server{in: in, out: out, version: version, byName: map[string]toolDef{}, baseDeps: deps}
	s.registerTools()
	return s
}

func (s *Server) register(t toolDef) {
	s.tools = append(s.tools, t)
	s.byName[t.Name] = t
}

// Serve reads newline-delimited JSON-RPC messages until EOF or ctx cancellation.
// Each message is dispatched synchronously; responses are written as single-line JSON.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Bytes()
		if len(trimSpace(line)) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.write(errorResponse(nil, codeParseError, "parse error: "+err.Error()))
			continue
		}
		if req.JSONRPC != jsonRPCVersion {
			if !req.isNotification() {
				s.write(errorResponse(req.ID, codeInvalidRequest, "unsupported jsonrpc version"))
			}
			continue
		}
		s.dispatch(ctx, req)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mcp: read: %w", err)
	}
	return nil
}

func (s *Server) dispatch(ctx context.Context, req request) {
	switch req.Method {
	case "initialize":
		s.write(resultResponse(req.ID, s.handleInitialize(req.Params)))
	case "notifications/initialized", "initialized":
		// Notification: no response.
	case "ping":
		s.write(resultResponse(req.ID, map[string]any{}))
	case "tools/list":
		s.write(resultResponse(req.ID, s.handleToolsList()))
	case "tools/call":
		resp := s.handleToolsCall(ctx, req.ID, req.Params)
		s.write(resp)
	default:
		if req.isNotification() {
			return
		}
		s.write(errorResponse(req.ID, codeMethodNotFound, "method not found: "+req.Method))
	}
}

func (s *Server) handleInitialize(params json.RawMessage) map[string]any {
	protocol := defaultProtocolVersion
	if len(params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(params, &p); err == nil && p.ProtocolVersion != "" {
			protocol = p.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": protocol,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    serverName,
			"version": s.version,
		},
		"instructions": "kprompt as an MCP tool provider: plan-gated Kubernetes ops. Tools are read-only and never apply a mutation.",
	}
}

func (s *Server) handleToolsList() map[string]any {
	list := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		list = append(list, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		})
	}
	return map[string]any{"tools": list}
}

func (s *Server) handleToolsCall(ctx context.Context, id json.RawMessage, params json.RawMessage) response {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return errorResponse(id, codeInvalidParams, "invalid params: "+err.Error())
	}
	tool, ok := s.byName[p.Name]
	if !ok {
		return errorResponse(id, codeMethodNotFound, "unknown tool: "+p.Name)
	}
	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}
	text, err := tool.Handler(ctx, p.Arguments)
	if err != nil {
		// Tool-level failures are reported inside result with isError, per MCP,
		// so the assistant can read the message rather than aborting the call.
		return resultResponse(id, toolResult(err.Error(), true))
	}
	return resultResponse(id, toolResult(text, false))
}

// toolResult builds an MCP tools/call result payload.
func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
		"isError": isError,
	}
}

func (s *Server) write(resp response) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = s.out.Write(b)
}

func trimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r' || b[start] == '\n') {
		start++
	}
	end := len(b)
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r' || b[end-1] == '\n') {
		end--
	}
	return b[start:end]
}
