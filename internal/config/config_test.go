package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigYAMLRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config with new fields
	cfg := DefaultConfig()
	cfg.Execution.CircuitBreakerThreshold = 5
	cfg.Verify.Security = "gosec ./..."

	// Write to disk
	if err := WriteConfig(tmpDir, cfg); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	// Read back
	loaded, err := ReadConfig(tmpDir)
	if err != nil {
		t.Fatalf("ReadConfig failed: %v", err)
	}

	// Verify new fields
	if loaded.Execution.CircuitBreakerThreshold != 5 {
		t.Errorf("CircuitBreakerThreshold: got %d, want 5", loaded.Execution.CircuitBreakerThreshold)
	}
	if loaded.Verify.Security != "gosec ./..." {
		t.Errorf("Verify.Security: got %q, want %q", loaded.Verify.Security, "gosec ./...")
	}
}

func TestDefaultConfigHasCircuitBreakerThreshold(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Execution.CircuitBreakerThreshold != 3 {
		t.Errorf("default CircuitBreakerThreshold: got %d, want 3", cfg.Execution.CircuitBreakerThreshold)
	}
}

func TestDefaultConfigHasSecurityFieldEmpty(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Verify.Security != "" {
		t.Errorf("default Verify.Security: got %q, want empty string", cfg.Verify.Security)
	}
}

func TestBackwardCompatibility(t *testing.T) {
	// Simulate an old config file without new fields
	tmpDir := t.TempDir()
	oldConfig := `version: 1
model: opus
execution:
  max_retries: 3
  timeout_per_bead: 600
  branch_prefix: "berth/"
  auto_commit: true
  auto_pr: false
  parallel_mode: auto
  max_parallel: 5
  parallel_threshold: 4
  merge_strategy: merge
knowledge_graph:
  enabled: auto
  mcp_timeout: 15000
  tool_call_timeout: 10000
  mcp_debug: false
beads:
  prefix: bt
cleanup:
  max_age_days: 30
`
	configPath := filepath.Join(tmpDir, ".berth")
	if err := os.MkdirAll(configPath, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configPath, "config.yaml"), []byte(oldConfig), 0644); err != nil {
		t.Fatalf("failed to write old config: %v", err)
	}

	// Read old config - should not error
	cfg, err := ReadConfig(tmpDir)
	if err != nil {
		t.Fatalf("ReadConfig failed on old config: %v", err)
	}

	// New fields should have zero values (defaults not applied on read, which is fine)
	// The important thing is that it doesn't crash
	if cfg == nil {
		t.Error("config should not be nil")
	}
}
