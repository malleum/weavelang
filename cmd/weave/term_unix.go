//go:build linux || darwin || freebsd || netbsd || openbsd

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// Raw mode, by hand.
//
// The compiler has no dependencies and this is not enough code to justify the
// first one. It is two ioctls: read the terminal settings, write them back with
// the flags that make the terminal deliver keystrokes as they are typed rather
// than a line at a time.

// isTerminal reports whether f is a terminal, which is what decides between the
// line editor and reading standard input as a script.
func isTerminal(f *os.File) bool {
	_, err := tcget(f.Fd())
	return err == nil
}

func tcget(fd uintptr) (syscall.Termios, error) {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, getAttr,
		uintptr(unsafe.Pointer(&t)), 0, 0, 0)
	if errno != 0 {
		return t, errno
	}
	return t, nil
}

func tcset(fd uintptr, t *syscall.Termios) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, setAttr,
		uintptr(unsafe.Pointer(t)), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// rawMode puts the terminal into character-at-a-time mode and returns a
// function that puts it back. Every path out of the editor has to call that,
// including a panic, or the shell is left unusable.
func rawMode(f *os.File) (restore func(), err error) {
	old, err := tcget(f.Fd())
	if err != nil {
		return nil, err
	}
	raw := old
	// No canonical line editing, no echo (the editor draws the line itself),
	// and no signal generation — Ctrl-C is a key here, not an interrupt.
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	// No CR/LF translation and no flow control, so every byte arrives as typed.
	raw.Iflag &^= syscall.IXON | syscall.ICRNL | syscall.BRKINT | syscall.INPCK | syscall.ISTRIP
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := tcset(f.Fd(), &raw); err != nil {
		return nil, err
	}
	return func() { _ = tcset(f.Fd(), &old) }, nil
}
