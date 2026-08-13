package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/docs"
)

// session drives the server with framed requests and returns every message it
// wrote back, so the tests exercise the real protocol rather than the handlers.
func session(t *testing.T, requests ...string) []map[string]any {
	t.Helper()

	var in bytes.Buffer
	for _, r := range requests {
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(r), r)
	}
	var out bytes.Buffer
	if err := Serve(&in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	var msgs []map[string]any
	rest := out.String()
	for {
		i := strings.Index(rest, "\r\n\r\n")
		if i < 0 {
			break
		}
		var n int
		fmt.Sscanf(rest[:i], "Content-Length: %d", &n)
		body := rest[i+4 : i+4+n]
		rest = rest[i+4+n:]
		var m map[string]any
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatalf("bad message %q: %v", body, err)
		}
		if _, isReply := m["id"]; isReply {
			// A response must carry one or the other. A frame with neither is
			// what an editor rejects as an invalid server message, and it is
			// what "nothing to say" used to marshal to.
			_, hasResult := m["result"]
			_, hasError := m["error"]
			if !hasResult && !hasError {
				t.Errorf("reply with neither result nor error: %s", body)
			}
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func open(uri, text string) string {
	p, _ := json.Marshal(map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "weave", "version": 1, "text": text},
	})
	return fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":%s}`, p)
}

func request(id int, method string, params any) string {
	p, _ := json.Marshal(params)
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, p)
}

func at(uri string, line, spark int) map[string]any {
	return map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line, "character": spark},
	}
}

// seek returns the first message matching a method, or the reply to an id.
func seek(t *testing.T, msgs []map[string]any, method string, id float64) map[string]any {
	t.Helper()
	for _, m := range msgs {
		if method != "" && m["method"] == method {
			return m
		}
		if method == "" {
			if got, ok := m["id"].(float64); ok && got == id {
				return m
			}
		}
	}
	t.Fatalf("no message for method=%q id=%v in %v", method, id, msgs)
	return nil
}

func TestInitializeAdvertisesCapabilities(t *testing.T) {
	msgs := session(t, request(1, "initialize", map[string]any{}))
	caps := seek(t, msgs, "", 1)["result"].(map[string]any)["capabilities"].(map[string]any)
	for _, want := range []string{"hoverProvider", "completionProvider", "signatureHelpProvider",
		"documentFormattingProvider"} {
		if caps[want] == nil {
			t.Errorf("missing capability %s", want)
		}
	}
}

func TestDiagnosticsReportTypeErrors(t *testing.T) {
	msgs := session(t, open("file:///a.weave", "a is add 1 \"two\"\na\n"))
	note := seek(t, msgs, "textDocument/publishDiagnostics", 0)
	list := note["params"].(map[string]any)["diagnostics"].([]any)
	if len(list) == 0 {
		t.Fatalf("expected a diagnostic, got none")
	}
	msg := list[0].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "Reckon") {
		t.Errorf("expected the type error, got %q", msg)
	}
}

func TestDiagnosticsIncludeTheHint(t *testing.T) {
	msgs := session(t, open("file:///a.weave", "a is\n  ward first [1]\n    Held n : n\na\n"))
	note := seek(t, msgs, "textDocument/publishDiagnostics", 0)
	list := note["params"].(map[string]any)["diagnostics"].([]any)
	if len(list) == 0 {
		t.Fatal("expected an exhaustiveness diagnostic")
	}
	msg := list[0].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "hint:") {
		t.Errorf("expected the compiler's hint to be carried through, got %q", msg)
	}
}

func TestCleanFileClearsDiagnostics(t *testing.T) {
	msgs := session(t, open("file:///a.weave", "a is 1\na\n"))
	note := seek(t, msgs, "textDocument/publishDiagnostics", 0)
	list := note["params"].(map[string]any)["diagnostics"].([]any)
	if len(list) != 0 {
		t.Errorf("expected no diagnostics, got %v", list)
	}
}

func TestHoverShowsInferredTypes(t *testing.T) {
	src := "double n is mul n 2\ndouble 21\n"
	uri := "file:///a.weave"

	// The definition's own name, on the line where it is used.
	msgs := session(t, open(uri, src), request(2, "textDocument/hover", at(uri, 1, 2)))
	got := fmt.Sprint(seek(t, msgs, "", 2)["result"])
	if !strings.Contains(got, "Earth -> Earth") {
		t.Errorf("expected the inferred type, got %s", got)
	}
}

