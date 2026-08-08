package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/check"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/parser"
	"github.com/malleum/weave/internal/style"
	"github.com/malleum/weave/internal/types"
)

// The REPL keeps a growing list of definitions and compiles the whole thing
// each time a line is entered. That is the honest model for a language whose
// programs are a set of definitions and one final expression, and it means
// what you see in the REPL is exactly what `weave run` would print.
//
// Rebuilding everything per line is only tolerable because the runtime is
// compiled once into cached object files and the C compiler runs at -O0, which
// keeps a turnaround in the tens of milliseconds.

const replBanner = `weave repl — the Wheel weaves as the Wheel wills

  type an expression to evaluate it, or a definition to add it
  a line ending in ` + "`is`" + ` or ` + "`:`" + ` starts a block; end it with a blank line
  up and down walk the history; Esc gives you vi motions on the line
  :help for commands, :quit to leave
`

const replHelp = `commands:
  :help            this message
  :list            show the definitions you have entered
  :type EXPR       show the type of an expression without running it
  :source PATH     read Source from this file when running
  :source          go back to an empty Source
  :drop            remove the definition entered most recently
  :clear           remove every definition
  :quit, :q        leave
`

type repl struct {
	lines      lineReader
	defs       []string // one entry per definition, in order
	sourcePath string   // the file feeding Source, if any
	cacheDir   string
	opts       build.Options
	out        io.Writer
}

func cmdRepl(args []string) error {
	fs := flag.NewFlagSet("repl", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := buildFlags(fs)
	if err := fs.Parse(args); err != nil {
		return errReported
	}
	// A REPL wants a fast compiler, not a fast program, so unless the flag was
	// given explicitly the C compiler runs without optimisation.
	explicitOpt := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "opt" {
			explicitOpt = true
		}
	})
	if !explicitOpt {
		opts.Opt = "-O0"
	}

	// Scratch space for the executables; the runtime cache is shared with the
	// rest of the toolchain and outlives the session.
	work, err := os.MkdirTemp("", "weave-repl-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	source := ""
	if fs.NArg() == 1 {
		// `weave repl input.txt` starts with Source already pointed at a file,
		// which is how an Advent of Code session usually begins.
		source = fs.Arg(0)
	}
	return runREPL(os.Stdin, os.Stdout, *opts, work, source)
}

// runREPL is the loop itself, reading from in and writing to out so that it can
// be driven by a test as well as by a terminal.
func runREPL(in io.Reader, out io.Writer, opts build.Options, work, sourcePath string) error {
	r := &repl{cacheDir: work, opts: opts, out: out, sourcePath: sourcePath}

	// A terminal gets the line editor; anything else is a script, and reading
	// it a line at a time is both simpler and what the tests need.
	if f, ok := in.(*os.File); ok && isTerminal(f) {
		r.lines = newEditor(f, out)
	} else {
		r.lines = newScriptReader(in, out)
	}

	fmt.Fprint(r.out, replBanner)
	if r.sourcePath != "" {
		fmt.Fprintf(r.out, "\nSource is %s\n", r.sourcePath)
	}
	fmt.Fprintln(r.out)

	// Warm the runtime cache so the first evaluation is as quick as the rest.
	r.warm()

	for {
		entry, err := r.readEntry()
		if err == errInterrupt {
			continue
		}
		if err != nil {
			fmt.Fprintln(r.out)
			return nil
		}
		if strings.TrimSpace(entry) == "" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(entry), ":") {
			if quit := r.command(strings.TrimSpace(entry)); quit {
				return nil
			}
			continue
		}
		r.eval(entry)
	}
}

