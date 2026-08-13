/// <reference types="tree-sitter-cli/dsl" />

// Weave's grammar for tree-sitter.
//
// Layout — the newline, indent and dedent tokens — comes from the external
// scanner in src/scanner.c, which ports the algorithm in internal/lexer. The
// two halves have to agree on one thing in particular: indentation opens a
// block only after something that wants one. The scanner learns that from this
// grammar rather than by tracking tokens itself, since a block is exactly the
// position where `$._indent` is a valid symbol.

const PARTICLES = ['|', 'where', 'as', 'through', 'else', 'failing'];

module.exports = grammar({
  name: 'weave',

  externals: $ => [$._newline, $._indent, $._dedent, $._error_sentinel],

  // A newline is whitespace to the internal lexer: the external scanner is
  // consulted first, so a line break the grammar wants becomes `_newline`, and
  // only the ones nothing wants — after a comment, between definitions — fall
  // through to here and are dropped.
  extras: $ => [/[ \t\r\n]/, $.comment],

  word: $ => $.identifier,

  // A definition and an output expression start alike — `f x ...` could be
  // either until the `is` turns up — and so do a lambda, a group and a twine.
  // Every conflict here is that kind: a shared prefix the parser resolves with
  // more lookahead, which is what tree-sitter's GLR is for.
  conflicts: $ => [
    [$.definition, $.output],
    [$.definition, $._atom],
    [$.definition, $.application],
    [$.definition, $.constructor_pattern],
    [$.type_declaration, $.output],
    [$.type_declaration, $.application],
    [$.local_binding, $.inline_binding],
    [$.lambda, $._group],
    [$.lambda, $.twine],
    [$.lambda, $.twine_pattern],
    [$._group, $.twine],
    [$._group, $.twine_pattern],
    // A `[` opens a Thread literal or a Thread pattern, and which it is only
    // becomes clear at the `is` — the same ambiguity the two above have.
    [$.thread, $.thread_pattern],
    [$.constructor_pattern, $.application],
    [$.type_application, $._type_atom],
    [$.pipeline, $.inline_let],
    [$._atom, $.constructor_pattern, $._pattern_atom],
    [$._atom, $._pattern_atom],
    [$.hole, $.wildcard],
    [$.type_declaration, $._atom],
    [$.application, $._atom],
    [$._application_or_atom, $.application],
    // After an inline ward's arms, another `(` is either one more arm or an
    // argument to whatever the ward is the function of. The compiler decides
    // by scanning ahead for a `:` at the group's own level; LR cannot, so both
    // readings are explored and the one that parses wins. When both parse,
    // `application`'s dynamic -1 gives the arm.
    [$._ward_inline],
  ],

  rules: {
    source_file: $ => repeat($._top_level),

    _top_level: $ => choice(
      $.signature,
      $.type_declaration,
      $.definition,
      $.output,
    ),

    // ------------------------------------------------------------ top level

    // `name :: Thread a -> Earth`
    signature: $ => seq(
      field('name', $.identifier),
      '::',
      field('type', $._type),
      $._newline,
    ),

    // `fib n is add (fib (sub n 1)) (fib (sub n 2))`
    // `(width, height) is dimsOf Source` takes the value apart into several
    // names, the same way a `weave` binding may.
    definition: $ => seq(
      choice(
        seq(
          optional('remember'),
          field('name', $.identifier),
          field('parameters', repeat($._pattern_atom)),
        ),
        field('pattern', $._destructuring),
      ),
      $._is,
      field('body', $._body),
    ),

    // The program's final bare expression: what it prints. Dynamically the
    // weakest top-level reading, so a line that could pass for one but has an
    // `is` in it is read as the declaration it is.
    output: $ => prec.dynamic(-1, seq($._expression, $._newline)),

    // `Direction is North | South | East | West`
    type_declaration: $ => seq(
      field('name', $.constructor),
      field('parameters', repeat($.identifier)),
      $._is,
      choice(
        seq($._constructor_list, $._newline),
        seq($._indent, $._constructor_list, $._newline, $._dedent),
      ),
    ),

    _constructor_list: $ => seq(
      $.constructor_declaration,
      repeat(seq('|', $.constructor_declaration)),
    ),

    constructor_declaration: $ => seq(
      field('name', $.constructor),
      field('fields', repeat($._type_atom)),
    ),

    // ---------------------------------------------------------------- bodies

    // The right-hand side of `is` or of a ward arm: an expression on the same
    // line, or an indented block.
    _body: $ => choice(
      seq($._expression, $._newline),
      seq($._indent, $._block, $._dedent),
    ),

    // A ward ends with its own dedent, so it needs no newline after it; every
    // other block result is an expression on one logical line.
    _block: $ => seq(
      repeat($.local_binding),
      choice(alias($._ward_block, $.ward), seq($._expression, $._newline)),
    ),

    // `weave (a, b) is p` takes the value apart instead of naming it. A bare
    // name is a name and not a pattern, so only the bracketed shapes and `_`
    // count — the same rule the parser follows.
    local_binding: $ => seq(
      field('keyword', choice('weave', 'channel')),
      choice(
        seq(
          field('name', $.identifier),
          field('parameters', repeat($._pattern_atom)),
        ),
        field('pattern', $._destructuring),
      ),
      $._is,
      field('value', $._body),
    ),

    _destructuring: $ => choice($.twine_pattern, $.thread_pattern, $.wildcard),

    // ----------------------------------------------------------- expressions

    // An inline let is an expression but never an atom: it runs to the end of
    // what it is in, so `weave x is 1 into add x 1` binds the whole tail rather
    // than becoming the head of a call.
    _expression: $ => choice($.pipeline, $.inline_let, $._application_or_atom),

    // `xs | sift even | sum`, and the particles that read as prose.
    pipeline: $ => prec.left(seq(
      $._expression,
      field('particle', choice(...PARTICLES)),
      $._application_or_atom,
    )),

    _application_or_atom: $ => choice($.application, $._atom),

    // Dynamically the weakest reading there is. Juxtaposition is application
    // nearly everywhere, so this changes nothing there; where something else
    // also fits — a run of atoms in `[1 2 3]`, the tail of an inline `into`, a
    // declaration whose head could pass for an expression — that other reading
    // is the right one.
    application: $ => prec.dynamic(-1, prec.left(seq(
      field('function', $._atom),
      field('arguments', repeat1($._atom)),
    ))),

    _atom: $ => choice(
      $.integer,
      $.float,
      $.character,
      $.text,
      $.identifier,
      $.constructor,
      $.hole,
      $.lambda,
      $.twine,
      $._group,
      $.thread,
      $.web,
      // A ward is an atom because the compiler's parser makes it one: the
      // inline form is an expression, so `add 1 (ward c (Light : 1) (_ : 0))`
      // is a call with a ward for an argument. Only the inline form, though —
      // letting the block form in here would let an application swallow an
      // indented block and read a whole definition as one long call.
      alias($._ward_inline, $.ward),
    ),

    // A word where a value goes: one of the arguments an enclosing bracket
    // group or pipeline stage stands ready to receive. `this` is `_` spelled as
    // a word, as `gives` is for `:` — see the lexer's keyword table. `that` is
    // the second argument. The rest are components of the first, one set per
    // width: `former` and `latter` for a Twine of two, `fore`, `mid` and `aft`
    // for one of three.
    hole: _ =>
      choice('_', 'this', 'that', 'former', 'latter', 'fore', 'mid', 'aft'),

    lambda: $ => seq(
      '(',
      field('parameters', repeat1($._pattern_atom)),
      choice(':', 'gives'),
      field('body', $._expression),
      ')',
    ),

    _group: $ => seq('(', $._expression, ')'),

    twine: $ => seq(
      '(',
      $._expression,
      repeat1(seq(',', $._expression)),
      ')',
    ),

    // The compiler reads `[1 2 3]` as three atoms and `[Step North 3, Rest]` as
    // two expressions, deciding on the presence of a comma. An LR automaton
    // cannot: a run of atoms and an application are the same item set. So the
    // grammar admits both readings and highlights the space-separated form as
    // elements rather than a call — see queries/highlights.scm.
    thread: $ => seq(
      '[',
      optional(seq($._expression, repeat(seq(optional(','), $._expression)))),
      ']',
    ),

    web: $ => seq(
      '{',
      repeat($.web_pair),
      '}',
    ),

    web_pair: $ => seq(
      field('key', $._application_or_atom),
      ':',
      field('value', $._application_or_atom),
    ),

    // A ward's arms go in an indented block, or — where there is no room for
    // one, which is anywhere a ward is an expression rather than a body — in
    // brackets on the same line.
    //
    // The two forms are genuinely ambiguous as far as the item sets go, and
    // deliberately so: a bracketed arm and a lambda are written the same, so
    // `ward c (Light : 1) (_ : 0)` also reads as `c` applied to two lambdas.
    // What separates them is the token after the subject. The block form must
    // see `_indent` there and the inline form must not, so exactly one of the
    // two GLR branches can finish, and the scanner decides which by whether the
    // next line is indented. Nothing here needs a precedence.
    _ward_block: $ => seq(
      'ward',
      field('subject', $._expression),
      $._indent,
      repeat1($.ward_arm),
      $._dedent,
    ),

    _ward_inline: $ => seq(
      'ward',
      field('subject', $._application_or_atom),
      repeat1($.ward_arm_inline),
    ),

    ward_arm: $ => seq(
      field('pattern', $._pattern),
      choice(':', 'gives'),
      field('body', $._body),
    ),

    ward_arm_inline: $ => seq(
      '(',
      field('pattern', $._pattern),
      choice(':', 'gives'),
      field('body', $._expression),
      ')',
    ),

    inline_let: $ => prec.right(seq(
      choice('weave', 'channel'),
      $.inline_binding,
      repeat(seq(',', $.inline_binding)),
      'into',
      $._expression,
    )),

    inline_binding: $ => seq(
      optional(choice('weave', 'channel')),
      choice(
        seq(
          field('name', $.identifier),
          field('parameters', repeat($._pattern_atom)),
        ),
        field('pattern', $._destructuring),
      ),
      $._is,
      field('value', $._application_or_atom),
    ),

    // -------------------------------------------------------------- patterns

    _pattern: $ => choice($.constructor_pattern, $._pattern_atom),

    constructor_pattern: $ => seq(
      field('name', choice($.constructor, $.identifier)),
      field('fields', repeat1($._pattern_atom)),
    ),

    _pattern_atom: $ => choice(
      $.wildcard,
      $.identifier,
      $.constructor,
      $.integer,
      $.float,
      $.character,
      $.text,
      seq('(', $._pattern, ')'),
      $.twine_pattern,
      $.thread_pattern,
    ),

    twine_pattern: $ => seq(
      '(',
      $._pattern,
      repeat1(seq(',', $._pattern)),
      ')',
    ),

    // `[a b]` takes a Thread apart by position; `[a ..rest]` names what is
    // left. Commas are optional, as in a Thread literal.
    thread_pattern: $ => seq(
      '[',
      repeat(seq($._pattern_atom, optional(','))),
      optional($.rest_pattern),
      ']',
    ),

    rest_pattern: $ => seq('..', optional(choice($.identifier, '_'))),

    wildcard: _ => choice('_', 'this'),

    // ----------------------------------------------------------------- types

    _type: $ => choice($.function_type, $._type_application),

    function_type: $ => prec.right(seq(
      $._type_application,
      '->',
      $._type,
    )),

    _type_application: $ => choice($.type_application, $._type_atom),

    type_application: $ => prec.left(seq(
      field('constructor', choice($.type_name, $.identifier)),
      repeat1($._type_atom),
    )),

    _type_atom: $ => choice(
      $.type_name,
      $.identifier,
      $.tuple_type,
      seq('(', $._type, ')'),
    ),

    tuple_type: $ => seq('(', $._type, repeat1(seq(',', $._type)), ')'),

    type_name: _ => /[A-Z][A-Za-z0-9]*/,

    // ------------------------------------------------------------- terminals

    // `is` and `=` are the same binder; `is` is idiomatic.
    _is: _ => choice('is', '='),

    identifier: _ => /[a-z][A-Za-z0-9]*/,
    constructor: _ => /[A-Z][A-Za-z0-9]*/,

    integer: _ => /[0-9][0-9_]*/,
    float: _ => /[0-9][0-9_]*\.[0-9][0-9_]*/,

    // Each literal is one token. Split into pieces they would be broken by
    // extras: a comment is always a candidate and matches more characters than
    // one character of string content does, so `'#'` and `"a#b"` would lex as
    // comments. The cost is that an escape inside a literal is not a node of
    // its own, and so takes the literal's colour.
    character: _ => token(seq("'", choice(/[^'\\\n]/, /\\[nrt0\\'"]/), "'")),

    text: _ => token(seq('"', repeat(choice(/[^"\\\n]/, /\\[nrt0\\'"]/)), '"')),

    comment: _ => token(seq('#', /[^\n]*/)),
  },
});
