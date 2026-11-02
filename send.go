package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"time"
)

const dialTimeout = 15 * time.Second

// buildMessage returns an RFC 5322 message. now supplies the Date header.
func buildMessage(cfg Config, to, subject, body string, now time.Time) ([]byte, error) {
	toAddr, err := mail.ParseAddress(strings.TrimSpace(to))
	if err != nil {
		return nil, fmt.Errorf("to: %w", err)
	}
	if strings.ContainsAny(subject, "\r\n") {
		return nil, fmt.Errorf("subject contains a line break")
	}

	encodedSubject := subject
	if !isASCII(subject) {
		encodedSubject = mime.QEncoding.Encode("utf-8", subject)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", cfg.FromHeader())
	fmt.Fprintf(&b, "To: %s\r\n", toAddr.String())
	fmt.Fprintf(&b, "Subject: %s\r\n", encodedSubject)
	fmt.Fprintf(&b, "Date: %s\r\n", now.UTC().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(normalizeCRLF(body))
	return []byte(b.String()), nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

func normalizeCRLF(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.ReplaceAll(body, "\n", "\r\n")
}

// clientTLSConfig builds the TLS settings for smtp_security starttls and tls.
// Tests replace it to trust a local certificate.
var clientTLSConfig = func(host string) *tls.Config {
	return &tls.Config{ServerName: host}
}

// sendMail delivers one plain-text message through the relay in cfg.
func sendMail(cfg Config, to, subject, body string) error {
	msg, err := buildMessage(cfg, to, subject, body, time.Now())
	if err != nil {
		return err
	}
	toAddr, err := mail.ParseAddress(strings.TrimSpace(to))
	if err != nil {
		return err
	}

	client, err := connectSMTP(cfg)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Mail(cfg.SenderFrom); err != nil {
		return err
	}
	if err := client.Rcpt(toAddr.Address); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// testConnection checks that the relay accepts this configuration.
// It greets the server, upgrades and logs in when those are configured, and quits.
func testConnection(cfg Config) error {
	if cfg.SMTPSecurity == "" || cfg.SMTPSecurity == "plain" {
		cfg.SMTPUser = ""
		cfg.SMTPPassword = ""
	}
	client, err := connectSMTP(cfg)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Quit()
}

func connectSMTP(cfg Config) (*smtp.Client, error) {
	security := cfg.SMTPSecurity
	if security == "" {
		security = "plain"
	}
	if cfg.SMTPPassword != "" && security == "plain" {
		return nil, fmt.Errorf("smtp_password requires smtp_security starttls or tls")
	}

	host, _, err := net.SplitHostPort(cfg.SMTP)
	if err != nil {
		return nil, fmt.Errorf("smtp address: %w", err)
	}

	conn, err := (&net.Dialer{Timeout: dialTimeout}).Dial("tcp", cfg.SMTP)
	if err != nil {
		return nil, err
	}
	// Bound the rest of the conversation so a silent relay cannot hang the UI.
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		conn.Close()
		return nil, err
	}

	use := conn
	if security == "tls" {
		tlsConn := tls.Client(conn, clientTLSConfig(host))
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return nil, err
		}
		use = tlsConn
	}

	client, err := smtp.NewClient(use, host)
	if err != nil {
		conn.Close()
		return nil, err
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" || strings.ContainsAny(hostname, " \t\r\n") {
		hostname = "localhost"
	}
	if err := client.Hello(hostname); err != nil {
		client.Close()
		return nil, err
	}
	if security == "starttls" {
		if err := client.StartTLS(clientTLSConfig(host)); err != nil {
			client.Close()
			return nil, err
		}
	}
	if cfg.SMTPUser != "" {
		auth, err := chooseAuth(client, cfg, host)
		if err != nil {
			client.Close()
			return nil, err
		}
		if err := client.Auth(auth); err != nil {
			client.Close()
			return nil, err
		}
	}
	return client, nil
}

func chooseAuth(client *smtp.Client, cfg Config, host string) (smtp.Auth, error) {
	ok, mechs := client.Extension("AUTH")
	if !ok {
		return nil, fmt.Errorf("smtp: server does not offer authentication")
	}
	if authOffered(mechs, "PLAIN") {
		return smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPassword, host), nil
	}
	if authOffered(mechs, "LOGIN") {
		return &loginAuth{username: cfg.SMTPUser, password: cfg.SMTPPassword, host: host}, nil
	}
	return nil, fmt.Errorf("smtp: server does not offer AUTH PLAIN or AUTH LOGIN")
}

func authOffered(mechs, name string) bool {
	for _, mech := range strings.Fields(mechs) {
		if strings.EqualFold(mech, name) {
			return true
		}
	}
	return false
}

// loginAuth is AUTH LOGIN, used when the server does not advertise PLAIN.
type loginAuth struct {
	username, password, host string
	step                     int
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		return "", nil, errors.New("unencrypted connection")
	}
	if server.Name != a.host {
		return "", nil, errors.New("wrong host name")
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	a.step++
	switch a.step {
	case 1:
		return []byte(a.username), nil
	case 2:
		return []byte(a.password), nil
	default:
		return nil, errors.New("unexpected server challenge")
	}
}
