package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/TitanSarim/myagent/internal/sysmon"
)

type Role int

const (
	RoleUser Role = iota
	RoleAssistant
	RoleSystem
	RoleTool
)

type ChatLine struct {
	Role Role
	Text string
}

type SlashCmd struct {
	Name string
	Desc string
	Run  string
}

var DefaultSlash = []SlashCmd{
	{Name: "/help", Desc: "Show help", Run: "/help"},
	{Name: "/status", Desc: "Repo, mode, ollama status", Run: "/status"},
	{Name: "/tools", Desc: "List available agent tools", Run: "/tools"},
	{Name: "/mode fast", Desc: "Qwen3.5 9B — quick answers", Run: "/mode fast"},
	{Name: "/mode smart", Desc: "Qwen3.8 27B — reasoning", Run: "/mode smart"},
	{Name: "/mode code", Desc: "Qwen3.6 27B — coding", Run: "/mode code"},
	{Name: "/mode auto", Desc: "Auto-pick the best model", Run: "/mode auto"},
	{Name: "/write on", Desc: "Allow file edits", Run: "/write on"},
	{Name: "/write off", Desc: "Read-only mode", Run: "/write off"},
	{Name: "/clear", Desc: "Clear chat history", Run: "/clear"},
	{Name: "/exit", Desc: "Quit the app", Run: "/exit"},
}

type Emitter interface {
	Info(msg string)
	Token(msg string)
}

type Config struct {
	AppName  string
	Version  string
	Repo     string
	Mode     string
	Writable bool
	Tools    []string
	OnSubmit func(ctx context.Context, text string, emit Emitter) error
	OnSlash  func(cmd string) (handled bool, reply string, newMode string, newWrite *bool)
}

type model struct {
	cfg       Config
	width     int
	height    int
	vp        viewport.Model
	input     textinput.Model
	lines     []ChatLine
	busy      bool
	stats     sysmon.Stats
	slashIdx  int
	cancel    context.CancelFunc
	streamBuf strings.Builder
	streamOn  bool
	program   *tea.Program

	// Input history (previous prompts) — ↑/↓ like a shell
	history    []string
	histIdx    int    // -1 = not browsing
	histDraft  string // text being typed before browsing history

	// Chat message selection — Ctrl+↑/↓ browse messages, auto-copy
	chatSel     int  // -1 = none
	copiedHint  string
	copiedUntil time.Time
}

type tickMsg sysmon.Stats
type doneMsg struct{ err error }
type infoMsg string
type tokenMsg string

type emitter struct{ p *tea.Program }

func (e emitter) Info(msg string) {
	if e.p != nil {
		e.p.Send(infoMsg(msg))
	}
}
func (e emitter) Token(msg string) {
	if e.p != nil {
		e.p.Send(tokenMsg(msg))
	}
}

func RunInteractive(cfg Config) error {
	m := newModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.program = p
	_, err := p.Run()
	return err
}

