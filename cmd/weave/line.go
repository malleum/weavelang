package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// A line editor with vi bindings, for `weave repl`.
//
// The REPL used to read standard input with a Scanner, which means the terminal
// does the editing: you get backspace and nothing else. No history, no arrow
// keys, no way to fix the middle of a line. This is the same shape of editor
// fish and zsh give you in vi mode, and no more: two modes, the motions and
// operators that come up while editing one line, history on the arrow keys and
// on k/j, and a history file that survives the session.
//
// It is deliberately small. There is no wrapping, no completion, no search, and
// no multi-line editing — the REPL reads a block a line at a time, so a line is
// all this ever has to draw.

// editor reads lines from a terminal in raw mode.
type editor struct {
	in   *os.File
	out  io.Writer
	hist []string
	// path is where history is saved, or empty to keep it in memory only.
	path string
	// buffered input, so an escape sequence arrives whole.
	pending []byte
	buf     []byte
}

// mode is which half of vi the editor is in.
type mode int

const (
	insert mode = iota
	normal
)

// newEditor opens the history file and returns an editor over in and out.
func newEditor(in *os.File, out io.Writer) *editor {
	e := &editor{in: in, out: out, buf: make([]byte, 256)}
	e.path = historyPath()
	e.load()
	return e
}

func historyPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "weave", "repl-history")
}

const historyLimit = 1000

func (e *editor) load() {
	if e.path == "" {
		return
	}
	data, err := os.ReadFile(e.path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" {
			e.hist = append(e.hist, line)
		}
	}
	if n := len(e.hist); n > historyLimit {
		e.hist = e.hist[n-historyLimit:]
	}
}