// An editor shows the reference's gloss, not the plain description the
// compiler keeps for diagnostics. Someone hovering a verb in their own file
// already writes Weave; someone reading a type error may not.
func TestHoverShowsBuiltinDocumentation(t *testing.T) {
	uri := "file:///a.weave"
	msgs := session(t, open(uri, "a is bend (x : x) [1]\na\n"),
		request(2, "textDocument/hover", at(uri, 0, 6)))
	got := fmt.Sprint(seek(t, msgs, "", 2)["result"])
	if !strings.Contains(got, "Thread a -> Thread b") {
		t.Errorf("expected bend's signature, got %s", got)
	}
	if !strings.Contains(got, docs.Gloss("bend")) {
		t.Errorf("expected bend's gloss, got %s", got)
	}
	if strings.Contains(got, "map a function") {
		t.Errorf("hover fell back to the plain description, got %s", got)
	}
}

// Every prose surface an editor has answers in the same voice, so that a verb
// does not describe itself one way in a hover and another in a completion.
func TestEditorProseIsTheGlossEverywhere(t *testing.T) {
	uri := "file:///a.weave"
	msgs := session(t, open(uri, "a is bend (x : x) [1]\na\n"),
		request(2, "textDocument/completion", at(uri, 1, 1)),
		request(3, "textDocument/signatureHelp", at(uri, 0, 10)))

	completion := fmt.Sprint(seek(t, msgs, "", 2)["result"])
	if !strings.Contains(completion, docs.Gloss("bend")) {
		t.Errorf("completion did not carry bend's gloss, got %s", completion)
	}

	help := fmt.Sprint(seek(t, msgs, "", 3)["result"])
	if !strings.Contains(help, docs.Gloss("bend")) {
		t.Errorf("signature help did not carry bend's gloss, got %s", help)
	}
}

// The gloss is the reference's, so a verb cannot be documented in an editor
// and undocumented on the page, or the two drift apart.
func TestEveryBuiltinHasAGloss(t *testing.T) {
	for _, g := range docs.Build().Groups {
		for _, v := range g.Verbs {
			if docs.Gloss(v.Name) == "" {
				t.Errorf("%s has no gloss for an editor to show", v.Name)
			}
		}
	}
}

func TestHoverOnALocalShowsItsType(t *testing.T) {
	uri := "file:///a.weave"
	src := "f n is add n 1\nf 1\n"
	msgs := session(t, open(uri, src), request(2, "textDocument/hover", at(uri, 0, 11)))
	got := fmt.Sprint(seek(t, msgs, "", 2)["result"])
	if !strings.Contains(got, "Earth") {
		t.Errorf("expected the parameter's type, got %s", got)
	}
}

func TestCompletionOffersBuiltinsAndLocals(t *testing.T) {
	uri := "file:///a.weave"
	msgs := session(t, open(uri, "mine is 1\nmine\n"),
		request(2, "textDocument/completion", at(uri, 1, 4)))
	items := seek(t, msgs, "", 2)["result"].(map[string]any)["items"].([]any)

	labels := map[string]string{}
	for _, it := range items {
		m := it.(map[string]any)
		labels[m["label"].(string)], _ = m["detail"].(string)
	}
	// The particles and the hole words are as much of the language as the
	// verbs, and reading them from the token table is what stops one being
	// added without the editor learning it.
	for _, want := range []string{"bend", "sift", "braid", "mine", "Held", "ward",
		"where", "as", "through", "else", "failing", "this", "that", "former", "latter", "fore", "mid", "aft"} {
		if _, ok := labels[want]; !ok {
			t.Errorf("completion is missing %q", want)
		}
	}
	if !strings.Contains(labels["bend"], "Thread") {
		t.Errorf("expected bend's signature as detail, got %q", labels["bend"])
	}
	if !strings.Contains(labels["mine"], "Earth") {
		t.Errorf("expected the local's inferred type, got %q", labels["mine"])
	}
}

