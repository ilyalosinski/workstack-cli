package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

type Session struct {
	ID        int
	Name      string
	Status    string
	CreatedAt time.Time
}

type Agent struct {
	ID          int
	SessionID   int
	Repo        string
	RepoPath    string
	Branch      string
	AgentType   string // "claude" or "codex"
	TmuxSession string
	WorktreePath string
	Status      string // "running", "idle", "done", "error"
	Prompt      string
	CreatedAt   time.Time
}

func dbPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "workstack")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "workstack.db")
}

func Open() (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath())
	if err != nil {
		return nil, err
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

func (d *DB) Close() {
	d.conn.Close()
}

func (d *DB) migrate() error {
	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS agents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			repo TEXT NOT NULL,
			repo_path TEXT NOT NULL,
			branch TEXT NOT NULL DEFAULT '',
			agent_type TEXT NOT NULL DEFAULT 'claude',
			tmux_session TEXT NOT NULL DEFAULT '',
			worktree_path TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'idle',
			prompt TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		);
	`)
	return err
}

func (d *DB) CreateSession(name string) (*Session, error) {
	res, err := d.conn.Exec("INSERT INTO sessions (name) VALUES (?)", name)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Session{ID: int(id), Name: name, Status: "active", CreatedAt: time.Now()}, nil
}

func (d *DB) ListSessions() ([]Session, error) {
	rows, err := d.conn.Query("SELECT id, name, status, created_at FROM sessions ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		var s Session
		rows.Scan(&s.ID, &s.Name, &s.Status, &s.CreatedAt)
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (d *DB) GetSession(name string) (*Session, error) {
	var s Session
	err := d.conn.QueryRow("SELECT id, name, status, created_at FROM sessions WHERE name = ?", name).
		Scan(&s.ID, &s.Name, &s.Status, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *DB) DeleteSession(name string) error {
	_, err := d.conn.Exec("DELETE FROM sessions WHERE name = ?", name)
	return err
}

func (d *DB) AddAgent(sessionID int, repo, repoPath, branch, agentType, prompt string) (*Agent, error) {
	res, err := d.conn.Exec(
		"INSERT INTO agents (session_id, repo, repo_path, branch, agent_type, prompt) VALUES (?, ?, ?, ?, ?, ?)",
		sessionID, repo, repoPath, branch, agentType, prompt,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Agent{
		ID: int(id), SessionID: sessionID, Repo: repo, RepoPath: repoPath,
		Branch: branch, AgentType: agentType, Prompt: prompt, Status: "idle",
		CreatedAt: time.Now(),
	}, nil
}

func (d *DB) GetAgents(sessionID int) ([]Agent, error) {
	rows, err := d.conn.Query(
		"SELECT id, session_id, repo, repo_path, branch, agent_type, tmux_session, worktree_path, status, prompt, created_at FROM agents WHERE session_id = ? ORDER BY created_at",
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []Agent
	for rows.Next() {
		var a Agent
		rows.Scan(&a.ID, &a.SessionID, &a.Repo, &a.RepoPath, &a.Branch, &a.AgentType, &a.TmuxSession, &a.WorktreePath, &a.Status, &a.Prompt, &a.CreatedAt)
		agents = append(agents, a)
	}
	return agents, nil
}

func (d *DB) UpdateAgentStatus(id int, status, tmuxSession, worktreePath, branch string) error {
	_, err := d.conn.Exec(
		"UPDATE agents SET status = ?, tmux_session = ?, worktree_path = ?, branch = ? WHERE id = ?",
		status, tmuxSession, worktreePath, branch, id,
	)
	return err
}

func (d *DB) DeleteAgent(id int) error {
	_, err := d.conn.Exec("DELETE FROM agents WHERE id = ?", id)
	return err
}
