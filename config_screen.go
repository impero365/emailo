package main

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const configHelp = "^S Save   ^N New   ^D Delete   ^T Test Connection"

const deleteConfirm = "Confirm deletion? (y/N)"

type connBanner int

const (
	connBannerNone connBanner = iota
	connBannerConnecting
	connBannerOK
	connBannerFailed
)

type connTestMsg struct {
	id  int
	err error
}

type connBannerClearMsg struct {
	id int
}

// connBannerDuration is how long a finished connection result stays on screen.
var connBannerDuration = 3 * time.Second

var (
	connConnectingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	connOKStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	connFailedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

var smtpSecurities = []string{"plain", "starttls", "tls"}

func securityChoice(v string) string {
	for _, s := range smtpSecurities {
		if s == v {
			return s
		}
	}
	return "plain"
}

func securityLabel(v string) string {
	return "< " + securityChoice(v) + " >"
}

// securityView draws the choice. While the field is focused the whole value
// blinks in reverse, on the same timer as the text cursor.
func (m *model) securityView() string {
	label := securityLabel(m.cfgSecurity.Value())
	if m.cfgFocus == cfgFocusSecurity && !m.cfgSecurity.Cursor.Blink {
		return cursorStyle.Render(label)
	}
	return label
}

// cfgLabel right-aligns a config key so its colon lines up with the others.
func cfgLabel(key string) string {
	const widest = len("smtp_password")
	pad := widest - len(key)
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + key + ": "
}

func (m *model) openConfig() tea.Cmd {
	m.screen = screenConfig
	m.cfgFocus = cfgFocusSMTP
	m.status = ""
	m.connErr = ""
	m.cfgConfirmDelete = false
	if m.cfgIndex >= 0 && m.cfgIndex < len(m.cfgs) {
		m.cfgs[m.cfgIndex] = m.cfg
	}
	m.loadConfigForm()
	return m.applyFocus()
}

// loadConfigForm copies the active config into the fields.
// Width must be set before the values, or a long signature scrolls and hides its start.
func (m *model) loadConfigForm() {
	m.resize()
	m.cfgSMTP.SetValue(m.cfg.SMTP)
	m.cfgSMTP.CursorEnd()
	m.cfgSecurity.SetValue(securityChoice(m.cfg.SMTPSecurity))
	m.cfgUser.SetValue(m.cfg.SMTPUser)
	m.cfgUser.CursorEnd()
	m.cfgPassword.SetValue(m.cfg.SMTPPassword)
	m.cfgPassword.CursorEnd()
	m.cfgName.SetValue(m.cfg.SenderName)
	m.cfgName.CursorEnd()
	m.cfgFrom.SetValue(m.cfg.SenderFrom)
	m.cfgFrom.CursorEnd()
	// Show the signature as it is stored in the file, with \n \t \s left written out.
	m.cfgSignature.SetValue(escapeConfig(m.cfg.Signature))
	m.cfgSignature.CursorEnd()
}

func (m *model) formConfig() (Config, error) {
	security := securityChoice(m.cfgSecurity.Value())
	user := m.cfgUser.Value()
	password := m.cfgPassword.Value()
	if security == "plain" {
		user, password = "", ""
	}
	return validateConfig(Config{
		SenderName:   m.cfgName.Value(),
		SenderFrom:   m.cfgFrom.Value(),
		SMTP:         m.cfgSMTP.Value(),
		SMTPSecurity: security,
		SMTPUser:     user,
		SMTPPassword: password,
		Signature:    unescapeConfig(m.cfgSignature.Value()),
	})
}

func (m *model) saveConfig() tea.Cmd {
	cfg, err := m.formConfig()
	if err != nil {
		m.status = err.Error()
		return nil
	}
	if m.cfgIndex < 0 || m.cfgIndex >= len(m.cfgs) {
		m.cfgIndex = 0
		m.cfgs = []Config{cfg}
	} else {
		m.cfgs[m.cfgIndex] = cfg
	}
	if err := SaveConfig(m.configPath, m.cfgs); err != nil {
		m.status = "could not save config: " + err.Error()
		return nil
	}
	m.cfg = cfg
	m.cfgConfirmDelete = false
	m.screen = screenCompose
	m.status = ""
	m.resize()
	return m.applyFocus()
}

func (m *model) addConfig() tea.Cmd {
	cfg, err := m.formConfig()
	if err != nil {
		m.status = err.Error()
		return nil
	}
	if m.cfgIndex < 0 || m.cfgIndex >= len(m.cfgs) {
		m.cfgs = []Config{cfg}
		m.cfgIndex = 0
	} else {
		m.cfgs[m.cfgIndex] = cfg
	}
	m.cfgs = append(m.cfgs, defaultConfig())
	m.cfgIndex = len(m.cfgs) - 1
	m.cfg = m.cfgs[m.cfgIndex]
	m.status = ""
	m.connErr = ""
	m.connBanner = connBannerNone
	m.connTestID++
	m.cfgFocus = cfgFocusSMTP
	m.loadConfigForm()
	return m.applyFocus()
}

func (m *model) askDeleteConfig() {
	if len(m.cfgs) < 2 {
		m.status = "cannot delete the only config"
		m.cfgConfirmDelete = false
		return
	}
	m.status = ""
	m.cfgConfirmDelete = true
}

func (m *model) deleteConfig() tea.Cmd {
	if len(m.cfgs) < 2 || m.cfgIndex < 0 || m.cfgIndex >= len(m.cfgs) {
		m.cfgConfirmDelete = false
		m.status = "cannot delete the only config"
		return nil
	}
	m.cfgs = append(m.cfgs[:m.cfgIndex], m.cfgs[m.cfgIndex+1:]...)
	if m.cfgIndex >= len(m.cfgs) {
		m.cfgIndex = len(m.cfgs) - 1
	}
	m.cfg = m.cfgs[m.cfgIndex]
	m.cfgConfirmDelete = false
	m.status = ""
	m.connErr = ""
	m.connBanner = connBannerNone
	m.connTestID++
	m.cfgFocus = cfgFocusSMTP
	m.loadConfigForm()
	return m.applyFocus()
}

func (m *model) updateConfigKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		m.cycleConfig(1)
		return m, m.applyFocus()
	case "shift+tab":
		m.cycleConfig(-1)
		return m, m.applyFocus()
	case "enter", "down":
		m.cycleConfig(1)
		return m, m.applyFocus()
	case "up":
		m.cycleConfig(-1)
		return m, m.applyFocus()
	case "left":
		if m.cfgFocus == cfgFocusSecurity {
			return m, m.cycleSecurity(-1)
		}
	case "right":
		if m.cfgFocus == cfgFocusSecurity {
			return m, m.cycleSecurity(1)
		}
	}
	return m.passInput(msg)
}

