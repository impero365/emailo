package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadAddressesMissing(t *testing.T) {
	got, err := LoadAddresses(filepath.Join(t.TempDir(), "addresses"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestLoadAddressesDedupes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addresses")
	body := "old@example.com\n\n# note\nNew@Example.com\nnew@example.com\nother@example.com\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadAddresses(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"old@example.com", "New@Example.com", "other@example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestFilterAddresses(t *testing.T) {
	addrs := []string{
		"newest@example.com",
		"maria@example.com",
		"other@domain.test",
		"mattia@example.com",
	}
	got := FilterAddresses(addrs, "EXA")
	want := []string{"newest@example.com", "maria@example.com", "mattia@example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if FilterAddresses(addrs, "  ") != nil {
		t.Fatal("blank query should match nothing")
	}

	many := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		many = append(many, "user"+string(rune('a'+i))+"@example.com")
	}
	if n := len(FilterAddresses(many, "example")); n != maxSuggestions {
		t.Fatalf("cap: got %d", n)
	}
}

func TestRememberAddressMovesToFront(t *testing.T) {
	path := filepath.Join(t.TempDir(), "addresses")
	existing := []string{"old@example.com", "New@Example.com"}
	next, err := RememberAddress(path, "new@example.com", existing)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"new@example.com", "old@example.com"}
	if !reflect.DeepEqual(next, want) {
		t.Fatalf("got %v", next)
	}
	loaded, err := LoadAddresses(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, want) {
		t.Fatalf("file: %v", loaded)
	}
}
