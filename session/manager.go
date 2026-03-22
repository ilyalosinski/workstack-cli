package session

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ilyalosinski/workstack-cli/db"
)

type Manager struct {
	db      *db.DB
	baseDir string
}

func NewManager(database *db.DB, baseDir string) *Manager {
	return &Manager{db: database, baseDir: baseDir}
}

func (m *Manager) CreateSession(name string) (*db.Session, error) {
	return m.db.CreateSession(name)
}

func (m *Manager) ListSessions() ([]db.Session, error) {
	return m.db.ListSessions()
}

func (m *Manager) GetSession(name string) (*db.Session, error) {
	return m.db.GetSession(name)
}

func (m *Manager) GetAgents(sessionID int) ([]db.Agent, error) {
	return m.db.GetAgents(sessionID)
}

func (m *Manager) DeleteSession(name string) error {
	sess, err := m.db.GetSession(name)
	if err != nil {
		return fmt.Errorf("session not found: %s", name)
	}
	agents, _ := m.db.GetAgents(sess.ID)
	for _, a := range agents {
		m.StopAgent(a)
	}
	return m.db.DeleteSession(name)
}

func (m *Manager) resolveRepoPath(repo string) string {
	if filepath.IsAbs(repo) {
		return repo
	}
	candidates := []string{
		filepath.Join(m.baseDir, repo),
		filepath.Join(m.baseDir, "boringrobots", "giftomania", repo),
		filepath.Join(m.baseDir, "boringrobots", repo),
	}
	for _, c := range candidates {
		out, err := exec.Command("git", "-C", c, "rev-parse", "--git-dir").Output()
		if err == nil && len(out) > 0 {
			return c
		}
	}
	return filepath.Join(m.baseDir, repo)
}

func (m *Manager) AddAgent(sessionName string, repo, agentType, prompt string) (*db.Agent, error) {
	sess, err := m.db.GetSession(sessionName)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionName)
	}

	repoPath := m.resolveRepoPath(repo)
	repoBase := filepath.Base(repoPath)
	branch := fmt.Sprintf("ws/%s/%s", sessionName, repoBase)

	agent, err := m.db.AddAgent(sess.ID, repoBase, repoPath, branch, agentType, prompt)
	if err != nil {
		return nil, err
	}

	return agent, nil
}

func (m *Manager) StartAgent(agent db.Agent) error {
	// Create git worktree
	worktreePath := filepath.Join(agent.RepoPath, ".worktrees", agent.Branch)
	exec.Command("git", "-C", agent.RepoPath, "worktree", "add", worktreePath, "-b", agent.Branch).Run()

	// Create tmux session
	tmuxName := fmt.Sprintf("ws_%s_%s_%d", strings.ReplaceAll(agent.Repo, "-", ""), agent.AgentType, agent.ID)

	cmd := exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", worktreePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Send agent command
	var agentCmd string
	switch agent.AgentType {
	case "claude":
		agentCmd = fmt.Sprintf("claude --dangerously-skip-permissions \"%s\"", agent.Prompt)
	case "codex":
		agentCmd = fmt.Sprintf("codex --full-auto \"%s\"", agent.Prompt)
	default:
		agentCmd = fmt.Sprintf("claude --dangerously-skip-permissions \"%s\"", agent.Prompt)
	}

	exec.Command("tmux", "send-keys", "-t", tmuxName, agentCmd, "Enter").Run()

	return m.db.UpdateAgentStatus(agent.ID, "running", tmuxName, worktreePath, agent.Branch)
}

func (m *Manager) StopAgent(agent db.Agent) error {
	if agent.TmuxSession != "" {
		exec.Command("tmux", "kill-session", "-t", agent.TmuxSession).Run()
	}
	if agent.WorktreePath != "" {
		exec.Command("git", "-C", agent.RepoPath, "worktree", "remove", agent.WorktreePath, "--force").Run()
	}
	return m.db.UpdateAgentStatus(agent.ID, "stopped", agent.TmuxSession, agent.WorktreePath, agent.Branch)
}

func (m *Manager) AttachAgent(agent db.Agent) error {
	if agent.TmuxSession == "" {
		return fmt.Errorf("agent has no tmux session")
	}
	cmd := exec.Command("tmux", "attach-session", "-t", agent.TmuxSession)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func (m *Manager) CheckAgentStatus(agent db.Agent) string {
	if agent.TmuxSession == "" {
		return "idle"
	}
	err := exec.Command("tmux", "has-session", "-t", agent.TmuxSession).Run()
	if err != nil {
		return "done"
	}
	return "running"
}

// CaptureOutput captures the current terminal output of an agent's tmux pane
func (m *Manager) CaptureOutput(agent db.Agent) (string, error) {
	if agent.TmuxSession == "" {
		return "", nil
	}
	out, err := exec.Command("tmux", "capture-pane", "-p", "-e", "-J",
		"-t", agent.TmuxSession).Output()
	return string(out), err
}
