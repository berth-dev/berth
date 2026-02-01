package session

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// Store provides SQLite-backed persistence for sessions.
type Store struct {
	db *sql.DB
}

// NewStore opens the SQLite database at dbPath and creates tables if they don't exist.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := createTables(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create tables: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		project TEXT NOT NULL,
		task TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS answers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		question_id TEXT NOT NULL,
		answer TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS beads_state (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		bead_id TEXT NOT NULL,
		status TEXT NOT NULL,
		output TEXT,
		tokens INTEGER DEFAULT 0,
		duration_ms INTEGER DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);
	`
	_, err := db.Exec(schema)
	return err
}

// CreateSession creates a new session with the given project and task.
func (s *Store) CreateSession(project, task string) (*Session, error) {
	id := uuid.New().String()
	now := time.Now()

	_, err := s.db.Exec(
		`INSERT INTO sessions (id, project, task, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'active', ?, ?)`,
		id, project, task, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}

	return &Session{
		ID:        id,
		Project:   project,
		Task:      task,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// GetSession retrieves a session by ID.
func (s *Store) GetSession(id string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, project, task, status, created_at, updated_at
		 FROM sessions WHERE id = ?`,
		id,
	)

	var sess Session
	err := row.Scan(&sess.ID, &sess.Project, &sess.Task, &sess.Status, &sess.CreatedAt, &sess.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}

	return &sess, nil
}

// UpdateSession updates an existing session.
func (s *Store) UpdateSession(session *Session) error {
	session.UpdatedAt = time.Now()

	_, err := s.db.Exec(
		`UPDATE sessions SET project = ?, task = ?, status = ?, updated_at = ?
		 WHERE id = ?`,
		session.Project, session.Task, session.Status, session.UpdatedAt, session.ID,
	)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	return nil
}

// GetLatestActive returns the most recently updated active session for the given project.
func (s *Store) GetLatestActive(project string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, project, task, status, created_at, updated_at
		 FROM sessions
		 WHERE project = ? AND status = 'active'
		 ORDER BY updated_at DESC
		 LIMIT 1`,
		project,
	)

	var sess Session
	err := row.Scan(&sess.ID, &sess.Project, &sess.Task, &sess.Status, &sess.CreatedAt, &sess.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}

	return &sess, nil
}

// ListSessions returns summaries of the most recent sessions.
func (s *Store) ListSessions(limit int) ([]Summary, error) {
	rows, err := s.db.Query(
		`SELECT s.id, s.task, s.status, s.updated_at,
		        COALESCE(SUM(CASE WHEN b.status = 'completed' THEN 1 ELSE 0 END), 0) as beads_completed,
		        COALESCE(COUNT(b.id), 0) as beads_total
		 FROM sessions s
		 LEFT JOIN beads_state b ON s.id = b.session_id
		 GROUP BY s.id
		 ORDER BY s.updated_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []Summary
	for rows.Next() {
		var sum Summary
		if err := rows.Scan(&sum.ID, &sum.Task, &sum.Status, &sum.UpdatedAt, &sum.BeadsCompleted, &sum.BeadsTotal); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		summaries = append(summaries, sum)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return summaries, nil
}

// AddMessage adds a chat message to the session.
func (s *Store) AddMessage(sessionID, role, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO messages (session_id, role, content, timestamp)
		 VALUES (?, ?, ?, ?)`,
		sessionID, role, content, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	return nil
}

// GetMessages retrieves all messages for a session.
func (s *Store) GetMessages(sessionID string) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, role, content, timestamp
		 FROM messages
		 WHERE session_id = ?
		 ORDER BY timestamp ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.Timestamp); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return messages, nil
}

// SaveAnswer saves an answer to an interview question.
func (s *Store) SaveAnswer(sessionID, questionID, answer string) error {
	_, err := s.db.Exec(
		`INSERT INTO answers (session_id, question_id, answer, timestamp)
		 VALUES (?, ?, ?, ?)`,
		sessionID, questionID, answer, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("insert answer: %w", err)
	}

	return nil
}

// GetAnswers retrieves all answers for a session.
func (s *Store) GetAnswers(sessionID string) ([]Answer, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, question_id, answer, timestamp
		 FROM answers
		 WHERE session_id = ?
		 ORDER BY timestamp ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query answers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var answers []Answer
	for rows.Next() {
		var ans Answer
		if err := rows.Scan(&ans.ID, &ans.SessionID, &ans.QuestionID, &ans.Answer, &ans.Timestamp); err != nil {
			return nil, fmt.Errorf("scan answer: %w", err)
		}
		answers = append(answers, ans)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return answers, nil
}

// UpdateBeadState updates or inserts the state of a bead.
func (s *Store) UpdateBeadState(sessionID, beadID, status string, tokens int, durationMs int64) error {
	now := time.Now()

	// Try to update existing record first
	result, err := s.db.Exec(
		`UPDATE beads_state
		 SET status = ?, tokens = ?, duration_ms = ?, updated_at = ?
		 WHERE session_id = ? AND bead_id = ?`,
		status, tokens, durationMs, now, sessionID, beadID,
	)
	if err != nil {
		return fmt.Errorf("update bead state: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	// If no rows were updated, insert a new record
	if rowsAffected == 0 {
		_, err = s.db.Exec(
			`INSERT INTO beads_state (session_id, bead_id, status, tokens, duration_ms, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			sessionID, beadID, status, tokens, durationMs, now,
		)
		if err != nil {
			return fmt.Errorf("insert bead state: %w", err)
		}
	}

	return nil
}

// GetBeadStates retrieves all bead states for a session.
func (s *Store) GetBeadStates(sessionID string) ([]BeadState, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, bead_id, status, COALESCE(output, ''), tokens, duration_ms, updated_at
		 FROM beads_state
		 WHERE session_id = ?
		 ORDER BY id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query bead states: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var states []BeadState
	for rows.Next() {
		var state BeadState
		if err := rows.Scan(&state.ID, &state.SessionID, &state.BeadID, &state.Status, &state.Output, &state.Tokens, &state.DurationMs, &state.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan bead state: %w", err)
		}
		states = append(states, state)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return states, nil
}