func newModel(cfg Config) *model {
	ti := textinput.New()
	ti.Placeholder = "Ask anything…  ↑↓ history · Ctrl+↑↓ chat · / commands"

	ti.Focus()
	ti.CharLimit = 8000
	ti.Width = 60
	ti.Prompt = " › "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	vp := viewport.New(80, 20)

	m := &model{
		cfg:     cfg,
		input:   ti,
		vp:      vp,
		stats:   sysmon.Sample(),
		histIdx: -1,
		chatSel: -1,
		lines: []ChatLine{
			{Role: RoleSystem, Text: fmt.Sprintf("Welcome to %s v%s", cfg.AppName, cfg.Version)},
			{Role: RoleSystem, Text: "Type a question.  / commands  ·  ↑↓ past prompts  ·  Ctrl+↑↓ browse chat (auto-copies)"},
		},
	}
	m.refreshViewport()
	return m
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(sysmon.TickEvery, func(time.Time) tea.Msg {
		return tickMsg(sysmon.Sample())
	})
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		m.refreshViewport()
		return m, nil

	case tickMsg:
		m.stats = sysmon.Stats(msg)
		return m, tickCmd()

	case infoMsg:
		m.flushStream()
		m.lines = append(m.lines, ChatLine{Role: RoleTool, Text: string(msg)})
		m.refreshViewport()
		return m, nil

	case tokenMsg:
		if !m.streamOn {
			m.streamOn = true
			m.streamBuf.Reset()
		}
		m.streamBuf.WriteString(string(msg))
		m.refreshViewport()
		return m, nil

	case doneMsg:
		m.busy = false
		m.flushStream()
		if msg.err != nil {
			m.lines = append(m.lines, ChatLine{Role: RoleSystem, Text: "error: " + msg.err.Error()})
		}
		m.refreshViewport()
		m.input.Focus()
		return m, textinput.Blink

	case tea.KeyMsg:
		if m.busy {
			if msg.Type == tea.KeyCtrlC && m.cancel != nil {
				m.cancel()
			}
			return m, nil
		}

		if m.showSlash() {
			filtered := m.filteredSlash()
			switch msg.Type {
			case tea.KeyUp:
				if m.slashIdx > 0 {
					m.slashIdx--
				}
				if len(filtered) > 0 {
					m.copyText(filtered[m.slashIdx].Run)
				}
				return m, nil
			case tea.KeyDown, tea.KeyTab:
				if m.slashIdx < len(filtered)-1 {
					m.slashIdx++
				}
				if len(filtered) > 0 {
					m.copyText(filtered[m.slashIdx].Run)
				}
				return m, nil
			case tea.KeyEnter:
				if len(filtered) > 0 {
					sel := filtered[m.slashIdx].Run
					m.copyText(sel)
					return m.doSubmit(sel)
				}
			case tea.KeyEsc:
				m.input.SetValue("")
				return m, nil
			}
		}

		// Ctrl+Up / Ctrl+Down — browse chat messages & copy
		if msg.Type == tea.KeyCtrlUp || (msg.Type == tea.KeyUp && msg.Alt) {
			m.browseChat(-1)
			return m, nil
		}
		if msg.Type == tea.KeyCtrlDown || (msg.Type == tea.KeyDown && msg.Alt) {
			m.browseChat(+1)
			return m, nil
		}

		// ↑/↓ — previous prompts into the input (shell-style) + copy
		if !m.showSlash() && (msg.Type == tea.KeyUp || msg.Type == tea.KeyDown) {
			if len(m.history) > 0 {
				m.browseInputHistory(msg.Type == tea.KeyUp)
				return m, nil
			}
			// No prompt history yet → browse chat instead
			if msg.Type == tea.KeyUp {
				m.browseChat(-1)
			} else {
				m.browseChat(+1)
			}
			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			if m.input.Value() != "" {
				m.input.SetValue("")
				m.histIdx = -1
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyEsc:
			if m.chatSel >= 0 {
				m.chatSel = -1
				m.refreshViewport()
				return m, nil
			}
			if m.input.Value() != "" {
				m.input.SetValue("")
				m.histIdx = -1
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyEnter:
			val := strings.TrimSpace(m.input.Value())
			if val == "" {
				// Enter on a selected chat message → copy again (confirm)
				if m.chatSel >= 0 && m.chatSel < len(m.lines) {
					m.copyText(m.lines[m.chatSel].Text)
					return m, nil
				}
				return m, nil
			}
			return m.doSubmit(val)
		case tea.KeyCtrlL:
			m.lines = m.lines[:0]
			m.chatSel = -1
			m.refreshViewport()
			return m, nil
		}

		// Any other typing leaves history browse mode
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
			m.histIdx = -1
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	if m.showSlash() {
		f := m.filteredSlash()
		if m.slashIdx >= len(f) {
			m.slashIdx = max(0, len(f)-1)
		}
	}
	m.vp, cmd = m.vp.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m *model) copyText(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	_ = clipboard.WriteAll(s)
	m.copiedHint = "copied"
	m.copiedUntil = time.Now().Add(2 * time.Second)
}

func (m *model) browseInputHistory(up bool) {
	if len(m.history) == 0 {
		return
	}
	if m.histIdx < 0 {
		m.histDraft = m.input.Value()
		m.histIdx = len(m.history)
	}
	if up {
		if m.histIdx > 0 {
			m.histIdx--
		}
	} else {
		if m.histIdx < len(m.history)-1 {
			m.histIdx++
		} else {
			// past newest → restore draft
			m.histIdx = -1
			m.input.SetValue(m.histDraft)
			m.input.CursorEnd()
			return
		}
	}
	val := m.history[m.histIdx]
	m.input.SetValue(val)
	m.input.CursorEnd()
	m.copyText(val)
}

func (m *model) browseChat(delta int) {
	if len(m.lines) == 0 {
		return
	}
	if m.chatSel < 0 {
		if delta < 0 {
			m.chatSel = len(m.lines) - 1
		} else {
			m.chatSel = 0
		}
	} else {
		m.chatSel += delta
		if m.chatSel < 0 {
			m.chatSel = 0
		}
		if m.chatSel >= len(m.lines) {
			m.chatSel = len(m.lines) - 1
		}
	}
	txt := m.lines[m.chatSel].Text
	m.copyText(txt)
	// Also put user messages into the input for easy re-run
	if m.lines[m.chatSel].Role == RoleUser {
		m.input.SetValue(txt)
		m.input.CursorEnd()
	}
	m.refreshViewport()
}

func (m *model) pushHistory(val string) {
	val = strings.TrimSpace(val)
	if val == "" {
		return
	}
	// Avoid consecutive duplicates
	if len(m.history) > 0 && m.history[len(m.history)-1] == val {
		m.histIdx = -1
		return
	}
	m.history = append(m.history, val)
	if len(m.history) > 200 {
		m.history = m.history[len(m.history)-200:]
	}
	m.histIdx = -1
}

func (m *model) doSubmit(val string) (tea.Model, tea.Cmd) {
	m.pushHistory(val)
	m.copyText(val)
	m.input.SetValue("")
	m.slashIdx = 0
	m.chatSel = -1
	m.lines = append(m.lines, ChatLine{Role: RoleUser, Text: val})
	m.refreshViewport()

	lower := strings.ToLower(strings.TrimSpace(val))
	if strings.HasPrefix(lower, "/") {
		if lower == "/exit" || lower == "/quit" {
			return m, tea.Quit
		}
		if lower == "/help" {
			m.lines = append(m.lines, ChatLine{Role: RoleSystem, Text: helpText()})
			m.refreshViewport()
			return m, nil
		}
		if lower == "/clear" {
			m.lines = []ChatLine{{Role: RoleSystem, Text: "Chat cleared."}}
			m.refreshViewport()
			return m, nil
		}
		if m.cfg.OnSlash != nil {
			handled, reply, newMode, newWrite := m.cfg.OnSlash(val)
			if newMode != "" {
				m.cfg.Mode = newMode
			}
			if newWrite != nil {
				m.cfg.Writable = *newWrite
			}
			if handled {
				if reply != "" {
					m.lines = append(m.lines, ChatLine{Role: RoleSystem, Text: reply})
					m.refreshViewport()
				}
				return m, nil
			}
		}
	}

	if m.cfg.OnSubmit == nil {
		return m, nil
	}

	m.busy = true
	m.streamOn = false
	m.streamBuf.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	prog := m.program

	return m, func() tea.Msg {
		em := emitter{p: prog}
		err := m.cfg.OnSubmit(ctx, val, em)
		cancel()
		return doneMsg{err: err}
	}
}

func (m *model) flushStream() {
	if m.streamOn && m.streamBuf.Len() > 0 {
		txt := strings.TrimRight(m.streamBuf.String(), "\n")
		m.lines = append(m.lines, ChatLine{Role: RoleAssistant, Text: txt})
		m.streamBuf.Reset()
		m.streamOn = false
	}
}

func (m *model) showSlash() bool {
	return strings.HasPrefix(m.input.Value(), "/") && !m.busy
}

func (m *model) filteredSlash() []SlashCmd {
	q := strings.ToLower(m.input.Value())
	var out []SlashCmd
	for _, c := range DefaultSlash {
		name := strings.ToLower(c.Name)
		if q == "/" || strings.HasPrefix(name, q) || strings.Contains(strings.ToLower(c.Desc), strings.TrimPrefix(q, "/")) {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return append([]SlashCmd{}, DefaultSlash...)
	}
	return out
}

func (m *model) layout() {
	slashH := 0
	if m.showSlash() {
		slashH = min(10, len(m.filteredSlash())+3)
	}
	chrome := 11 + slashH
	h := m.height - chrome
	if h < 6 {
		h = 6
	}
	w := m.width - 4
	if w < 30 {
		w = 30
	}
	m.vp.Width = w
	m.vp.Height = h
	m.input.Width = w - 6
}

func (m *model) refreshViewport() {
	var b strings.Builder
	for i, ln := range m.lines {
		b.WriteString(renderLine(ln, m.vp.Width, i == m.chatSel))
		b.WriteString("\n\n")
	}
	if m.streamOn {
		b.WriteString(renderLine(ChatLine{Role: RoleAssistant, Text: m.streamBuf.String() + "▌"}, m.vp.Width, false))
		b.WriteByte('\n')
	}
	m.vp.SetContent(b.String())
	m.vp.GotoBottom()
}

func renderLine(ln ChatLine, width int, selected bool) string {
	var label, color string
	switch ln.Role {
	case RoleUser:
		label, color = "You", "86"
	case RoleAssistant:
		label, color = "Agent", "219"
	case RoleTool:
		label, color = "Tool", "214"
	default:
		label, color = "System", "245"
	}
	if selected {
		label = "▸ " + label + "  (copied)"
		color = "230"
	}
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Width(max(20, width-2))
	if selected {
		bodyStyle = bodyStyle.
			Background(lipgloss.Color("63")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1)
	}
	return labelStyle.Render(label) + "\n" + bodyStyle.Render(ln.Text)
}

func (m *model) View() string {
	if m.width == 0 {
		return "\n  starting…\n"
	}

	title := fmt.Sprintf(" %s  ·  repo:%s  ·  mode:%s  ·  write:%v ",
		m.cfg.AppName, m.cfg.Repo, m.cfg.Mode, m.cfg.Writable)
	if m.busy {
		title += "· ⏳ thinking "
	}
	if m.copiedHint != "" && time.Now().Before(m.copiedUntil) {
		title += "· 📋 copied "
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("63")).
		Width(max(m.width, 40)).
		Padding(0, 1)

	chatBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1).
		Width(m.width - 2)

	inputBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("86")).
		Padding(0, 1).
		Width(m.width - 2)

	chatBox := chatBorder.Render(m.vp.View())
	inputBox := inputBorder.Render(m.input.View())

	var slashBox string
	if m.showSlash() {
		filtered := m.filteredSlash()
		var sb strings.Builder
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render("Commands & tools") + "\n")
		for i, c := range filtered {
			if i > 7 {
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("  …") + "\n")
				break
			}
			line := fmt.Sprintf("  %-14s %s", c.Name, c.Desc)
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
			if i == m.slashIdx {
				line = "▸ " + strings.TrimPrefix(line, "  ")
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63")).Bold(true)
			}
			sb.WriteString(style.Width(max(20, m.width-6)).Render(line))
			sb.WriteByte('\n')
		}
		// also show tool names if any
		if len(m.cfg.Tools) > 0 {
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(
				"Tools: "+strings.Join(m.cfg.Tools, ", ")) + "\n")
		}
		slashBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("214")).
			Padding(0, 1).
			Width(m.width - 2).
			Render(strings.TrimRight(sb.String(), "\n"))
	}

	stats := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236")).
		Width(max(m.width, 40)).
		Padding(0, 1).
		Render(m.stats.Line())

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render(
		" ↑↓ past prompts (auto-copy)  ·  Ctrl+↑↓ browse chat (auto-copy)  ·  / menu  ·  ctrl+c quit")

	parts := []string{headerStyle.Render(title), chatBox}
	if slashBox != "" {
		parts = append(parts, slashBox)
	}
	parts = append(parts, inputBox, hint, stats)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func helpText() string {
	var b strings.Builder
	b.WriteString("Slash commands:\n")
	for _, c := range DefaultSlash {
		fmt.Fprintf(&b, "  %-14s  %s\n", c.Name, c.Desc)
	}
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
