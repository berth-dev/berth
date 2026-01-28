package coordinator

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Server is the coordinator HTTP server that agents use for file locking,
// decision broadcasting, intent announcements, and artifact publishing.
type Server struct {
	state    *State
	listener net.Listener
	server   *http.Server
	stopCh   chan struct{}
}

// NewServer creates a coordinator server bound to a random port on localhost.
func NewServer() (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("coordinator: binding listener: %w", err)
	}

	s := &Server{
		state:    NewState(),
		listener: ln,
		stopCh:   make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/acquire_lock", s.handleAcquireLock)
	mux.HandleFunc("/release_lock", s.handleReleaseLock)
	mux.HandleFunc("/check_lock", s.handleCheckLock)
	mux.HandleFunc("/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("/write_decision", s.handleWriteDecision)
	mux.HandleFunc("/read_decisions", s.handleReadDecisions)
	mux.HandleFunc("/announce_intent", s.handleAnnounceIntent)
	mux.HandleFunc("/publish_artifact", s.handlePublishArtifact)
	mux.HandleFunc("/query_artifacts", s.handleQueryArtifacts)
	mux.HandleFunc("/report_status", s.handleReportStatus)
	mux.HandleFunc("/get_all_status", s.handleGetAllStatus)

	s.server = &http.Server{Handler: mux}
	return s, nil
}

// Addr returns the address the server is listening on (e.g. "127.0.0.1:12345").
func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

// Start begins serving HTTP requests. Call in a goroutine.
func (s *Server) Start() error {
	return s.server.Serve(s.listener)
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	close(s.stopCh)
	return s.server.Close()
}

// StartLockReaper starts a goroutine that periodically removes stale locks
// (those with no heartbeat for longer than the given interval).
func (s *Server) StartLockReaper(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.reapStaleLocks(interval)
			}
		}
	}()
}

func (s *Server) reapStaleLocks(maxAge time.Duration) {
	now := time.Now()
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	for path, lock := range s.state.Locks {
		if now.Sub(lock.LastHeartbeat) > maxAge {
			delete(s.state.Locks, path)
		}
	}
}

// --- Handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleAcquireLock(w http.ResponseWriter, r *http.Request) {
	var req AcquireLockRequest
	if !readJSON(w, r, &req) {
		return
	}

	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	existing, held := s.state.Locks[req.FilePath]
	if held && existing.BeadID != req.BeadID {
		writeJSON(w, AcquireLockResponse{Acquired: false, BlockedBy: existing.BeadID})
		return
	}

	now := time.Now()
	s.state.Locks[req.FilePath] = &FileLock{
		BeadID:        req.BeadID,
		FilePath:      req.FilePath,
		AcquiredAt:    now,
		LastHeartbeat: now,
	}
	writeJSON(w, AcquireLockResponse{Acquired: true})
}

func (s *Server) handleReleaseLock(w http.ResponseWriter, r *http.Request) {
	var req ReleaseLockRequest
	if !readJSON(w, r, &req) {
		return
	}

	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	existing, held := s.state.Locks[req.FilePath]
	if held && existing.BeadID == req.BeadID {
		delete(s.state.Locks, req.FilePath)
		writeJSON(w, ReleaseLockResponse{Released: true})
		return
	}
	writeJSON(w, ReleaseLockResponse{Released: false})
}

func (s *Server) handleCheckLock(w http.ResponseWriter, r *http.Request) {
	var req CheckLockRequest
	if !readJSON(w, r, &req) {
		return
	}

	s.state.mu.RLock()
	defer s.state.mu.RUnlock()

	existing, held := s.state.Locks[req.FilePath]
	if held {
		writeJSON(w, CheckLockResponse{Locked: true, HeldBy: existing.BeadID})
		return
	}
	writeJSON(w, CheckLockResponse{Locked: false})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req HeartbeatRequest
	if !readJSON(w, r, &req) {
		return
	}

	now := time.Now()
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	s.state.Heartbeats[req.BeadID] = now

	// Refresh all locks held by this bead.
	for _, lock := range s.state.Locks {
		if lock.BeadID == req.BeadID {
			lock.LastHeartbeat = now
		}
	}

	writeJSON(w, HeartbeatResponse{OK: true})
}

func (s *Server) handleWriteDecision(w http.ResponseWriter, r *http.Request) {
	var req WriteDecisionRequest
	if !readJSON(w, r, &req) {
		return
	}

	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	s.state.Decisions = append(s.state.Decisions, Decision{
		BeadID:    req.BeadID,
		Key:       req.Key,
		Value:     req.Value,
		Rationale: req.Rationale,
		Tags:      req.Tags,
		Timestamp: time.Now(),
	})

	writeJSON(w, WriteDecisionResponse{OK: true})
}

