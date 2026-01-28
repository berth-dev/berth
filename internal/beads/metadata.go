package beads

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// BeadMeta holds plan-level metadata that the bd CLI can't store.
type BeadMeta struct {
	Files       []string `json:"files"`
	VerifyExtra []string `json:"verify_extra"`
}

// WriteBeadMeta writes sidecar metadata for a bead into .berth/bead-meta/.
func WriteBeadMeta(projectRoot, beadID string, meta BeadMeta) error {
	dir := filepath.Join(projectRoot, ".berth", "bead-meta")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, beadID+".json"), data, 0644)
}

// ReadBeadMeta reads sidecar metadata for a bead from .berth/bead-meta/.
func ReadBeadMeta(projectRoot, beadID string) (*BeadMeta, error) {
	path := filepath.Join(projectRoot, ".berth", "bead-meta", beadID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta BeadMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}
