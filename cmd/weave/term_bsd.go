//go:build darwin || freebsd || netbsd || openbsd

package main

import "syscall"

const (
	getAttr = syscall.TIOCGETA
	setAttr = syscall.TIOCSETA
)
