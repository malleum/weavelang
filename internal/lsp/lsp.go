// Package lsp serves the Language Server Protocol for Weave over stdio.
//
// Everything it reports comes from the compiler's own front end, so the editor
// and `weave check` can never disagree. A full parse and type-check takes about
// four milliseconds, which is fast enough to redo on every keystroke rather
// than debouncing or maintaining an incremental model.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/check"
	"github.com/malleum/weave/internal/codegen"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/format"
	"github.com/malleum/weave/internal/lexer"
	"github.com/malleum/weave/internal/mdblock"
	"github.com/malleum/weave/internal/parser"
	"github.com/malleum/weave/internal/prelude"
	"github.com/malleum/weave/internal/token"
	"github.com/malleum/weave/internal/types"
)

// Serve runs the language server until its input closes.
func Serve(in io.Reader, out io.Writer) error {
	s := &server{
		r:    bufio.NewReader(in),
		w:    out,
		docs: map[string]string{},
	}
	return s.run()
}

type server struct {
	r  *bufio.Reader
	w  io.Writer
	mu sync.Mutex

	docs map[string]string // uri -> text
}

// ------------------------------------------------------------- JSON-RPC

// message is one JSON-RPC frame, in either direction.
//
// Result is raw rather than `any` on purpose. A response to a request must
// carry `result` or `error`, and `result: null` is the answer for "there is
// nothing here" — a hover over a word with no documentation, or signature help
// where no verb is being applied. With `any` those answers marshalled to a
// frame holding neither field, which is not a valid response and which Neovim
// rejects outright with INVALID_SERVER_MESSAGE. Marshalling the result before
// it reaches this struct means nil becomes the four bytes `null`, which
// omitempty keeps.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *server) run() error {
	for {
		msg, err := s.read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.handle(msg); err != nil {
			return err
		}
	}
}

