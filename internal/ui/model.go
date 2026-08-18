package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/browser"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/disclaimer"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/version"
)

const accent = "#0a66c2"

var (
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent))
	styleMuted    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8a96a3"))
	styleOK       = lipgloss.NewStyle().Foreground(lipgloss.Color("#3dd68c")).Bold(true)
	stylePending  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f5a524")).Bold(true)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b")).Bold(true)
	styleSelected = lipgloss.NewStyle().Background(lipgloss.Color("#1a2a3a")).Foreground(lipgloss.Color("#e8eef4"))
	styleHelp     = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7785"))
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e8eef4"))
	styleBox      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(accent)).Padding(1, 2)
	styleKeyChip  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color(accent)).Padding(0, 1)
	styleBarFill  = lipgloss.NewStyle().Foreground(lipgloss.Color(accent))
	styleHint     = lipgloss.NewStyle().Foreground(lipgloss.Color("#3dd68c"))
)

// ActionBus carries UI actions to the app runner.
var ActionBus = make(chan tea.Msg, 32)

// Screen identifies the current TUI view.
type Screen int

const (
	ScreenDisclaimer Screen = iota
	ScreenConnecting
	ScreenLoginWait
	ScreenInbox
	ScreenBackup
	ScreenPathEdit
	ScreenFatal
)

// ConvoRow is one conversation in the inbox list.
type ConvoRow struct {
	Convo    browser.Conversation
	BackedUp bool
	Selected bool
}

// Model is the Bubble Tea model for the whole app.
type Model struct {
	Screen   Screen
	Width    int
	Height   int
	Version  string
	Browser  string
	Download string
	Flash    string

	// Loading / connecting progress
	LoadTitle   string
	LoadStatus  string
	LoadPercent int
	SubLabel    string
	SubCurrent  int
	SubTotal    int
	SubIndeterminate bool

	Spinner spinner.Model
	PathIn  textinput.Model
	Filter  textinput.Model
	Filtering bool
	Convos    []ConvoRow
	Cursor    int
	Viewport  int

	BackupTotal   int
	BackupDone    int
	BackupCurrent string
	BackupError   string
	BackupSubLabel         string
	BackupSubDetail        string
	BackupSubCurrent       int
	BackupSubTotal         int
	BackupSubIndeterminate bool

	ErrTitle   string
	ErrSummary string
	ErrHint    string
	ErrDetails string

	DeclineRequested bool
	Quit             bool
}

// New creates the initial model.
func New(showDisclaimer bool, downloadPath string) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(accent))

	pi := textinput.New()
	pi.Placeholder = "download path"
	pi.CharLimit = 512
	pi.Width = 60

	fi := textinput.New()
	fi.Placeholder = "filter…"
	fi.CharLimit = 80
	fi.Width = 40

	m := Model{
		Version:  version.Version,
		Download: downloadPath,
		Spinner:  sp,
		PathIn:   pi,
		Filter:   fi,
	}
	if showDisclaimer {
		m.Screen = ScreenDisclaimer
	} else {
		m.Screen = ScreenConnecting
		m.LoadTitle = "Starting"
		m.LoadStatus = "Preparing…"
		m.LoadPercent = 0
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.Screen == ScreenConnecting {
		return tea.Batch(m.Spinner.Tick, emit(ReadyMsg{}))
	}
	return nil
}

func emit(msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		select {
		case ActionBus <- msg:
		default:
		}
		return nil
	}
}

// Messages from the app layer → UI

// LoadProgressMsg drives the shared connecting/loading card.
type LoadProgressMsg struct {
	Title            string
	Status           string
	Percent          int
	SubLabel         string
	SubCurrent       int
	SubTotal         int
	SubIndeterminate bool
	LoginWait        bool
	BrowserName      string
}

