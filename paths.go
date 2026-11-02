package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func userPaths() (configPath, addressesPath string, err error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", "", fmt.Errorf("cannot find the home directory")
	}
	dir := filepath.Join(home, ".emailo")
	return filepath.Join(dir, "config"), filepath.Join(dir, "addresses"), nil
}

func ensureParent(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o700)
}
