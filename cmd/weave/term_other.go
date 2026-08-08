//go:build !(linux || darwin || freebsd || netbsd || openbsd)

package main

import (
	"errors"
	"os"
)

// Without raw mode there is no line editor, so the REPL reads standard input
// the way it always did.
func isTerminal(*os.File) bool { return false }

func rawMode(*os.File) (func(), error) {
	return nil, errors.New("raw mode is not supported on this platform")
}