func (m *model) cycleSecurity(dir int) tea.Cmd {
	cur := securityChoice(m.cfgSecurity.Value())
	idx := 0
	for i, s := range smtpSecurities {
		if s == cur {
			idx = i
			break
		}
	}
	n := len(smtpSecurities)
	idx = (idx + dir) % n
	if idx < 0 {
		idx += n
	}
	m.cfgSecurity.SetValue(smtpSecurities[idx])
	m.status = ""
	m.cfgSecurity.Cursor.Blink = false
	return m.cfgSecurity.Cursor.BlinkCmd()
}

func (m *model) configFields() []cfgFocus {
	fields := []cfgFocus{cfgFocusSMTP, cfgFocusSecurity}
	if securityChoice(m.cfgSecurity.Value()) != "plain" {
		fields = append(fields, cfgFocusUser, cfgFocusPassword)
	}
	return append(fields, cfgFocusName, cfgFocusFrom, cfgFocusSignature)
}

func (m *model) cycleConfig(dir int) {
	fields := m.configFields()
	idx := 0
	for i, f := range fields {
		if f == m.cfgFocus {
			idx = i
			break
		}
	}
	idx = (idx + dir) % len(fields)
	if idx < 0 {
		idx += len(fields)
	}
	m.cfgFocus = fields[idx]
	m.resize()
}

func (m *model) passConfigInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.cfgFocus {
	case cfgFocusSMTP:
		prev := m.cfgSMTP.Value()
		m.cfgSMTP, cmd = m.cfgSMTP.Update(msg)
		if m.cfgSMTP.Value() != prev {
			m.status = ""
		}
	case cfgFocusSecurity:
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, nil
		}
		m.cfgSecurity, cmd = m.cfgSecurity.Update(msg)
		return m, cmd
	case cfgFocusUser:
		prev := m.cfgUser.Value()
		m.cfgUser, cmd = m.cfgUser.Update(msg)
		if m.cfgUser.Value() != prev {
			m.status = ""
		}
	case cfgFocusPassword:
		prev := m.cfgPassword.Value()
		m.cfgPassword, cmd = m.cfgPassword.Update(msg)
		if m.cfgPassword.Value() != prev {
			m.status = ""
		}
	case cfgFocusName:
		prev := m.cfgName.Value()
		m.cfgName, cmd = m.cfgName.Update(msg)
		if m.cfgName.Value() != prev {
			m.status = ""
		}
	case cfgFocusFrom:
		prev := m.cfgFrom.Value()
		m.cfgFrom, cmd = m.cfgFrom.Update(msg)
		if m.cfgFrom.Value() != prev {
			m.status = ""
		}
	default:
		prev := m.cfgSignature.Value()
		m.cfgSignature, cmd = m.cfgSignature.Update(msg)
		if m.cfgSignature.Value() != prev {
			m.status = ""
		}
	}
	return m, cmd
}

