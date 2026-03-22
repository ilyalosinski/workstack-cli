package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ilyalosinski/workstack-cli/db"
	"github.com/ilyalosinski/workstack-cli/session"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	sessionStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	agentStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	statusRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render("●")
	statusIdle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("○")
	statusDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Render("✓")
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

type viewState int

const (
	viewList viewState = iota
	viewNewSession
	viewAddAgent
)

type sessionWithAgents struct {
	session db.Session
	agents  []db.Agent
}

// flatItem represents a row in the TUI - either a session header or an agent
type flatItem struct {
	isSession bool
	sessionIdx int
	agentIdx   int // -1 for session rows
}

type model struct {
	mgr       *session.Manager
	items     []sessionWithAgents
	flat      []flatItem
	cursor    int
	state     viewState
	input     textinput.Model
	inputStep int
	newName   string
	newRepo   string
	newAgent  string
	width     int
	height    int
	err       string
}

func NewModel(mgr *session.Manager) model {
	ti := textinput.New()
	ti.Focus()
	return model{
		mgr: mgr,
		input: ti,
	}
}

func (m *model) rebuildFlat() {
	m.flat = nil
	for si, item := range m.items {
		m.flat = append(m.flat, flatItem{isSession: true, sessionIdx: si, agentIdx: -1})
		for ai := range item.agents {
			m.flat = append(m.flat, flatItem{isSession: false, sessionIdx: si, agentIdx: ai})
		}
	}
}

func (m model) Init() tea.Cmd {
	return m.refresh()
}

type refreshMsg []sessionWithAgents
type attachDoneMsg struct{}

func (m model) refresh() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.mgr.ListSessions()
		if err != nil {
			return refreshMsg{}
		}
		var items []sessionWithAgents
		for _, s := range sessions {
			agents, _ := m.mgr.GetAgents(s.ID)
			for i, a := range agents {
				agents[i].Status = m.mgr.CheckAgentStatus(a)
			}
			items = append(items, sessionWithAgents{session: s, agents: agents})
		}
		return refreshMsg(items)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		m.items = msg
		m.rebuildFlat()
		if m.cursor >= len(m.flat) && len(m.flat) > 0 {
			m.cursor = len(m.flat) - 1
		}
		return m, nil

	case attachDoneMsg:
		return m, m.refresh()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.state == viewNewSession {
			return m.updateNewSession(msg)
		}
		if m.state == viewAddAgent {
			return m.updateAddAgent(msg)
		}
		return m.updateList(msg)
	}

	return m, nil
}

func (m *model) currentSession() *sessionWithAgents {
	if m.cursor >= len(m.flat) {
		return nil
	}
	fi := m.flat[m.cursor]
	return &m.items[fi.sessionIdx]
}

func (m *model) currentAgent() *db.Agent {
	if m.cursor >= len(m.flat) {
		return nil
	}
	fi := m.flat[m.cursor]
	if fi.isSession || fi.agentIdx < 0 {
		return nil
	}
	return &m.items[fi.sessionIdx].agents[fi.agentIdx]
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case tea.KeyDown:
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}
		return m, nil
	case tea.KeyEnter:
		agent := m.currentAgent()
		if agent != nil && agent.TmuxSession != "" {
			return m, m.attachToAgent(*agent)
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "j":
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}
	case "n":
		m.state = viewNewSession
		m.inputStep = 0
		m.input.SetValue("")
		m.input.Placeholder = "Session name (e.g. referral-system)"
		m.err = ""
		return m, textinput.Blink
	case "a":
		sess := m.currentSession()
		if sess != nil {
			m.state = viewAddAgent
			m.inputStep = 0
			m.input.SetValue("")
			m.input.Placeholder = "Repo name (e.g. starsfortasks-backend)"
			m.newName = sess.session.Name
			m.err = ""
			return m, textinput.Blink
		}
	case "s":
		// Start a single agent
		agent := m.currentAgent()
		if agent != nil && agent.Status == "idle" {
			m.mgr.StartAgent(*agent)
			return m, m.refresh()
		}
		// If on session header, start all idle agents in session
		if m.cursor < len(m.flat) && m.flat[m.cursor].isSession {
			sess := m.currentSession()
			if sess != nil {
				for _, a := range sess.agents {
					if a.Status == "idle" {
						m.mgr.StartAgent(a)
					}
				}
				return m, m.refresh()
			}
		}
	case "S":
		// Stop agent or all agents in session
		agent := m.currentAgent()
		if agent != nil && agent.Status == "running" {
			m.mgr.StopAgent(*agent)
			return m, m.refresh()
		}
		if m.cursor < len(m.flat) && m.flat[m.cursor].isSession {
			sess := m.currentSession()
			if sess != nil {
				for _, a := range sess.agents {
					m.mgr.StopAgent(a)
				}
				return m, m.refresh()
			}
		}
	case "D":
		sess := m.currentSession()
		if sess != nil {
			m.mgr.DeleteSession(sess.session.Name)
			if m.cursor > 0 {
				m.cursor--
			}
			return m, m.refresh()
		}
	case "R":
		return m, m.refresh()
	}
	return m, nil
}

