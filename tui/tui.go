package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ilyalosinski/workstack-cli/session"
)

// Styles — following claude-squad patterns
var (
	highlightColor = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}

	// Menu/help bar
	menuKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#655F5F", Dark: "#7F7A7A"})
	menuActionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99"))

	// Input overlay
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2)
	inputTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true).
			MarginBottom(1)
	inputErrStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000"))
)

type layoutMode int

const (
	layoutSplit    layoutMode = iota // wide: sidebar + terminal
	layoutTerminal                   // narrow: terminal only
	layoutList                       // narrow: list only
)

type focusPane int

const (
	focusSidebar focusPane = iota
	focusTerminal
)

type viewState int

const (
	viewMain viewState = iota
	viewNewSession
	viewAddAgent
)

type terminalTickMsg struct{}
type metadataTickMsg struct{}
type attachDoneMsg struct{}

type model struct {
	mgr       *session.Manager
	sidebar   Sidebar
	terminal  TerminalPane
	layout    layoutMode
	focus     focusPane
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
		mgr:      mgr,
		sidebar:  NewSidebar(),
		terminal: NewTerminalPane(),
		layout:   layoutSplit,
		focus:    focusSidebar,
		input:    ti,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), terminalTick(), metadataTick())
}

func terminalTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return terminalTickMsg{}
	})
}

func metadataTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return metadataTickMsg{}
	})
}

type refreshMsg []sessionWithAgents

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
		m.sidebar.SetItems(msg)
		return m, nil

	case terminalTickMsg:
		m.updateTerminalContent()
		return m, terminalTick()

	case metadataTickMsg:
		return m, tea.Batch(m.refresh(), metadataTick())

	case attachDoneMsg:
		return m, m.refresh()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil

	case tea.KeyMsg:
		if m.state == viewNewSession {
			return m.updateNewSession(msg)
		}
		if m.state == viewAddAgent {
			return m.updateAddAgent(msg)
		}
		return m.updateMain(msg)
	}

	return m, nil
}

func (m *model) recalcLayout() {
	menuHeight := 1
	borderSize := 2 // top + bottom border lines

	if m.width < 80 {
		// Narrow / mobile
		if m.layout == layoutSplit {
			m.layout = layoutTerminal
		}
		// Inner content area = total - menu - borders
		innerH := m.height - menuHeight - borderSize
		innerW := m.width - borderSize
		m.sidebar.SetSize(innerW, innerH)
		m.terminal.SetSize(innerW, innerH)
	} else {
		// Wide — 30/70 split like claude-squad
		m.layout = layoutSplit
		listOuter := int(float32(m.width) * 0.3)
		if listOuter < 24 {
			listOuter = 24
		}
		if listOuter > 40 {
			listOuter = 40
		}
		termOuter := m.width - listOuter
		innerH := m.height - menuHeight - borderSize
		// Each panel has left+right border = 2 chars
		m.sidebar.SetSize(listOuter-borderSize, innerH)
		m.terminal.SetSize(termOuter-borderSize, innerH)
	}
	m.sidebar.SetFocused(m.focus == focusSidebar)
}

func (m *model) updateTerminalContent() {
	agent := m.sidebar.SelectedAgent()
	if agent == nil {
		m.terminal.SetContent("")
		return
	}
	output, err := m.mgr.CaptureOutput(*agent)
	if err != nil {
		m.terminal.SetContent("")
		return
	}
	m.terminal.SetContent(output)
}