type ConvListMsg struct {
	Rows []ConvoRow
	Err  error
}
type BackupStartMsg struct{ Total int }
type BackupProgressMsg struct {
	Done             int
	Total            int
	Current          string // conversation name (main status)
	Err              string
	SubLabel         string
	SubDetail        string // e.g. PDF filename on the save step
	SubCurrent       int
	SubTotal         int
	SubIndeterminate bool
	// Percent overrides Done/Total for the global bar when > 0.
	Percent int
}
type BackupDoneMsg struct{ Flash string }
type FatalMsg struct {
	Title   string
	Summary string
	Hint    string
	Err     error
}
type PathSavedMsg struct{ Path string }

// Messages from the UI → app layer
type ReadyMsg struct{}
type AcceptedMsg struct{}
type DeclinedMsg struct{}
type ReloadMsg struct{}
type BackupRequestMsg struct{ Convos []browser.Conversation }
type PathEditSubmitMsg struct{ Path string }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		if m.Screen == ScreenConnecting || m.Screen == ScreenLoginWait || m.Screen == ScreenBackup {
			return m, cmd
		}
		return m, nil

	case LoadProgressMsg:
		if msg.LoginWait {
			m.Screen = ScreenLoginWait
			m.Browser = msg.BrowserName
		} else if m.Screen != ScreenBackup {
			m.Screen = ScreenConnecting
		}
		m.LoadTitle = msg.Title
		m.LoadStatus = msg.Status
		m.LoadPercent = clampPct(msg.Percent)
		m.SubLabel = msg.SubLabel
		m.SubCurrent = msg.SubCurrent
		m.SubTotal = msg.SubTotal
		m.SubIndeterminate = msg.SubIndeterminate
		return m, m.Spinner.Tick

	case ConvListMsg:
		if msg.Err != nil {
			return m.applyFatal(ClassifyError(msg.Err))
		}
		m.Convos = msg.Rows
		m.Cursor = 0
		m.Viewport = 0
		m.Screen = ScreenInbox
		m.clearLoad()
		return m, nil

	case BackupStartMsg:
		m.Screen = ScreenBackup
		m.BackupTotal = msg.Total
		m.BackupDone = 0
		m.BackupCurrent = ""
		m.BackupError = ""
		m.BackupSubLabel = "Preparing…"
		m.BackupSubDetail = ""
		m.BackupSubCurrent = 0
		m.BackupSubTotal = 0
		m.BackupSubIndeterminate = false
		m.LoadTitle = "Backing up conversations"
		m.LoadStatus = "Starting…"
		m.LoadPercent = 0
		m.SubLabel = "Preparing…"
		m.SubCurrent = 0
		m.SubTotal = 0
		m.SubIndeterminate = false
		return m, m.Spinner.Tick

	case BackupProgressMsg:
		m.Screen = ScreenBackup
		m.BackupDone = msg.Done
		m.BackupTotal = msg.Total
		m.BackupCurrent = msg.Current
		if msg.Err != "" {
			m.BackupError = msg.Err
		}
		m.BackupSubLabel = msg.SubLabel
		m.BackupSubDetail = msg.SubDetail
		m.BackupSubCurrent = msg.SubCurrent
		m.BackupSubTotal = msg.SubTotal
		m.BackupSubIndeterminate = msg.SubIndeterminate
		if msg.Percent > 0 {
			m.LoadPercent = clampPct(msg.Percent)
		} else {
			pct := 0
			if msg.Total > 0 {
				pct = msg.Done * 100 / msg.Total
			}
			m.LoadPercent = clampPct(pct)
		}
		if msg.Current != "" {
			m.LoadStatus = msg.Current
		}
		m.LoadTitle = "Backing up conversations"
		m.SubLabel = msg.SubLabel
		m.SubCurrent = msg.SubCurrent
		m.SubTotal = msg.SubTotal
		m.SubIndeterminate = msg.SubIndeterminate
		return m, m.Spinner.Tick

	case BackupDoneMsg:
		m.Screen = ScreenInbox
		m.Flash = msg.Flash
		for i := range m.Convos {
			if m.Convos[i].BackedUp {
				m.Convos[i].Selected = false
			}
		}
		m.clearLoad()
		return m, nil

	case FatalMsg:
		return m.applyFatal(msg)

	case PathSavedMsg:
		m.Download = msg.Path
		if m.Screen == ScreenInbox {
			m.Flash = "Download path updated"
		}
		return m, nil
	}
	return m, nil
}

