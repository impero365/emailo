// Package clipboard reads and writes the system clipboard.
// Tool lookup is deferred until the first paste or copy. The upstream
// package does that lookup in init, and searching PATH for xclip, xsel,
// and wl-copy was delaying program startup by about a second.
package clipboard

import (
	"errors"
	"os"
	"os/exec"
	"sync"
)

// Unsupported reports that no clipboard command is available.
// It is false until the first read or write looks for one.
var Unsupported bool

var (
	once         sync.Once
	pasteArgs    []string
	copyArgs     []string
	errNoCommand = errors.New("no clipboard command available")
)

func prepare() {
	if os.Getenv("WAYLAND_DISPLAY") != "" && look("wl-copy") && look("wl-paste") {
		copyArgs = []string{"wl-copy"}
		pasteArgs = []string{"wl-paste", "--no-newline"}
		return
	}
	if look("xclip") {
		copyArgs = []string{"xclip", "-in", "-selection", "clipboard"}
		pasteArgs = []string{"xclip", "-out", "-selection", "clipboard"}
		return
	}
	if look("xsel") {
		copyArgs = []string{"xsel", "--input", "--clipboard"}
		pasteArgs = []string{"xsel", "--output", "--clipboard"}
		return
	}
	Unsupported = true
}

func look(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// ReadAll reads the system clipboard.
func ReadAll() (string, error) {
	once.Do(prepare)
	if Unsupported {
		return "", errNoCommand
	}
	out, err := exec.Command(pasteArgs[0], pasteArgs[1:]...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// WriteAll writes text to the system clipboard.
func WriteAll(text string) error {
	once.Do(prepare)
	if Unsupported {
		return errNoCommand
	}
	cmd := exec.Command(copyArgs[0], copyArgs[1:]...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := in.Write([]byte(text)); err != nil {
		return err
	}
	if err := in.Close(); err != nil {
		return err
	}
	return cmd.Wait()
}