// remember adds a line to the history, skipping blanks and immediate repeats,
// and appends it to the history file.
func (e *editor) remember(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	if n := len(e.hist); n > 0 && e.hist[n-1] == line {
		return
	}
	e.hist = append(e.hist, line)
	if e.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(e.path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(e.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	// A multi-line entry is stored as one line, since history is recalled a
	// line at a time.
	fmt.Fprintln(f, strings.ReplaceAll(line, "\n", " "))
}

// state is one line being edited.
type state struct {
	buf  []rune
	pos  int
	mode mode
	// hist is where in the history the recall is, len(hist) meaning the line
	// being typed, which is kept in fresh while older entries are shown.
	hist  int
	fresh []rune
	// pending is an operator waiting for a motion: "d", "c" or "y".
	pending rune
	// yank is what the last delete or change took, for p and P.
	yank []rune
	// undo is the buffer before the last change, for u.
	undo    []rune
	undoPos int
}

// errInterrupt is Ctrl-C: abandon this line, but keep the REPL running.
var errInterrupt = fmt.Errorf("interrupted")

// ReadLine draws the prompt and edits one line, returning it without the
// newline. It returns io.EOF on Ctrl-D at an empty line, and errInterrupt on
// Ctrl-C.
func (e *editor) ReadLine(prompt string) (string, error) {
	restore, err := rawMode(e.in)
	if err != nil {
		return "", err
	}
	defer restore()

	s := &state{mode: insert}
	s.hist = len(e.hist)
	e.draw(prompt, s)
	defer fmt.Fprint(e.out, cursorBar) // leave the terminal's cursor as found

	for {
		r, err := e.next()
		if err != nil {
			return "", err
		}
		done, err := e.key(s, r)
		if err != nil {
			fmt.Fprint(e.out, "\r\n")
			return "", err
		}
		if done {
			fmt.Fprint(e.out, "\r\n")
			line := string(s.buf)
			e.remember(line)
			return line, nil
		}
		e.draw(prompt, s)
	}
}

// key is one keystroke. It reports whether the line is finished.
func (e *editor) key(s *state, r rune) (bool, error) {
	switch r {
	case keyEnter, keyReturn:
		return true, nil
	case keyCtrlC:
		return false, errInterrupt
	case keyCtrlD:
		if len(s.buf) == 0 {
			return false, io.EOF
		}
		s.delete(s.pos, s.pos+1)
		return false, nil
	case keyUp:
		e.recall(s, -1)
		return false, nil
	case keyDown:
		e.recall(s, +1)
		return false, nil
	case keyLeft:
		s.left()
		return false, nil
	case keyRight:
		s.right()
		return false, nil
	case keyHome:
		s.pos = 0
		return false, nil
	case keyEnd:
		s.pos = len(s.buf)
		return false, nil
	case keyDelete:
		s.delete(s.pos, s.pos+1)
		return false, nil
	}
	if s.mode == insert {
		return false, e.insertKey(s, r)
	}
	return e.normalKey(s, r)
}

func (e *editor) insertKey(s *state, r rune) error {
	switch r {
	case keyEsc:
		s.mode = normal
		// vi leaves the cursor on the last character typed, not past it.
		if s.pos > 0 {
			s.pos--
		}
	case keyBackspace, keyBackspaceAlt:
		if s.pos > 0 {
			s.delete(s.pos-1, s.pos)
			s.pos--
		}
	case keyCtrlA:
		s.pos = 0
	case keyCtrlE:
		s.pos = len(s.buf)
	case keyCtrlU:
		s.save()
		s.buf = append([]rune{}, s.buf[s.pos:]...)
		s.pos = 0
	case keyCtrlK:
		s.save()
		s.buf = s.buf[:s.pos]
	case keyCtrlW:
		s.save()
		start := wordStart(s.buf, s.pos)
		s.delete(start, s.pos)
		s.pos = start
	case keyCtrlL:
		fmt.Fprint(e.out, "\x1b[H\x1b[2J")
	default:
		if unicode.IsPrint(r) {
			s.save()
			s.insert(r)
		}
	}
	return nil
}

// normalKey handles vi's normal mode: motions, operators, and the keys that go
// back to inserting.
func (e *editor) normalKey(s *state, r rune) (bool, error) {
	// An operator is waiting, so this key is its motion.
	if s.pending != 0 {
		op := s.pending
		s.pending = 0
		// `dd` and `cc` take the whole line.
		if r == op {
			s.save()
			s.yank = append([]rune{}, s.buf...)
			s.buf = nil
			s.pos = 0
			if op == 'c' {
				s.mode = insert
			}
			return false, nil
		}
		from, to, ok := s.motion(r)
		if !ok {
			return false, nil
		}
		s.save()
		s.yank = append([]rune{}, s.buf[from:to]...)
		s.delete(from, to)
		s.pos = from
		if op == 'c' {
			s.mode = insert
		} else if s.pos >= len(s.buf) && s.pos > 0 {
			s.pos = len(s.buf) - 1
		}
		return false, nil
	}

	switch r {
	case 'i':
		s.mode = insert
	case 'a':
		s.mode = insert
		s.right0()
	case 'I':
		s.mode = insert
		s.pos = 0
	case 'A':
		s.mode = insert
		s.pos = len(s.buf)
	case 'd', 'c':
		s.pending = r
	case 'D':
		s.save()
		s.yank = append([]rune{}, s.buf[s.pos:]...)
		s.buf = s.buf[:s.pos]
		s.clamp()
	case 'C':
		s.save()
		s.yank = append([]rune{}, s.buf[s.pos:]...)
		s.buf = s.buf[:s.pos]
		s.mode = insert
	case 'S':
		s.save()
		s.buf, s.pos = nil, 0
		s.mode = insert
	case 'x':
		s.save()
		if s.pos < len(s.buf) {
			s.yank = []rune{s.buf[s.pos]}
			s.delete(s.pos, s.pos+1)
			s.clamp()
		}
	case 'X':
		s.save()
		if s.pos > 0 {
			s.yank = []rune{s.buf[s.pos-1]}
			s.delete(s.pos-1, s.pos)
			s.pos--
		}
	case 'p':
		s.save()
		s.right0()
		s.insertAll(s.yank)
		s.left()
	case 'P':
		s.save()
		s.insertAll(s.yank)
		s.left()
	case 'r':
		next, err := e.next()
		if err != nil {
			return false, err
		}
		if unicode.IsPrint(next) && s.pos < len(s.buf) {
			s.save()
			s.buf[s.pos] = next
		}
	case 'u':
		if s.undo != nil {
			s.buf, s.pos = s.undo, s.undoPos
			s.undo = nil
			s.clamp()
		}
	case 'k':
		e.recall(s, -1)
	case 'j':
		e.recall(s, +1)
	case 'h':
		s.left()
	case 'l':
		s.right()
	case '0':
		s.pos = 0
	case '^':
		s.pos = firstWord(s.buf)
	case '$':
		s.pos = len(s.buf)
		s.clamp()
	case 'w', 'W':
		s.pos = nextWord(s.buf, s.pos, r == 'W')
		s.clamp()
	case 'b', 'B':
		s.pos = prevWord(s.buf, s.pos, r == 'B')
	case 'e', 'E':
		s.pos = wordEnd(s.buf, s.pos, r == 'E')
	}
	return false, nil
}

// motion resolves a motion key into the span it covers from the cursor.
func (s *state) motion(r rune) (from, to int, ok bool) {
	switch r {
	case 'w', 'W':
		return s.pos, nextWord(s.buf, s.pos, r == 'W'), true
	case 'b', 'B':
		return prevWord(s.buf, s.pos, r == 'B'), s.pos, true
	case 'e', 'E':
		end := wordEnd(s.buf, s.pos, r == 'E')
		return s.pos, min(end+1, len(s.buf)), true
	case '$':
		return s.pos, len(s.buf), true
	case '0':
		return 0, s.pos, true
	case '^':
		return firstWord(s.buf), s.pos, true
	case 'l':
		return s.pos, min(s.pos+1, len(s.buf)), true
	case 'h':
		return max(s.pos-1, 0), s.pos, true
	}
	return 0, 0, false
}

// ---------------------------------------------------------------- the buffer

func (s *state) save() {
	s.undo = append([]rune{}, s.buf...)
	s.undoPos = s.pos
}

func (s *state) insert(r rune) {
	s.buf = append(s.buf, 0)
	copy(s.buf[s.pos+1:], s.buf[s.pos:])
	s.buf[s.pos] = r
	s.pos++
}

func (s *state) insertAll(rs []rune) {
	for _, r := range rs {
		s.insert(r)
	}
}

func (s *state) delete(from, to int) {
	from, to = max(from, 0), min(to, len(s.buf))
	if from >= to {
		return
	}
	s.buf = append(s.buf[:from], s.buf[to:]...)
}

func (s *state) left() {
	if s.pos > 0 {
		s.pos--
	}
}

// right stops one short of the end in normal mode, as vi does.
func (s *state) right() {
	limit := len(s.buf)
	if s.mode == normal && limit > 0 {
		limit--
	}
	if s.pos < limit {
		s.pos++
	}
}

// right0 moves past the last character, which `a` and `p` need.
func (s *state) right0() {
	if s.pos < len(s.buf) {
		s.pos++
	}
}

func (s *state) clamp() {
	if s.mode == normal && s.pos >= len(s.buf) {
		s.pos = max(len(s.buf)-1, 0)
	}
	if s.pos > len(s.buf) {
		s.pos = len(s.buf)
	}
}

// ---------------------------------------------------------------- word moves

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// nextWord is `w`: the start of the next word. big treats any run of
// non-blanks as one word, which is `W`.
func nextWord(buf []rune, pos int, big bool) int {
	n := len(buf)
	if pos >= n {
		return n
	}
	class := func(r rune) int {
		switch {
		case unicode.IsSpace(r):
			return 0
		case big || isWordRune(r):
			return 1
		default:
			return 2
		}
	}
	start := class(buf[pos])
	i := pos
	for i < n && class(buf[i]) == start && start != 0 {
		i++
	}
	for i < n && unicode.IsSpace(buf[i]) {
		i++
	}
	return i
}

// prevWord is `b`.
func prevWord(buf []rune, pos int, big bool) int {
	i := pos - 1
	for i >= 0 && unicode.IsSpace(buf[i]) {
		i--
	}
	if i < 0 {
		return 0
	}
	inWord := big || isWordRune(buf[i])
	for i >= 0 && !unicode.IsSpace(buf[i]) && (big || isWordRune(buf[i]) == inWord) {
		i--
	}
	return i + 1
}

// wordEnd is `e`: the last character of the word the cursor is in or before.
func wordEnd(buf []rune, pos int, big bool) int {
	n := len(buf)
	i := pos + 1
	for i < n && unicode.IsSpace(buf[i]) {
		i++
	}
	if i >= n {
		return max(n-1, 0)
	}
	inWord := big || isWordRune(buf[i])
	for i+1 < n && !unicode.IsSpace(buf[i+1]) && (big || isWordRune(buf[i+1]) == inWord) {
		i++
	}
	return i
}

// wordStart is what Ctrl-W deletes back to.
func wordStart(buf []rune, pos int) int {
	i := pos
	for i > 0 && unicode.IsSpace(buf[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(buf[i-1]) {
		i--
	}
	return i
}

func firstWord(buf []rune) int {
	i := 0
	for i < len(buf) && unicode.IsSpace(buf[i]) {
		i++
	}
	return i
}

// ------------------------------------------------------------------- history

// recall steps through the history, keeping the half-typed line so that coming
// back down returns it.
func (e *editor) recall(s *state, delta int) {
	if len(e.hist) == 0 {
		return
	}
	if s.hist == len(e.hist) {
		s.fresh = append([]rune{}, s.buf...)
	}
	next := s.hist + delta
	if next < 0 {
		next = 0
	}
	if next > len(e.hist) {
		next = len(e.hist)
	}
	if next == s.hist {
		return
	}
	s.hist = next
	if s.hist == len(e.hist) {
		s.buf = append([]rune{}, s.fresh...)
	} else {
		s.buf = []rune(e.hist[s.hist])
	}
	s.pos = len(s.buf)
	s.clamp()
}

// ------------------------------------------------------------------- drawing

const (
	cursorBlock = "\x1b[2 q"
	cursorBar   = "\x1b[6 q"
)

// draw repaints the line in place. The prompt is redrawn every time, which is
// simpler than tracking what changed and costs nothing at this size.
func (e *editor) draw(prompt string, s *state) {
	var b strings.Builder
	b.WriteString("\r\x1b[K")
	b.WriteString(prompt)
	b.WriteString(string(s.buf))
	// Put the cursor where it belongs by going back to column zero and
	// stepping forward, which needs no knowledge of the terminal width.
	b.WriteString("\r")
	if n := len([]rune(prompt)) + s.pos; n > 0 {
		fmt.Fprintf(&b, "\x1b[%dC", n)
	}
	if s.mode == normal {
		b.WriteString(cursorBlock)
	} else {
		b.WriteString(cursorBar)
	}
	fmt.Fprint(e.out, b.String())
}

// --------------------------------------------------------------- input decode

// The keys the editor names. Anything above keyMax is a real rune.
const (
	keyCtrlA        = 1
	keyCtrlC        = 3
	keyCtrlD        = 4
	keyCtrlE        = 5
	keyBackspaceAlt = 8
	keyEnter        = 10
	keyCtrlK        = 11
	keyCtrlL        = 12
	keyReturn       = 13
	keyCtrlU        = 21
	keyCtrlW        = 23
	keyEsc          = 27
	keyBackspace    = 127

	// Above the Unicode range, so they cannot collide with a typed character.
	keyUp = 0x110000 + iota
	keyDown
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyDelete
)

// next returns one keystroke, decoding escape sequences into the key constants
// above. Reading a chunk at a time is what separates a bare Esc from an arrow
// key: a terminal sends the three bytes of an arrow key together, so if Esc
// arrives alone in a read, it was typed.
func (e *editor) next() (rune, error) {
	if len(e.pending) == 0 {
		n, err := e.in.Read(e.buf)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, io.EOF
		}
		e.pending = append(e.pending[:0], e.buf[:n]...)
	}

	b := e.pending[0]
	if b != keyEsc || len(e.pending) == 1 {
		e.pending = e.pending[1:]
		if b < 0x80 {
			return rune(b), nil
		}
		// A multi-byte character: put the byte back and decode the run.
		e.pending = append([]byte{b}, e.pending...)
		r, size := decodeRune(e.pending)
		e.pending = e.pending[size:]
		return r, nil
	}

	// An escape sequence. CSI is `Esc [`, then parameters, then a final byte.
	if len(e.pending) >= 3 && e.pending[1] == '[' {
		switch e.pending[2] {
		case 'A', 'B', 'C', 'D', 'H', 'F':
			k := map[byte]rune{
				'A': keyUp, 'B': keyDown, 'C': keyRight, 'D': keyLeft,
				'H': keyHome, 'F': keyEnd,
			}[e.pending[2]]
			e.pending = e.pending[3:]
			return k, nil
		}
		// `Esc [ 3 ~` is Delete; anything else with a numeric parameter is
		// swallowed rather than typed into the line.
		end := 2
		for end < len(e.pending) && e.pending[end] >= '0' && e.pending[end] <= '9' {
			end++
		}
		if end < len(e.pending) && e.pending[end] == '~' {
			param := string(e.pending[2:end])
			e.pending = e.pending[end+1:]
			if param == "3" {
				return keyDelete, nil
			}
			return e.next()
		}
	}
	e.pending = e.pending[1:]
	return keyEsc, nil
}

// decodeRune reads one UTF-8 character out of b.
func decodeRune(b []byte) (rune, int) {
	for _, r := range string(b) {
		return r, len(string(r))
	}
	return 0, 1
}

// --------------------------------------------------------------- the fallback

// scriptReader reads lines from something that is not a terminal, which is how
// the tests and `weave repl < file` drive the REPL.
type scriptReader struct {
	in  *bufio.Scanner
	out io.Writer
}

func newScriptReader(in io.Reader, out io.Writer) *scriptReader {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	return &scriptReader{in: sc, out: out}
}

func (r *scriptReader) ReadLine(prompt string) (string, error) {
	fmt.Fprint(r.out, prompt)
	if !r.in.Scan() {
		return "", io.EOF
	}
	return r.in.Text(), nil
}

// lineReader is what the REPL loop reads through, so that the terminal and the
// script cases are the same code above.
type lineReader interface {
	ReadLine(prompt string) (string, error)
}
