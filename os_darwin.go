//go:build darwin

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func stateRoot() string {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return expandHome(base)
	}
	return filepath.Join(homeDir(), ".local", "state")
}

func dataRoot() string {
	if base := os.Getenv("XDG_DATA_HOME"); base != "" {
		return expandHome(base)
	}
	return filepath.Join(homeDir(), ".local", "share")
}

// setupDirs are the directories setup considers for the aitools binary.
func setupDirs() []string {
	h := homeDir()
	return []string{filepath.Join(h, "bin"), filepath.Join(h, ".local", "bin")}
}

// selfExe is the absolute path of the running binary.
func selfExe() string {
	exe, err := os.Executable()
	if err != nil {
		die("cannot locate the running executable: %v", err)
	}
	return resolve(exe)
}

// trashPath moves path to ~/.Trash. When the Trash can't take it (e.g. the
// path is on another volume), it deletes permanently.
func trashPath(path string) {
	trashDir := filepath.Join(homeDir(), ".Trash")
	name := filepath.Base(path)
	dest := filepath.Join(trashDir, name)
	for i := 2; present(dest); i++ {
		dest = filepath.Join(trashDir, fmt.Sprintf("%s %d", name, i))
	}
	if err := os.MkdirAll(trashDir, 0o700); err == nil {
		if err := os.Rename(path, dest); err == nil {
			return
		}
	}
	if err := os.RemoveAll(path); err != nil {
		die("cannot move %s to the Trash or delete it: %v", display(path), err)
	}
	fmt.Fprintf(os.Stderr, "note: %s could not be moved to the Trash and was deleted\n", display(path))
}

// darwin termios for the hidden prompt. Defined locally so the binary stays
// stdlib-only (no golang.org/x/sys).
const (
	ioctlGetTermios = 0x40487413 // TIOCGETA
	ioctlSetTermios = 0x80487414 // TIOCSETA
	termiosEcho     = 0x8        // ECHO
)

type termios struct {
	Iflag, Oflag, Cflag, Lflag uint64
	Cc                         [20]uint8
	Ispeed, Ospeed             uint64
}

func tcGetAttr(fd uintptr) (*termios, error) {
	t := &termios{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlGetTermios, uintptr(unsafe.Pointer(t)))
	if errno != 0 {
		return nil, errno
	}
	return t, nil
}

func tcSetAttr(fd uintptr, t *termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlSetTermios, uintptr(unsafe.Pointer(t)))
	if errno != 0 {
		return errno
	}
	return nil
}

// hiddenPrompt reads a line with echo disabled, like Python's getpass.
// It uses /dev/tty when available so piped stdin still gets a prompt.
func hiddenPrompt(prompt string) string {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprint(os.Stderr, prompt)
		return readLine(bufio.NewReader(os.Stdin))
	}
	defer tty.Close()
	fmt.Fprint(tty, prompt)
	orig, terr := tcGetAttr(tty.Fd())
	if terr == nil {
		noEcho := *orig
		noEcho.Lflag &^= termiosEcho
		if tcSetAttr(tty.Fd(), &noEcho) == nil {
			defer tcSetAttr(tty.Fd(), orig)
			defer fmt.Fprint(tty, "\n")
		}
	}
	return readLine(bufio.NewReader(tty))
}

func readLine(r *bufio.Reader) string {
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimRight(line, "\r\n")
}