// readEntry reads one entry, which continues across lines while the block is
// still open. A blank line ends a multi-line entry.
func (r *repl) readEntry() (string, error) {
	first, err := r.lines.ReadLine("weave> ")
	if err != nil || !opensBlock(first) {
		return first, err
	}

	lines := []string{first}
	for {
		line, err := r.lines.ReadLine(".....> ")
		if err == errInterrupt {
			// Abandoning a block abandons the whole entry, not just this line.
			return "", errInterrupt
		}
		if err != nil {
			break
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

// opensBlock reports whether a line leaves a block open, so the REPL should
// keep reading. These are the same two openers the layout rule recognises.
func opensBlock(line string) bool {
	t := strings.TrimSpace(stripComment(line))
	return strings.HasSuffix(t, " is") || t == "is" || strings.HasSuffix(t, ":")
}

func stripComment(line string) string {
	// Good enough for deciding whether a block is open: a `#` inside a literal
	// only ever makes the REPL ask for another line, which a blank line ends.
	if i := strings.IndexByte(line, '#'); i >= 0 {
		return line[:i]
	}
	return line
}

// program assembles the accumulated definitions plus an optional final
// expression.
func (r *repl) program(final string) string {
	var sb strings.Builder
	for _, d := range r.defs {
		sb.WriteString(d)
		sb.WriteString("\n\n")
	}
	if final != "" {
		sb.WriteString(final)
		sb.WriteString("\n")
	}
	return sb.String()
}

// eval decides whether the entry is a definition or an expression, and either
// records it or runs it.
func (r *repl) eval(entry string) {
	src := r.program(entry)
	bag := r.bag(src)
	file := parser.Parse(src, bag)
	if !bag.Empty() {
		r.report(bag)
		return
	}

	// A bare expression becomes the program's output; anything else is a
	// definition to keep.
	if file.Output() == nil {
		check.File(file, bag)
		if !bag.Empty() {
			r.report(bag)
			return
		}
		r.defs = append(r.defs, entry)
		r.notes(bag)
		r.confirm()
		return
	}

	r.run(src)
}

// bag returns a diagnostic bag that lets an incomplete definition through.
//
// `fib 0 is 1` does not handle every case, and in a program that is an error.
// In a REPL it is halfway through being typed: the next line is `fib n is …`.
// So the REPL is lenient — the definition is kept, the gap is reported as a
// note, and if you run something that reaches the missing case the program
// stops there and says so.
func (r *repl) bag(src string) *diag.Bag {
	b := diag.New("repl", src)
	b.Lenient = true
	return b
}

// notes prints the soft diagnostics of an accepted definition.
func (r *repl) notes(bag *diag.Bag) {
	st := style.For(r.out)
	for _, d := range bag.Notes() {
		fmt.Fprintf(r.out, "%s %s\n", st.Yellow("note:"), d.Msg)
		if d.Hint != "" {
			fmt.Fprintf(r.out, "      %s\n", st.Dim(d.Hint))
		}
	}
}

// confirm names what was just defined, with its inferred type.
func (r *repl) confirm() {
	src := r.program("")
	bag := r.bag(src)
	parsed := parser.Parse(src, bag)
	info := check.File(parsed, bag)
	if !bag.Empty() {
		return
	}
	// Report the definition this entry introduced, which is the last one.
	if len(parsed.Decls) == 0 {
		return
	}
	last := parsed.Decls[len(parsed.Decls)-1]
	if sch, ok := info.Decls[last.Name]; ok {
		fmt.Fprintf(r.out, "%s :: %s\n", last.Name, types.SchemeString(sch))
	}
}

// run compiles and executes a complete program, streaming its output.
func (r *repl) run(src string) {
	dir, err := os.MkdirTemp(r.cacheDir, "run-")
	if err != nil {
		fmt.Fprintf(r.out, "weave: %v\n", err)
		return
	}
	defer os.RemoveAll(dir)

	opts := r.opts
	opts.Output = filepath.Join(dir, "program")
	opts.Lenient = true

	bag := r.bag(src)
	res, err := build.Compile("repl.weave", src, opts, bag)
	if err != nil {
		if !bag.Empty() {
			r.report(bag)
			return
		}
		fmt.Fprintf(r.out, "weave: %v\n", err)
		return
	}

	cmd := exec.Command(res.Executable)
	cmd.Stdout = r.out
	cmd.Stderr = r.out
	if r.sourcePath != "" {
		f, err := os.Open(r.sourcePath)
		if err != nil {
			fmt.Fprintf(r.out, "weave: cannot read %s: %v\n", r.sourcePath, err)
			return
		}
		defer f.Close()
		cmd.Stdin = f
	} else {
		cmd.Stdin = strings.NewReader("")
	}
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(r.out, "weave: the program stopped: %v\n", err)
	}
}

// warm builds the runtime object cache up front.
func (r *repl) warm() {
	dir, err := os.MkdirTemp(r.cacheDir, "warm-")
	if err != nil {
		return
	}
	defer os.RemoveAll(dir)
	opts := r.opts
	opts.Output = filepath.Join(dir, "warm")
	bag := diag.New("warm", "0\n")
	_, _ = build.Compile("warm.weave", "0\n", opts, bag)
}

// command handles a `:` directive, reporting whether the REPL should stop.
func (r *repl) command(line string) bool {
	name, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimSpace(rest)

	switch name {
	case ":quit", ":q":
		return true
	case ":help", ":h":
		fmt.Fprint(r.out, replHelp)
	case ":list":
		if len(r.defs) == 0 {
			fmt.Fprintln(r.out, "no definitions yet")
		}
		for _, d := range r.defs {
			fmt.Fprintln(r.out, d)
		}
	case ":drop":
		if len(r.defs) == 0 {
			fmt.Fprintln(r.out, "nothing to drop")
			break
		}
		r.defs = r.defs[:len(r.defs)-1]
		fmt.Fprintln(r.out, "dropped the last definition")
	case ":clear":
		r.defs = nil
		fmt.Fprintln(r.out, "cleared")
	case ":source":
		if rest == "" {
			r.sourcePath = ""
			fmt.Fprintln(r.out, "Source is empty")
			break
		}
		if _, err := os.Stat(rest); err != nil {
			fmt.Fprintf(r.out, "weave: cannot read %s\n", rest)
			break
		}
		r.sourcePath = rest
		fmt.Fprintf(r.out, "Source is %s\n", rest)
	case ":type", ":t":
		r.showType(rest)
	default:
		fmt.Fprintf(r.out, "unknown command %s, try :help\n", name)
	}
	return false
}

// typeProbe is the name :type binds an expression to. A program's output must
// have the Show Talent, but a type query should work for anything — including a
// function — so the expression is checked as a definition instead.
const typeProbe = "_typeOf"

// showType checks an expression and prints its type without running it.
func (r *repl) showType(expr string) {
	if expr == "" {
		fmt.Fprintln(r.out, "usage: :type EXPRESSION")
		return
	}
	src := r.program(typeProbe + " is " + expr)
	bag := r.bag(src)
	file := parser.Parse(src, bag)
	if !bag.Empty() {
		r.report(bag)
		return
	}
	info := check.File(file, bag)
	if !bag.Empty() {
		r.report(bag)
		return
	}
	sch, ok := info.Decls[typeProbe]
	if !ok {
		fmt.Fprintln(r.out, "that is not an expression")
		return
	}
	fmt.Fprintf(r.out, "%s :: %s\n", expr, types.SchemeString(sch))
}

// report prints diagnostics without the file name, which is noise in a REPL.
func (r *repl) report(bag *diag.Bag) {
	for _, line := range strings.Split(bag.String(), "\n") {
		fmt.Fprintln(r.out, strings.TrimPrefix(line, "repl:"))
	}
}
