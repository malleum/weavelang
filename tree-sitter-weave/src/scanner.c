// Weave's layout scanner: newline, indent and dedent.
//
// This is the tree-sitter half of the algorithm in internal/lexer/lexer.go, and
// it has to agree with it. Two rules make Weave's layout unusual:
//
//   1. Indentation opens a block only after something that wants one — `is`, a
//      ward arm's `:`, or the `ward` line itself. Everywhere else a deeper line
//      *continues* the line above, so an application can span lines.
//
//   2. A line opening with `|`, `where`, `as` or `through` continues the line
//      above too, so a long pipeline can breathe.
//
// The compiler's lexer decides (1) by remembering the last token of the
// previous line. Here the grammar answers it directly: a block can start
// exactly where `_indent` is a valid symbol, so `valid_symbols` is the whole
// signal, and there is no state to drift out of step with the parser.

#include "tree_sitter/parser.h"

#include <stdlib.h>
#include <string.h>

enum TokenType {
  NEWLINE,
  INDENT,
  DEDENT,
  ERROR_SENTINEL,
};

// A tab counts as this many columns, matching lexer.IndentWidth.
#define TAB_WIDTH 2

// Deeper nesting than this is a runaway file, not a program.
#define MAX_INDENTS 64

typedef struct {
  uint16_t indents[MAX_INDENTS];
  uint8_t depth; // number of entries in use; indents[0] is always 0

  // A layout token is empty, so emitting one leaves the position exactly where
  // it was: at the first thing on the new line, with the line break already
  // behind us. These two remember that, so the calls that follow — the indent
  // after a newline, the second of two dedents — still know where the line
  // began and how deep it is.
  bool at_line_start;
  uint16_t line_width;
} Scanner;

static inline void skip(TSLexer *lexer) { lexer->advance(lexer, true); }

static inline uint16_t top_indent(const Scanner *s) {
  return s->indents[s->depth - 1];
}

// continues_line reports whether the text at the cursor begins with a token
// that can only continue the line above. The token's end is already marked, so
// reading ahead here costs nothing.
static bool continues_line(TSLexer *lexer) {
  if (lexer->lookahead == '|') {
    return true;
  }
  if (lexer->lookahead != 'w' && lexer->lookahead != 'a' && lexer->lookahead != 't') {
    return false;
  }

  char word[10];
  size_t len = 0;
  while (len + 1 < sizeof(word) &&
         ((lexer->lookahead >= 'a' && lexer->lookahead <= 'z') ||
          (lexer->lookahead >= 'A' && lexer->lookahead <= 'Z') ||
          (lexer->lookahead >= '0' && lexer->lookahead <= '9'))) {
    word[len++] = (char)lexer->lookahead;
    skip(lexer);
  }
  word[len] = '\0';

  // The comparison is against the whole word, so `whereabouts` is a name and
  // not the particle.
  return strcmp(word, "where") == 0 || strcmp(word, "as") == 0 ||
         strcmp(word, "through") == 0 || strcmp(word, "else") == 0 ||
         strcmp(word, "failing") == 0;
}