func (m Model) applyFatal(msg FatalMsg) (tea.Model, tea.Cmd) {
	m.Screen = ScreenFatal
	m.ErrTitle = msg.Title
	if m.ErrTitle == "" {
		m.ErrTitle = "Something went wrong"
	}
	m.ErrSummary = msg.Summary
	m.ErrHint = msg.Hint
	if msg.Err != nil {
		m.ErrDetails = msg.Err.Error()
	} else {
		m.ErrDetails = ""
	}
	if m.ErrSummary == "" && m.ErrDetails != "" {
		m.ErrSummary = m.ErrDetails
		m.ErrDetails = ""
	}
	return m, nil
}

func (m *Model) clearLoad() {
	m.LoadTitle = ""
	m.LoadStatus = ""
	m.LoadPercent = 0
	m.SubLabel = ""
	m.SubCurrent = 0
	m.SubTotal = 0
	m.SubIndeterminate = false
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.Screen {
	case ScreenDisclaimer:
		switch key {
		case "y", "enter":
			m.Screen = ScreenConnecting
			m.LoadTitle = "Starting"
			m.LoadStatus = "Preparing…"
			m.LoadPercent = 0
			return m, tea.Batch(m.Spinner.Tick, emit(AcceptedMsg{}))
		case "n", "q", "ctrl+c", "esc":
			m.DeclineRequested = true
			m.Quit = true
			return m, tea.Batch(emit(DeclinedMsg{}), tea.Quit)
		}

	case ScreenFatal:
		if key == "q" || key == "ctrl+c" || key == "esc" || key == "enter" {
			m.Quit = true
			return m, tea.Quit
		}

	case ScreenLoginWait, ScreenConnecting, ScreenBackup:
		if key == "ctrl+c" || key == "q" {
			m.Quit = true
			return m, tea.Quit
		}

	case ScreenPathEdit:
		switch key {
		case "esc":
			m.Screen = ScreenInbox
			return m, nil
		case "enter":
			path := strings.TrimSpace(m.PathIn.Value())
			m.Screen = ScreenInbox
			return m, emit(PathEditSubmitMsg{Path: path})
		}
		var cmd tea.Cmd
		m.PathIn, cmd = m.PathIn.Update(msg)
		return m, cmd

	case ScreenInbox:
		if m.Filtering {
			switch key {
			case "esc":
				m.Filtering = false
				m.Filter.Blur()
				m.Filter.SetValue("")
				return m, nil
			case "enter":
				m.Filtering = false
				m.Filter.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.Filter, cmd = m.Filter.Update(msg)
			return m, cmd
		}
		switch key {
		case "q", "ctrl+c":
			m.Quit = true
			return m, tea.Quit
		case "/", "f":
			m.Filtering = true
			m.Filter.Focus()
			return m, nil
		case "j", "down":
			m.moveCursor(1)
		case "k", "up":
			m.moveCursor(-1)
		case "g":
			m.Cursor = 0
			m.Viewport = 0
		case "G":
			vis := m.visible()
			if len(vis) > 0 {
				m.Cursor = len(vis) - 1
				m.ensureVisible()
			}
		case " ":
			vis := m.visible()
			if m.Cursor >= 0 && m.Cursor < len(vis) {
				idx := vis[m.Cursor]
				m.Convos[idx].Selected = !m.Convos[idx].Selected
			}
		case "a":
			for i := range m.Convos {
				if !m.Convos[i].BackedUp {
					m.Convos[i].Selected = true
				}
			}
		case "A":
			for i := range m.Convos {
				m.Convos[i].Selected = false
			}
		case "enter":
			var sel []browser.Conversation
			for _, c := range m.Convos {
				if c.Selected {
					sel = append(sel, c.Convo)
				}
			}
			if len(sel) == 0 {
				vis := m.visible()
				if m.Cursor >= 0 && m.Cursor < len(vis) {
					sel = []browser.Conversation{m.Convos[vis[m.Cursor]].Convo}
				}
			}
			if len(sel) == 0 {
				return m, nil
			}
			return m, emit(BackupRequestMsg{Convos: sel})
		case "r":
			m.Screen = ScreenConnecting
			m.LoadTitle = "Reloading"
			m.LoadStatus = "Loading conversations…"
			m.LoadPercent = 70
			m.SubLabel = ""
			return m, tea.Batch(m.Spinner.Tick, emit(ReloadMsg{}))
		case "p":
			m.PathIn.SetValue(m.Download)
			m.PathIn.Focus()
			m.Screen = ScreenPathEdit
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	vis := m.visible()
	if len(vis) == 0 {
		return
	}
	m.Cursor += delta
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(vis) {
		m.Cursor = len(vis) - 1
	}
	m.ensureVisible()
}

func (m *Model) ensureVisible() {
	listH := m.listHeight()
	if m.Cursor < m.Viewport {
		m.Viewport = m.Cursor
	}
	if m.Cursor >= m.Viewport+listH {
		m.Viewport = m.Cursor - listH + 1
	}
}

func (m Model) listHeight() int {
	h := m.Height - 12
	if h < 5 {
		h = 5
	}
	return h
}

func (m Model) visible() []int {
	q := strings.ToLower(strings.TrimSpace(m.Filter.Value()))
	var out []int
	for i, c := range m.Convos {
		if q == "" || strings.Contains(strings.ToLower(c.Convo.NameStr()), q) ||
			strings.Contains(strings.ToLower(c.Convo.SnippetStr()), q) {
			out = append(out, i)
		}
	}
	return out
}

func (m Model) View() string {
	switch m.Screen {
	case ScreenDisclaimer:
		return m.viewDisclaimer()
	case ScreenConnecting, ScreenLoginWait:
		return m.viewLoading()
	case ScreenBackup:
		return m.viewBackup()
	case ScreenPathEdit:
		return m.viewPathEdit()
	case ScreenFatal:
		return m.viewFatal()
	case ScreenInbox:
		return m.viewInbox()
	default:
		return ""
	}
}

func (m Model) appTitleLine() string {
	return styleTitle.Render("LinkedIn Inbox Downloader") + "  " + styleMuted.Render("v"+m.Version)
}

func (m Model) viewDisclaimer() string {
	body := styleBox.Render(
		styleTitle.Render("Before you continue") + "\n\n" +
			styleMuted.Render(disclaimer.Text) + "\n\n" +
			renderHelp([]helpItem{{"y", "accept"}, {"n", "decline"}}, m.helpWidth()),
	)
	return lipgloss.Place(max(m.Width, 40), max(m.Height, 20), lipgloss.Center, lipgloss.Center, body)
}

func (m Model) viewLoading() string {
	title := m.LoadTitle
	if title == "" {
		title = "Connecting"
	}
	var b strings.Builder
	b.WriteString(m.appTitleLine() + "\n\n")
	b.WriteString(styleHeader.Render(title) + "\n")
	// Global section: category only (no counts / harvest details).
	if m.LoadStatus != "" {
		b.WriteString(m.Spinner.View() + "  " + m.LoadStatus + "\n")
	} else {
		b.WriteString(m.Spinner.View() + "\n")
	}
	b.WriteString(progressBar(m.LoadPercent, 40) + "  " + styleMuted.Render(fmt.Sprintf("%d%%", m.LoadPercent)) + "\n")
	if m.SubLabel != "" {
		b.WriteString("\n" + styleHeader.Render(m.SubLabel) + "\n")
		b.WriteString(styleMuted.Render(subStatusLine(m.SubLabel, m.SubCurrent, m.SubTotal, m.SubIndeterminate)) + "\n")
		b.WriteString(subProgressBar(m.SubCurrent, m.SubTotal, m.SubIndeterminate, 40) + "\n")
	}
	b.WriteString("\n" + renderHelp([]helpItem{{"q", "quit"}}, m.helpWidth()))
	return lipgloss.Place(max(m.Width, 40), max(m.Height, 12), lipgloss.Center, lipgloss.Center, styleBox.Render(b.String()))
}

func (m Model) viewBackup() string {
	var b strings.Builder
	b.WriteString(m.appTitleLine() + "\n\n")
	b.WriteString(styleHeader.Render("Backing up conversations") + "\n")
	status := m.BackupCurrent
	if status == "" {
		status = m.LoadStatus
	}
	if status == "" {
		status = "Working…"
	}
	b.WriteString(m.Spinner.View() + "  " + status + "\n")
	globalLabel := styleMuted.Render(fmt.Sprintf("%d%%", m.LoadPercent))
	if m.BackupTotal > 0 {
		globalLabel = styleMuted.Render(fmt.Sprintf("%d / %d  ·  %d%%", m.BackupDone, m.BackupTotal, m.LoadPercent))
	}
	b.WriteString(progressBar(m.LoadPercent, 40) + "  " + globalLabel + "\n")

	subLabel := m.SubLabel
	if subLabel == "" {
		subLabel = "Preparing…"
	}
	b.WriteString("\n" + styleHeader.Render(subLabel) + "\n")
	if m.BackupSubDetail != "" {
		b.WriteString(styleMuted.Render(m.BackupSubDetail) + "\n")
	}
	b.WriteString(styleMuted.Render(subStatusLine(subLabel, m.SubCurrent, m.SubTotal, m.SubIndeterminate)) + "\n")
	b.WriteString(subProgressBar(m.SubCurrent, m.SubTotal, m.SubIndeterminate, 40) + "\n")

	if m.BackupError != "" {
		b.WriteString("\n" + styleError.Render("Last error") + "\n")
		b.WriteString(styleMuted.Render(wrapWords(m.BackupError, m.boxInnerWidth())) + "\n")
	}
	b.WriteString("\n" + renderHelp([]helpItem{{"q", "abort"}}, m.helpWidth()))
	return lipgloss.Place(max(m.Width, 40), max(m.Height, 12), lipgloss.Center, lipgloss.Center, styleBox.Render(b.String()))
}

func (m Model) viewPathEdit() string {
	body := styleBox.Render(
		styleTitle.Render("Download path") + "\n\n" +
			m.PathIn.View() + "\n\n" +
			renderHelp([]helpItem{{"enter", "save"}, {"esc", "cancel"}}, m.helpWidth()),
	)
	return lipgloss.Place(max(m.Width, 40), max(m.Height, 12), lipgloss.Center, lipgloss.Center, body)
}

func (m Model) viewFatal() string {
	w := m.boxInnerWidth()
	var b strings.Builder
	b.WriteString(styleError.Render(m.ErrTitle) + "\n\n")
	if m.ErrSummary != "" {
		b.WriteString(wrapWords(m.ErrSummary, w) + "\n")
	}
	if m.ErrHint != "" {
		b.WriteString("\n" + styleHint.Render("What to try") + "\n")
		b.WriteString(styleMuted.Render(wrapWords(m.ErrHint, w)) + "\n")
	}
	if m.ErrDetails != "" && m.ErrDetails != m.ErrSummary {
		b.WriteString("\n" + styleMuted.Render("Details") + "\n")
		b.WriteString(styleHelp.Render(wrapWords(m.ErrDetails, w)) + "\n")
	}
	b.WriteString("\n" + renderHelp([]helpItem{{"enter", "quit"}, {"q", "quit"}}, m.helpWidth()))
	return lipgloss.Place(max(m.Width, 40), max(m.Height, 12), lipgloss.Center, lipgloss.Center, styleBox.Render(b.String()))
}

func (m Model) viewInbox() string {
	var b strings.Builder
	b.WriteString(m.appTitleLine() + "\n")
	b.WriteString(styleMuted.Render("PDFs → "+m.Download) + "\n")
	if m.Flash != "" {
		b.WriteString(styleOK.Render(m.Flash) + "\n")
	}
	b.WriteString("\n")

	vis := m.visible()
	pending, backed := 0, 0
	for _, c := range m.Convos {
		if c.BackedUp {
			backed++
		} else {
			pending++
		}
	}
	b.WriteString(fmt.Sprintf("%s  ·  %s\n\n",
		stylePending.Render(fmt.Sprintf("%d not backed up", pending)),
		styleOK.Render(fmt.Sprintf("%d backed up", backed)),
	))

	if m.Filtering || m.Filter.Value() != "" {
		b.WriteString("Filter: " + m.Filter.View() + "\n\n")
	}

	listH := m.listHeight()
	end := m.Viewport + listH
	if end > len(vis) {
		end = len(vis)
	}
	for i := m.Viewport; i < end; i++ {
		idx := vis[i]
		row := m.Convos[idx]
		pill := stylePending.Render("○ not backed up")
		if row.BackedUp {
			pill = styleOK.Render("● backed up")
		}
		mark := " "
		if row.Selected {
			mark = "✓"
		}
		snippet := strings.ReplaceAll(row.Convo.SnippetStr(), "\n", " ")
		if len(snippet) > 48 {
			snippet = snippet[:48] + "…"
		}
		line := fmt.Sprintf(" %s %s  %-28s  %s  %s",
			mark, pill, truncate(row.Convo.NameStr(), 28), styleMuted.Render(row.Convo.TimeStr()), styleMuted.Render(snippet))
		if i == m.Cursor {
			line = styleSelected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if len(vis) == 0 {
		b.WriteString(styleMuted.Render("  No conversations match.") + "\n")
	}

	b.WriteString("\n")
	primary := []helpItem{
		{"enter", "backup"},
		{"space", "(un)select"},
		{"↑↓", "move"},
	}
	secondary := []helpItem{
		{"a", "all pending"},
		{"f", "filter"},
		{"r", "reload"},
		{"p", "path"},
		{"q", "quit"},
	}
	w := max(m.Width, 40)
	b.WriteString(renderHelp(primary, w) + "\n")
	b.WriteString(renderHelp(secondary, w))
	return b.String()
}

type helpItem struct {
	Key  string
	Label string
}

func renderHelp(items []helpItem, width int) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, styleKeyChip.Render(it.Key)+" "+styleMuted.Render(it.Label))
	}
	sep := styleHelp.Render(" | ")
	line := strings.Join(parts, sep)
	// Rough wrap: if too wide, join with newlines in halves.
	if width > 20 && lipgloss.Width(line) > width {
		mid := len(parts) / 2
		if mid < 1 {
			mid = 1
		}
		return strings.Join(parts[:mid], sep) + "\n" + strings.Join(parts[mid:], sep)
	}
	return line
}

func progressBar(pct, width int) string {
	pct = clampPct(pct)
	if width < 8 {
		width = 8
	}
	filled := width * pct / 100
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return styleBarFill.Render(bar)
}

func subProgressBar(current, total int, indeterminate bool, width int) string {
	if !indeterminate && total > 0 {
		return progressBar(current*100/total, width)
	}
	// Indeterminate: fill based on a soft cap so the bar grows with discoveries.
	pct := 0
	if current > 0 {
		pct = current * 100 / (current + 8)
		if pct > 95 {
			pct = 95
		}
		if pct < 5 {
			pct = 5
		}
	}
	return progressBar(pct, width)
}

func subStatusLine(label string, current, total int, indeterminate bool) string {
	if !indeterminate && total > 0 {
		return fmt.Sprintf("%d / %d", current, total)
	}
	if current <= 0 {
		return "Starting…"
	}
	low := strings.ToLower(label)
	switch {
	case strings.Contains(low, "message"):
		return fmt.Sprintf("%d loaded", current)
	case strings.Contains(low, "conversation") || strings.Contains(low, "found") || strings.Contains(low, "inbox"):
		return fmt.Sprintf("%d found", current)
	default:
		return fmt.Sprintf("%d", current)
	}
}

func (m Model) helpWidth() int {
	w := m.Width - 8
	if w < 30 {
		w = 30
	}
	return w
}

func (m Model) boxInnerWidth() int {
	w := m.Width - 10
	if w < 40 {
		w = 40
	}
	if w > 72 {
		w = 72
	}
	return w
}

func clampPct(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func wrapWords(s string, width int) string {
	s = strings.TrimSpace(s)
	if width < 20 {
		width = 20
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	var lines []string
	var cur strings.Builder
	for _, w := range words {
		if cur.Len() == 0 {
			cur.WriteString(w)
			continue
		}
		if cur.Len()+1+len(w) > width {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(w)
			continue
		}
		cur.WriteByte(' ')
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ClassifyError turns a raw error into a FatalMsg with title, summary, and hint.
func ClassifyError(err error) FatalMsg {
	if err == nil {
		return FatalMsg{Title: "Something went wrong", Summary: "An unknown error occurred."}
	}
	msg := err.Error()
	low := strings.ToLower(msg)

	switch {
	case strings.Contains(low, "no supported browser"):
		return FatalMsg{
			Title:   "Could not find a browser",
			Summary: msg,
			Hint:    "Install Microsoft Edge, Google Chrome, or Chromium, then run this app again.",
			Err:     err,
		}
	case strings.Contains(low, "checkpoint") || strings.Contains(low, "authwall") || strings.Contains(low, "session challenge"):
		return FatalMsg{
			Title:   "LinkedIn asked you to verify again",
			Summary: "The saved session hit a challenge, checkpoint, or login wall.",
			Hint:    "Delete linkedin_cookies.json next to the program, run again, and log in in the browser window.",
			Err:     err,
		}
	case strings.Contains(low, "cookie") || strings.Contains(low, "linkedin_cookies"):
		return FatalMsg{
			Title:   "Could not use the saved session",
			Summary: "The LinkedIn cookie file could not be read or applied.",
			Hint:    "Delete linkedin_cookies.json next to the program and log in again.",
			Err:     err,
		}
	case strings.Contains(low, "login timed out") || (strings.Contains(low, "context deadline") && strings.Contains(low, "login")):
		return FatalMsg{
			Title:   "Login timed out",
			Summary: "LinkedIn login did not finish in time.",
			Hint:    "Log in in the browser window within 10 minutes, then wait for the inbox to appear.",
			Err:     err,
		}
	case strings.Contains(low, "conversation list not found"):
		return FatalMsg{
			Title:   "Could not load the inbox",
			Summary: "The LinkedIn messaging list was not found on the page.",
			Hint:    "Make sure you are logged in. Press r to reload after fixing the browser session, or delete linkedin_cookies.json and log in again.",
			Err:     err,
		}
	case strings.Contains(low, "no chat pane") || strings.Contains(low, "no message content"):
		return FatalMsg{
			Title:   "Could not read this conversation",
			Summary: msg,
			Hint:    "Try another conversation, or reload the list and try again.",
			Err:     err,
		}
	case strings.Contains(low, "start browser") || strings.Contains(low, "chromedp") || strings.Contains(low, "navigate"):
		return FatalMsg{
			Title:   "Could not start or control the browser",
			Summary: "The app failed while launching the browser or opening LinkedIn.",
			Hint:    "Close other stuck browser windows from this app, check your network, and try again.",
			Err:     err,
		}
	case strings.Contains(low, "download") || strings.Contains(low, "mkdir") || strings.Contains(low, "permission"):
		return FatalMsg{
			Title:   "Could not prepare the download folder",
			Summary: msg,
			Hint:    "Check that the download path exists and is writable, or change it with p after a successful start.",
			Err:     err,
		}
	default:
		summary := msg
		if len(summary) > 160 {
			summary = trimSentence(summary, 160)
		}
		return FatalMsg{
			Title:   "Something went wrong",
			Summary: summary,
			Hint:    "Quit, fix the issue if you can, and run the app again. Keep the details below if you need help.",
			Err:     err,
		}
	}
}

func trimSentence(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndexFunc(cut, unicode.IsSpace); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, ".,;:") + "…"
}
