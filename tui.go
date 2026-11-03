package main

import (
	"fmt"
	"net/mail"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// cursorStyle marks the character under the cursor in the body.
var cursorStyle = lipgloss.NewStyle().Reverse(true)

// subjectTextStyle is the color of the text typed in Subject.
var subjectTextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

type focus int

const (
	focusTo focus = iota
	focusSubject
	focusBody
)

type screen int

const (
	screenCompose screen = iota
	screenConfig
)

type cfgFocus int

const (
	cfgFocusSMTP cfgFocus = iota
	cfgFocusSecurity
	cfgFocusUser
	cfgFocusPassword
	cfgFocusName
	cfgFocusFrom
	cfgFocusSignature
	cfgFieldCount
)

// outcome is what main prints after the TUI closes.
type outcome struct {
	cancelled bool
	sent      bool
	to        string
	err       error
}

type sentMsg struct {
	to  string
	err error
}

const composeHelp = "^S Send+Exit   ^X Config   ^Up/^Down Switch Config"

type model struct {
	cfg              Config
	cfgs             []Config
	cfgIndex         int
	cfgConfirmDelete bool
	addrs            []string
	to               textinput.Model
	subject          textinput.Model
	body             textarea.Model
	focus            focus
	suggestions      []string
	suggestIdx       int
	suggestHidden    bool
	status           string
	sending          bool
	width            int
	height           int
	outcome          outcome
	configPath       string
	screen           screen
	connTestID       int
	connBanner       connBanner
	connErr          string
	cfgFocus         cfgFocus
	cfgSMTP          textinput.Model
	cfgSecurity      textinput.Model
	cfgUser          textinput.Model
	cfgPassword      textinput.Model
	cfgName          textinput.Model
	cfgFrom          textinput.Model
	cfgSignature     textinput.Model
}

func newModel(cfgs []Config, addrs []string) *model {
	if len(cfgs) == 0 {
		cfgs = []Config{defaultConfig()}
	}
	cfg := cfgs[0]

	to := textinput.New()
	to.Prompt = ""
	to.Width = 20

	subject := textinput.New()
	subject.Prompt = ""
	subject.Width = 20
	subject.TextStyle = subjectTextStyle
	subject.Cursor.Style = subjectTextStyle
	subject.Cursor.TextStyle = subjectTextStyle

	body := textarea.New()
	body.Prompt = ""
	body.ShowLineNumbers = false
	// Defaults cap a message at 400 characters and 99 lines.
	body.CharLimit = 0
	body.MaxHeight = 0
	body.MaxWidth = 0
	seedMessage(&body, cfg.Signature)

	cfgSMTP := plainInput()
	cfgSecurity := plainInput()
	cfgUser := plainInput()
	cfgPassword := plainInput()
	cfgPassword.EchoMode = textinput.EchoPassword
	cfgPassword.EchoCharacter = '*'
	cfgName := plainInput()
	cfgFrom := plainInput()
	cfgSignature := plainInput()

	m := &model{
		cfg:          cfg,
		cfgs:         cfgs,
		addrs:        addrs,
		to:           to,
		subject:      subject,
		body:         body,
		focus:        focusTo,
		cfgSMTP:      cfgSMTP,
		cfgSecurity:  cfgSecurity,
		cfgUser:      cfgUser,
		cfgPassword:  cfgPassword,
		cfgName:      cfgName,
		cfgFrom:      cfgFrom,
		cfgSignature: cfgSignature,
	}
	m.to.Focus()
	return m
}

func plainInput() textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.Width = 20
	return in
}

func (m *model) Init() tea.Cmd {
	return textinput.Blink
}

