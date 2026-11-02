package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const maxSuggestions = 8

// LoadAddresses reads one address per line. A missing file is an empty list.
// Order is preserved and duplicates are dropped, keeping the first occurrence.
func LoadAddresses(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []string
	seen := make(map[string]struct{})
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key := strings.ToLower(line)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read addresses: %w", err)
	}
	return out, nil
}

// FilterAddresses returns addresses that contain query, case-insensitively,
// in the original order, capped at maxSuggestions. An empty query matches nothing.
func FilterAddresses(addrs []string, query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var out []string
	for _, addr := range addrs {
		if strings.Contains(strings.ToLower(addr), q) {
			out = append(out, addr)
			if len(out) == maxSuggestions {
				break
			}
		}
	}
	return out
}

// RememberAddress puts addr at the front of the list, drops any
// case-insensitive duplicate, and rewrites path. The returned list is the
// new order.
func RememberAddress(path, addr string, existing []string) ([]string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return existing, fmt.Errorf("empty address")
	}
	key := strings.ToLower(addr)
	next := make([]string, 0, len(existing)+1)
	next = append(next, addr)
	for _, item := range existing {
		if strings.ToLower(item) == key {
			continue
		}
		next = append(next, item)
	}

	var b strings.Builder
	for _, item := range next {
		b.WriteString(item)
		b.WriteByte('\n')
	}
	if err := ensureParent(path); err != nil {
		return nil, fmt.Errorf("write addresses: %w", err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return nil, fmt.Errorf("write addresses: %w", err)
	}
	return next, nil
}