// read pulls one Content-Length framed message.
func (s *server) read() (*message, error) {
	length := 0
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if name, value, ok := strings.Cut(line, ":"); ok &&
			strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, err
			}
		}
	}
	if length == 0 {
		return nil, io.EOF
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(s.r, body); err != nil {
		return nil, err
	}
	var msg message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (s *server) write(msg *message) error {
	msg.JSONRPC = "2.0"
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := fmt.Fprintf(s.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = s.w.Write(body)
	return err
}

func (s *server) reply(id json.RawMessage, result any) error {
	if id == nil {
		return nil // a notification wants no answer
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.write(&message{ID: id, Result: raw})
}

func (s *server) notify(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return s.write(&message{Method: method, Params: raw})
}

// ------------------------------------------------------------- dispatch

func (s *server) handle(msg *message) error {
	switch msg.Method {
	case "initialize":
		return s.reply(msg.ID, map[string]any{
			"capabilities": map[string]any{
				// Full sync: re-checking a whole file costs milliseconds, so
				// there is nothing to gain from incremental updates.
				"textDocumentSync":   1,
				"hoverProvider":      true,
				"completionProvider": map[string]any{"triggerCharacters": []string{"|", " "}},
				"signatureHelpProvider": map[string]any{
					"triggerCharacters": []string{" ", "("},
				},
				"documentFormattingProvider": true,
			},
			"serverInfo": map[string]any{"name": "weave", "version": "dev"},
		})

	case "initialized", "$/cancelRequest", "$/setTrace",
		"workspace/didChangeConfiguration", "workspace/didChangeWatchedFiles",
		"textDocument/didSave":
		return nil

	case "shutdown":
		return s.reply(msg.ID, nil)

	case "exit":
		return io.EOF

	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"textDocument"`
		}
		json.Unmarshal(msg.Params, &p)
		s.docs[p.TextDocument.URI] = p.TextDocument.Text
		return s.publish(p.TextDocument.URI)

	case "textDocument/didChange":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		json.Unmarshal(msg.Params, &p)
		if len(p.ContentChanges) > 0 {
			s.docs[p.TextDocument.URI] = p.ContentChanges[len(p.ContentChanges)-1].Text
		}
		return s.publish(p.TextDocument.URI)

	case "textDocument/didClose":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		json.Unmarshal(msg.Params, &p)
		delete(s.docs, p.TextDocument.URI)
		return s.notify("textDocument/publishDiagnostics", map[string]any{
			"uri": p.TextDocument.URI, "diagnostics": []any{},
		})

	case "textDocument/hover":
		return s.reply(msg.ID, s.hover(msg.Params))

	case "textDocument/completion":
		return s.reply(msg.ID, s.complete(msg.Params))

	case "textDocument/signatureHelp":
		return s.reply(msg.ID, s.signature(msg.Params))

	case "textDocument/formatting":
		return s.reply(msg.ID, s.format(msg.Params))
	}

	if msg.ID != nil {
		return s.write(&message{ID: msg.ID, Error: &rpcError{
			Code: -32601, Message: "unsupported method " + msg.Method,
		}})
	}
	return nil
}

// ---------------------------------------------------------------- views
//
// A .weave file is one Weave program starting at line zero. A Markdown file is
// several, each starting wherever its fence does. Everything below works on a
// view rather than on a document, so the two cases are the same code.

// view is a Weave program and where it starts in the document that holds it.
type view struct {
	src string
	// line is the zero-based document line the program's first line sits on.
	line int
}

// views returns every Weave program in a document.
func (s *server) views(uri string) []view {
	src, ok := s.docs[uri]
	if !ok {
		return nil
	}
	if !mdblock.IsMarkdown(uri) {
		return []view{{src: src}}
	}
	var out []view
	for _, b := range mdblock.Blocks(src) {
		out = append(out, view{src: b.Src, line: b.Line})
	}
	return out
}

// viewAt returns the program containing a position, with the position
// translated into it. A position outside every block has no program.
func (s *server) viewAt(uri string, pos token.Pos) (view, token.Pos, bool) {
	src, ok := s.docs[uri]
	if !ok {
		return view{}, pos, false
	}
	if !mdblock.IsMarkdown(uri) {
		return view{src: src}, pos, true
	}
	b, found := mdblock.At(mdblock.Blocks(src), pos.Line-1)
	if !found {
		return view{}, pos, false
	}
	return view{src: b.Src, line: b.Line}, token.Pos{Line: pos.Line - b.Line, Col: pos.Col}, true
}

// document maps a position inside a view back to the document.
func (v view) document(pos token.Pos) token.Pos {
	return token.Pos{Line: pos.Line + v.line, Col: pos.Col}
}

// ------------------------------------------------------------ diagnostics

func (s *server) publish(uri string) error {
	out := []any{}
	// Each program is checked on its own — a Markdown file's blocks are
	// separate programs, and a name defined in one is not in scope in the next.
	for _, v := range s.views(uri) {
		bag := diag.New(uri, v.src)
		file := parser.Parse(v.src, bag)
		if bag.Empty() {
			info := check.File(file, bag)
			if bag.Empty() {
				// Some problems only the backend sees — an endless `flow` with
				// nothing to stop it, a verb the runtime has yet to implement —
				// and the editor should show those too. The C is thrown away.
				codegen.Generate(file, info, bag, codegen.Options{})
			}
		}
		for _, d := range bag.All() {
			message := d.Msg
			if d.Hint != "" {
				message += "\n\nhint: " + d.Hint
			}
			out = append(out, map[string]any{
				"range":    rangeAt(v.document(d.Pos), wordLenAt(v.src, d.Pos)),
				"severity": severity(d),
				"source":   "weave",
				"message":  message,
			})
		}
	}
	return s.notify("textDocument/publishDiagnostics", map[string]any{
		"uri": uri, "diagnostics": out,
	})
}

// severity maps a diagnostic onto the protocol's scale. A definition that does
// not cover every case is the one thing the compiler could proceed past, so it
// is a warning rather than an error while you are still writing the clauses.
func severity(d diag.Diagnostic) int {
	if d.Soft {
		return 2 // warning
	}
	return 1 // error
}

// ------------------------------------------------------------------ hover

func (s *server) hover(params json.RawMessage) any {
	uri, pos := documentPosition(params)
	v, local, ok := s.viewAt(uri, pos)
	if !ok {
		return nil
	}
	word, at := wordAt(v.src, local)
	if word == "" {
		return nil
	}

	if doc := s.describe(v.src, uri, word, at); doc != "" {
		return map[string]any{
			"contents": map[string]any{"kind": "markdown", "value": doc},
			"range":    rangeAt(v.document(at), len(word)),
		}
	}
	return nil
}

// describe renders what is known about a name at a position: its inferred
// type, and its documentation when it is a built-in.
func (s *server) describe(src, uri, word string, at token.Pos) string {
	bag := diag.New(uri, src)
	file := parser.Parse(src, bag)
	info := check.File(file, diag.New(uri, src))

	var b strings.Builder

	// A local or an argument: the checker recorded the type of the very
	// expression sitting at this position.
	if t := typeAtPos(info, at); t != "" {
		fmt.Fprintf(&b, "```weave\n%s :: %s\n```", word, t)
	}

	if sch, ok := info.Decls[word]; ok {
		b.Reset()
		fmt.Fprintf(&b, "```weave\n%s :: %s\n```", word, types.SchemeString(sch))
	}

	if e, ok := preludeEntry(word); ok {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		sig := e.Sig
		if e.Where != "" {
			sig += "  where " + e.Where
		}
		fmt.Fprintf(&b, "```weave\n%s :: %s\n```\n\n%s", word, sig, e.Doc)
	}
	if c, ok := preludeCtor(word); ok {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "```weave\n%s :: %s\n```\n\n%s", word, c.Sig, c.Doc)
	}
	for _, td := range file.Types {
		if td.Name == word {
			b.Reset()
			fmt.Fprintf(&b, "```weave\n%s\n```\n\ndeclared in this file", typeDeclString(td))
		}
		for _, ct := range td.Ctors {
			if ct.Name != word {
				continue
			}
			b.Reset()
			fmt.Fprintf(&b, "```weave\n%s :: %s\n```\n\na %s", word, ctorSig(td, ct), td.Name)
		}
	}
	return b.String()
}

// ctorSig renders a declared constructor's type, `Direction -> Earth -> Move`.
func ctorSig(td *ast.TypeDecl, ct *ast.CtorDecl) string {
	out := ""
	for _, f := range ct.Fields {
		out += typeExprString(f, true) + " -> "
	}
	return out + typeExprString(headType(td), false)
}

// typeDeclString renders a declaration the way it was written.
func typeDeclString(td *ast.TypeDecl) string {
	out := typeExprString(headType(td), false) + " is"
	for i, ct := range td.Ctors {
		if i > 0 {
			out += " |"
		}
		out += " " + ct.Name
		for _, f := range ct.Fields {
			out += " " + typeExprString(f, true)
		}
	}
	return out
}

func headType(td *ast.TypeDecl) *ast.TypeExpr {
	head := &ast.TypeExpr{Name: td.Name, P: td.NamePos}
	for _, p := range td.Params {
		head.Args = append(head.Args, &ast.TypeExpr{Name: p, P: td.NamePos})
	}
	return head
}

// typeExprString prints written type syntax; atom brackets an applied type, as
// a constructor field must be.
func typeExprString(t *ast.TypeExpr, atom bool) string {
	switch t.Name {
	case ast.FuncTypeName:
		if len(t.Args) != 2 {
			return "?"
		}
		return "(" + typeExprString(t.Args[0], false) + " -> " + typeExprString(t.Args[1], false) + ")"
	case ast.TwineTypeName:
		parts := make([]string, len(t.Args))
		for i, a := range t.Args {
			parts[i] = typeExprString(a, false)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	}
	out := t.Name
	for _, a := range t.Args {
		out += " " + typeExprString(a, true)
	}
	if atom && len(t.Args) > 0 {
		return "(" + out + ")"
	}
	return out
}

// typeAtPos finds the expression that starts exactly here and returns its type.
func typeAtPos(info *check.Info, at token.Pos) string {
	for e, t := range info.Types {
		p := e.Pos()
		if p.Line != at.Line || p.Col != at.Col {
			continue
		}
		switch e.(type) {
		case *ast.Var, *ast.Ctor:
			return types.String(t)
		}
	}
	return ""
}

// -------------------------------------------------------------- completion

func (s *server) complete(params json.RawMessage) any {
	uri, pos := documentPosition(params)
	src := s.docs[uri]
	// Inside a Markdown fence the program is the block, so the definitions
	// offered are the block's own.
	if v, _, ok := s.viewAt(uri, pos); ok {
		src = v.src
	}

	items := []any{}
	seen := map[string]bool{}

	// insertText is always the bare name, and insertTextFormat says it is text
	// rather than a snippet. Weave applies by juxtaposition — `bend f xs`, never
	// `bend(f, xs)` — so a client that helpfully adds a pair of parentheses
	// after a completion is adding a syntax error. Saying so explicitly is what
	// a well-behaved client reads; the ones that guess from the item kind are
	// handled in weave.nvim, which turns their guessing off for Weave buffers.
	add := func(label, detail, doc string, kind int) {
		if seen[label] {
			return
		}
		seen[label] = true
		items = append(items, map[string]any{
			"label":            label,
			"kind":             kind,
			"detail":           detail,
			"documentation":    doc,
			"insertText":       label,
			"insertTextFormat": 1, // plain text, not a snippet
		})
	}

	// The program's own definitions first: they are what the author just wrote.
	bag := diag.New(uri, src)
	file := parser.Parse(src, bag)
	info := check.File(file, diag.New(uri, src))
	for _, d := range file.Decls {
		detail := ""
		if sch, ok := info.Decls[d.Name]; ok {
			detail = types.SchemeString(sch)
		}
		kind := 3 // function
		if d.Arity() == 0 {
			kind = 6 // variable
		}
		add(d.Name, detail, "defined in this file", kind)
	}
	for _, td := range file.Types {
		add(td.Name, typeDeclString(td), "declared in this file", 22) // struct
		for _, ct := range td.Ctors {
			add(ct.Name, ctorSig(td, ct), "a "+td.Name, 4) // constructor
		}
	}

	for _, e := range prelude.Values {
		detail := e.Sig
		if e.Where != "" {
			detail += "  where " + e.Where
		}
		add(e.Name, detail, e.Doc, 3)
	}
	for _, c := range prelude.Ctors {
		add(c.Name, c.Sig, c.Doc, 4) // constructor
	}
	// Read from the token table rather than written out again, so a word
	// cannot be reserved without the editor knowing it. `it` and `_` are the
	// same token as `this` and are left out: three ways to complete one thing
	// is noise, and `this` is the one the formatter prints.
	for _, kw := range lexer.CompletionKeywords() {
		add(kw, "keyword", "", 14)
	}

	return map[string]any{"isIncomplete": false, "items": items}
}

// ----------------------------------------------------------- signature help

// signature answers with the type of the verb whose arguments are being
// written, which is the thing worth knowing in a language of curried,
// data-last functions.
func (s *server) signature(params json.RawMessage) any {
	uri, pos := documentPosition(params)
	v, local, ok := s.viewAt(uri, pos)
	if !ok {
		return nil
	}
	src, pos := v.src, local

	name := verbBefore(src, pos)
	if name == "" {
		return nil
	}
	label, doc := "", ""
	if e, ok := preludeEntry(name); ok {
		label = name + " :: " + e.Sig
		if e.Where != "" {
			label += "  where " + e.Where
		}
		doc = e.Doc
	} else {
		bag := diag.New(uri, src)
		file := parser.Parse(src, bag)
		info := check.File(file, diag.New(uri, src))
		_ = file
		if sch, found := info.Decls[name]; found {
			label = name + " :: " + types.SchemeString(sch)
		}
	}
	if label == "" {
		return nil
	}
	return map[string]any{
		"signatures": []any{map[string]any{
			"label":         label,
			"documentation": doc,
		}},
		"activeSignature": 0,
	}
}

// ------------------------------------------------------------- formatting

func (s *server) format(params json.RawMessage) any {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	json.Unmarshal(params, &p)
	src, ok := s.docs[p.TextDocument.URI]
	if !ok || mdblock.IsMarkdown(p.TextDocument.URI) {
		// Reformatting a Markdown file is not this server's business; the
		// blocks inside it are, but rewriting only those would fight with
		// whatever else formats the document.
		return nil
	}
	out, err := format.Source(src, p.TextDocument.URI)
	if err != nil {
		return nil // a program that does not parse is left alone
	}
	lines := strings.Count(src, "\n") + 1
	return []any{map[string]any{
		"range": map[string]any{
			"start": map[string]any{"line": 0, "character": 0},
			"end":   map[string]any{"line": lines, "character": 0},
		},
		"newText": out,
	}}
}

// ------------------------------------------------------------------ pieces

func documentPosition(params json.RawMessage) (string, token.Pos) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}
	json.Unmarshal(params, &p)
	// LSP counts from zero; the compiler counts from one.
	return p.TextDocument.URI, token.Pos{Line: p.Position.Line + 1, Col: p.Position.Character + 1}
}

func rangeAt(pos token.Pos, length int) map[string]any {
	if length < 1 {
		length = 1
	}
	return map[string]any{
		"start": map[string]any{"line": pos.Line - 1, "character": pos.Col - 1},
		"end":   map[string]any{"line": pos.Line - 1, "character": pos.Col - 1 + length},
	}
}

// lineOf returns a one-based line of the document.
func lineOf(src string, line int) string {
	lines := strings.Split(src, "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	return lines[line-1]
}

// wordAt returns the identifier under a position and where it starts.
func wordAt(src string, pos token.Pos) (string, token.Pos) {
	line := lineOf(src, pos.Line)
	i := pos.Col - 1
	if i < 0 || i > len(line) {
		return "", pos
	}
	start := i
	for start > 0 && isIdent(line[start-1]) {
		start--
	}
	end := i
	for end < len(line) && isIdent(line[end]) {
		end++
	}
	if start == end {
		return "", pos
	}
	return line[start:end], token.Pos{Line: pos.Line, Col: start + 1}
}

func wordLenAt(src string, pos token.Pos) int {
	w, _ := wordAt(src, pos)
	if w == "" {
		return 1
	}
	return len(w)
}

// verbBefore finds the name that heads the call being written, skipping back
// over the arguments already typed.
func verbBefore(src string, pos token.Pos) string {
	line := lineOf(src, pos.Line)
	i := pos.Col - 1
	if i > len(line) {
		i = len(line)
	}
	head := line[:i]

	// Cut back to whatever last opened a fresh application: a bracket, a
	// pipeline stage, a binder, or an arrow. Whatever heads the remaining text
	// is the verb whose arguments are being written.
	cut := strings.LastIndexAny(head, "(|[{,")
	for _, word := range []string{" is ", " : ", " where ", " as ", " through "} {
		if i := strings.LastIndex(head, word); i >= 0 && i+len(word)-1 > cut {
			cut = i + len(word) - 1
		}
	}
	if cut >= 0 {
		head = head[cut+1:]
	}
	fields := strings.Fields(head)
	if len(fields) == 0 {
		return ""
	}
	name := fields[0]
	for _, r := range name {
		if !isIdent(byte(r)) {
			return ""
		}
	}
	return name
}

func isIdent(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func preludeEntry(name string) (prelude.Entry, bool) {
	for _, e := range prelude.Values {
		if e.Name == name {
			return e, true
		}
	}
	return prelude.Entry{}, false
}

func preludeCtor(name string) (prelude.Ctor, bool) {
	for _, c := range prelude.Ctors {
		if c.Name == name {
			return c, true
		}
	}
	return prelude.Ctor{}, false
}

// Names lists every built-in, for tooling that wants the vocabulary. It is
// sorted so the output is stable.
func Names() []string {
	var out []string
	for _, e := range prelude.Values {
		out = append(out, e.Name)
	}
	for _, c := range prelude.Ctors {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}

var _ = lexer.Lex // the server re-lexes through the parser only