func TestSignatureHelpNamesTheVerbBeingApplied(t *testing.T) {
	uri := "file:///a.weave"
	// The cursor sits after `braid `, mid-application.
	msgs := session(t, open(uri, "a is braid \na\n"),
		request(2, "textDocument/signatureHelp", at(uri, 0, 11)))
	got := fmt.Sprint(seek(t, msgs, "", 2)["result"])
	if !strings.Contains(got, "braid ::") {
		t.Errorf("expected braid's signature, got %s", got)
	}
}

func TestFormattingReturnsCanonicalText(t *testing.T) {
	uri := "file:///a.weave"
	msgs := session(t, open(uri, "a   =   1\na\n"),
		request(2, "textDocument/formatting", map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"options":      map[string]any{},
		}))
	edits := seek(t, msgs, "", 2)["result"].([]any)
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %v", edits)
	}
	text := edits[0].(map[string]any)["newText"].(string)
	if !strings.Contains(text, "a is 1") {
		t.Errorf("expected canonical text, got %q", text)
	}
}

func TestUnknownMethodIsAnError(t *testing.T) {
	msgs := session(t, request(9, "textDocument/nonsense", map[string]any{}))
	if seek(t, msgs, "", 9)["error"] == nil {
		t.Error("expected an error reply for an unsupported method")
	}
}

// The three requests that can honestly answer "nothing here". Each must still
// be a well-formed response: `result: null`, not a frame with no result at all.
func TestNothingToSayIsStillAValidReply(t *testing.T) {
	uri := "file:///a.weave"
	src := "a is 1\na\n"
	cases := []struct {
		name   string
		method string
		params any
	}{
		// A blank column: no word to describe.
		{"hover over nothing", "textDocument/hover", at(uri, 0, 6)},
		// No verb is being applied here, so there is no signature.
		{"signature help with no verb", "textDocument/signatureHelp", at(uri, 1, 1)},
		// A document the server was never told about.
		{"formatting an unknown file", "textDocument/formatting", map[string]any{
			"textDocument": map[string]any{"uri": "file:///gone.weave"},
			"options":      map[string]any{},
		}},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id := float64(100 + i)
			msgs := session(t, open(uri, src), request(int(id), c.method, c.params))
			reply := seek(t, msgs, "", id)
			if _, ok := reply["result"]; !ok {
				t.Errorf("no result field in %v", reply)
			}
		})
	}
}

// Hovering a word the compiler knows nothing about — a comment, a keyword with
// no entry — is the case the user hits most often, and it answered with an
// invalid frame.
func TestHoverOverAnUndocumentedWordIsValid(t *testing.T) {
	uri := "file:///a.weave"
	msgs := session(t, open(uri, "# just a comment\na is 1\na\n"),
		request(3, "textDocument/hover", at(uri, 0, 8)))
	reply := seek(t, msgs, "", 3)
	if _, ok := reply["result"]; !ok {
		t.Errorf("no result field in %v", reply)
	}
}

// ------------------------------------------------------- Markdown documents

// The grammar already highlights ```weave blocks inside Markdown, because
// tree-sitter sees the whole file. The server sees a document with no idea
// which bytes are Weave, so it finds the blocks itself and treats each as its
// own program.
const markdownDoc = "# Notes\n" +
	"\n" +
	"Some prose about `bend`.\n" +
	"\n" +
	"```weave\n" +
	"double n is mul n 2\n" +
	"double 21\n" +
	"```\n" +
	"\n" +
	"More prose.\n" +
	"\n" +
	"```weave\n" +
	"a is add 1 \"two\"\n" +
	"a\n" +
	"```\n"

func TestHoverInsideAMarkdownBlock(t *testing.T) {
	uri := "file:///notes.md"

	// `mul`, on line 5 (zero-based) of the document, inside the first block.
	msgs := session(t, open(uri, markdownDoc), request(2, "textDocument/hover", at(uri, 5, 12)))
	got := fmt.Sprint(seek(t, msgs, "", 2)["result"])
	if !strings.Contains(got, "Earth") {
		t.Errorf("expected mul's signature, got %s", got)
	}

	// The range comes back in document coordinates, not block-relative ones.
	res := seek(t, msgs, "", 2)["result"].(map[string]any)
	line := res["range"].(map[string]any)["start"].(map[string]any)["line"].(float64)
	if line != 5 {
		t.Errorf("hover range is on line %v, want 5", line)
	}
}

