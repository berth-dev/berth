package coordinator

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// jsonRPCRequest is a JSON-RPC 2.0 request from Claude's MCP layer.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response written back to stdout.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolCallParams holds the params for a tools/call request.
type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// toolListResult is the response for tools/list.
type toolListResult struct {
	Tools []toolDef `json:"tools"`
}

type toolDef struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	InputSchema toolDefInputSchema  `json:"inputSchema"`
}

type toolDefInputSchema struct {
	Type       string                     `json:"type"`
	Properties map[string]toolDefProperty `json:"properties"`
	Required   []string                   `json:"required"`
}

type toolDefProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Items       *toolDefProperty `json:"items,omitempty"`
}

// mcpContentBlock is a single content block in the MCP response.
type mcpContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// toolNameToEndpoint maps MCP tool names to coordinator HTTP endpoints.
var toolNameToEndpoint = map[string]string{
	"acquire_lock":     "/acquire_lock",
	"release_lock":     "/release_lock",
	"check_lock":       "/check_lock",
	"heartbeat":        "/heartbeat",
	"write_decision":   "/write_decision",
	"read_decisions":   "/read_decisions",
	"announce_intent":  "/announce_intent",
	"publish_artifact": "/publish_artifact",
	"query_artifacts":  "/query_artifacts",
	"report_status":    "/report_status",
	"get_all_status":   "/get_all_status",
}

// coordinatorTools defines the MCP tool list returned by tools/list.
var coordinatorTools = []toolDef{
	{
		Name:        "acquire_lock",
		Description: "Acquire an exclusive lock on a file before editing it",
		InputSchema: toolDefInputSchema{
			Type: "object",
			Properties: map[string]toolDefProperty{
				"bead_id":   {Type: "string", Description: "Your bead ID"},
				"file_path": {Type: "string", Description: "Path to the file to lock"},
			},
			Required: []string{"bead_id", "file_path"},
		},
	},
	{
		Name:        "release_lock",
		Description: "Release a file lock after finishing edits",
		InputSchema: toolDefInputSchema{
			Type: "object",
			Properties: map[string]toolDefProperty{
				"bead_id":   {Type: "string", Description: "Your bead ID"},
				"file_path": {Type: "string", Description: "Path to the file to unlock"},
			},
			Required: []string{"bead_id", "file_path"},
		},
	},
	{
		Name:        "check_lock",
		Description: "Check if a file is currently locked by another agent",
		InputSchema: toolDefInputSchema{
			Type: "object",
			Properties: map[string]toolDefProperty{
				"file_path": {Type: "string", Description: "Path to the file to check"},
			},
			Required: []string{"file_path"},
		},
	},
	{
		Name:        "heartbeat",
		Description: "Send heartbeat to keep locks alive (call periodically)",
		InputSchema: toolDefInputSchema{
			Type: "object",
			Properties: map[string]toolDefProperty{
				"bead_id": {Type: "string", Description: "Your bead ID"},
			},
			Required: []string{"bead_id"},
		},
	},
	{
		Name:        "write_decision",
		Description: "Record an architectural or structural decision for other agents",
		InputSchema: toolDefInputSchema{
			Type: "object",
			Properties: map[string]toolDefProperty{
				"bead_id":   {Type: "string", Description: "Your bead ID"},
				"key":       {Type: "string", Description: "Decision key/name"},
				"value":     {Type: "string", Description: "Decision value"},
				"rationale": {Type: "string", Description: "Why this decision was made"},
				"tags":      {Type: "array", Description: "Optional tags for filtering", Items: &toolDefProperty{Type: "string"}},
			},
			Required: []string{"bead_id", "key", "value", "rationale"},
		},
	},
	{
		Name:        "read_decisions",
		Description: "Read decisions made by all agents, optionally filtered by tag",
		InputSchema: toolDefInputSchema{
			Type: "object",
			Properties: map[string]toolDefProperty{
				"tag": {Type: "string", Description: "Optional tag to filter by"},
			},
		},
	},
	{
		Name:        "announce_intent",
		Description: "Announce your intent to modify files (call at start of work)",
		InputSchema: toolDefInputSchema{
			Type: "object",
			Properties: map[string]toolDefProperty{
				"bead_id":     {Type: "string", Description: "Your bead ID"},
				"action":      {Type: "string", Description: "What you plan to do"},
				"description": {Type: "string", Description: "Detailed description"},
				"files":       {Type: "array", Description: "Files you plan to modify", Items: &toolDefProperty{Type: "string"}},
			},
			Required: []string{"bead_id", "action", "description", "files"},
		},
	},
	{
		Name:        "publish_artifact",
		Description: "Publish a new artifact (e.g. new export) for other agents",
		InputSchema: toolDefInputSchema{
			Type: "object",
			Properties: map[string]toolDefProperty{
				"bead_id":   {Type: "string", Description: "Your bead ID"},
				"name":      {Type: "string", Description: "Artifact name"},
				"file_path": {Type: "string", Description: "File containing the artifact"},
				"exports":   {Type: "array", Description: "Exported symbols", Items: &toolDefProperty{Type: "string"}},
			},
			Required: []string{"bead_id", "name", "file_path"},
		},
	},
	{
		Name:        "query_artifacts",
		Description: "Query published artifacts from other agents",
		InputSchema: toolDefInputSchema{
			Type: "object",
			Properties: map[string]toolDefProperty{
				"name": {Type: "string", Description: "Optional artifact name filter"},
			},
		},
	},
	{
		Name:        "report_status",
		Description: "Report your bead's current status",
		InputSchema: toolDefInputSchema{
			Type: "object",
			Properties: map[string]toolDefProperty{
				"bead_id": {Type: "string", Description: "Your bead ID"},
				"status":  {Type: "string", Description: "Current status"},
				"summary": {Type: "string", Description: "Optional summary"},
			},
			Required: []string{"bead_id", "status"},
		},
	},
	{
		Name:        "get_all_status",
		Description: "Get the status of all beads",
		InputSchema: toolDefInputSchema{
			Type: "object",
			Properties: map[string]toolDefProperty{},
		},
	},
}

