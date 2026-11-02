package main

import (
	"bufio"
	"fmt"
	"net"
	"net/mail"
	"os"
	"strings"
)

// Config is the mail relay settings read from the config file.
type Config struct {
	SenderName   string
	SenderFrom   string
	SMTP         string
	SMTPSecurity string
	SMTPUser     string
	SMTPPassword string
	Signature    string
}

// LoadConfig reads a key=value config file. Blank lines and lines starting
// with # are ignored. sender_name, sender_from, and smtp are required.
// smtp_security is plain, starttls, or tls, and defaults to plain.
// smtp_user and smtp_password are optional and must be set together.
// signature is optional. In values, \n is a newline, \t is a tab,
// \s is a space, and \\ is a backslash.
func LoadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	vals := make(map[string]string)
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("config line %d: missing '='", lineNo)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			return Config{}, fmt.Errorf("config line %d: empty key", lineNo)
		}
		vals[key] = val
	}
	if err := sc.Err(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	cfg := Config{
		SenderName:   unescapeConfig(vals["sender_name"]),
		SenderFrom:   unescapeConfig(vals["sender_from"]),
		SMTP:         unescapeConfig(vals["smtp"]),
		SMTPSecurity: unescapeConfig(vals["smtp_security"]),
		SMTPUser:     unescapeConfig(vals["smtp_user"]),
		SMTPPassword: unescapeConfig(vals["smtp_password"]),
		Signature:    unescapeConfig(vals["signature"]),
	}
	return validateConfig(cfg)
}

// validateConfig checks the settings a mail needs and returns the cleaned values.
func validateConfig(cfg Config) (Config, error) {
	cfg.SenderName = strings.TrimSpace(cfg.SenderName)
	cfg.SenderFrom = strings.TrimSpace(cfg.SenderFrom)
	cfg.SMTP = strings.TrimSpace(cfg.SMTP)
	if cfg.SenderName == "" || strings.ContainsAny(cfg.SenderName, "\r\n") {
		return Config{}, fmt.Errorf("config: sender_name is required")
	}
	if cfg.SenderFrom == "" || strings.ContainsAny(cfg.SenderFrom, "\r\n") {
		return Config{}, fmt.Errorf("config: sender_from is required")
	}
	from, err := mail.ParseAddress(cfg.SenderFrom)
	if err != nil {
		return Config{}, fmt.Errorf("config: sender_from: %w", err)
	}
	if from.Address == "" {
		return Config{}, fmt.Errorf("config: sender_from is required")
	}
	cfg.SenderFrom = from.Address
	if cfg.SMTP == "" {
		return Config{}, fmt.Errorf("config: smtp is required")
	}
	host, port, err := net.SplitHostPort(cfg.SMTP)
	if err != nil || host == "" || port == "" || strings.ContainsAny(cfg.SMTP, "\r\n") {
		return Config{}, fmt.Errorf("config: smtp must be host:port")
	}
	cfg.SMTPSecurity = strings.ToLower(strings.TrimSpace(cfg.SMTPSecurity))
	if cfg.SMTPSecurity == "" {
		cfg.SMTPSecurity = "plain"
	}
	switch cfg.SMTPSecurity {
	case "plain", "starttls", "tls":
	default:
		return Config{}, fmt.Errorf("config: smtp_security must be plain, starttls, or tls")
	}
	cfg.SMTPUser = strings.TrimSpace(cfg.SMTPUser)
	if strings.ContainsAny(cfg.SMTPUser, "\r\n") || strings.ContainsAny(cfg.SMTPPassword, "\r\n") {
		return Config{}, fmt.Errorf("config: smtp_user and smtp_password must be one line")
	}
	if cfg.SMTPUser == "" && cfg.SMTPPassword == "" {
		return cfg, nil
	}
	if cfg.SMTPUser == "" || cfg.SMTPPassword == "" {
		return Config{}, fmt.Errorf("config: smtp_user and smtp_password are both required")
	}
	if cfg.SMTPSecurity == "plain" {
		return Config{}, fmt.Errorf("config: smtp_password requires smtp_security starttls or tls")
	}
	return cfg, nil
}

// SaveConfig rewrites the config file with the known settings.
// The file is always readable only by the owner.
func SaveConfig(path string, cfg Config) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "smtp=%s\n", escapeConfig(cfg.SMTP))
	fmt.Fprintf(&b, "smtp_security=%s\n", escapeConfig(cfg.SMTPSecurity))
	fmt.Fprintf(&b, "smtp_user=%s\n", escapeConfig(cfg.SMTPUser))
	fmt.Fprintf(&b, "smtp_password=%s\n", escapeConfig(cfg.SMTPPassword))
	fmt.Fprintf(&b, "sender_name=%s\n", escapeConfig(cfg.SenderName))
	fmt.Fprintf(&b, "sender_from=%s\n", escapeConfig(cfg.SenderFrom))
	fmt.Fprintf(&b, "signature=%s\n", escapeConfig(cfg.Signature))
	if err := ensureParent(path); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// escapeConfig writes a value so LoadConfig reads it back.
// Newlines, tabs, and backslashes are escaped. Spaces that TrimSpace would
// drop, and spaces at the start of a line, are written as \s.
func escapeConfig(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lineStart := true
	pendingSpaces := 0
	flushSpaces := func(escape bool) {
		for i := 0; i < pendingSpaces; i++ {
			if escape {
				b.WriteString(`\s`)
			} else {
				b.WriteByte(' ')
			}
		}
		pendingSpaces = 0
	}
	for _, r := range s {
		if r == ' ' {
			pendingSpaces++
			continue
		}
		if pendingSpaces > 0 {
			flushSpaces(lineStart)
		}
		switch r {
		case '\\':
			b.WriteString(`\\`)
			lineStart = false
		case '\n', '\r':
			b.WriteString(`\n`)
			lineStart = true
		case '\t':
			b.WriteString(`\t`)
			lineStart = false
		default:
			b.WriteRune(r)
			lineStart = false
		}
	}
	if pendingSpaces > 0 {
		flushSpaces(true)
	}
	return b.String()
}

// unescapeConfig turns \n, \t, \s, and \\ into the characters they stand for.
func unescapeConfig(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case 'n':
			b.WriteByte('\n')
			i++
		case 't':
			b.WriteByte('\t')
			i++
		case 's':
			b.WriteByte(' ')
			i++
		case '\\':
			b.WriteByte('\\')
			i++
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// FromHeader formats the From header, quoting the display name when needed.
func (c Config) FromHeader() string {
	return (&mail.Address{Name: c.SenderName, Address: c.SenderFrom}).String()
}
