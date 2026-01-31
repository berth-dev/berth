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
	"sync/atomic"
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

// CallerEntry represents a caller in the understand_file response.
type CallerEntry struct {
	Caller string `json:"caller"`
	File   string `json:"file"`
	Line   int    `json:"line"`
}

// TypeUsageEntry represents a type usage in the understand_file response.
type TypeUsageEntry struct {
	File      string `json:"file"`
	UsageKind string `json:"usage_kind"`
	Line      int    `json:"line"`
}

// FileUnderstanding is the result of understand_file.
type FileUnderstanding struct {
	File      string                       `json:"file"`
	Exports   []ExportResult               `json:"exports"`
	Importers []ImporterResult             `json:"importers"`
	Callers   map[string][]CallerEntry     `json:"callers"`
	Types     map[string][]TypeUsageEntry  `json:"types"`
}

// TransitiveDependent represents a transitive dependency in the impact analysis.
type TransitiveDependent struct {
	File string `json:"file"`
	Via  string `json:"via"`
}

// ImpactAnalysis is the result of analyze_impact.
type ImpactAnalysis struct {
	DirectDependents     []DependentResult     `json:"direct_dependents"`
	TransitiveDependents []TransitiveDependent  `json:"transitive_dependents"`
	AffectedTests        []string               `json:"affected_tests"`
}

// ArchitectureNode represents a file in the architecture diagram.
type ArchitectureNode struct {
	File    string
	Exports []string
	Imports []string
	Depth   int
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

// mcpContent represents a single content block in an MCP tool result.
type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// mcpToolResult represents the MCP CallToolResult envelope.
type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
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
	mu      sync.RWMutex
	nextID  atomic.Int64
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

	client := &Client{
		cmd:     cmd,
		stdin:   stdinPipe,
		stdout:  scanner,
		timeout: timeout,
	}
	client.nextID.Store(1)
	return client, nil
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

// callToolRead sends a JSON-RPC tools/call request for read-only operations
// and unmarshals the response into result. Uses RLock to allow concurrent reads.
func (c *Client) callToolRead(name string, args map[string]any, result any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.callToolLocked(name, args, result)
}

// callToolWrite sends a JSON-RPC tools/call request for write operations
// and unmarshals the response into result. Uses exclusive Lock.
func (c *Client) callToolWrite(name string, args map[string]any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.callToolLocked(name, args, result)
}

// callToolLocked performs the actual JSON-RPC call. Caller must hold the lock.
func (c *Client) callToolLocked(name string, args map[string]any, result any) error {
	id := int(c.nextID.Add(1))

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
			var envelope mcpToolResult
			if err := json.Unmarshal(resp.Result, &envelope); err != nil {
				return fmt.Errorf("graph: unmarshalling MCP envelope: %w", err)
			}
			if envelope.IsError {
				text := ""
				if len(envelope.Content) > 0 {
					text = envelope.Content[0].Text
				}
				return fmt.Errorf("graph: MCP tool error: %s", text)
			}
			if len(envelope.Content) == 0 || envelope.Content[0].Type != "text" {
				return fmt.Errorf("graph: unexpected MCP response: no text content")
			}
			if err := json.Unmarshal([]byte(envelope.Content[0].Text), result); err != nil {
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
	err := c.callToolRead("get_callers", map[string]any{"symbol_name": name}, &results)
	return results, err
}

// QueryCallees returns all functions called by the named function.
func (c *Client) QueryCallees(name string) ([]CalleeResult, error) {
	var results []CalleeResult
	err := c.callToolRead("get_callees", map[string]any{"symbol_name": name}, &results)
	return results, err
}

// QueryDependents returns all files dependent on the specified file.
func (c *Client) QueryDependents(filePath string) ([]DependentResult, error) {
	var results []DependentResult
	err := c.callToolRead("get_dependents", map[string]any{"file_path": filePath}, &results)
	return results, err
}

// QueryExports returns all exported symbols from the specified file.
func (c *Client) QueryExports(file string) ([]ExportResult, error) {
	var results []ExportResult
	err := c.callToolRead("get_exports", map[string]any{"file_path": file}, &results)
	return results, err
}

// QueryImporters returns all files that import from the specified file.
func (c *Client) QueryImporters(file string) ([]ImporterResult, error) {
	var results []ImporterResult
	err := c.callToolRead("get_importers", map[string]any{"file_path": file}, &results)
	return results, err
}

// QueryTypeUsages returns all locations where the named type is used.
func (c *Client) QueryTypeUsages(typeName string) ([]TypeUsageResult, error) {
	var results []TypeUsageResult
	err := c.callToolRead("get_type_usages", map[string]any{"type_name": typeName}, &results)
	return results, err
}

// UnderstandFile returns a comprehensive understanding of the specified file
// including exports and importers.
func (c *Client) UnderstandFile(file string) (*FileUnderstanding, error) {
	var result FileUnderstanding
	err := c.callToolRead("understand_file", map[string]any{"file_path": file}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// AnalyzeImpact returns the impact analysis for changes to the specified file.
func (c *Client) AnalyzeImpact(filePath string) (*ImpactAnalysis, error) {
	var result ImpactAnalysis
	err := c.callToolRead("analyze_impact", map[string]any{"file_path": filePath}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ReindexFiles triggers reindexing of the specified files in the KG.
func (c *Client) ReindexFiles(files []string) error {
	return c.callToolWrite("reindex_files", map[string]any{"file_paths": files}, nil)
}

// FullReindex triggers a full reindex of the entire project.
func (c *Client) FullReindex() error {
	return c.callToolWrite("reindex", nil, nil)
}

// GetArchitectureDiagram builds a layered dependency view from a root file.
func (c *Client) GetArchitectureDiagram(rootFile string, maxDepth int) (map[string]ArchitectureNode, error) {
	nodes := make(map[string]ArchitectureNode)

	// BFS from root file
	queue := []string{rootFile}
	depths := map[string]int{rootFile: 0}

	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]
		depth := depths[file]

		if depth > maxDepth {
			continue
		}

		// Get exports
		exports, err := c.QueryExports(file)
		if err != nil {
			continue // skip on error
		}

		// Get importers
		importers, err := c.QueryImporters(file)
		if err != nil {
			continue // skip on error
		}

		// Build export names list
		exportNames := make([]string, len(exports))
		for i, exp := range exports {
			exportNames[i] = exp.Name
		}

		// Build importer files list
		importerFiles := make([]string, len(importers))
		for i, imp := range importers {
			importerFiles[i] = imp.File
		}

		nodes[file] = ArchitectureNode{
			File:    file,
			Exports: exportNames,
			Imports: importerFiles,
			Depth:   depth,
		}

		// Add importers to queue
		for _, imp := range importers {
			if _, seen := depths[imp.File]; !seen {
				depths[imp.File] = depth + 1
				queue = append(queue, imp.File)
			}
		}
	}

	return nodes, nil
}
