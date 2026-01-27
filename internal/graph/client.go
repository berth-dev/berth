// Package graph manages the Knowledge Graph MCP server integration.
// This file provides the client for querying the MCP server with timeouts.
package graph

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// CallerResult represents a single caller of a function.
type CallerResult struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Name string `json:"name"` // Calling function name
}

// CalleeResult represents a function called by another function.
type CalleeResult struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Name string `json:"name"`
}

// DependentResult represents a module/symbol dependent on another.
type DependentResult struct {
	File string `json:"file"`
	Name string `json:"name"`
	Kind string `json:"kind"` // "import" | "call" | "type_usage" | "extends"
}

// ExportResult represents an exported symbol from a file.
type ExportResult struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "function" | "type" | "const" | "class" | "interface" | "enum"
	Line int    `json:"line"`
}

// ImporterResult represents a file that imports from another.
type ImporterResult struct {
	File          string   `json:"file"`
	ImportedNames []string `json:"imported_names"`
}

// TypeUsageResult represents where a type is used.
type TypeUsageResult struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Kind string `json:"kind"` // "type_usage" | "extends"
}

// FileUnderstanding is the result of understand_file.
type FileUnderstanding struct {
	File      string           `json:"file"`
	Exports   []ExportResult   `json:"exports"`
	Importers []ImporterResult `json:"importers"`
}

// ImpactAnalysis is the result of analyze_impact.
type ImpactAnalysis struct {
	AffectedFiles   []string `json:"affected_files"`
	AffectedSymbols []string `json:"affected_symbols"`
}

// mcpRequest is a JSON-RPC 2.0 request.
type mcpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// mcpResponse is a JSON-RPC 2.0 response.
type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

// mcpError represents a JSON-RPC error object.
type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolCallParams holds the parameters for an MCP tools/call request.
type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// Client communicates with the Knowledge Graph MCP server.
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Scanner
	mu      sync.Mutex
	nextID  int
	timeout time.Duration
}

// NewClient creates a new Client by attaching to the command's stdin/stdout
// pipes and starting the process.
func NewClient(cmd *exec.Cmd, timeout time.Duration) (*Client, error) {
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("graph: creating stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdinPipe.Close()
		return nil, fmt.Errorf("graph: creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdinPipe.Close()
		return nil, fmt.Errorf("graph: starting MCP process: %w", err)
	}

	scanner := bufio.NewScanner(stdoutPipe)
	// Allow up to 10 MB per line for large JSON responses.
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	return &Client{
		cmd:     cmd,
		stdin:   stdinPipe,
		stdout:  scanner,
		nextID:  1,
		timeout: timeout,
	}, nil
}

// Close shuts down the MCP process gracefully.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_ = c.stdin.Close()

	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}

	return c.cmd.Wait()
}

// callTool sends a JSON-RPC tools/call request and unmarshals the response
// into result. It is thread-safe via c.mu.
func (c *Client) callTool(name string, args map[string]any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextID
	c.nextID++

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/call",
		Params: toolCallParams{
			Name:      name,
			Arguments: args,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("graph: marshalling request: %w", err)
	}

	// Write the request followed by a newline (line-delimited JSON-RPC).
	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("graph: writing request: %w", err)
	}

	// Read response with timeout.
	type scanResult struct {
		line []byte
		ok   bool
	}
	ch := make(chan scanResult, 1)
	go func() {
		ok := c.stdout.Scan()
		ch <- scanResult{line: c.stdout.Bytes(), ok: ok}
	}()

	select {
	case sr := <-ch:
		if !sr.ok {
			if err := c.stdout.Err(); err != nil {
				return fmt.Errorf("graph: reading response: %w", err)
			}
			return fmt.Errorf("graph: MCP process closed stdout")
		}

		var resp mcpResponse
		if err := json.Unmarshal(sr.line, &resp); err != nil {
			return fmt.Errorf("graph: unmarshalling response: %w", err)
		}

		if resp.Error != nil {
			return fmt.Errorf("graph: MCP error %d: %s", resp.Error.Code, resp.Error.Message)
		}

		if result != nil && resp.Result != nil {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("graph: unmarshalling result: %w", err)
			}
		}

		return nil

	case <-time.After(c.timeout):
		return fmt.Errorf("graph: tool call %q timed out after %s", name, c.timeout)
	}
}

// QueryCallers returns all callers of the named function.
func (c *Client) QueryCallers(name string) ([]CallerResult, error) {
	var results []CallerResult
	err := c.callTool("query_callers", map[string]any{"name": name}, &results)
	return results, err
}

// QueryCallees returns all functions called by the named function.
func (c *Client) QueryCallees(name string) ([]CalleeResult, error) {
	var results []CalleeResult
	err := c.callTool("query_callees", map[string]any{"name": name}, &results)
	return results, err
}

// QueryDependents returns all symbols dependent on the named symbol.
func (c *Client) QueryDependents(name string) ([]DependentResult, error) {
	var results []DependentResult
	err := c.callTool("query_dependents", map[string]any{"name": name}, &results)
	return results, err
}

// QueryExports returns all exported symbols from the specified file.
func (c *Client) QueryExports(file string) ([]ExportResult, error) {
	var results []ExportResult
	err := c.callTool("query_exports", map[string]any{"file": file}, &results)
	return results, err
}

// QueryImporters returns all files that import from the specified file.
func (c *Client) QueryImporters(file string) ([]ImporterResult, error) {
	var results []ImporterResult
	err := c.callTool("query_importers", map[string]any{"file": file}, &results)
	return results, err
}

// QueryTypeUsages returns all locations where the named type is used.
func (c *Client) QueryTypeUsages(typeName string) ([]TypeUsageResult, error) {
	var results []TypeUsageResult
	err := c.callTool("query_type_usages", map[string]any{"type_name": typeName}, &results)
	return results, err
}

// UnderstandFile returns a comprehensive understanding of the specified file
// including exports and importers.
func (c *Client) UnderstandFile(file string) (*FileUnderstanding, error) {
	var result FileUnderstanding
	err := c.callTool("understand_file", map[string]any{"file": file}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// AnalyzeImpact returns the impact analysis for changes to the specified files.
func (c *Client) AnalyzeImpact(files []string) (*ImpactAnalysis, error) {
	var result ImpactAnalysis
	err := c.callTool("analyze_impact", map[string]any{"files": files}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ReindexFiles triggers reindexing of the specified files in the KG.
func (c *Client) ReindexFiles(files []string) error {
	return c.callTool("reindex_files", map[string]any{"files": files}, nil)
}

// FullReindex triggers a full reindex of the entire project.
func (c *Client) FullReindex() error {
	return c.callTool("full_reindex", nil, nil)
}
