package tui

import (
	"crypto/sha256"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var (
	terminalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})

	terminalPlaceholderStyle = lipgloss.NewStyle().
					Foreground(lipgloss.AdaptiveColor{Light: "#808080", Dark: "#808080"})
)

// TerminalPane displays captured tmux pane output with ANSI colors preserved.
type TerminalPane struct {
	content  string
	prevHash [32]byte
	viewport viewport.Model
	width    int
	height   int
	ready    bool
}

func NewTerminalPane() TerminalPane {
	return TerminalPane{}
}

func (t *TerminalPane) SetSize(width, height int) {
	t.width = width
	t.height = height
	if !t.ready {
		t.viewport = viewport.New(width, height)
		t.ready = true
	} else {
		t.viewport.Width = width
		t.viewport.Height = height
	}
}

// SetContent updates the terminal content only if it changed (SHA256 comparison).
func (t *TerminalPane) SetContent(content string) {
	hash := sha256.Sum256([]byte(content))
	if hash == t.prevHash {
		return
	}
	t.prevHash = hash
	t.content = content

	// Trim trailing blank lines
	lines := strings.Split(content, "\n")
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	if last >= 0 {
		lines = lines[:last+1]
	}

	t.viewport.SetContent(strings.Join(lines, "\n"))
	t.viewport.GotoBottom()
}

// View renders the terminal pane content (without border — parent handles border).
func (t *TerminalPane) View() string {
	if !t.ready {
		return ""
	}
	if t.content == "" {
		return terminalPlaceholderStyle.Render("Select an agent to view output")
	}
	return t.viewport.View()
}