func (m *model) resizeConfig() {
	field := m.width - len(cfgLabel("smtp_password"))
	if field < 1 {
		field = 1
	}
	m.cfgSMTP.Width = field
	m.cfgSecurity.Width = field
	m.cfgUser.Width = field
	m.cfgPassword.Width = field
	m.cfgName.Width = field
	m.cfgFrom.Width = field
	m.cfgSignature.Width = field
}

func (m *model) configPosLabel() string {
	n := len(m.cfgs)
	if n < 1 {
		n = 1
	}
	idx := m.cfgIndex + 1
	if idx < 1 {
		idx = 1
	}
	if idx > n {
		idx = n
	}
	return fmt.Sprintf("[%d/%d]", idx, n)
}

func (m *model) configView() string {
	lines := []string{
		m.configPosLabel(),
		m.placeConnBanner(cfgLabel("smtp") + m.cfgSMTP.View()),
		cfgLabel("smtp_security") + m.securityView(),
	}
	if securityChoice(m.cfgSecurity.Value()) != "plain" {
		lines = append(lines,
			cfgLabel("smtp_user")+m.cfgUser.View(),
			cfgLabel("smtp_password")+m.cfgPassword.View(),
		)
	}
	lines = append(lines,
		cfgLabel("sender_name")+m.cfgName.View(),
		cfgLabel("sender_from")+m.cfgFrom.View(),
		cfgLabel("signature")+m.cfgSignature.View(),
	)
	if m.connErr != "" {
		lines = append(lines, "", connFailedStyle.Render("ERROR: "+oneLine(m.connErr)))
	}
	help := configHelp
	if m.cfgConfirmDelete {
		help = deleteConfirm
	}
	return m.finishView(lines, m.status, help)
}

// oneLine keeps a connection error on the single row under the config block.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "\r", " ")
}

func (m *model) startConnTest() tea.Cmd {
	m.connTestID++
	id := m.connTestID
	m.connBanner = connBannerConnecting
	m.connErr = ""
	cfg := Config{
		SMTP:         strings.TrimSpace(m.cfgSMTP.Value()),
		SMTPSecurity: securityChoice(m.cfgSecurity.Value()),
		SMTPUser:     strings.TrimSpace(m.cfgUser.Value()),
		SMTPPassword: m.cfgPassword.Value(),
	}
	return func() tea.Msg {
		return connTestMsg{id: id, err: testConnection(cfg)}
	}
}

func (m *model) finishConnTest(msg connTestMsg) (tea.Model, tea.Cmd) {
	if msg.id != m.connTestID {
		return m, nil
	}
	m.connBanner = connBannerOK
	m.connErr = ""
	if msg.err != nil {
		m.connBanner = connBannerFailed
		m.connErr = msg.err.Error()
	}
	id := msg.id
	return m, func() tea.Msg {
		time.Sleep(connBannerDuration)
		return connBannerClearMsg{id: id}
	}
}

func (m *model) connBannerText() (string, lipgloss.Style, bool) {
	switch m.connBanner {
	case connBannerConnecting:
		return "CONNECTING", connConnectingStyle, true
	case connBannerOK:
		return "CONNECTION OK", connOKStyle, true
	case connBannerFailed:
		return "CONNECTION FAILED", connFailedStyle, true
	default:
		return "", lipgloss.Style{}, false
	}
}

func (m *model) placeConnBanner(line string) string {
	text, style, ok := m.connBannerText()
	if !ok || m.width <= 0 {
		return line
	}
	return placeRight(line, style.Render(text), m.width)
}

// placeRight keeps text flush with the right edge of width.
// A later redraw with a new width moves it with the window.
func placeRight(line, text string, width int) string {
	if text == "" || width <= 0 {
		return line
	}
	room := width - visibleWidth(text)
	if room < 1 {
		return text
	}
	line = truncateVisible(line, room-1)
	if strings.Contains(line, "\x1b") {
		line += "\x1b[0m"
	}
	pad := width - visibleWidth(line) - visibleWidth(text)
	if pad < 1 {
		pad = 1
	}
	return line + strings.Repeat(" ", pad) + text
}

func visibleWidth(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !(s[i] >= 0x40 && s[i] <= 0x7e) {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		n++
		i += size
	}
	return n
}

func truncateVisible(s string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(s) && n < width; {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
				j++
			}
			if j < len(s) {
				j++
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		b.WriteString(s[i : i+size])
		n++
		i += size
	}
	return b.String()
}
