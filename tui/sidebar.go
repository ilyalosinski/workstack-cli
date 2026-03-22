package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ilyalosinski/workstack-cli/db"
)

var (
	// List styles — matching claude-squad approach
	listTitleStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1).
			Bold(true)

	sessionNameStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#86efac"})

	agentLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#655F5F", Dark: "#bcbcbc"})

	selectedTitleStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#dde4f0")).
				Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"}).
				Bold(true)

	selectedDescStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#dde4f0")).
				Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"})

	runningDot = lipgloss.NewStyle().Foreground(lipgloss.Color("#51bd73")).Render("●")
	idleDot    = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render("○")
	doneDot    = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Render("✓")
	stoppedDot = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444")).Render("⏹")
)

type sessionWithAgents struct {
	session db.Session
	agents  []db.Agent
}

// flatItem represents a row in the sidebar — either a session header or an agent.
type flatItem struct {
	isSession  bool
	sessionIdx int
	agentIdx   int // -1 for session rows
}

// Sidebar is the session/agent list component.
type Sidebar struct {
	items   []sessionWithAgents
	flat    []flatItem
	cursor  int
	width   int
	height  int
	focused bool
}

func NewSidebar() Sidebar {
	return Sidebar{focused: true}
}

func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *Sidebar) SetFocused(focused bool) {
	s.focused = focused
}

func (s *Sidebar) SetItems(items []sessionWithAgents) {
	s.items = items
	s.rebuildFlat()
	if s.cursor >= len(s.flat) && len(s.flat) > 0 {
		s.cursor = len(s.flat) - 1
	}
}

func (s *Sidebar) rebuildFlat() {
	s.flat = nil
	for si, item := range s.items {
		s.flat = append(s.flat, flatItem{isSession: true, sessionIdx: si, agentIdx: -1})
		for ai := range item.agents {
			s.flat = append(s.flat, flatItem{isSession: false, sessionIdx: si, agentIdx: ai})
		}
	}
}

func (s *Sidebar) MoveUp() {
	if s.cursor > 0 {
		s.cursor--
	}
}

func (s *Sidebar) MoveDown() {
	if s.cursor < len(s.flat)-1 {
		s.cursor++
	}
}

func (s *Sidebar) CurrentSession() *sessionWithAgents {
	if s.cursor >= len(s.flat) || len(s.flat) == 0 {
		return nil
	}
	fi := s.flat[s.cursor]
	return &s.items[fi.sessionIdx]
}

func (s *Sidebar) CurrentAgent() *db.Agent {
	if s.cursor >= len(s.flat) || len(s.flat) == 0 {
		return nil
	}
	fi := s.flat[s.cursor]
	if fi.isSession || fi.agentIdx < 0 {
		return nil
	}
	return &s.items[fi.sessionIdx].agents[fi.agentIdx]
}

func (s *Sidebar) IsOnSession() bool {
	if s.cursor >= len(s.flat) || len(s.flat) == 0 {
		return false
	}
	return s.flat[s.cursor].isSession
}

// SelectedAgent returns the agent to show in the terminal pane.
func (s *Sidebar) SelectedAgent() *db.Agent {
	agent := s.CurrentAgent()
	if agent != nil {
		return agent
	}
	sess := s.CurrentSession()
	if sess != nil && len(sess.agents) > 0 {
		return &sess.agents[0]
	}
	return nil
}

// View renders the sidebar content, truncated to fit s.height lines.
func (s *Sidebar) View() string {
	var lines []string

	// Title — always visible
	lines = append(lines, listTitleStyle.Render(" Sessions "))
	lines = append(lines, "")

	if len(s.flat) == 0 {
		lines = append(lines, agentLabelStyle.Render("  No sessions."))
		lines = append(lines, agentLabelStyle.Render("  Press 'n' to create."))
		return s.truncate(lines)
	}

	// Build all item lines
	var itemLines []string
	for fi, item := range s.flat {
		isSelected := fi == s.cursor && s.focused

		if item.isSession {
			sess := s.items[item.sessionIdx]

			running := 0
			for _, a := range sess.agents {
				if a.Status == "running" {
					running++
				}
			}

			var summary string
			if len(sess.agents) == 0 {
				summary = "empty"
			} else if running > 0 {
				summary = fmt.Sprintf("%d/%d", running, len(sess.agents))
			} else {
				summary = fmt.Sprintf("%d", len(sess.agents))
			}

			if isSelected {
				itemLines = append(itemLines, selectedTitleStyle.Render(fmt.Sprintf(" ▶ %s  %s ", sess.session.Name, summary)))
			} else {
				itemLines = append(itemLines, fmt.Sprintf("   %s  %s", sessionNameStyle.Render(sess.session.Name), agentLabelStyle.Render(summary)))
			}
		} else {
			agent := s.items[item.sessionIdx].agents[item.agentIdx]

			status := idleDot
			switch agent.Status {
			case "running":
				status = runningDot
			case "done":
				status = doneDot
			case "stopped":
				status = stoppedDot
			}

			typeLabel := string(agent.AgentType[0])
			line := fmt.Sprintf("├─ %s [%s] %s", agent.Repo, typeLabel, status)

			if isSelected {
				itemLines = append(itemLines, selectedDescStyle.Render(fmt.Sprintf("   %s ", line)))
			} else {
				itemLines = append(itemLines, fmt.Sprintf("      %s", agentLabelStyle.Render(line)))
			}
		}
	}

	// Window the item list so cursor is always visible
	headerLines := len(lines) // title + blank line
	availableRows := s.height - headerLines
	if availableRows < 1 {
		availableRows = 1
	}

	if len(itemLines) <= availableRows {
		// Everything fits
		lines = append(lines, itemLines...)
	} else {
		// Scroll window around cursor
		start := 0
		if s.cursor >= availableRows {
			start = s.cursor - availableRows + 1
		}
		end := start + availableRows
		if end > len(itemLines) {
			end = len(itemLines)
			start = end - availableRows
			if start < 0 {
				start = 0
			}
		}
		lines = append(lines, itemLines[start:end]...)
	}

	return s.truncate(lines)
}

// truncate ensures output is exactly s.height lines.
func (s *Sidebar) truncate(lines []string) string {
	if s.height <= 0 {
		return ""
	}
	// Trim to height
	if len(lines) > s.height {
		lines = lines[:s.height]
	}
	return strings.Join(lines, "\n")
}

func padRight(s string, width int) string {
	// Simple padding — doesn't account for ANSI but works for plain text
	visible := len([]rune(s))
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}
