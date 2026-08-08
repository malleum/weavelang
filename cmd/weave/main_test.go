package main

import (
	"os"
	"strings"
	"testing"
)

// `weave fmt -` formats standard input to standard output, which is how an
// editor formats a buffer it has not written to disk.
func TestFmtReadsStdin(t *testing.T) {
	cases := []struct {
		name, in, want string
		args           []string
		wantErr        bool
	}{
		{
			name: "formats",
			in:   "a   =  1\n\n\nb is add   1 2\na\n",
			args: []string{"-"},
			want: "a is 1\n\nb is add 1 2\n\na\n",
		},
		{
			name: "check accepts formatted input",
			in:   "a is 1\n\na\n",
			args: []string{"-check", "-"},
			want: "",
		},
		{
			name:    "check rejects unformatted input",
			in:      "a=1\na\n",
			args:    []string{"-check", "-"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := withStdio(t, tc.in, func() error { return cmdFmt(tc.args) })
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// withStdio runs f with in on standard input and captures standard output.
func withStdio(t *testing.T, in string, f func() error) (string, error) {
	t.Helper()

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		stdinW.WriteString(in)
		stdinW.Close()
	}()

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = stdinR, stdoutW
	defer func() { os.Stdin, os.Stdout = oldIn, oldOut }()

	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := stdoutR.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- sb.String()
	}()

	runErr := f()
	stdoutW.Close()
	return <-done, runErr
}