func (m model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "tab" {
		switch m.layout {
		case layoutSplit:
			if m.focus == focusSidebar {
				m.focus = focusTerminal
			} else {
				m.focus = focusSidebar
			}
			m.sidebar.SetFocused(m.focus == focusSidebar)
		case layoutTerminal:
			m.layout = layoutList
		case layoutList:
			m.layout = layoutTerminal
		}
		return m, nil
	}

	if m.focus == focusSidebar || m.layout == layoutList {
		switch msg.Type {
		case tea.KeyUp:
			m.sidebar.MoveUp()
			return m, nil
		case tea.KeyDown:
			m.sidebar.MoveDown()
			return m, nil
		case tea.KeyEnter:
			agent := m.sidebar.CurrentAgent()
			if agent != nil && agent.TmuxSession != "" {
				return m, tea.ExecProcess(
					exec.Command("tmux", "attach-session", "-t", agent.TmuxSession),
					func(err error) tea.Msg { return attachDoneMsg{} },
				)
			}
			if m.layout == layoutList {
				m.layout = layoutTerminal
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "k":
			m.sidebar.MoveUp()
		case "j":
			m.sidebar.MoveDown()
		case "n":
			m.state = viewNewSession
			m.inputStep = 0
			m.input.SetValue("")
			m.input.Placeholder = "Session name (e.g. referral-system)"
			m.err = ""
			return m, textinput.Blink
		case "a":
			sess := m.sidebar.CurrentSession()
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
			agent := m.sidebar.CurrentAgent()
			if agent != nil && agent.Status == "idle" {
				m.mgr.StartAgent(*agent)
				return m, m.refresh()
			}
			if m.sidebar.IsOnSession() {
				sess := m.sidebar.CurrentSession()
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
			agent := m.sidebar.CurrentAgent()
			if agent != nil && agent.Status == "running" {
				m.mgr.StopAgent(*agent)
				return m, m.refresh()
			}
			if m.sidebar.IsOnSession() {
				sess := m.sidebar.CurrentSession()
				if sess != nil {
					for _, a := range sess.agents {
						m.mgr.StopAgent(a)
					}
					return m, m.refresh()
				}
			}
		case "D":
			sess := m.sidebar.CurrentSession()
			if sess != nil {
				m.mgr.DeleteSession(sess.session.Name)
				return m, m.refresh()
			}
		}
	} else {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) updateNewSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.state = viewMain
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
		m.state = viewMain
		return m, m.refresh()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateAddAgent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.state = viewMain
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if val == "" {
			return m, nil
		}
		switch m.inputStep {
		case 0:
			m.newRepo = val
			m.inputStep = 1
			m.input.SetValue("")
			m.input.Placeholder = "Agent: claude or codex"
			return m, nil
		case 1:
			if val != "claude" && val != "codex" {
				m.err = "must be 'claude' or 'codex'"
				return m, nil
			}
			m.newAgent = val
			_, err := m.mgr.AddAgent(m.newName, m.newRepo, m.newAgent, "")
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
			m.state = viewMain
			return m, m.refresh()
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.state == viewNewSession {
		return m.viewInputOverlay("New Session", "Enter session name:")
	}
	if m.state == viewAddAgent {
		labels := []string{"Enter repo name:", "Select agent (claude/codex):"}
		return m.viewInputOverlay(fmt.Sprintf("Add Agent to %s", m.newName), labels[m.inputStep])
	}
	return m.viewMain()
}

func (m model) viewInputOverlay(title, label string) string {
	// Render the main view as background
	bg := m.viewMain()

	// Build overlay
	var b strings.Builder
	b.WriteString(inputTitleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(label + "\n")
	b.WriteString(m.input.View())
	if m.err != "" {
		b.WriteString("\n" + inputErrStyle.Render(m.err))
	}
	b.WriteString("\n\n")
	b.WriteString(menuKeyStyle.Render("enter") + " confirm  " + menuKeyStyle.Render("esc") + " cancel")

	overlay := inputBoxStyle.Render(b.String())

	// Center the overlay on background
	_ = bg
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		overlay,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("0")),
	)
}

func (m model) viewMain() string {
	var content string

	switch m.layout {
	case layoutSplit:
		sidebarContent := m.sidebar.View()
		termContent := m.terminal.View()

		// Sidebar panel
		sidebarBorderColor := lipgloss.Color("#808080")
		if m.focus == focusSidebar {
			sidebarBorderColor = lipgloss.Color("#7D56F4")
		}
		sidebarPanel := lipgloss.NewStyle().
			Width(m.sidebar.width).
			Height(m.sidebar.height).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(sidebarBorderColor).
			Render(sidebarContent)

		// Terminal panel — shared left border with sidebar
		termBorderColor := lipgloss.Color("#808080")
		if m.focus == focusTerminal {
			termBorderColor = lipgloss.Color("#7D56F4")
		}
		termBorder := lipgloss.RoundedBorder()
		termBorder.TopLeft = "┬"
		termBorder.BottomLeft = "┴"

		termPanel := lipgloss.NewStyle().
			Width(m.terminal.width).
			Height(m.terminal.height).
			Border(termBorder).
			BorderForeground(termBorderColor).
			Render(termContent)

		content = lipgloss.JoinHorizontal(lipgloss.Top, sidebarPanel, termPanel)

	case layoutTerminal:
		content = lipgloss.NewStyle().
			Width(m.terminal.width).
			Height(m.terminal.height).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#808080")).
			Render(m.terminal.View())

	case layoutList:
		content = lipgloss.NewStyle().
			Width(m.sidebar.width).
			Height(m.sidebar.height).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Render(m.sidebar.View())
	}

	// Menu bar
	menu := m.renderMenu()

	return lipgloss.JoinVertical(lipgloss.Left, content, menu)
}

func (m model) renderMenu() string {
	type binding struct {
		key    string
		action string
	}

	var bindings []binding
	switch m.layout {
	case layoutSplit:
		bindings = []binding{
			{"n", "new"}, {"a", "add"}, {"s", "start"}, {"S", "stop"},
			{"Tab", "toggle"}, {"Enter", "attach"}, {"D", "delete"}, {"q", "quit"},
		}
	case layoutTerminal:
		bindings = []binding{
			{"Tab", "sessions"}, {"Enter", "attach"}, {"q", "quit"},
		}
	case layoutList:
		bindings = []binding{
			{"n", "new"}, {"a", "add"}, {"s", "start"}, {"S", "stop"},
			{"Tab", "back"}, {"Enter", "select"}, {"D", "delete"}, {"q", "quit"},
		}
	}

	var parts []string
	for _, b := range bindings {
		parts = append(parts, menuActionStyle.Render(b.key)+menuKeyStyle.Render(":"+b.action))
	}
	return " " + strings.Join(parts, "  ")
}

func Run(mgr *session.Manager) {
	p := tea.NewProgram(NewModel(mgr), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
