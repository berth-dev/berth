// Package coordinator provides a lightweight HTTP server for inter-agent
// coordination during parallel bead execution. Agents communicate via
// file locks, decision broadcasting, intent announcements, and artifact
// publishing.
package coordinator

import (
	"sync"
	"time"
)

// FileLock represents an exclusive lock on a file held by a bead's agent.
type FileLock struct {
	BeadID        string    `json:"bead_id"`
	FilePath      string    `json:"file_path"`
	AcquiredAt    time.Time `json:"acquired_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

// Decision records an architectural or structural decision made by an agent.
type Decision struct {
	BeadID    string    `json:"bead_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Rationale string    `json:"rationale"`
	Tags      []string  `json:"tags,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Intent declares what an agent plans to do, including which files it
// intends to modify. Other agents can check for overlapping intents.
type Intent struct {
	BeadID      string    `json:"bead_id"`
	Action      string    `json:"action"`
	Description string    `json:"description"`
	Files       []string  `json:"files"`
	Timestamp   time.Time `json:"timestamp"`
}

// Artifact represents a published output from an agent (e.g. a new export
// that other agents may need to import).
type Artifact struct {
	BeadID   string   `json:"bead_id"`
	Name     string   `json:"name"`
	FilePath string   `json:"file_path"`
	Exports  []string `json:"exports,omitempty"`
}

// BeadStatus tracks the current status of a bead during parallel execution.
type BeadStatus struct {
	BeadID  string `json:"bead_id"`
	Status  string `json:"status"`
	Summary string `json:"summary,omitempty"`
}

// Conflict describes a detected overlap between agents' intentions or locks.
type Conflict struct {
	FilePath    string `json:"file_path"`
	ClaimedBy   string `json:"claimed_by"`
	Description string `json:"description"`
}

// State is the in-memory coordination state shared by all agents.
// All access is protected by mu.
type State struct {
	mu         sync.RWMutex
	Locks      map[string]*FileLock   // filepath -> lock
	Decisions  []Decision
	Intents    map[string]*Intent     // beadID -> intent
	Artifacts  []Artifact
	Statuses   map[string]*BeadStatus // beadID -> status
	Heartbeats map[string]time.Time   // beadID -> last heartbeat
}

// NewState creates an empty coordination state.
func NewState() *State {
	return &State{
		Locks:      make(map[string]*FileLock),
		Intents:    make(map[string]*Intent),
		Statuses:   make(map[string]*BeadStatus),
		Heartbeats: make(map[string]time.Time),
	}
}

// --- Request types ---

// AcquireLockRequest is sent by an agent to acquire a file lock.
type AcquireLockRequest struct {
	BeadID   string `json:"bead_id"`
	FilePath string `json:"file_path"`
}

// AcquireLockResponse is the server response to an acquire lock request.
type AcquireLockResponse struct {
	Acquired  bool   `json:"acquired"`
	BlockedBy string `json:"blocked_by,omitempty"`
}

// ReleaseLockRequest is sent by an agent to release a file lock.
type ReleaseLockRequest struct {
	BeadID   string `json:"bead_id"`
	FilePath string `json:"file_path"`
}

// ReleaseLockResponse is the server response to a release lock request.
type ReleaseLockResponse struct {
	Released bool `json:"released"`
}

// CheckLockRequest checks whether a file is currently locked.
type CheckLockRequest struct {
	FilePath string `json:"file_path"`
}

// CheckLockResponse returns the lock status for a file.
type CheckLockResponse struct {
	Locked bool   `json:"locked"`
	HeldBy string `json:"held_by,omitempty"`
}

// HeartbeatRequest is sent periodically by agents to keep locks alive.
type HeartbeatRequest struct {
	BeadID string `json:"bead_id"`
}

// HeartbeatResponse acknowledges a heartbeat.
type HeartbeatResponse struct {
	OK bool `json:"ok"`
}

// WriteDecisionRequest records a decision.
type WriteDecisionRequest struct {
	BeadID    string   `json:"bead_id"`
	Key       string   `json:"key"`
	Value     string   `json:"value"`
	Rationale string   `json:"rationale"`
	Tags      []string `json:"tags,omitempty"`
}

// WriteDecisionResponse acknowledges a decision write.
type WriteDecisionResponse struct {
	OK bool `json:"ok"`
}

// ReadDecisionsRequest reads all or filtered decisions.
type ReadDecisionsRequest struct {
	Tag string `json:"tag,omitempty"`
}

// ReadDecisionsResponse returns matching decisions.
type ReadDecisionsResponse struct {
	Decisions []Decision `json:"decisions"`
}

// AnnounceIntentRequest declares an agent's intended actions.
type AnnounceIntentRequest struct {
	BeadID      string   `json:"bead_id"`
	Action      string   `json:"action"`
	Description string   `json:"description"`
	Files       []string `json:"files"`
}

// AnnounceIntentResponse returns conflicts and related decisions.
type AnnounceIntentResponse struct {
	Conflicts []Conflict `json:"conflicts,omitempty"`
	Decisions []Decision `json:"decisions,omitempty"`
}

// PublishArtifactRequest publishes an artifact from an agent.
type PublishArtifactRequest struct {
	BeadID   string   `json:"bead_id"`
	Name     string   `json:"name"`
	FilePath string   `json:"file_path"`
	Exports  []string `json:"exports,omitempty"`
}

// PublishArtifactResponse acknowledges an artifact publish.
type PublishArtifactResponse struct {
	OK bool `json:"ok"`
}

// QueryArtifactsRequest queries published artifacts.
type QueryArtifactsRequest struct {
	Name string `json:"name,omitempty"`
}

// QueryArtifactsResponse returns matching artifacts.
type QueryArtifactsResponse struct {
	Artifacts []Artifact `json:"artifacts"`
}

// ReportStatusRequest reports a bead's current status.
type ReportStatusRequest struct {
	BeadID  string `json:"bead_id"`
	Status  string `json:"status"`
	Summary string `json:"summary,omitempty"`
}

// ReportStatusResponse acknowledges a status report.
type ReportStatusResponse struct {
	OK bool `json:"ok"`
}

// GetAllStatusRequest requests all bead statuses.
type GetAllStatusRequest struct{}

// GetAllStatusResponse returns all bead statuses.
type GetAllStatusResponse struct {
	Statuses map[string]*BeadStatus `json:"statuses"`
}
