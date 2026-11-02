package main

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		SenderName:   "Mattia Peronio",
		SenderFrom:   "noreply@mpero.net",
		SMTP:         "smtp.mpero.net:25",
		SMTPSecurity: "plain",
	}
}

func TestBuildMessage(t *testing.T) {
	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	msg, err := buildMessage(testConfig(), " Person <dest@example.com> ", "Hello", "line1\nline2\r\nline3", now)
	if err != nil {
		t.Fatal(err)
	}
	got := string(msg)
	for _, want := range []string{
		"From: \"Mattia Peronio\" <noreply@mpero.net>\r\n",
		"To: \"Person\" <dest@example.com>\r\n",
		"Subject: Hello\r\n",
		"Date: Fri, 10 Apr 2026 12:00:00 +0000\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
		"\r\nline1\r\nline2\r\nline3",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "\n\n") || strings.Contains(got, "\r\r") {
		t.Fatalf("bad newlines:\n%q", got)
	}
}

func TestBuildMessageEncodesSubject(t *testing.T) {
	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	msg, err := buildMessage(testConfig(), "dest@example.com", "caf\u00e9", "hi", now)
	if err != nil {
		t.Fatal(err)
	}
	got := string(msg)
	if !strings.Contains(got, "Subject: =?utf-8?q?caf=C3=A9?=") {
		t.Fatalf("subject not encoded:\n%s", got)
	}
}

func TestBuildMessageSendsBodyAsWritten(t *testing.T) {
	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	cfg := testConfig()
	cfg.Signature = "Best regards,\n\tMattia Peronio"
	body := "thanks\n\nBest regards,\n\tMattia Peronio"
	msg, err := buildMessage(cfg, "dest@example.com", "Hello", body, now)
	if err != nil {
		t.Fatal(err)
	}
	got := string(msg)
	want := "\r\nthanks\r\n\r\nBest regards,\r\n\tMattia Peronio"
	if !strings.Contains(got, want) {
		t.Fatalf("body:\n%q", got)
	}
	if strings.Count(got, "Best regards,") != 1 {
		t.Fatalf("signature was added twice:\n%s", got)
	}
}

func TestBuildMessageRejectsBreakInSubject(t *testing.T) {
	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	if _, err := buildMessage(testConfig(), "dest@example.com", "bad\nsubject", "hi", now); err == nil {
		t.Fatal("expected error")
	}
}

func TestSendMailLocalRelay(t *testing.T) {
	addr, got := startRelay(t)
	cfg := testConfig()
	cfg.SMTP = addr
	if err := sendMail(cfg, "Dest <dest@example.com>", "Hello", "line1\nline2"); err != nil {
		t.Fatal(err)
	}
	msg := waitRelay(t, got)
	for _, want := range []string{
		"From: \"Mattia Peronio\" <noreply@mpero.net>",
		"To: \"Dest\" <dest@example.com>",
		"Subject: Hello",
		"line1",
		"line2",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("missing %q in\n%s", want, msg)
		}
	}
}

func TestSendMailRefusesCleartextPassword(t *testing.T) {
	cfg := testConfig()
	cfg.SMTP = "127.0.0.1:1"
	cfg.SMTPUser = "user"
	cfg.SMTPPassword = "secret"
	err := sendMail(cfg, "dest@example.com", "Hello", "body")
	if err == nil || !strings.Contains(err.Error(), "starttls") {
		t.Fatalf("err: %v", err)
	}
}

func TestSendMailSTARTTLSAndPLAIN(t *testing.T) {
	addr := startSecureRelay(t, secureRelay{mech: "PLAIN", user: "user", pass: "secret"})
	cfg := testConfig()
	cfg.SMTP = addr
	cfg.SMTPSecurity = "starttls"
	cfg.SMTPUser = "user"
	cfg.SMTPPassword = "secret"
	if err := sendMail(cfg, "dest@example.com", "Hello", "body"); err != nil {
		t.Fatal(err)
	}
}

func TestSendMailImplicitTLSAndLOGIN(t *testing.T) {
	addr := startSecureRelay(t, secureRelay{implicit: true, mech: "LOGIN", user: "user", pass: "secret"})
	cfg := testConfig()
	cfg.SMTP = addr
	cfg.SMTPSecurity = "tls"
	cfg.SMTPUser = "user"
	cfg.SMTPPassword = "secret"
	if err := sendMail(cfg, "dest@example.com", "Hello", "body"); err != nil {
		t.Fatal(err)
	}
}

func TestSendMailRejectsUnknownAuth(t *testing.T) {
	addr := startSecureRelay(t, secureRelay{mech: "CRAM-MD5", user: "user", pass: "secret"})
	cfg := testConfig()
	cfg.SMTP = addr
	cfg.SMTPSecurity = "starttls"
	cfg.SMTPUser = "user"
	cfg.SMTPPassword = "secret"
	err := sendMail(cfg, "dest@example.com", "Hello", "body")
	if err == nil || !strings.Contains(err.Error(), "AUTH PLAIN") {
		t.Fatalf("err: %v", err)
	}
}

func TestSendMailRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	cfg := testConfig()
	cfg.SMTP = addr
	if err := sendMail(cfg, "dest@example.com", "Hello", "body"); err == nil {
		t.Fatal("expected dial error")
	}
}

type secureRelay struct {
	implicit   bool
	mech       string
	user, pass string
}