func TestHoverInProseIsNothing(t *testing.T) {
	uri := "file:///notes.md"
	// Line 2 is prose, outside every block.
	msgs := session(t, open(uri, markdownDoc), request(2, "textDocument/hover", at(uri, 2, 19)))
	reply := seek(t, msgs, "", 2)
	if _, ok := reply["result"]; !ok {
		t.Errorf("no result field in %v", reply)
	}
	if reply["result"] != nil {
		t.Errorf("prose is not Weave, so there is nothing to say: %v", reply["result"])
	}
}

func TestDiagnosticsInAMarkdownBlockAreOffset(t *testing.T) {
	msgs := session(t, open("file:///notes.md", markdownDoc))
	note := seek(t, msgs, "textDocument/publishDiagnostics", 0)
	list := note["params"].(map[string]any)["diagnostics"].([]any)
	if len(list) != 1 {
		t.Fatalf("expected one diagnostic, got %v", list)
	}
	d := list[0].(map[string]any)
	if !strings.Contains(d["message"].(string), "Reckon") {
		t.Errorf("expected the type error, got %q", d["message"])
	}
	// `add 1 "two"` is on document line 12, not on line 0 of its block.
	line := d["range"].(map[string]any)["start"].(map[string]any)["line"].(float64)
	if line != 12 {
		t.Errorf("diagnostic reported on line %v, want 12", line)
	}
}

func TestCompletionInsideAMarkdownBlockSeesTheBlock(t *testing.T) {
	uri := "file:///notes.md"
	msgs := session(t, open(uri, markdownDoc),
		request(2, "textDocument/completion", at(uri, 6, 6)))
	items := seek(t, msgs, "", 2)["result"].(map[string]any)["items"].([]any)

	labels := map[string]bool{}
	for _, it := range items {
		labels[it.(map[string]any)["label"].(string)] = true
	}
	// The block's own definition, and the built-ins.
	for _, want := range []string{"double", "bend", "seek"} {
		if !labels[want] {
			t.Errorf("completion is missing %q", want)
		}
	}
}

// Formatting a Markdown file is not this server's business.
func TestMarkdownIsNotFormatted(t *testing.T) {
	uri := "file:///notes.md"
	msgs := session(t, open(uri, markdownDoc),
		request(2, "textDocument/formatting", map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"options":      map[string]any{},
		}))
	if got := seek(t, msgs, "", 2)["result"]; got != nil {
		t.Errorf("expected no edits for a Markdown file, got %v", got)
	}
}

// An incomplete definition is a warning rather than an error, which is what
// makes it possible to write a function one clause at a time in an editor.
func TestAMissingCaseIsAWarning(t *testing.T) {
	msgs := session(t, open("file:///a.weave", "fib 0 is 1\nfib 1\n"))
	note := seek(t, msgs, "textDocument/publishDiagnostics", 0)
	list := note["params"].(map[string]any)["diagnostics"].([]any)
	if len(list) == 0 {
		t.Fatal("expected the exhaustiveness diagnostic")
	}
	if got := list[0].(map[string]any)["severity"].(float64); got != 2 {
		t.Errorf("severity %v, want 2 (warning)", got)
	}
}

// Hovering a name where it is *bound* — a parameter in the list, a `weave`
// name, a name a pattern takes apart — used to say nothing at all. The checker
// records types for uses, and a binder is not a use of itself, so the question
// went unanswered exactly where it gets asked most.
func TestHoverOnABinderShowsItsType(t *testing.T) {
	uri := "file:///a.weave"
	src := strings.Join([]string{
		"f n xs is",
		"  weave m is add n 1",
		"  weave (p, q) is (m, xs)",
		"  ward q",
		"    [h ..r] : add p h",
		"    _ : p",
		"",
		"f 1 [2]",
		"",
	}, "\n")

	cases := []struct {
		what      string
		line, col int
		want      string
	}{
		{"a parameter in the list", 0, 2, "n :: Earth"},
		{"a parameter used nowhere but the list", 0, 4, "xs :: Thread Earth"},
		{"a weave name", 1, 8, "m :: Earth"},
		{"a name a Twine pattern binds", 2, 9, "p :: Earth"},
		{"a name a Thread pattern binds", 4, 5, "h :: Earth"},
		{"a name the rest of a Thread binds", 4, 9, "r :: Thread Earth"},
	}
	for _, c := range cases {
		t.Run(c.what, func(t *testing.T) {
			msgs := session(t, open(uri, src), request(2, "textDocument/hover", at(uri, c.line, c.col)))
			got := fmt.Sprint(seek(t, msgs, "", 2)["result"])
			if !strings.Contains(got, c.want) {
				t.Errorf("expected %q, got %s", c.want, got)
			}
		})
	}
}