func (s *Server) handleReadDecisions(w http.ResponseWriter, r *http.Request) {
	var req ReadDecisionsRequest
	if !readJSON(w, r, &req) {
		return
	}

	s.state.mu.RLock()
	defer s.state.mu.RUnlock()

	var result []Decision
	if req.Tag == "" {
		result = make([]Decision, len(s.state.Decisions))
		copy(result, s.state.Decisions)
	} else {
		for _, d := range s.state.Decisions {
			for _, t := range d.Tags {
				if t == req.Tag {
					result = append(result, d)
					break
				}
			}
		}
	}

	writeJSON(w, ReadDecisionsResponse{Decisions: result})
}

func (s *Server) handleAnnounceIntent(w http.ResponseWriter, r *http.Request) {
	var req AnnounceIntentRequest
	if !readJSON(w, r, &req) {
		return
	}

	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	// Check for file overlap with other beads' intents and locks.
	var conflicts []Conflict
	requestedFiles := make(map[string]bool, len(req.Files))
	for _, f := range req.Files {
		requestedFiles[f] = true
	}

	// Check against other intents.
	for otherBeadID, intent := range s.state.Intents {
		if otherBeadID == req.BeadID {
			continue
		}
		for _, f := range intent.Files {
			if requestedFiles[f] {
				conflicts = append(conflicts, Conflict{
					FilePath:    f,
					ClaimedBy:   otherBeadID,
					Description: fmt.Sprintf("bead %s also intends to modify %s", otherBeadID, f),
				})
			}
		}
	}

	// Check against existing locks.
	for _, f := range req.Files {
		if lock, held := s.state.Locks[f]; held && lock.BeadID != req.BeadID {
			conflicts = append(conflicts, Conflict{
				FilePath:    f,
				ClaimedBy:   lock.BeadID,
				Description: fmt.Sprintf("file %s is locked by bead %s", f, lock.BeadID),
			})
		}
	}

	// Record the intent.
	s.state.Intents[req.BeadID] = &Intent{
		BeadID:      req.BeadID,
		Action:      req.Action,
		Description: req.Description,
		Files:       req.Files,
		Timestamp:   time.Now(),
	}

	// Return related decisions.
	var relatedDecisions []Decision
	for _, d := range s.state.Decisions {
		relatedDecisions = append(relatedDecisions, d)
	}

	writeJSON(w, AnnounceIntentResponse{
		Conflicts: conflicts,
		Decisions: relatedDecisions,
	})
}

func (s *Server) handlePublishArtifact(w http.ResponseWriter, r *http.Request) {
	var req PublishArtifactRequest
	if !readJSON(w, r, &req) {
		return
	}

	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	s.state.Artifacts = append(s.state.Artifacts, Artifact{
		BeadID:   req.BeadID,
		Name:     req.Name,
		FilePath: req.FilePath,
		Exports:  req.Exports,
	})

	writeJSON(w, PublishArtifactResponse{OK: true})
}

func (s *Server) handleQueryArtifacts(w http.ResponseWriter, r *http.Request) {
	var req QueryArtifactsRequest
	if !readJSON(w, r, &req) {
		return
	}

	s.state.mu.RLock()
	defer s.state.mu.RUnlock()

	var result []Artifact
	if req.Name == "" {
		result = make([]Artifact, len(s.state.Artifacts))
		copy(result, s.state.Artifacts)
	} else {
		for _, a := range s.state.Artifacts {
			if a.Name == req.Name {
				result = append(result, a)
			}
		}
	}

	writeJSON(w, QueryArtifactsResponse{Artifacts: result})
}

func (s *Server) handleReportStatus(w http.ResponseWriter, r *http.Request) {
	var req ReportStatusRequest
	if !readJSON(w, r, &req) {
		return
	}

	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	s.state.Statuses[req.BeadID] = &BeadStatus{
		BeadID:  req.BeadID,
		Status:  req.Status,
		Summary: req.Summary,
	}

	writeJSON(w, ReportStatusResponse{OK: true})
}

func (s *Server) handleGetAllStatus(w http.ResponseWriter, _ *http.Request) {
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()

	// Copy statuses to avoid holding the lock during serialization.
	statuses := make(map[string]*BeadStatus, len(s.state.Statuses))
	for k, v := range s.state.Statuses {
		copied := *v
		statuses[k] = &copied
	}

	writeJSON(w, GetAllStatusResponse{Statuses: statuses})
}

// --- Helpers ---

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil {
		// Allow empty body for requests with no fields.
		return true
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, fmt.Sprintf("encoding response: %v", err), http.StatusInternalServerError)
	}
}