func (m model) attachToAgent(agent db.Agent) tea.Cmd {
	return tea.ExecProcess(
		exec.Command("tmux", "attach-session", "-t", agent.TmuxSession),
		func(err error) tea.Msg {
			return attachDoneMsg{}
		},
	)
}

func (m model) updateNewSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.state = viewList
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if val == "" {
			return m, nil
		}
		_, err := m.mgr.CreateSession(val)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.state = viewList
		return m, m.refresh()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateAddAgent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.state = viewList
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if val == "" {
			return m, nil
		}
		switch m.inputStep {
		case 0: // repo
			m.newRepo = val
			m.inputStep = 1
			m.input.SetValue("")
			m.input.Placeholder = "Agent: claude or codex"
			return m, nil
		case 1: // agent type
			if val != "claude" && val != "codex" {
				m.err = "must be 'claude' or 'codex'"
				return m, nil
			}
			m.newAgent = val
			// Done - add agent but don't start it
			_, err := m.mgr.AddAgent(m.newName, m.newRepo, m.newAgent, "")
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
			m.state = viewList
			return m, m.refresh()
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.state == viewNewSession {
		return m.viewInput("New Session", "Enter session name:")
	}
	if m.state == viewAddAgent {
		labels := []string{"Enter repo name:", "Select agent (claude/codex):"}
		label := labels[m.inputStep]
		return m.viewInput(fmt.Sprintf("Add Agent to %s", m.newName), label)
	}
	return m.viewList()
}

func (m model) viewInput(title, label string) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(label + "\n")
	b.WriteString(m.input.View())
	if m.err != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.err))
	}
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("enter: confirm  esc: cancel"))
	return b.String()
}

func (m model) viewList() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(titleStyle.Render(" workstack-cli"))
	b.WriteString("\n\n")

	if len(m.flat) == 0 {
		b.WriteString(agentStyle.Render("  No sessions. Press 'n' to create one.\n"))
	}

	for fi, item := range m.flat {
		isSelected := fi == m.cursor

		if item.isSession {
			sess := m.items[item.sessionIdx]
			prefix := "  "
			name := sessionStyle.Render(sess.session.Name)
			if isSelected {
				prefix = "▶ "
				name = selectedStyle.Render(sess.session.Name)
			}

			running := 0
			total := len(sess.agents)
			for _, a := range sess.agents {
				if a.Status == "running" {
					running++
				}
			}

			summary := fmt.Sprintf("%d agents", total)
			if running > 0 {
				summary = fmt.Sprintf("%d/%d running", running, total)
			}
			if total == 0 {
				summary = "no agents"
			}

			b.WriteString(fmt.Sprintf("%s%s  %s\n", prefix, name, agentStyle.Render(summary)))
		} else {
			agent := m.items[item.sessionIdx].agents[item.agentIdx]
			status := statusIdle
			switch agent.Status {
			case "running":
				status = statusRunning
			case "done":
				status = statusDone
			}

			prefix := "    "
			agentLabel := fmt.Sprintf("[%s]", agent.AgentType)
			line := fmt.Sprintf("├─ %s %s %s", agent.Repo, agentLabel, status)

			if isSelected {
				line = selectedStyle.Render(line)
			} else {
				line = agentStyle.Render(line)
			}

			b.WriteString(fmt.Sprintf("%s%s\n", prefix, line))
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("n:new  a:add agent  s:start  S:stop  enter:attach  D:delete  R:refresh  q:quit"))
	b.WriteString("\n")

	return b.String()
}

func Run(mgr *session.Manager) {
	p := tea.NewProgram(NewModel(mgr), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