func startSecureRelay(t *testing.T, opt secureRelay) string {
	t.Helper()
	cert := relayCert(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		c := conn
		if opt.implicit {
			tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}})
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			c = tlsConn
		}
		_ = serveSecureSMTP(c, cert, opt)
	}()
	return ln.Addr().String()
}

func serveSecureSMTP(conn net.Conn, cert tls.Certificate, opt secureRelay) error {
	c := conn
	if err := writeSMTP(c, "220 ready\r\n"); err != nil {
		return err
	}
	authed := false
	for {
		line, err := readSMTPLine(c)
		if err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
			var b strings.Builder
			b.WriteString("250-hello\r\n")
			_, encrypted := c.(*tls.Conn)
			if !encrypted {
				b.WriteString("250-STARTTLS\r\n")
			}
			if encrypted && opt.mech != "" {
				b.WriteString("250 AUTH " + opt.mech + "\r\n")
			} else {
				b.WriteString("250 ok\r\n")
			}
			if err := writeSMTP(c, b.String()); err != nil {
				return err
			}
		case line == "STARTTLS":
			if err := writeSMTP(c, "220 ready\r\n"); err != nil {
				return err
			}
			tlsConn := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{cert}})
			if err := tlsConn.Handshake(); err != nil {
				return err
			}
			c = tlsConn
		case strings.HasPrefix(line, "AUTH PLAIN "):
			raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "AUTH PLAIN "))
			parts := bytes.Split(raw, []byte{0})
			if err != nil || len(parts) != 3 || string(parts[1]) != opt.user || string(parts[2]) != opt.pass {
				_ = writeSMTP(c, "535 no\r\n")
				return fmt.Errorf("bad PLAIN auth")
			}
			authed = true
			if err := writeSMTP(c, "235 ok\r\n"); err != nil {
				return err
			}
		case line == "AUTH LOGIN":
			if err := writeSMTP(c, "334 "+base64.StdEncoding.EncodeToString([]byte("Username:"))+"\r\n"); err != nil {
				return err
			}
			userLine, err := readSMTPLine(c)
			if err != nil {
				return err
			}
			user, err := base64.StdEncoding.DecodeString(userLine)
			if err != nil {
				return err
			}
			if err := writeSMTP(c, "334 "+base64.StdEncoding.EncodeToString([]byte("Password:"))+"\r\n"); err != nil {
				return err
			}
			passLine, err := readSMTPLine(c)
			if err != nil {
				return err
			}
			pass, err := base64.StdEncoding.DecodeString(passLine)
			if err != nil {
				return err
			}
			if string(user) != opt.user || string(pass) != opt.pass {
				_ = writeSMTP(c, "535 no\r\n")
				return fmt.Errorf("bad LOGIN auth")
			}
			authed = true
			if err := writeSMTP(c, "235 ok\r\n"); err != nil {
				return err
			}
		case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
			if !authed {
				_ = writeSMTP(c, "530 auth\r\n")
				return fmt.Errorf("mail before auth")
			}
			if err := writeSMTP(c, "250 ok\r\n"); err != nil {
				return err
			}
		case line == "DATA":
			if !authed {
				_ = writeSMTP(c, "530 auth\r\n")
				return fmt.Errorf("data before auth")
			}
			if err := writeSMTP(c, "354 go\r\n"); err != nil {
				return err
			}
			for {
				dataLine, err := readSMTPLine(c)
				if err != nil {
					return err
				}
				if dataLine == "." {
					if err := writeSMTP(c, "250 accepted\r\n"); err != nil {
						return err
					}
					break
				}
			}
		case line == "QUIT":
			return writeSMTP(c, "221 bye\r\n")
		default:
			_ = writeSMTP(c, "500 unexpected\r\n")
			return fmt.Errorf("unexpected: %s", line)
		}
	}
}

func readSMTPLine(c net.Conn) (string, error) {
	var buf []byte
	one := []byte{0}
	for {
		if _, err := io.ReadFull(c, one); err != nil {
			return "", err
		}
		if one[0] == '\n' {
			return strings.TrimRight(string(buf), "\r"), nil
		}
		buf = append(buf, one[0])
	}
}

func writeSMTP(c net.Conn, s string) error {
	_, err := io.WriteString(c, s)
	return err
}

func relayCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	prev := clientTLSConfig
	clientTLSConfig = func(host string) *tls.Config {
		return &tls.Config{ServerName: host, RootCAs: pool}
	}
	t.Cleanup(func() { clientTLSConfig = prev })
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func startRelay(t *testing.T) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	got := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		r := bufio.NewReader(conn)
		_, _ = conn.Write([]byte("220 ready\r\n"))
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if inData {
				if line == "." {
					_, _ = conn.Write([]byte("250 accepted\r\n"))
					inData = false
					continue
				}
				data.WriteString(strings.TrimPrefix(line, "."))
				data.WriteByte('\n')
				continue
			}
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				_, _ = conn.Write([]byte("250 hello\r\n"))
			case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
				_, _ = conn.Write([]byte("250 ok\r\n"))
			case line == "DATA":
				_, _ = conn.Write([]byte("354 go\r\n"))
				inData = true
			case line == "QUIT":
				_, _ = conn.Write([]byte("221 bye\r\n"))
				got <- data.String()
				return
			default:
				_, _ = conn.Write([]byte("500 unexpected\r\n"))
				got <- "unexpected: " + line
				return
			}
		}
	}()
	return ln.Addr().String(), got
}

func waitRelay(t *testing.T, got <-chan string) string {
	t.Helper()
	select {
	case msg := <-got:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for relay")
	}
	return ""
}
