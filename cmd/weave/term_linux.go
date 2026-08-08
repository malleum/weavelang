//go:build linux

package main

import "syscall"

// The ioctls that read and write terminal settings. Linux spells them TCGETS
// and TCSETS; the BSDs spell them TIOCGETA and TIOCSETA.
const (
	getAttr = syscall.TCGETS
	setAttr = syscall.TCSETS
)
