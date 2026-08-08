package main

import (
	"io"
	"strings"
	"testing"
)

// The editor's keystroke handling, driven directly. ReadLine itself needs a
// terminal — raw mode is an ioctl on a real tty — but everything that decides
// what a key does lives below it and is ordinary code.

// typing feeds a script of keys to a fresh line and returns the buffer with the
// cursor marked by `|`, which makes the expectations readable.
func typing(t *testing.T, hist []string, keys ...rune) string {
	t.Helper()
	e := &editor{out: io.Discard, hist: hist}
	s := &state{mode: insert, hist: len(hist)}
	for _, k := range keys {
		if _, err := e.key(s, k); err != nil {
			t.Fatalf("key %q: %v", k, err)
		}
	}
	return string(s.buf[:s.pos]) + "|" + string(s.buf[s.pos:])
}

// text spells a string out as keystrokes.
func text(s string) []rune { return []rune(s) }

func keys(parts ...any) []rune {
	var out []rune
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			out = append(out, text(v)...)
		case rune:
			out = append(out, v)
		case int:
			out = append(out, rune(v))
		}
	}
	return out
}

func TestInsertingAndMovingAround(t *testing.T) {
	cases := []struct {
		name string
		keys []rune
		want string
	}{
		{"plain typing", keys("bend f xs"), "bend f xs|"},
		{"backspace", keys("bend", keyBackspace), "ben|"},
		{"home and end", keys("abc", keyCtrlA), "|abc"},
		{"left then insert", keys("ac", keyLeft, "b"), "ab|c"},
		{"kill to start", keys("bend f xs", keyCtrlU), "|"},
		{"kill to end", keys("bend f", keyCtrlA, keyCtrlK), "|"},
		{"delete a word back", keys("bend f xs", keyCtrlW), "bend f |"},
		{"arrow keys move", keys("abc", keyLeft, keyLeft, keyRight), "ab|c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := typing(t, nil, c.keys...); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestNormalModeMotions(t *testing.T) {
	// Esc leaves the cursor on the last character typed, as vi does.
	cases := []struct {
		name string
		keys []rune
		want string
	}{
		{"escape steps back", keys("abc", keyEsc), "ab|c"},
		{"zero and dollar", keys("bend f", keyEsc, '0'), "|bend f"},
		{"dollar", keys("bend f", keyEsc, '0', '$'), "bend |f"},
		{"word forward", keys("bend f xs", keyEsc, '0', 'w'), "bend |f xs"},
		{"word back", keys("bend f xs", keyEsc, 'b'), "bend f |xs"},
		{"end of word", keys("bend f xs", keyEsc, '0', 'e'), "ben|d f xs"},
		{"h and l", keys("abc", keyEsc, 'h', 'h', 'l'), "a|bc"},
		{"caret skips indent", keys("  ab", keyEsc, '0', '^'), "  |ab"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := typing(t, nil, c.keys...); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestNormalModeEditing(t *testing.T) {
	cases := []struct {
		name string
		keys []rune
		want string
	}{
		{"x deletes under the cursor", keys("abc", keyEsc, '0', 'x'), "|bc"},
		{"X deletes before", keys("abc", keyEsc, 'X'), "a|c"},
		{"dw deletes a word", keys("bend f xs", keyEsc, '0', 'd', 'w'), "|f xs"},
		{"db deletes back", keys("bend f xs", keyEsc, 'd', 'b'), "bend f |s"},
		{"dd clears the line", keys("bend f xs", keyEsc, 'd', 'd'), "|"},
		{"D deletes to the end", keys("bend f xs", keyEsc, '0', 'w', 'D'), "bend| "},
		{"cw then type", keys("bend f", keyEsc, '0', 'c', 'w', "sift "), "sift |f"},
		{"a appends after", keys("ab", keyEsc, '0', 'a', "X"), "aX|b"},
		{"A appends at the end", keys("ab", keyEsc, '0', 'A', "X"), "abX|"},
		{"I inserts at the start", keys("ab", keyEsc, 'I', "X"), "X|ab"},
		{"S replaces the line", keys("ab", keyEsc, 'S', "cd"), "cd|"},
		{"p pastes what x took", keys("ab", keyEsc, '0', 'x', 'p'), "b|a"},
		{"u undoes the last change", keys("abc", keyEsc, '0', 'x', 'u'), "|abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := typing(t, nil, c.keys...); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestHistoryRecall(t *testing.T) {
	hist := []string{"add 1 2", "bend f xs"}
	cases := []struct {
		name string
		keys []rune
		want string
	}{
		{"up gives the last line", keys(keyUp), "bend f xs|"},
		{"up twice goes further back", keys(keyUp, keyUp), "add 1 2|"},
		{"up then down returns", keys(keyUp, keyUp, keyDown), "bend f xs|"},
		{"down past the end restores what was typed",
			keys("half", keyUp, keyDown), "half|"},
		{"k and j do the same in normal mode",
			keys(keyEsc, 'k', 'k', 'j'), "bend f x|s"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := typing(t, hist, c.keys...); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestCtrlCAndCtrlD(t *testing.T) {
	e := &editor{out: io.Discard}

	s := &state{mode: insert}
	if _, err := e.key(s, keyCtrlC); err != errInterrupt {
		t.Errorf("Ctrl-C should abandon the line, got %v", err)
	}

	s = &state{mode: insert}
	if _, err := e.key(s, keyCtrlD); err != io.EOF {
		t.Errorf("Ctrl-D on an empty line should end the session, got %v", err)
	}

	// With something typed it deletes forwards instead, which is what every
	// other shell does.
	s = &state{mode: insert, buf: []rune("ab")}
	if _, err := e.key(s, keyCtrlD); err != nil {
		t.Fatal(err)
	}
	if string(s.buf) != "b" {
		t.Errorf("Ctrl-D mid-line should delete forwards, got %q", string(s.buf))
	}
}

func TestEnterFinishesTheLine(t *testing.T) {
	e := &editor{out: io.Discard}
	for _, k := range []rune{keyEnter, keyReturn} {
		s := &state{mode: insert, buf: []rune("add 1 2")}
		done, err := e.key(s, k)
		if err != nil || !done {
			t.Errorf("key %d: done=%v err=%v", k, done, err)
		}
	}
}

// Escape sequences arrive as one read, so the decoder has to take them apart —
// and a lone Esc has to stay a lone Esc, or vi mode is unreachable.
func TestKeyDecoding(t *testing.T) {
	cases := []struct {
		name  string
		bytes string
		want  []rune
	}{
		{"plain text", "abc", []rune{'a', 'b', 'c'}},
		{"arrow keys", "\x1b[A\x1b[B\x1b[C\x1b[D", []rune{keyUp, keyDown, keyRight, keyLeft}},
		{"home and end", "\x1b[H\x1b[F", []rune{keyHome, keyEnd}},
		{"delete", "\x1b[3~", []rune{keyDelete}},
		{"a lone escape", "\x1b", []rune{keyEsc}},
		{"escape then a key in the same read", "\x1bx", []rune{keyEsc, 'x'}},
		{"a multi-byte character", "é!", []rune{'é', '!'}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := &editor{out: io.Discard, pending: []byte(c.bytes), buf: make([]byte, 64)}
			for i, want := range c.want {
				got, err := e.next()
				if err != nil {
					t.Fatalf("key %d: %v", i, err)
				}
				if got != want {
					t.Errorf("key %d: got %d, want %d", i, got, want)
				}
			}
		})
	}
}

// History is written to a file so it survives the session, and neither blanks
// nor an immediately repeated line are worth keeping.
func TestHistoryIsDeduplicated(t *testing.T) {
	path := t.TempDir() + "/history"
	e := &editor{out: io.Discard, path: path}
	for _, line := range []string{"add 1 2", "add 1 2", "  ", "bend f xs"} {
		e.remember(line)
	}
	if len(e.hist) != 2 {
		t.Errorf("expected two entries, got %q", e.hist)
	}

	// A block is stored on one line, since history is recalled a line at a
	// time.
	e.remember("f x is\n  add x 1")
	back := &editor{out: io.Discard, path: path}
	back.load()
	if len(back.hist) != 3 {
		t.Fatalf("expected the history to be reloaded, got %q", back.hist)
	}
	if strings.Contains(back.hist[2], "\n") {
		t.Errorf("a stored entry should be one line, got %q", back.hist[2])
	}
}

// The script reader is what the tests and `weave repl < file` go through.
func TestScriptReaderPrintsThePrompt(t *testing.T) {
	var out strings.Builder
	r := newScriptReader(strings.NewReader("add 1 2\n"), &out)
	line, err := r.ReadLine("weave> ")
	if err != nil || line != "add 1 2" {
		t.Fatalf("got %q, %v", line, err)
	}
	if out.String() != "weave> " {
		t.Errorf("expected the prompt to be printed, got %q", out.String())
	}
	if _, err := r.ReadLine("weave> "); err != io.EOF {
		t.Errorf("expected EOF at the end of the script, got %v", err)
	}
}