func runTUI(configPath string, cfgs []Config, addrs []string) (outcome, error) {
	m := newModel(cfgs, addrs)
	m.configPath = configPath
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return outcome{}, err
	}
	return final.(*model).outcome, nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case sentMsg:
		m.sending = false
		m.outcome = outcome{sent: msg.err == nil, to: msg.to, err: msg.err}
		return m, tea.Quit
	case connTestMsg:
		return m.finishConnTest(msg)
	case connBannerClearMsg:
		if msg.id != m.connTestID || m.connBanner == connBannerConnecting {
			return m, nil
		}
		m.connBanner = connBannerNone
		return m, nil
	case tea.KeyMsg:
		if m.sending {
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			m.outcome = outcome{cancelled: true}
			return m, tea.Quit
		}
		if m.screen == screenConfig && m.cfgConfirmDelete {
			if msg.String() == "y" || msg.String() == "Y" {
				return m, m.deleteConfig()
			}
			m.cfgConfirmDelete = false
			return m, nil
		}
		switch msg.String() {
		case "ctrl+x":
			if m.screen == screenConfig {
				return m, nil
			}
			return m, m.openConfig()
		case "ctrl+s":
			if m.screen == screenConfig {
				return m, m.saveConfig()
			}
			return m.startSend()
		case "ctrl+t":
			if m.screen != screenConfig {
				return m, nil
			}
			return m, m.startConnTest()
		case "ctrl+n":
			if m.screen != screenConfig {
				return m, nil
			}
			return m, m.addConfig()
		case "ctrl+d":
			if m.screen == screenConfig {
				m.askDeleteConfig()
				return m, nil
			}
		case "ctrl+up":
			if m.screen != screenCompose {
				return m, nil
			}
			m.switchConfig(-1)
			return m, nil
		case "ctrl+down":
			if m.screen != screenCompose {
				return m, nil
			}
			m.switchConfig(1)
			return m, nil
		}
		if m.screen == screenConfig {
			return m.updateConfigKeys(msg)
		}
		switch msg.String() {
		case "tab":
			if m.canCycleSuggestions() {
				m.moveSuggest(1)
				return m, nil
			}
			m.cycle(1)
			return m, m.applyFocus()
		case "shift+tab":
			if m.canCycleSuggestions() {
				m.moveSuggest(-1)
				return m, nil
			}
			m.cycle(-1)
			return m, m.applyFocus()
		case "up":
			if m.showSuggestions() {
				m.moveSuggest(-1)
				return m, nil
			}
			if m.focus != focusBody {
				m.cycle(-1)
				return m, m.applyFocus()
			}
		case "down":
			if m.showSuggestions() {
				m.moveSuggest(1)
				return m, nil
			}
			if m.focus != focusBody {
				m.cycle(1)
				return m, m.applyFocus()
			}
		case "enter":
			if m.focus != focusBody {
				if m.showSuggestions() {
					m.acceptSuggestion()
				}
				m.cycle(1)
				return m, m.applyFocus()
			}
		}
		return m.passInput(msg)
	default:
		return m.passInput(msg)
	}
}

func (m *model) passInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.screen == screenConfig {
		return m.passConfigInput(msg)
	}
	var cmd tea.Cmd
	switch m.focus {
	case focusTo:
		prev := m.to.Value()
		m.to, cmd = m.to.Update(msg)
		if m.to.Value() != prev {
			m.suggestHidden = false
			m.status = ""
			m.refreshSuggestions()
		}
	case focusSubject:
		prev := m.subject.Value()
		m.subject, cmd = m.subject.Update(msg)
		if m.subject.Value() != prev {
			m.status = ""
		}
	default:
		prev := m.body.Value()
		m.body, cmd = m.body.Update(msg)
		if m.body.Value() != prev {
			m.resize()
		}
	}
	return m, cmd
}

func (m *model) startSend() (tea.Model, tea.Cmd) {
	addr, subject, err := validateMail(m.to.Value(), m.subject.Value())
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.status = ""
	m.sending = true
	body := m.body.Value()
	cfg := m.cfg
	return m, func() tea.Msg {
		return sentMsg{to: addr, err: sendMail(cfg, addr, subject, body)}
	}
}

func validateMail(to, subject string) (string, string, error) {
	to = strings.TrimSpace(to)
	subject = strings.TrimSpace(subject)
	if to == "" {
		return "", "", fmt.Errorf("To is empty")
	}
	addr, err := mail.ParseAddress(to)
	if err != nil || addr.Address == "" || strings.ContainsAny(addr.Address, "\r\n") {
		return "", "", fmt.Errorf("To is not an email address")
	}
	if subject == "" {
		return "", "", fmt.Errorf("Subject is empty")
	}
	if strings.ContainsAny(subject, "\r\n") {
		return "", "", fmt.Errorf("Subject contains a line break")
	}
	return addr.Address, subject, nil
}

func (m *model) cycle(dir int) {
	next := (int(m.focus) + dir) % 3
	if next < 0 {
		next += 3
	}
	m.focus = focus(next)
	m.resize()
}