// RunBridge runs the MCP stdio bridge, reading JSON-RPC from stdin and
// forwarding tool calls to the coordinator HTTP server.
func RunBridge(addr, beadID string) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeResponse(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonRPCError{Code: -32700, Message: fmt.Sprintf("parse error: %v", err)},
			})
			continue
		}

		switch req.Method {
		case "initialize":
			result, _ := json.Marshal(map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "coordinator",
					"version": "1.0.0",
				},
			})
			writeResponse(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result})

		case "notifications/initialized":
			// No response needed for notifications.

		case "tools/list":
			result, _ := json.Marshal(toolListResult{Tools: coordinatorTools})
			writeResponse(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result})

		case "tools/call":
			var params toolCallParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeResponse(jsonRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &jsonRPCError{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)},
				})
				continue
			}

			endpoint, ok := toolNameToEndpoint[params.Name]
			if !ok {
				writeResponse(jsonRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &jsonRPCError{Code: -32601, Message: fmt.Sprintf("unknown tool: %s", params.Name)},
				})
				continue
			}

			// Auto-inject bead_id into the request body so agents don't
			// have to provide it manually for every tool call.
			body := injectBeadID(params.Arguments, beadID)
			if len(body) == 0 {
				body = []byte("{}")
			}

			url := fmt.Sprintf("http://%s%s", addr, endpoint)
			resp, err := http.Post(url, "application/json", bytes.NewReader(body))
			if err != nil {
				errResult := marshalMCPContent(fmt.Sprintf("coordinator request failed: %v", err), true)
				writeResponse(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: errResult})
				continue
			}

			respBody, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				errResult := marshalMCPContent(fmt.Sprintf("reading coordinator response: %v", err), true)
				writeResponse(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: errResult})
				continue
			}

			mcpResult := marshalMCPContent(string(respBody), false)
			writeResponse(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: mcpResult})

		default:
			writeResponse(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonRPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
			})
		}
	}

	return scanner.Err()
}

func marshalMCPContent(text string, isError bool) json.RawMessage {
	result := map[string]any{
		"content": []mcpContentBlock{{Type: "text", Text: text}},
	}
	if isError {
		result["isError"] = true
	}
	data, _ := json.Marshal(result)
	return data
}

func writeResponse(resp jsonRPCResponse) {
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	_, _ = os.Stdout.Write(data)
}

// injectBeadID adds bead_id to the JSON arguments if it isn't already present
// and the bridge was started with a non-empty beadID. This allows agents to
// omit bead_id from every tool call since the bridge knows which bead it serves.
func injectBeadID(args json.RawMessage, beadID string) json.RawMessage {
	if beadID == "" || len(args) == 0 {
		return args
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(args, &m); err != nil {
		return args
	}

	if _, exists := m["bead_id"]; !exists {
		quoted, _ := json.Marshal(beadID)
		m["bead_id"] = quoted
		data, _ := json.Marshal(m)
		return data
	}

	return args
}
