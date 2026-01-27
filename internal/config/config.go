// Package config handles reading and writing .berth/config.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the top-level structure for .berth/config.yaml.
type Config struct {
	Version        int             `yaml:"version"`
	Project        ProjectConfig   `yaml:"project"`
	Model          string          `yaml:"model"`
	Execution      ExecutionConfig `yaml:"execution"`
	VerifyPipeline []string        `yaml:"verify_pipeline"`
	KnowledgeGraph KGConfig        `yaml:"knowledge_graph"`
	Beads          BeadsConfig     `yaml:"beads"`
}

// ProjectConfig holds project metadata detected or supplied during init.
type ProjectConfig struct {
	Name           string `yaml:"name"`
	Language       string `yaml:"language"`
	Framework      string `yaml:"framework"`
	PackageManager string `yaml:"package_manager"`
}

// ExecutionConfig controls bead execution behaviour.
type ExecutionConfig struct {
	MaxRetries     int    `yaml:"max_retries"`
	TimeoutPerBead int    `yaml:"timeout_per_bead"` // seconds
	BranchPrefix   string `yaml:"branch_prefix"`
	AutoCommit     bool   `yaml:"auto_commit"`
	AutoPR         bool   `yaml:"auto_pr"`
}

// KGConfig controls the Knowledge Graph MCP server integration.
type KGConfig struct {
	Enabled         string `yaml:"enabled"`           // "auto" | "always" | "never"
	MCPCommand      string `yaml:"mcp_command"`
	MCPTimeout      int    `yaml:"mcp_timeout"`       // ms
	ToolCallTimeout int    `yaml:"tool_call_timeout"` // ms
	MCPDebug        bool   `yaml:"mcp_debug"`
}

// BeadsConfig holds configuration for the beads subsystem.
type BeadsConfig struct {
	Prefix string `yaml:"prefix"` // e.g. "bt"
}

// configFileName is the path relative to the project root.
const configDir = ".berth"
const configFile = "config.yaml"

// ReadConfig reads .berth/config.yaml from the given project directory.
// dir is the project root (not .berth/ itself).
// Returns an error if the file is not found or YAML is malformed.
func ReadConfig(dir string) (*Config, error) {
	path := filepath.Join(dir, configDir, configFile)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}

// WriteConfig writes cfg to .berth/config.yaml in the given project directory.
// Creates the .berth/ directory if it does not exist.
func WriteConfig(dir string, cfg *Config) error {
	dirPath := filepath.Join(dir, configDir)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	path := filepath.Join(dirPath, configFile)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Version: 1,
		Model:   "opus",
		Execution: ExecutionConfig{
			MaxRetries:     3,
			TimeoutPerBead: 600,
			BranchPrefix:   "berth/",
			AutoCommit:     true,
			AutoPR:         false,
		},
		KnowledgeGraph: KGConfig{
			Enabled:         "auto",
			MCPTimeout:      15000,
			ToolCallTimeout: 10000,
			MCPDebug:        false,
		},
		Beads: BeadsConfig{
			Prefix: "bt",
		},
	}
}
