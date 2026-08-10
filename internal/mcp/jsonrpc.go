// Package mcp implements a minimal Model Context Protocol (MCP) server that
// exposes kprompt as a read/plan-only tool provider for IDE assistants
// (Cursor, Claude Desktop, …). It never applies a mutation: mutating prompts
// return a typed PlanResult and stop. See ADR-0024 (MCP interop surface).
package mcp

import (
	"encoding/json"
)

// jsonRPCVersion is the only supported JSON-RPC version.
const jsonRPCVersion = "2.0"

// request is an incoming JSON-RPC 2.0 request or notification. A notification
// has a nil ID and expects no response.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// isNotification reports whether the message is a notification (no ID → no reply).
func (r request) isNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

// response is an outgoing JSON-RPC 2.0 response.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is a JSON-RPC 2.0 error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

func resultResponse(id json.RawMessage, result any) response {
	return response{JSONRPC: jsonRPCVersion, ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, message string) response {
	return response{JSONRPC: jsonRPCVersion, ID: id, Error: &rpcError{Code: code, Message: message}}
}