bool tree_sitter_weave_external_scanner_scan(void *payload, TSLexer *lexer,
                                             const bool *valid_symbols) {
  Scanner *s = (Scanner *)payload;

  // In error recovery tree-sitter marks every external token valid at once.
  // Emitting layout then only makes the damage spread, so stand down.
  if (valid_symbols[ERROR_SENTINEL]) {
    return false;
  }

  bool crossed_newline = false;
  uint32_t width = 0;
  // A comment-only line has no layout meaning, so the scan reads past one to
  // reach the next line of code. It must not swallow the comment, though, or it
  // would vanish from the tree: the layout token is cut short here instead, and
  // the comment is lexed after it, as the extra it is.
  bool marked = false;

  for (;;) {
    if (lexer->lookahead == '\n') {
      crossed_newline = true;
      width = 0;
      skip(lexer);
    } else if (lexer->lookahead == ' ') {
      width++;
      skip(lexer);
    } else if (lexer->lookahead == '\t') {
      width += TAB_WIDTH;
      skip(lexer);
    } else if (lexer->lookahead == '\r') {
      width = 0;
      skip(lexer);
    } else if (lexer->lookahead == '#') {
      if (!marked) {
        lexer->mark_end(lexer);
        marked = true;
      }
      while (lexer->lookahead != '\n' && !lexer->eof(lexer)) {
        skip(lexer);
      }
    } else if (lexer->eof(lexer)) {
      crossed_newline = true;
      width = 0;
      break;
    } else {
      break;
    }
  }

  if (crossed_newline) {
    s->at_line_start = true;
    s->line_width = (uint16_t)width;
  } else if (s->at_line_start) {
    // A layout token already took us over the line break; this line's width is
    // the one measured then.
    width = s->line_width;
  } else {
    return false; // in the middle of a line: nothing to say
  }

  // Everything consumed so far is whitespace: the layout token itself is empty
  // and sits here, before the first thing on the new line — or, when a comment
  // was passed over, just before that.
  if (!marked) {
    lexer->mark_end(lexer);
  }

  // At end of input, close every block and finish the last line.
  if (lexer->eof(lexer)) {
    if (valid_symbols[DEDENT] && s->depth > 1) {
      s->depth--;
      lexer->result_symbol = DEDENT;
      return true;
    }
    if (crossed_newline && valid_symbols[NEWLINE]) {
      lexer->result_symbol = NEWLINE;
      return true;
    }
    s->at_line_start = false;
    return false;
  }

  if (crossed_newline && continues_line(lexer)) {
    s->at_line_start = false;
    return false;
  }

  uint16_t top = top_indent(s);

  if (width > top) {
    // Deeper than the enclosing block. It opens one only where the grammar
    // says a block may begin; otherwise it is the line above, continued.
    s->at_line_start = false;
    if (valid_symbols[INDENT] && s->depth < MAX_INDENTS) {
      s->indents[s->depth++] = (uint16_t)width;
      lexer->result_symbol = INDENT;
      return true;
    }
    return false;
  }

  // A dedent leaves the flag set: closing several blocks at once takes one
  // token per call, from the same position.
  if (width < top && valid_symbols[DEDENT] && s->depth > 1) {
    s->depth--;
    lexer->result_symbol = DEDENT;
    return true;
  }

  // The newline likewise, since the indent or dedent for this line comes next.
  if (crossed_newline && valid_symbols[NEWLINE]) {
    lexer->result_symbol = NEWLINE;
    return true;
  }

  s->at_line_start = false;
  return false;
}

// ------------------------------------------------------------- housekeeping

void *tree_sitter_weave_external_scanner_create(void) {
  Scanner *s = (Scanner *)calloc(1, sizeof(Scanner));
  s->depth = 1;
  s->indents[0] = 0;
  s->at_line_start = false;
  s->line_width = 0;
  return s;
}

void tree_sitter_weave_external_scanner_destroy(void *payload) { free(payload); }

unsigned tree_sitter_weave_external_scanner_serialize(void *payload, char *buffer) {
  Scanner *s = (Scanner *)payload;
  unsigned size = 0;
  buffer[size++] = (char)s->at_line_start;
  buffer[size++] = (char)(s->line_width & 0xFF);
  buffer[size++] = (char)(s->line_width >> 8);
  buffer[size++] = (char)s->depth;
  for (uint8_t i = 0; i < s->depth; i++) {
    buffer[size++] = (char)(s->indents[i] & 0xFF);
    buffer[size++] = (char)(s->indents[i] >> 8);
  }
  return size;
}

void tree_sitter_weave_external_scanner_deserialize(void *payload, const char *buffer,
                                                    unsigned length) {
  Scanner *s = (Scanner *)payload;
  s->depth = 1;
  s->indents[0] = 0;
  s->at_line_start = false;
  s->line_width = 0;
  if (length < 4) {
    return;
  }
  unsigned size = 0;
  s->at_line_start = buffer[size++] != 0;
  uint16_t lw_lo = (uint8_t)buffer[size++];
  uint16_t lw_hi = (uint8_t)buffer[size++];
  s->line_width = (uint16_t)(lw_lo | (lw_hi << 8));
  uint8_t depth = (uint8_t)buffer[size++];
  if (depth > MAX_INDENTS) {
    depth = MAX_INDENTS;
  }
  for (uint8_t i = 0; i < depth && size + 1 < length; i++) {
    uint16_t lo = (uint8_t)buffer[size++];
    uint16_t hi = (uint8_t)buffer[size++];
    s->indents[i] = (uint16_t)(lo | (hi << 8));
  }
  s->depth = depth < 1 ? 1 : depth;
  s->indents[0] = 0;
}
