package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func readyModel(addrs []string) *model {
	m := newModel([]Config{testConfig()}, addrs)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

func TestSubjectTextIsBlue(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	m := readyModel(nil)
	m.subject.SetValue("hello")
	line := strings.Split(m.View(), "\n")[1]
	if !strings.Contains(line, "\x1b[94mhello") {
		t.Fatalf("subject text should be blue: %q", line)
	}
	if !strings.HasPrefix(line, "Subject: ") {
		t.Fatalf("label should stay uncolored: %q", line)
	}
}

func TestToTextAlignsWithSubject(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	m := readyModel(nil)
	m.to.SetValue("person@example.com")
	m.subject.SetValue("hello")
	lines := plainLines(m.View())
	toCol := strings.Index(lines[0], "person@example.com")
	subjectCol := strings.Index(lines[1], "hello")
	if toCol < 0 || toCol != subjectCol {
		t.Fatalf("to column %d, subject column %d\n%s\n%s", toCol, subjectCol, lines[0], lines[1])
	}
	if toColon, subjectColon := strings.Index(lines[0], ":"), strings.Index(lines[1], ":"); toColon < 0 || toColon != subjectColon {
		t.Fatalf("colon columns %d and %d\n%s\n%s", toColon, subjectColon, lines[0], lines[1])
	}
}

func TestViewHelpPinned(t *testing.T) {
	m := readyModel(nil)
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 24 {
		t.Fatalf("lines: %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], toLabel()) {
		t.Fatalf("first line: %q", lines[0])
	}
	if lines[1] != "Subject: " && !strings.HasPrefix(lines[1], "Subject: ") {
		t.Fatalf("subject line: %q", lines[1])
	}
	if lines[2] != "" {
		t.Fatalf("blank line under Subject: %q", lines[2])
	}
	if lines[3] != "---" {
		t.Fatalf("separator: %q", lines[3])
	}
	if lines[4] != "" {
		t.Fatalf("blank line under ---: %q", lines[4])
	}
	if lines[len(lines)-1] != composeHelp {
		t.Fatalf("help: %q", lines[len(lines)-1])
	}
	if lines[len(lines)-2] != "" {
		t.Fatalf("status line: %q", lines[len(lines)-2])
	}
	if !strings.HasSuffix(lines[0], "smtp.mpero.net:25") || lipgloss.Width(lines[0]) != 80 {
		t.Fatalf("smtp label: width %d line %q", lipgloss.Width(lines[0]), lines[0])
	}
	if !strings.HasSuffix(lines[1], "noreply@mpero.net") || lipgloss.Width(lines[1]) != 80 {
		t.Fatalf("from label: width %d line %q", lipgloss.Width(lines[1]), lines[1])
	}

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	lines = strings.Split(m.View(), "\n")
	if !strings.HasSuffix(lines[0], "smtp.mpero.net:25") || lipgloss.Width(lines[0]) != 100 {
		t.Fatalf("smtp label after resize: width %d line %q", lipgloss.Width(lines[0]), lines[0])
	}
	if !strings.HasSuffix(lines[1], "noreply@mpero.net") || lipgloss.Width(lines[1]) != 100 {
		t.Fatalf("from label after resize: width %d line %q", lipgloss.Width(lines[1]), lines[1])
	}
}

func modelWithSignature() *model {
	cfg := testConfig()
	cfg.Signature = "Best regards,\n\tMattia Peronio"
	m := newModel([]Config{cfg}, nil)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

func TestSignatureIsEditableMessageText(t *testing.T) {
	m := modelWithSignature()
	if m.body.Line() != 0 {
		t.Fatalf("cursor should start above the signature, line %d", m.body.Line())
	}
	value := m.body.Value()
	if !strings.HasPrefix(value, "\n\nBest regards,\n") || !strings.Contains(value, "Mattia Peronio") {
		t.Fatalf("signature should be two lines below the cursor: %q", value)
	}

	lines := plainLines(m.View())
	sep := lineIndex(lines, "---")
	sigAt := lineIndex(lines, "Best regards,")
	if sep < 0 || sigAt != sep+4 {
		t.Fatalf("signature should be in the message area:\n%s", strings.Join(lines[sep:], "\n"))
	}

	// Editing the signature changes the text that will be sent.
	m.Update(key(tea.KeyTab))
	m.Update(key(tea.KeyTab))
	for m.body.Line() < 2 {
		m.Update(key(tea.KeyDown))
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m = next.(*model)
	if !strings.Contains(m.body.Value(), "XBest regards,") && !strings.Contains(m.body.Value(), "Best regards,X") {
		// Cursor may land at the start of the signature line.
		if !strings.Contains(m.body.Value(), "X") || !strings.Contains(m.body.Value(), "Best regards,") {
			t.Fatalf("signature was not edited: %q", m.body.Value())
		}
	}
}

func TestEnterKeepsBodyLineAndSignature(t *testing.T) {
	m := modelWithSignature()
	m.Update(key(tea.KeyTab))
	m.Update(key(tea.KeyTab))
	if !m.body.Focused() {
		t.Fatal("body should be focused")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	m = next.(*model)
	if !strings.HasPrefix(m.body.Value(), "hello\n\nBest regards,") {
		t.Fatalf("value: %q", m.body.Value())
	}
	next, _ = m.Update(key(tea.KeyEnter))
	m = next.(*model)

	lines := plainLines(m.View())
	helloAt := -1
	for i, line := range lines {
		if strings.Contains(line, "hello") {
			helloAt = i
			break
		}
	}
	if helloAt < 0 {
		t.Fatalf("first line disappeared:\n%s", strings.Join(lines, "\n"))
	}
	sigAt := lineIndex(lines, "Best regards,")
	if sigAt < 0 || !strings.Contains(lines[sigAt+1], "Mattia Peronio") {
		t.Fatalf("signature corrupted:\n%s", strings.Join(lines, "\n"))
	}
	if sigAt <= helloAt {
		t.Fatalf("signature should stay below the message:\n%s", strings.Join(lines[helloAt:], "\n"))
	}
}

func TestCtrlSValidationStaysOpen(t *testing.T) {
	m := readyModel(nil)
	next, cmd := m.Update(key(tea.KeyCtrlS))
	m = next.(*model)
	if cmd != nil {
		t.Fatal("expected to stay open")
	}
	if m.status != "To is empty" {
		t.Fatalf("status: %q", m.status)
	}

	m.to.SetValue("not-an-email")
	next, cmd = m.Update(key(tea.KeyCtrlS))
	m = next.(*model)
	if cmd != nil || m.status != "To is not an email address" {
		t.Fatalf("cmd %v status %q", cmd, m.status)
	}

	m.to.SetValue("dest@example.com")
	next, cmd = m.Update(key(tea.KeyCtrlS))
	m = next.(*model)
	if cmd != nil || m.status != "Subject is empty" {
		t.Fatalf("cmd %v status %q", cmd, m.status)
	}
	if m.outcome.sent || m.outcome.err != nil {
		t.Fatal("should not have finished")
	}
}

func TestSuggestionEnterFillsTo(t *testing.T) {
	m := readyModel([]string{"maria@example.com", "mattia@example.com", "other@zzz.test"})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ma")})
	m = next.(*model)
	if !m.showSuggestions() {
		t.Fatal("expected suggestions")
	}
	if got, want := m.suggestions, []string{"maria@example.com", "mattia@example.com"}; !sameStrings(got, want) {
		t.Fatalf("suggestions: %v", got)
	}

	if m.suggestIdx != 0 {
		t.Fatalf("first hint should start highlighted, idx %d", m.suggestIdx)
	}
	next, _ = m.Update(key(tea.KeyTab))
	m = next.(*model)
	if m.suggestIdx != 1 || m.focus != focusTo {
		t.Fatalf("tab should highlight the next hint, idx %d focus %d", m.suggestIdx, m.focus)
	}
	next, _ = m.Update(key(tea.KeyTab))
	m = next.(*model)
	if m.suggestIdx != 0 {
		t.Fatalf("tab should wrap to the first hint, idx %d", m.suggestIdx)
	}
	next, _ = m.Update(key(tea.KeyShiftTab))
	m = next.(*model)
	if m.suggestIdx != 1 {
		t.Fatalf("shift+tab should highlight the previous hint, idx %d", m.suggestIdx)
	}
	next, _ = m.Update(key(tea.KeyEnter))
	m = next.(*model)
	if m.to.Value() != "mattia@example.com" {
		t.Fatalf("to: %q", m.to.Value())
	}
	if m.showSuggestions() {
		t.Fatal("list should close after enter")
	}
	if !m.subject.Focused() {
		t.Fatal("enter should move from To to Subject")
	}
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 24 || lines[len(lines)-1] != composeHelp {
		t.Fatalf("layout drifted: %d lines, help %q", len(lines), lines[len(lines)-1])
	}
}

func TestTabLeavesToWhenOnlyOneHint(t *testing.T) {
	m := readyModel([]string{"maria@example.com", "other@zzz.test"})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("mar")})
	m = next.(*model)
	if !m.showSuggestions() || m.suggestionRows() != 1 {
		t.Fatalf("expected one hint, rows %d", m.suggestionRows())
	}
	next, _ = m.Update(key(tea.KeyTab))
	m = next.(*model)
	if !m.subject.Focused() {
		t.Fatal("tab should leave To when there is only one hint")
	}
	if m.to.Value() != "mar" {
		t.Fatalf("tab should not replace the typed address, got %q", m.to.Value())
	}
}

func TestBodyReceivesText(t *testing.T) {
	m := readyModel(nil)
	m.Update(key(tea.KeyTab))
	m.Update(key(tea.KeyTab))
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")})
	m = next.(*model)
	if m.body.Value() != "hi" {
		t.Fatalf("body: %q", m.body.Value())
	}
}

func TestCtrlSSendsAndQuits(t *testing.T) {
	addr, got := startRelay(t)
	m := readyModel(nil)
	m.cfg.SMTP = addr
	m.to.SetValue("Person <dest@example.com>")
	m.subject.SetValue("  Hello  ")
	m.body.SetValue("hi")

	next, cmd := m.Update(key(tea.KeyCtrlS))
	m = next.(*model)
	if !m.sending || cmd == nil {
		t.Fatal("expected send to start")
	}
	if !strings.Contains(m.View(), "sending…") {
		t.Fatal("help line should show sending")
	}
	msg := cmd().(sentMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if msg.to != "dest@example.com" {
		t.Fatalf("to: %q", msg.to)
	}
	body := waitRelay(t, got)
	if !strings.Contains(body, "Subject: Hello") || !strings.Contains(body, "hi") {
		t.Fatalf("relay body:\n%s", body)
	}

	next, quit := m.Update(msg)
	m = next.(*model)
	if !m.outcome.sent || m.outcome.err != nil || m.outcome.to != "dest@example.com" {
		t.Fatalf("outcome: %+v", m.outcome)
	}
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("expected quit after send")
	}
}

func TestCtrlXConfigSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	m := readyModel(nil)
	m.configPath = path
	m.cfg.Signature = "Best regards,\n\n  Mattia Peronio"
	m.to.SetValue("dest@example.com")
	m.subject.SetValue("Hello")
	m.body.SetValue("draft")

	next, _ := m.Update(key(tea.KeyCtrlX))
	m = next.(*model)
	if m.screen != screenConfig || !m.cfgSMTP.Focused() {
		t.Fatal("ctrl+x should open config on smtp")
	}
	lines := plainLines(m.View())
	if len(lines) != 24 || lines[0] != "[1/1]" || lines[len(lines)-1] != configHelp {
		t.Fatalf("help: %q indicator %q lines %d", lines[len(lines)-1], lines[0], len(lines))
	}
	smtpCol := strings.Index(lines[1], "smtp.mpero.net:25")
	secCol := strings.Index(lines[2], "< plain >")
	nameCol := strings.Index(lines[3], "Mattia Peronio")
	fromCol := strings.Index(lines[4], "noreply@mpero.net")
	sigCol := strings.Index(lines[5], `Best regards,\n\n\s\sMattia Peronio`)
	if smtpCol < 0 || smtpCol != secCol || smtpCol != nameCol || smtpCol != fromCol || smtpCol != sigCol {
		t.Fatalf("columns smtp %d security %d name %d from %d sig %d\n%s", smtpCol, secCol, nameCol, fromCol, sigCol, strings.Join(lines[:6], "\n"))
	}
	if strings.Contains(strings.Join(lines, "\n"), "smtp_user:") || strings.Contains(strings.Join(lines, "\n"), "smtp_password:") {
		t.Fatal("plain security should hide user and password")
	}

	m.cfgSMTP.SetValue("nope")
	next, cmd := m.Update(key(tea.KeyCtrlS))
	m = next.(*model)
	if cmd != nil || m.screen != screenConfig || !strings.Contains(m.status, "host:port") {
		t.Fatalf("invalid smtp should stay open, status %q", m.status)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid config was written")
	}

	next, cmd = m.Update(key(tea.KeyCtrlX))
	m = next.(*model)
	if cmd != nil || m.sending || m.screen != screenConfig {
		t.Fatal("ctrl+x should not save or leave the config view")
	}

	m.cfgSMTP.SetValue("smtp.example.net:2525")
	next, _ = m.Update(key(tea.KeyCtrlS))
	m = next.(*model)
	if m.screen != screenCompose || m.status != "" {
		t.Fatalf("save should return to compose, status %q", m.status)
	}
	if m.cfg.SMTP != "smtp.example.net:2525" || m.cfg.SenderFrom != "noreply@mpero.net" {
		t.Fatalf("cfg: %+v", m.cfg)
	}
	if m.body.Value() != "draft" {
		t.Fatalf("open message changed: %q", m.body.Value())
	}
	view := strings.Split(m.View(), "\n")
	if view[len(view)-1] != composeHelp {
		t.Fatalf("compose help: %q", view[len(view)-1])
	}
	if !strings.HasSuffix(view[0], "smtp.example.net:2525") || !strings.HasSuffix(view[1], "noreply@mpero.net") {
		t.Fatalf("config label:\n%s\n%s", view[0], view[1])
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].SMTP != "smtp.example.net:2525" || loaded[0].Signature != "Best regards,\n\n  Mattia Peronio" {
		t.Fatalf("saved: %+v", loaded)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantFile := "smtp=smtp.example.net:2525\nsmtp_security=plain\nsmtp_user=\nsmtp_password=\nsender_name=Mattia Peronio\nsender_from=noreply@mpero.net\nsignature=Best regards,\\n\\n\\s\\sMattia Peronio\n"
	if string(saved) != wantFile {
		t.Fatalf("file:\n%s", saved)
	}
}

func TestPasswordIsMasked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	m := readyModel(nil)
	m.configPath = path
	m.cfg.SMTPSecurity = "starttls"
	m.cfg.SMTPPassword = "secret"
	next, _ := m.Update(key(tea.KeyCtrlX))
	m = next.(*model)
	for i := 0; i < 3; i++ {
		next, _ = m.Update(key(tea.KeyTab))
		m = next.(*model)
	}
	if m.cfgFocus != cfgFocusPassword {
		t.Fatal("tab should focus smtp_password")
	}
	line := plainLines(m.View())[4]
	if strings.Contains(line, "secret") || !strings.Contains(line, "******") {
		t.Fatalf("password line: %q", line)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab")})
	m = next.(*model)
	line = plainLines(m.View())[4]
	if m.cfgPassword.Value() != "secretab" || strings.Contains(line, "secretab") || !strings.Contains(line, "********") {
		t.Fatalf("value %q line %q", m.cfgPassword.Value(), line)
	}
	m.cfgSecurity.SetValue("starttls")
	m.cfgUser.SetValue("user")
	next, _ = m.Update(key(tea.KeyCtrlS))
	m = next.(*model)
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "smtp_password=secretab\n") || strings.Contains(string(saved), "*") {
		t.Fatalf("file:\n%s", saved)
	}
}

func TestPlainSecurityHidesCredentials(t *testing.T) {
	m := readyModel(nil)
	m.cfg.SMTPSecurity = "starttls"
	m.cfg.SMTPUser = "user"
	m.cfg.SMTPPassword = "secret"
	next, _ := m.Update(key(tea.KeyCtrlX))
	m = next.(*model)
	if !strings.Contains(m.View(), "smtp_user:") || !strings.Contains(m.View(), "******") {
		t.Fatal("starttls should show user and password")
	}
	next, _ = m.Update(key(tea.KeyTab))
	m = next.(*model)
	next, _ = m.Update(key(tea.KeyLeft))
	m = next.(*model)
	view := strings.Join(plainLines(m.View()), "\n")
	if m.cfgSecurity.Value() != "plain" || strings.Contains(view, "smtp_user:") || strings.Contains(view, "smtp_password:") || strings.Contains(view, "secret") {
		t.Fatalf("plain view:\n%s", view)
	}
	next, _ = m.Update(key(tea.KeyDown))
	m = next.(*model)
	if m.cfgFocus != cfgFocusName {
		t.Fatal("down on plain security should skip user and password")
	}
	next, _ = m.Update(key(tea.KeyUp))
	m = next.(*model)
	if m.cfgFocus != cfgFocusSecurity {
		t.Fatal("up should return to smtp_security")
	}
	next, _ = m.Update(key(tea.KeyRight))
	m = next.(*model)
	next, _ = m.Update(key(tea.KeyDown))
	m = next.(*model)
	if m.cfgSecurity.Value() != "starttls" || m.cfgFocus != cfgFocusUser {
		t.Fatalf("security %q focus %d", m.cfgSecurity.Value(), m.cfgFocus)
	}
}

func TestConnectionBanner(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	prev := connBannerDuration
	connBannerDuration = time.Millisecond
	t.Cleanup(func() { connBannerDuration = prev })

	addr, _ := startRelay(t)
	m := readyModel(nil)
	m.cfg.SMTP = addr
	next, _ := m.Update(key(tea.KeyCtrlX))
	m = next.(*model)
	if plainLines(m.View())[len(plainLines(m.View()))-1] != configHelp {
		t.Fatalf("help: %q", plainLines(m.View())[len(plainLines(m.View()))-1])
	}

	next, cmd := m.Update(key(tea.KeyCtrlT))
	m = next.(*model)
	top := strings.Split(m.View(), "\n")[1]
	if !strings.Contains(top, "\x1b[94mCONNECTING") || lipgloss.Width(top) != 80 {
		t.Fatalf("connecting width %d line %q", lipgloss.Width(top), top)
	}
	if cmd == nil {
		t.Fatal("expected connection test")
	}
	next, cmd = m.Update(cmd())
	m = next.(*model)
	top = strings.Split(m.View(), "\n")[1]
	if !strings.Contains(top, "\x1b[92mCONNECTION OK") || lipgloss.Width(top) != 80 {
		t.Fatalf("ok width %d line %q", lipgloss.Width(top), top)
	}
	if cmd == nil {
		t.Fatal("expected banner clear")
	}
	next, _ = m.Update(cmd())
	m = next.(*model)
	if strings.Contains(m.View(), "CONNECTION") {
		t.Fatal("banner should disappear")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := ln.Addr().String()
	ln.Close()
	m.cfgSMTP.SetValue(closed)
	next, cmd = m.Update(key(tea.KeyCtrlT))
	m = next.(*model)
	if cmd == nil {
		t.Fatal("expected connection test")
	}
	next, cmd = m.Update(cmd())
	m = next.(*model)
	top = strings.Split(m.View(), "\n")[1]
	if !strings.Contains(top, "\x1b[91mCONNECTION FAILED") {
		t.Fatalf("failed line %q", top)
	}
	lines := plainLines(m.View())
	sig := configLine(lines, "signature:")
	if sig < 0 || sig+2 >= len(lines) || lines[sig+1] != "" || !strings.HasPrefix(lines[sig+2], "ERROR: ") || !strings.Contains(lines[sig+2], "connection refused") {
		t.Fatalf("error under config:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(m.View(), "\x1b[91mERROR: ") {
		t.Fatal("error line should use the failure color")
	}
	if cmd == nil {
		t.Fatal("expected banner clear")
	}
	next, _ = m.Update(cmd())
	m = next.(*model)
	lines = plainLines(m.View())
	if strings.Contains(m.View(), "CONNECTION") {
		t.Fatal("failed banner should disappear")
	}
	sig = configLine(lines, "signature:")
	if sig < 0 || lines[sig+1] != "" || !strings.HasPrefix(lines[sig+2], "ERROR: ") || !strings.Contains(lines[sig+2], "connection refused") {
		t.Fatalf("error should stay under config:\n%s", strings.Join(lines, "\n"))
	}
}

func configLine(lines []string, label string) int {
	for i, line := range lines {
		if strings.Contains(line, label) {
			return i
		}
	}
	return -1
}

func TestSecurityArrows(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	m := readyModel(nil)
	m.cfgSecurity.Cursor.BlinkSpeed = time.Millisecond
	next, _ := m.Update(key(tea.KeyCtrlX))
	m = next.(*model)
	if strings.Contains(m.View(), "\x1b[7m< plain >") {
		t.Fatal("security value should not be highlighted before it is focused")
	}
	next, cmd := m.Update(key(tea.KeyTab))
	m = next.(*model)
	if m.cfgFocus != cfgFocusSecurity {
		t.Fatal("tab should focus smtp_security")
	}
	if !strings.Contains(plainLines(m.View())[2], "< plain >") {
		t.Fatalf("security line: %q", plainLines(m.View())[2])
	}
	if !strings.Contains(m.View(), "\x1b[7m< plain >") {
		t.Fatal("focused security value should be reverse")
	}
	if cmd == nil {
		t.Fatal("expected blink")
	}
	next, cmd = m.Update(cmd())
	m = next.(*model)
	if strings.Contains(m.View(), "\x1b[7m< plain >") {
		t.Fatal("security value should blink off")
	}
	if cmd == nil {
		t.Fatal("expected the next blink")
	}
	next, _ = m.Update(cmd())
	m = next.(*model)
	if !strings.Contains(m.View(), "\x1b[7m< plain >") {
		t.Fatal("security value should blink on again")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(*model)
	if m.cfgSecurity.Value() != "plain" {
		t.Fatalf("typing changed security to %q", m.cfgSecurity.Value())
	}

	next, _ = m.Update(key(tea.KeyRight))
	m = next.(*model)
	if m.cfgSecurity.Value() != "starttls" || !strings.Contains(plainLines(m.View())[2], "< starttls >") {
		t.Fatalf("right: %q line %q", m.cfgSecurity.Value(), plainLines(m.View())[2])
	}
	next, _ = m.Update(key(tea.KeyRight))
	m = next.(*model)
	if m.cfgSecurity.Value() != "tls" || !strings.Contains(plainLines(m.View())[2], "< tls >") {
		t.Fatalf("right: %q line %q", m.cfgSecurity.Value(), plainLines(m.View())[2])
	}
	next, _ = m.Update(key(tea.KeyRight))
	m = next.(*model)
	if m.cfgSecurity.Value() != "plain" {
		t.Fatalf("right wrap: %q", m.cfgSecurity.Value())
	}
	next, _ = m.Update(key(tea.KeyLeft))
	m = next.(*model)
	if m.cfgSecurity.Value() != "tls" {
		t.Fatalf("left wrap: %q", m.cfgSecurity.Value())
	}
}

func TestCtrlCCancels(t *testing.T) {
	m := readyModel(nil)
	next, cmd := m.Update(key(tea.KeyCtrlC))
	m = next.(*model)
	if !m.outcome.cancelled {
		t.Fatal("expected cancel")
	}
	if cmd == nil {
		t.Fatal("expected quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("quit msg: %T", cmd())
	}
}

func TestEnterAdvancesSingleLineFields(t *testing.T) {
	m := readyModel(nil)
	next, _ := m.Update(key(tea.KeyEnter))
	m = next.(*model)
	if !m.subject.Focused() || m.to.Focused() {
		t.Fatal("enter in To should focus Subject")
	}
	next, _ = m.Update(key(tea.KeyEnter))
	m = next.(*model)
	if !m.body.Focused() {
		t.Fatal("enter in Subject should focus Body")
	}
	next, _ = m.Update(key(tea.KeyEnter))
	m = next.(*model)
	if !m.body.Focused() || m.body.Value() != "\n" {
		t.Fatalf("enter in Body should insert a newline, value %q focused %v", m.body.Value(), m.body.Focused())
	}
}

func TestArrowsCycleComposeFields(t *testing.T) {
	m := readyModel(nil)
	next, _ := m.Update(key(tea.KeyDown))
	m = next.(*model)
	if !m.subject.Focused() || m.to.Focused() {
		t.Fatal("down in To should focus Subject")
	}
	next, _ = m.Update(key(tea.KeyDown))
	m = next.(*model)
	if !m.body.Focused() {
		t.Fatal("down in Subject should focus Body")
	}
	before := m.body.Value()
	next, _ = m.Update(key(tea.KeyDown))
	m = next.(*model)
	next, _ = m.Update(key(tea.KeyUp))
	m = next.(*model)
	if !m.body.Focused() || m.body.Value() != before {
		t.Fatalf("arrows in Body should move inside the message, value %q focused %v", m.body.Value(), m.body.Focused())
	}

	next, _ = m.Update(key(tea.KeyShiftTab))
	m = next.(*model)
	next, _ = m.Update(key(tea.KeyUp))
	m = next.(*model)
	if !m.to.Focused() {
		t.Fatal("up in Subject should focus To")
	}
	next, _ = m.Update(key(tea.KeyUp))
	m = next.(*model)
	if !m.body.Focused() {
		t.Fatal("up in To should wrap to Body")
	}

	m = readyModel([]string{"maria@example.com", "mattia@example.com"})
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ma")})
	m = next.(*model)
	next, _ = m.Update(key(tea.KeyDown))
	m = next.(*model)
	if !m.to.Focused() || m.suggestIdx != 1 {
		t.Fatalf("down should move through hints, idx %d focus %d", m.suggestIdx, m.focus)
	}
}

func TestTabCyclesFocus(t *testing.T) {
	m := readyModel(nil)
	if !m.to.Focused() {
		t.Fatal("to should start focused")
	}
	next, _ := m.Update(key(tea.KeyTab))
	m = next.(*model)
	if !m.subject.Focused() || m.to.Focused() {
		t.Fatal("tab should focus subject")
	}
	next, _ = m.Update(key(tea.KeyTab))
	m = next.(*model)
	if !m.body.Focused() {
		t.Fatal("tab should focus body")
	}
	next, _ = m.Update(key(tea.KeyShiftTab))
	m = next.(*model)
	if !m.subject.Focused() {
		t.Fatal("shift+tab should return to subject")
	}
}

func TestAddAndDeleteConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	m := readyModel(nil)
	m.configPath = path
	next, _ := m.Update(key(tea.KeyCtrlX))
	m = next.(*model)

	next, _ = m.Update(key(tea.KeyCtrlD))
	m = next.(*model)
	lines := plainLines(m.View())
	if lines[len(lines)-1] != configHelp || !strings.Contains(m.status, "only config") || strings.Contains(m.View(), deleteConfirm) {
		t.Fatalf("only config: status %q help %q", m.status, lines[len(lines)-1])
	}

	m.cfgSMTP.SetValue("smtp.example.net:2525")
	next, _ = m.Update(key(tea.KeyCtrlN))
	m = next.(*model)
	lines = plainLines(m.View())
	if lines[0] != "[2/2]" || !strings.Contains(lines[1], "localhost:25") || !strings.Contains(lines[3], "John Doe") || !strings.Contains(lines[4], "john.doe@example.com") || !strings.Contains(lines[5], `Greetings,\n\n\s\sJohn Doe`) {
		t.Fatalf("new config:\n%s", strings.Join(lines[:6], "\n"))
	}
	if m.cfgs[0].SMTP != "smtp.example.net:2525" || m.cfg.SMTP != "localhost:25" {
		t.Fatalf("list: %+v active %+v", m.cfgs, m.cfg)
	}

	m.cfgSMTP.SetValue("nope")
	next, _ = m.Update(key(tea.KeyCtrlN))
	m = next.(*model)
	if plainLines(m.View())[0] != "[2/2]" || !strings.Contains(m.status, "host:port") {
		t.Fatalf("invalid add status %q indicator %q", m.status, plainLines(m.View())[0])
	}
	m.cfgSMTP.SetValue("localhost:25")

	next, _ = m.Update(key(tea.KeyCtrlS))
	m = next.(*model)
	if m.screen != screenCompose || m.cfg.SenderFrom != "john.doe@example.com" {
		t.Fatalf("save should return on the new config: %+v", m.cfg)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "smtp=smtp.example.net:2525\n") || !strings.Contains(string(saved), "---\n") || !strings.Contains(string(saved), "smtp=localhost:25\n") {
		t.Fatalf("file:\n%s", saved)
	}

	next, _ = m.Update(key(tea.KeyCtrlX))
	m = next.(*model)
	if plainLines(m.View())[0] != "[2/2]" {
		t.Fatalf("reopen: %q", plainLines(m.View())[0])
	}
	next, _ = m.Update(key(tea.KeyCtrlD))
	m = next.(*model)
	if plainLines(m.View())[len(plainLines(m.View()))-1] != deleteConfirm {
		t.Fatalf("confirm: %q", plainLines(m.View())[len(plainLines(m.View()))-1])
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(*model)
	if plainLines(m.View())[0] != "[2/2]" || plainLines(m.View())[len(plainLines(m.View()))-1] != configHelp {
		t.Fatal("n should cancel deletion")
	}

	next, _ = m.Update(key(tea.KeyCtrlD))
	m = next.(*model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(*model)
	lines = plainLines(m.View())
	if lines[0] != "[1/1]" || !strings.Contains(lines[1], "smtp.example.net:2525") || len(m.cfgs) != 1 {
		t.Fatalf("after delete: %q %q cfgs %d", lines[0], lines[1], len(m.cfgs))
	}
	next, _ = m.Update(key(tea.KeyCtrlS))
	m = next.(*model)
	saved, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "---") || !strings.Contains(string(saved), "smtp=smtp.example.net:2525\n") {
		t.Fatalf("file:\n%s", saved)
	}
	if m.screen != screenCompose || !strings.HasSuffix(plainLines(m.View())[0], "smtp.example.net:2525") || !strings.HasSuffix(plainLines(m.View())[1], "noreply@mpero.net") {
		t.Fatalf("compose after delete:\n%s\n%s", plainLines(m.View())[0], plainLines(m.View())[1])
	}
}

func TestSwitchConfigOnCompose(t *testing.T) {
	addr, got := startRelay(t)
	m := readyModel(nil)
	second := testConfig()
	second.SMTP = addr
	second.SenderName = "Other"
	second.SenderFrom = "other@example.com"
	m.cfgs = []Config{m.cfg, second}
	m.to.SetValue("dest@example.com")
	m.subject.SetValue("Hello")
	m.body.SetValue("hi")
	body := m.body.Value()

	lines := plainLines(m.View())
	if !strings.HasSuffix(lines[0], "smtp.mpero.net:25") || !strings.HasSuffix(lines[1], "noreply@mpero.net") || lines[len(lines)-1] != composeHelp {
		t.Fatalf("smtp %q from %q help %q", lines[0], lines[1], lines[len(lines)-1])
	}

	next, _ := m.Update(key(tea.KeyCtrlDown))
	m = next.(*model)
	if m.cfg.SMTP != addr || m.cfg.SenderFrom != "other@example.com" || m.body.Value() != body {
		t.Fatalf("switch: cfg %+v body %q", m.cfg, m.body.Value())
	}
	lines = plainLines(m.View())
	if !strings.HasSuffix(lines[0], addr) || !strings.HasSuffix(lines[1], "other@example.com") || len(lines[0]) != 80 || len(lines[1]) != 80 {
		t.Fatalf("smtp %q from %q", lines[0], lines[1])
	}

	next, _ = m.Update(key(tea.KeyCtrlDown))
	m = next.(*model)
	if m.cfg.SenderFrom != "noreply@mpero.net" || m.body.Value() != body {
		t.Fatal("down should wrap to the first config and leave the message")
	}
	next, _ = m.Update(key(tea.KeyCtrlUp))
	m = next.(*model)
	if m.cfg.SMTP != addr || m.body.Value() != body {
		t.Fatal("up should wrap to the last config without editing the body")
	}

	next, cmd := m.Update(key(tea.KeyCtrlS))
	m = next.(*model)
	if cmd == nil {
		t.Fatal("expected send")
	}
	msg := cmd().(sentMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	sent := waitRelay(t, got)
	if !strings.Contains(sent, "other@example.com") || !strings.Contains(sent, "hi") {
		t.Fatalf("relay:\n%s", sent)
	}
}

func key(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func plainLines(view string) []string {
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		lines[i] = stripANSI(line)
	}
	return lines
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		i++
		for i < len(s) && s[i] != 'm' {
			i++
		}
	}
	return b.String()
}

func hasPlainLine(lines []string, want string) bool {
	return lineIndex(lines, want) >= 0
}

func lineIndex(lines []string, want string) int {
	for i, line := range lines {
		if line == want {
			return i
		}
	}
	return -1
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