// A binder is answered as itself. A parameter named after a verb is a
// parameter, and saying otherwise would be worse than saying nothing.
func TestHoverOnABinderDoesNotShowTheVerbItShadows(t *testing.T) {
	uri := "file:///a.weave"
	src := "f sum is add sum 1\nf 1\n"
	msgs := session(t, open(uri, src), request(2, "textDocument/hover", at(uri, 0, 2)))
	got := fmt.Sprint(seek(t, msgs, "", 2)["result"])
	if !strings.Contains(got, "sum :: Earth") {
		t.Errorf("expected the parameter's type, got %s", got)
	}
	if strings.Contains(got, "Thread") {
		t.Errorf("hover answered with the verb the parameter shadows, got %s", got)
	}
}

// frames wraps message bodies for a server to read, so a test can hand it
// something the helpers above would never produce.
func frames(bodies ...string) string {
	var in strings.Builder
	for _, b := range bodies {
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(b), b)
	}
	return in.String()
}

// The server outlives whatever any one message does to it. A process that dies
// takes the client with it, and a client that has gone needs the editor
// restarted rather than the server — which is the failure this exists to stop.
func TestAMessageThatCannotBeReadDoesNotStopTheServer(t *testing.T) {
	// A body that is not JSON, then an ordinary request. The bad one is read
	// whole, so the stream is still in step and the good one is answered.
	in := frames("{not json at all", `{"jsonrpc":"2.0","id":2,"method":"shutdown"}`)

	var out bytes.Buffer
	if err := Serve(strings.NewReader(in), &out); err != nil {
		t.Fatalf("the server stopped: %v", err)
	}
	if !strings.Contains(out.String(), `"id":2`) {
		t.Errorf("the message after the unreadable one went unanswered:\n%s", out.String())
	}
}

// A header block with no Content-Length has no body to step over, so the next
// message begins where this one left off. It used to be read as end of input,
// which stopped the server on a stray blank line.
func TestAHeaderWithNoLengthDoesNotStopTheServer(t *testing.T) {
	in := "X-Nothing: here\r\n\r\n" +
		frames(`{"jsonrpc":"2.0","id":7,"method":"shutdown"}`)

	var out bytes.Buffer
	if err := Serve(strings.NewReader(in), &out); err != nil {
		t.Fatalf("the server stopped: %v", err)
	}
	if !strings.Contains(out.String(), `"id":7`) {
		t.Errorf("the message after the empty header went unanswered:\n%s", out.String())
	}
}

// A panic in the front end is the front end meeting something it was not
// written for, which is what a program being typed into is made of. The request
// is answered so the editor is not left waiting, and the server carries on.
func TestAPanicIsAnsweredRatherThanFatal(t *testing.T) {
	s := &server{docs: map[string]string{}}
	var out bytes.Buffer
	s.w = &out

	err := s.serve(&message{ID: json.RawMessage("3"), Method: "boom"})
	if err != nil {
		t.Fatalf("a panic became a fatal error: %v", err)
	}
	// Nothing panics on an unknown method, so provoke one directly.
	s.panicOn = "boom"
	out.Reset()
	if err := s.serve(&message{ID: json.RawMessage("4"), Method: "boom"}); err != nil {
		t.Fatalf("a panic became a fatal error: %v", err)
	}
	if !strings.Contains(out.String(), `"id":4`) || !strings.Contains(out.String(), "error") {
		t.Errorf("the panicking request was not answered:\n%s", out.String())
	}
}