func (m *model) applyFocus() tea.Cmd {
	m.to.Blur()
	m.subject.Blur()
	m.body.Blur()
	m.cfgSMTP.Blur()
	m.cfgSecurity.Blur()
	m.cfgUser.Blur()
	m.cfgPassword.Blur()
	m.cfgName.Blur()
	m.cfgFrom.Blur()
	m.cfgSignature.Blur()
	if m.screen == screenConfig {
		switch m.cfgFocus {
		case cfgFocusSMTP:
			return m.cfgSMTP.Focus()
		case cfgFocusSecurity:
			return m.cfgSecurity.Focus()
		case cfgFocusUser:
			return m.cfgUser.Focus()
		case cfgFocusPassword:
			return m.cfgPassword.Focus()
		case cfgFocusName:
			return m.cfgName.Focus()
		case cfgFocusFrom:
			return m.cfgFrom.Focus()
		default:
			return m.cfgSignature.Focus()
		}
	}
	switch m.focus {
	case focusTo:
		return m.to.Focus()
	case focusSubject:
		return m.subject.Focus()
	default:
		return m.body.Focus()
	}
}

func (m *model) refreshSuggestions() {
	m.suggestions = FilterAddresses(m.addrs, m.to.Value())
	if rows := m.suggestionRows(); m.suggestIdx >= rows {
		m.suggestIdx = 0
	}
	m.resize()
}

func (m *model) showSuggestions() bool {
	return m.focus == focusTo && !m.suggestHidden && m.suggestionRows() > 0
}

// canCycleSuggestions is true when Tab moves through hints instead of leaving To.
func (m *model) canCycleSuggestions() bool {
	return m.showSuggestions() && m.suggestionRows() > 1
}

func (m *model) suggestionRows() int {
	n := len(m.suggestions)
	if n > maxSuggestions {
		n = maxSuggestions
	}
	if m.height > 0 {
		// Keep one body row plus the fixed chrome.
		maxRows := m.height - m.fixedChrome() - 1
		if maxRows < 1 {
			return 0
		}
		if n > maxRows {
			n = maxRows
		}
	}
	return n
}

func (m *model) moveSuggest(dir int) {
	n := m.suggestionRows()
	if n == 0 {
		return
	}
	m.suggestIdx = (m.suggestIdx + dir + n) % n
}

func (m *model) acceptSuggestion() {
	rows := m.suggestionRows()
	if m.suggestIdx < 0 || m.suggestIdx >= rows {
		return
	}
	m.to.SetValue(m.suggestions[m.suggestIdx])
	m.to.CursorEnd()
	m.suggestHidden = true
	m.status = ""
	m.resize()
}

func (m *model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	if m.screen == screenConfig {
		m.resizeConfig()
		return
	}
	field := m.width - len(subjectLabel) - 1
	if field < 1 {
		field = 1
	}
	m.to.Width = field
	m.subject.Width = field

	// The textarea is only the editor. The screen paints Value() itself, so the
	// widget's own viewport must be tall enough that Enter does not scroll the
	// first line out of its internal buffer before we can grow it.
	bodyH := m.height - m.chromeLines()
	if bodyH < 1 {
		bodyH = 1
	}
	m.body.SetWidth(m.width)
	m.body.SetHeight(bodyH)
}

const subjectLabel = "Subject: "

// toLabel right-aligns "To:" so its colon is in the same column as "Subject:".
func toLabel() string {
	pad := len("Subject:") - len("To:")
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + "To: "
}

// fixedChrome is every reserved row except the suggestion list and the editable body.
// To, Subject, blank, "---", blank, status, help.
func (m *model) fixedChrome() int {
	return 7
}

// switchConfig selects another saved config for sending. The message stays as it is.
func (m *model) switchConfig(dir int) {
	n := len(m.cfgs)
	if n == 0 {
		return
	}
	if m.cfgIndex < 0 || m.cfgIndex >= n {
		m.cfgIndex = 0
	}
	m.cfgs[m.cfgIndex] = m.cfg
	m.cfgIndex = (m.cfgIndex + dir) % n
	if m.cfgIndex < 0 {
		m.cfgIndex += n
	}
	m.cfg = m.cfgs[m.cfgIndex]
}

func (m *model) chromeLines() int {
	n := m.fixedChrome()
	if m.showSuggestions() {
		n += m.suggestionRows()
	}
	return n
}

func (m *model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.screen == screenConfig {
		return m.configView()
	}
	return m.composeView()
}

