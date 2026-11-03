package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	path := writeTemp(t, `
# comment
sender_name=Mattia Peronio

sender_from=noreply@mpero.net
smtp=smtp.mpero.net:25
`)
	cfgs, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfgs) != 1 {
		t.Fatalf("configs: %d", len(cfgs))
	}
	cfg := cfgs[0]
	if cfg.SenderName != "Mattia Peronio" {
		t.Fatalf("name: %q", cfg.SenderName)
	}
	if cfg.SenderFrom != "noreply@mpero.net" {
		t.Fatalf("from: %q", cfg.SenderFrom)
	}
	if cfg.SMTP != "smtp.mpero.net:25" {
		t.Fatalf("smtp: %q", cfg.SMTP)
	}
	if got := cfg.FromHeader(); got != `"Mattia Peronio" <noreply@mpero.net>` {
		t.Fatalf("from header: %q", got)
	}
}

func TestConfigTemplate(t *testing.T) {
	cfgs, err := LoadConfig("config.template")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfgs) != 1 {
		t.Fatalf("configs: %d", len(cfgs))
	}
	cfg := cfgs[0]
	if cfg != defaultConfig() {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Signature != "Greetings,\n\n  John Doe" {
		t.Fatalf("signature: %q", cfg.Signature)
	}
	if cfg.SMTPSecurity != "plain" || cfg.SMTPUser != "" || cfg.SMTPPassword != "" {
		t.Fatalf("security: %+v", cfg)
	}
}

func TestLoadConfigSignature(t *testing.T) {
	path := writeTemp(t, "sender_name=A\nsender_from=a@b.c\nsmtp=h:25\nsignature=Best regards,\\n\\n\\s\\sMattia Peronio\n")
	cfgs, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := cfgs[0]
	if cfg.Signature != "Best regards,\n\n  Mattia Peronio" {
		t.Fatalf("signature: %q", cfg.Signature)
	}

	path = writeTemp(t, "sender_name=A\nsender_from=a@b.c\nsmtp=h:25\n")
	cfgs, err = LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg = cfgs[0]
	if cfg.Signature != "" {
		t.Fatalf("missing signature should be empty, got %q", cfg.Signature)
	}
}

func TestEscapeAndSaveConfig(t *testing.T) {
	got := escapeConfig("Best regards,\n\n  Mattia Peronio")
	if got != `Best regards,\n\n\s\sMattia Peronio` {
		t.Fatalf("escape: %q", got)
	}
	if escapeConfig("a b") != "a b" {
		t.Fatalf("internal space: %q", escapeConfig("a b"))
	}
	if escapeConfig(" a") != `\sa` {
		t.Fatalf("leading space: %q", escapeConfig(" a"))
	}
	if escapeConfig("a ") != `a\s` {
		t.Fatalf("trailing space: %q", escapeConfig("a "))
	}
	if escapeConfig("a\tb\\c") != `a\tb\\c` {
		t.Fatalf("tab and slash: %q", escapeConfig("a\tb\\c"))
	}

	cfg := Config{
		SenderName:   "Mattia Peronio",
		SenderFrom:   "noreply@mpero.net",
		SMTP:         "smtp.mpero.net:25",
		SMTPSecurity: "plain",
		Signature:    "Best regards,\n\n  Mattia Peronio",
	}
	path := filepath.Join(t.TempDir(), "config")
	if err := SaveConfig(path, []Config{cfg}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "smtp=smtp.mpero.net:25\nsmtp_security=plain\nsmtp_user=\nsmtp_password=\nsender_name=Mattia Peronio\nsender_from=noreply@mpero.net\nsignature=Best regards,\\n\\n\\s\\sMattia Peronio\n"
	if string(body) != want {
		t.Fatalf("file:\n%q", string(body))
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0] != cfg {
		t.Fatalf("round trip: %+v", loaded)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", fi.Mode().Perm())
	}
}

func TestSaveConfigPasswordIsPrivate(t *testing.T) {
	cfg := Config{
		SenderName:   "A",
		SenderFrom:   "a@b.c",
		SMTP:         "h:587",
		SMTPSecurity: "starttls",
		SMTPUser:     "user",
		SMTPPassword: "secret",
	}
	path := filepath.Join(t.TempDir(), "config")
	if err := SaveConfig(path, []Config{cfg}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", fi.Mode().Perm())
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].SMTPUser != "user" || loaded[0].SMTPPassword != "secret" || loaded[0].SMTPSecurity != "starttls" {
		t.Fatalf("%+v", loaded)
	}
}

func TestLoadSaveMultipleConfigs(t *testing.T) {
	first := Config{
		SenderName:   "A",
		SenderFrom:   "a@b.c",
		SMTP:         "h:25",
		SMTPSecurity: "plain",
		Signature:    "Hi\nthere",
	}
	second := defaultConfig()
	second.SMTPSecurity = "starttls"
	second.SMTPUser = "user"
	second.SMTPPassword = "secret"
	path := filepath.Join(t.TempDir(), "config")
	if err := SaveConfig(path, []Config{first, second}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "smtp=h:25\nsmtp_security=plain\nsmtp_user=\nsmtp_password=\nsender_name=A\nsender_from=a@b.c\nsignature=Hi\\nthere\n---\nsmtp=localhost:25\nsmtp_security=starttls\nsmtp_user=user\nsmtp_password=secret\nsender_name=John Doe\nsender_from=john.doe@example.com\nsignature=Greetings,\\n\\n\\s\\sJohn Doe\n"
	if string(body) != want {
		t.Fatalf("file:\n%s", body)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[0] != first || loaded[1] != second {
		t.Fatalf("%+v", loaded)
	}

	_, err = LoadConfig(writeTemp(t, "sender_name=A\nsender_from=a@b.c\nsmtp=h:25\n---\nsender_name=B\nsender_from=b@b.c\nsmtp=not-a-port\n"))
	if err == nil || !strings.Contains(err.Error(), "host:port") {
		t.Fatalf("second config: %v", err)
	}
	_, err = LoadConfig(writeTemp(t, "# nothing\n"))
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty: %v", err)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "missing name", body: "sender_from=a@b.c\nsmtp=h:25\n", want: "sender_name"},
		{name: "missing from", body: "sender_name=A\nsmtp=h:25\n", want: "sender_from"},
		{name: "bad from", body: "sender_name=A\nsender_from=not-an-email\nsmtp=h:25\n", want: "sender_from"},
		{name: "missing smtp", body: "sender_name=A\nsender_from=a@b.c\n", want: "smtp"},
		{name: "bad smtp", body: "sender_name=A\nsender_from=a@b.c\nsmtp=smtp.example.net\n", want: "host:port"},
		{name: "bad security", body: "sender_name=A\nsender_from=a@b.c\nsmtp=h:25\nsmtp_security=ssl\n", want: "smtp_security"},
		{name: "password on plain", body: "sender_name=A\nsender_from=a@b.c\nsmtp=h:25\nsmtp_user=a\nsmtp_password=b\n", want: "starttls"},
		{name: "user only", body: "sender_name=A\nsender_from=a@b.c\nsmtp=h:587\nsmtp_security=starttls\nsmtp_user=a\n", want: "both required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadConfig(writeTemp(t, tc.body))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}
