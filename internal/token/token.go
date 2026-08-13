// Package token defines the lexical tokens of the Weave language.
package token

import "fmt"

// Kind classifies a token.
type Kind uint8

const (
	EOF Kind = iota

	// Layout tokens, synthesised by the lexer from indentation.
	Newline // end of a logical line
	Indent  // start of an indented block
	Dedent  // end of an indented block

	// Literals.
	Int   // 42, 1_000
	Float // 3.14
	Char  // '#'
	Text  // "hello"

	// Identifiers.
	Lower // value or function name: xs, addAll
	Upper // type or constructor name: Earth, Held, Light

	// Keywords.
	Is       // is (and its alias =)
	Weave    // weave
	Channel  // channel
	Ward     // ward
	Into     // into
	Remember // remember (marks a definition as memoised)
	Where    // where   (particle: xs where p  ==  sift p xs)
	As       // as      (particle: xs as f    ==  bend f xs)
	Through  // through (particle: x through f == f x)
	Else     // else    (particle: h else d    ==  otherwise d h)
	Failing  // failing (particle: w failing d ==  snag d w)

	// Symbols.
	Pipe       // |
	Colon      // :
	ColonColon // ::
	Arrow      // ->  (type signatures only)
	Comma      // ,
	LParen     // (
	RParen     // )
	LBracket   // [
	RBracket   // ]
	LBrace     // {
	RBrace     // }
	Underscore // _ , also spelled `this`
	Partner    // that , the second argument
	Former     // former , the first half of a Twine of two
	Latter     // latter , the second half
	Fore       // fore , the first of a Twine of three
	Mid        // mid , the second
	Aft        // aft , the third
	DotDot     // .. , which introduces a rest pattern
)

var kindNames = [...]string{
	EOF:        "end of file",
	Newline:    "newline",
	Indent:     "indent",
	Dedent:     "dedent",
	Int:        "int literal",
	Float:      "float literal",
	Char:       "char literal",
	Text:       "text literal",
	Lower:      "identifier",
	Upper:      "constructor",
	Is:         "is",
	Weave:      "weave",
	Channel:    "channel",
	Ward:       "ward",
	Into:       "into",
	Remember:   "remember",
	Where:      "where",
	As:         "as",
	Through:    "through",
	Else:       "else",
	Failing:    "failing",
	Pipe:       "|",
	Colon:      ":",
	ColonColon: "::",
	Arrow:      "->",
	Comma:      ",",
	LParen:     "(",
	RParen:     ")",
	LBracket:   "[",
	RBracket:   "]",
	LBrace:     "{",
	RBrace:     "}",
	Underscore: "_",
	Partner:    "that",
	Former:     "former",
	Latter:     "latter",
	Fore:       "fore",
	Mid:        "mid",
	Aft:        "aft",
	DotDot:     "..",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) && kindNames[k] != "" {
		return kindNames[k]
	}
	return fmt.Sprintf("token(%d)", int(k))
}

// keywords maps reserved words to their kind. Note that `pick` and `flow` are
// deliberately absent: they are ordinary identifiers resolved to builtins by
// the compiler, which keeps them usable as pipeline stages and arguments.
//
// Some of these are spellings of a symbol rather than words of their own:
// `gives` is `:` and `this` is `_`. They lex to the same tokens, so nothing
// below the lexer knows the difference and `weave fmt` prints whichever the
// chosen style asks for, the way it already prints `is` or `=`. Each was
// chosen because it could never be a verb — the rule that had `of`, `at` and
// `from` cut — and none is a name anyone reaches for.
//
// The rest are words with no symbol, and they answer two different questions.
// `that` is the second *argument*, so `braid (add this that) 0` needs no
// parameter names. The others are *components* of the first argument, so a
// stage handed a Twine can name them without writing a pattern.
//
// There is a set of component words per width, and that is the point rather
// than an accident. `former` and `latter` are the halves of a Twine of two —
// which is what those words mean in English and the only width they mean it at.
// `fore`, `mid` and `aft` are the three parts of a Twine of three. Each word
// therefore fixes both the position *and* the width, so a group holding one can
// be desugared where it stands.
//
// Two earlier spellings named a component relative to a width instead, and both
// failed the same way: "the latter of three" does not exist, and "the last of
// however many" cannot be resolved until the type is known, which is later than
// the parser and — for a bracket group, whose parameter type is settled before
// its argument arrives — later than it can be asked for at all. Nothing names a
// component of a Twine of four; at that width a pattern is clearer anyway.
var keywords = map[string]Kind{
	"is":       Is,
	"weave":    Weave,
	"channel":  Channel,
	"ward":     Ward,
	"into":     Into,
	"remember": Remember,
	"where":    Where,
	"as":       As,
	"through":  Through,
	"else":     Else,
	"failing":  Failing,
	"gives":    Colon,
	"this":     Underscore,
	"that":     Partner,
	"former":   Former,
	"latter":   Latter,
	"fore":     Fore,
	"mid":      Mid,
	"aft":      Aft,
}

// Keywords returns the reserved words. The reference page walks it, so a word
// cannot be reserved without turning up there.
func Keywords() map[string]Kind {
	out := make(map[string]Kind, len(keywords))
	for name, kind := range keywords {
		out[name] = kind
	}
	return out
}

// LookupIdent returns the keyword kind for name, or Lower if it is not
// reserved. Capitalised names are never keywords.
func LookupIdent(name string) Kind {
	if k, ok := keywords[name]; ok {
		return k
	}
	return Lower
}

// Pos is a source position. Line and Col are 1-based; Col counts bytes.
type Pos struct {
	Line int
	Col  int
}

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// IsValid reports whether p refers to a real source location.
func (p Pos) IsValid() bool { return p.Line > 0 }

// Token is a single lexical token.
type Token struct {
	Kind Kind
	Pos  Pos

	// Lit holds the source text for identifiers and literals. For Text and
	// Char it holds the decoded value, not the quoted source.
	Lit string

	// Int and Float hold decoded numeric values for their respective kinds.
	Int   int64
	Float float64
	// Char holds the decoded value of a Char token.
	Char rune
}

func (t Token) String() string {
	switch t.Kind {
	case Lower, Upper:
		return t.Lit
	case Int, Float, Char, Text:
		return fmt.Sprintf("%s(%s)", t.Kind, t.Lit)
	default:
		return t.Kind.String()
	}
}

// Comment is a `#` comment, kept so that `weave fmt` can put it back.
type Comment struct {
	Pos  Pos
	Text string // without the leading `#`, trailing space trimmed
}