func (m *model) composeView() string {
	lines := []string{placeRight(toLabel()+m.to.View(), m.cfg.SMTP, m.width)}
	if m.showSuggestions() {
		rows := m.suggestionRows()
		for i := 0; i < rows; i++ {
			prefix := "  "
			if i == m.suggestIdx {
				prefix = "> "
			}
			lines = append(lines, prefix+m.suggestions[i])
		}
	}
	lines = append(lines, placeRight(subjectLabel+m.subject.View(), m.cfg.SenderFrom, m.width), "", "---", "")
	head := len(lines)

	bodyLines := m.bodyVisualLines()
	overflow := head + len(bodyLines) - (m.height - 2)
	if overflow > 0 && overflow < len(bodyLines) {
		bodyLines = bodyLines[overflow:]
	}
	lines = append(lines, bodyLines...)

	help := composeHelp
	if m.sending {
		help = "sending…"
	}
	status := ""
	if !m.sending {
		status = m.status
	}
	return m.finishView(lines, status, help)
}

func (m *model) finishView(lines []string, footer ...string) string {
	reserve := len(footer)
	if reserve > m.height {
		footer = footer[len(footer)-m.height:]
		reserve = m.height
	}
	room := m.height - reserve
	for len(lines) < room {
		lines = append(lines, "")
	}
	if len(lines) > room {
		lines = lines[:room]
	}
	lines = append(lines, footer...)
	return strings.Join(lines, "\n")
}

// seedMessage places the signature in the message, two lines below the cursor,
// so it can be edited with the rest of the text.
func seedMessage(body *textarea.Model, signature string) {
	signature = strings.TrimRight(signature, "\n")
	if strings.TrimSpace(signature) == "" {
		return
	}
	body.SetValue("\n\n" + signature)
	for i := 0; i < body.LineCount() && body.Line() > 0; i++ {
		body.CursorUp()
	}
	body.CursorStart()
}

// bodyVisualLines paints the message from its text. The textarea viewport
// scrolls on Enter while its height is still the old value, which hid the
// first line and let its padding collide with the signature.
func (m *model) bodyVisualLines() []string {
	logical := strings.Split(m.body.Value(), "\n")
	row := -1
	col := 0
	if m.body.Focused() {
		row = m.body.Line()
		info := m.body.LineInfo()
		col = info.StartColumn + info.ColumnOffset
	}
	width := m.width
	if width < 1 {
		width = 1
	}

	var lines []string
	cursorAt := -1
	for i, line := range logical {
		cursor := -1
		if i == row {
			cursor = col
		}
		wrapped, at := wrapBodyLine(line, width, cursor)
		if cursor >= 0 {
			cursorAt = len(lines) + at
		}
		lines = append(lines, wrapped...)
	}

	maxBody := m.height - m.chromeLines()
	if maxBody < 1 {
		maxBody = 1
	}
	if len(lines) <= maxBody {
		return lines
	}
	start := len(lines) - maxBody
	if cursorAt >= 0 {
		start = cursorAt - maxBody + 1
		if start < 0 {
			start = 0
		}
		if start > len(lines)-maxBody {
			start = len(lines) - maxBody
		}
	}
	return lines[start : start+maxBody]
}

// wrapBodyLine breaks a logical line into screen rows. cursor is the rune
// index to paint, or -1 when this line has no cursor. The second return is
// the row within the result that holds the cursor.
func wrapBodyLine(line string, width, cursor int) ([]string, int) {
	if width < 1 {
		width = 1
	}
	runes := []rune(line)
	if cursor > len(runes) {
		cursor = len(runes)
	}
	cursorAt := -1
	var out []string
	for start := 0; ; start += width {
		end := start + width
		if end > len(runes) {
			end = len(runes)
		}
		chunk := runes[start:end]
		if cursor >= start && cursor <= end && (cursor < end || end == len(runes)) {
			local := cursor - start
			ch := " "
			var rest []rune
			if local < len(chunk) {
				ch = string(chunk[local])
				rest = chunk[local+1:]
			}
			out = append(out, string(chunk[:local])+cursorStyle.Render(ch)+string(rest))
			cursorAt = len(out) - 1
		} else if len(chunk) == 0 {
			out = append(out, "")
		} else {
			out = append(out, string(chunk))
		}
		if end == len(runes) {
			break
		}
	}
	return out, cursorAt
}
