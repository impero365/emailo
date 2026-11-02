package main

import (
	"fmt"
	"os"
)

func main() {
	configPath, addressesPath, err := userPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	os.Exit(run(configPath, addressesPath))
}

func run(configPath, addressesPath string) int {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	addrs, err := LoadAddresses(addressesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	result, err := runTUI(configPath, cfg, addrs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	if result.cancelled || (!result.sent && result.err == nil) {
		return 0
	}
	if result.err != nil {
		fmt.Fprintf(os.Stderr, "Send failed: %s\n", result.err)
		return 1
	}
	fmt.Printf("Sent to %s\n", result.to)
	if _, err := RememberAddress(addressesPath, result.to, addrs); err != nil {
		fmt.Fprintf(os.Stderr, "could not save address: %s\n", err)
	}
	return 0
}
