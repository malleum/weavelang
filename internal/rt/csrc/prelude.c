// prelude.c — the built-in verbs of Weave, as C functions.
//
// Each verb here matches the signature declared in internal/prelude/prelude.go,
// with arguments in the same order. Verbs that take a function call back
// through w_call, so a partially applied closure works as a pipeline stage.
//
// Threads are strict vectors in this version of the runtime. Fusing a chain of
// bend/sift/seek into a single pass with no intermediate Thread is the next
// optimisation; the semantics are already what fusion would produce, apart
// from `flow`, which needs real laziness and is not implemented yet.

#include "weave.h"

#include <ctype.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// A growable Value buffer, used to build Threads.
typedef struct {
  Value *items;
  size_t len, cap;
} Buf;

static void buf_push(Buf *b, Value v) {
  if (b->len == b->cap) {
    size_t cap = b->cap ? b->cap * 2 : 8;
    Value *items = (Value *)w_alloc(sizeof(Value) * cap);
    if (b->len) {
      memcpy(items, b->items, sizeof(Value) * b->len);
      // The old array is unreachable the moment the new one replaces it: a Buf
      // is a local, and nothing has seen its storage yet.
      w_free(b->items, sizeof(Value) * b->cap);
    }
    b->items = items;
    b->cap = cap;
  }
  b->items[b->len++] = v;
}

// buf_thread hands back the doubling slack as it seals the buffer: a Buf that
// grew to 2048 to hold 1300 elements has 11 KB nobody will ever ask it for.
static Value buf_thread(Buf *b) { return w_thread_fit(b->items, b->len, b->cap); }

// EBuf is Buf for a verb that knows every element will carry the same tag: the
// same doubling, eight bytes to the element rather than sixteen, sealed into a
// packed Thread. See the layout note in weave.h.
typedef struct {
  int64_t *items;
  size_t len, cap;
} EBuf;

static void ebuf_push(EBuf *b, int64_t v) {
  if (b->len == b->cap) {
    size_t cap = b->cap ? b->cap * 2 : 8;
    int64_t *items = (int64_t *)w_alloc(sizeof(int64_t) * cap);
    if (b->len) {
      memcpy(items, b->items, sizeof(int64_t) * b->len);
      w_free(b->items, sizeof(int64_t) * b->cap);
    }
    b->items = items;
    b->cap = cap;
  }
  b->items[b->len++] = v;
}

static Value ebuf_thread(EBuf *b, uint32_t elem) {
  return w_thread_packed_fit(b->items, b->len, b->cap, elem);
}

static WAir *air_of(Value v) { return (WAir *)v.obj; }
static WThread *thr_of(Value v) { return (WThread *)v.obj; }

// call1 and call2 go through the inline fast paths rather than w_call itself:
// a saturated closure is then an indirect jump with no call in front of it,
// which on a verb that runs its function once per element is worth having.
static Value call1(Value f, Value a) { return w_call1(f, a); }
static Value call2(Value f, Value a, Value b) { return w_call2(f, a, b); }

// ------------------------------------------------------------------- numbers

#define BOTH_WATER(a, b) ((a).tag == W_WATER || (b).tag == W_WATER)
static double as_d(Value v) { return v.tag == W_WATER ? v.water : (double)v.earth; }

// inc and dec are `add x 1` and `sub x 1` under a name, because stepping by one
// is what half the arithmetic in an Advent of Code program is and `add 1` reads
// like the number matters.
Value wp_inc(Value a) {
  return a.tag == W_WATER ? w_water(a.water + 1.0) : w_add_e(a, w_earth(1));
}
Value wp_dec(Value a) {
  return a.tag == W_WATER ? w_water(a.water - 1.0) : w_sub_e(a, w_earth(1));
}

// The general forms dispatch on the tag and then hand the Earth case to the
// specialised helpers, so `-overflow` covers both spellings of `add`.
Value wp_add(Value a, Value b) {
  return BOTH_WATER(a, b) ? w_water(as_d(a) + as_d(b)) : w_add_e(a, b);
}
Value wp_sub(Value a, Value b) {
  return BOTH_WATER(a, b) ? w_water(as_d(a) - as_d(b)) : w_sub_e(a, b);
}
Value wp_mul(Value a, Value b) {
  return BOTH_WATER(a, b) ? w_water(as_d(a) * as_d(b)) : w_mul_e(a, b);
}
Value wp_div(Value a, Value b) {
  if (BOTH_WATER(a, b))
    return w_water(as_d(a) / as_d(b));
  if (b.earth == 0)
    w_fail("divided by zero");
  return w_earth(a.earth / b.earth);
}
Value wp_mod(Value a, Value b) {
  if (b.earth == 0)
    w_fail("took a remainder by zero");
  return w_earth(a.earth % b.earth);
}
Value wp_abs(Value a) {
  if (a.tag == W_WATER)
    return w_water(a.water < 0 ? -a.water : a.water);
  return w_earth(a.earth < 0 ? -a.earth : a.earth);
}
Value wp_neg(Value a) {
  return a.tag == W_WATER ? w_water(-a.water) : w_earth(-a.earth);
}
Value wp_min(Value a, Value b) { return w_compare(a, b) <= 0 ? a : b; }
Value wp_max(Value a, Value b) { return w_compare(a, b) >= 0 ? a : b; }
Value wp_even(Value n) { return w_spirit(n.earth % 2 == 0); }
Value wp_odd(Value n) { return w_spirit(n.earth % 2 != 0); }
// divBy d n: is n divisible by d.
Value wp_divBy(Value d, Value n) {
  if (d.earth == 0)
    w_fail("divBy was given zero");
  return w_spirit(n.earth % d.earth == 0);
}

// ---------------------------------------------------------- compare & logic

Value wp_eq(Value a, Value b) { return w_spirit(w_equal(a, b)); }
Value wp_neq(Value a, Value b) { return w_spirit(!w_equal(a, b)); }
// Comparisons read as `gt bound value`, so the pipeline `| sift (gt 10)` keeps
// 'greater than 10'.
Value wp_lt(Value b, Value a) { return w_spirit(w_compare(a, b) < 0); }
Value wp_lte(Value b, Value a) { return w_spirit(w_compare(a, b) <= 0); }
Value wp_gt(Value b, Value a) { return w_spirit(w_compare(a, b) > 0); }
Value wp_gte(Value b, Value a) { return w_spirit(w_compare(a, b) >= 0); }

Value wp_and(Value a, Value b) { return w_spirit(a.spirit && b.spirit); }
Value wp_or(Value a, Value b) { return w_spirit(a.spirit || b.spirit); }
Value wp_not(Value a) { return w_spirit(!a.spirit); }
Value wp_pick(Value c, Value a, Value b) { return c.spirit ? a : b; }

// --------------------------------------------------------------------- fire

Value wp_isDigit(Value c) { return w_spirit(c.fire < 128 && isdigit((int)c.fire)); }
Value wp_isAlpha(Value c) { return w_spirit(c.fire < 128 && isalpha((int)c.fire)); }
Value wp_isSpace(Value c) { return w_spirit(c.fire < 128 && isspace((int)c.fire)); }

// ---------------------------------------------------------------------- air

Value wp_lines(Value text) {
  WAir *a = air_of(text);
  Buf b = {0};
  size_t start = 0;
  for (size_t i = 0; i <= a->len; i++) {
    if (i == a->len || a->bytes[i] == '\n') {
      // A trailing newline ends the last line rather than starting an empty
      // one, which is what every input file looks like.
      if (i == a->len && i == start)
        break;
      buf_push(&b, w_air(a->bytes + start, i - start));
      start = i + 1;
    }
  }
  return buf_thread(&b);
}

Value wp_words(Value text) {
  WAir *a = air_of(text);
  Buf b = {0};
  size_t i = 0;
  while (i < a->len) {
    while (i < a->len && isspace((unsigned char)a->bytes[i]))
      i++;
    size_t start = i;
    while (i < a->len && !isspace((unsigned char)a->bytes[i]))
      i++;
    if (i > start)
      buf_push(&b, w_air(a->bytes + start, i - start));
  }
  return buf_thread(&b);
}

// carve is `words` with the separators named: the runs of text lying between
// any of these characters, empty runs dropped.
//
// It is the verb for input with punctuation in it, which is most input. Weave
// had `split`, which takes one separator, and `words`, which takes whitespace
// and nothing else — so a line like `Machine [1,2] (3)` was six chained
// `replace` calls before `words` could be reached for, which is eight lines
// saying one thing. `carve "[](){}, " l` is the line.
//
// Separators are bytes, which is what every input that needs this uses; a rune
// above ASCII is never a separator and so passes through inside a run.
Value wp_carve(Value seps, Value text) {
  WAir *s = air_of(seps);
  WAir *a = air_of(text);

  bool cut[256] = {false};
  for (size_t i = 0; i < s->len; i++)
    cut[(unsigned char)s->bytes[i]] = true;

  Buf b = {0};
  size_t i = 0;
  while (i < a->len) {
    while (i < a->len && cut[(unsigned char)a->bytes[i]])
      i++;
    size_t start = i;
    while (i < a->len && !cut[(unsigned char)a->bytes[i]])
      i++;
    if (i > start)
      buf_push(&b, w_air(a->bytes + start, i - start));
  }
  return buf_thread(&b);
}

// rune_width is how many bytes the character starting at i occupies, never
// running past the end of the text.
static size_t rune_width(const WAir *a, size_t i) {
  unsigned char c = (unsigned char)a->bytes[i];
  size_t n = 1;
  if (c >= 0xF0)
    n = 4;
  else if (c >= 0xE0)
    n = 3;
  else if (c >= 0xC0)
    n = 2;
  return i + n > a->len ? 1 : n;
}

Value wp_fires(Value text) {
  WAir *a = air_of(text);
  Buf b = {0};
  for (size_t i = 0; i < a->len;) {
    unsigned char c = (unsigned char)a->bytes[i];
    uint32_t r = c;
    size_t n = rune_width(a, i);
    if (n == 2)
      r = c & 0x1F;
    else if (n == 3)
      r = c & 0x0F;
    else if (n == 4)
      r = c & 0x07;
    for (size_t k = 1; k < n; k++)
      r = (r << 6) | ((unsigned char)a->bytes[i + k] & 0x3F);
    buf_push(&b, w_fire(r));
    i += n;
  }
  return buf_thread(&b);
}

Value wp_split(Value sep, Value text) {
  WAir *s = air_of(sep), *a = air_of(text);
  Buf b = {0};
  if (s->len == 0) {
    // Splitting on nothing gives one piece per character — as text, since that
    // is what `split` promises. Handing back `fires` here was a Thread of Fires
    // wearing the type of a Thread of Air, and the first verb to treat one as
    // text read a character code as a pointer.
    for (size_t i = 0; i < a->len;) {
      size_t n = rune_width(a, i);
      buf_push(&b, w_air(a->bytes + i, n));
      i += n;
    }
    return buf_thread(&b);
  }
  size_t start = 0;
  for (size_t i = 0; i + s->len <= a->len;) {
    if (memcmp(a->bytes + i, s->bytes, s->len) == 0) {
      buf_push(&b, w_air(a->bytes + start, i - start));
      i += s->len;
      start = i;
    } else {
      i++;
    }
  }
  buf_push(&b, w_air(a->bytes + start, a->len - start));
  return buf_thread(&b);
}

Value wp_strip(Value text) {
  WAir *a = air_of(text);
  size_t i = 0, j = a->len;
  while (i < j && isspace((unsigned char)a->bytes[i]))
    i++;
  while (j > i && isspace((unsigned char)a->bytes[j - 1]))
    j--;
  return w_air(a->bytes + i, j - i);
}

Value wp_join(Value sep, Value parts) {
  WThread *t = thr_of(parts);
  WAir *s = air_of(sep);
  size_t total = 0;
  for (size_t i = 0; i < t->len; i++)
    total += air_of(thr_at(t, i))->len;
  if (t->len > 1)
    total += s->len * (t->len - 1);
  char *out = (char *)w_alloc(total ? total : 1);
  size_t at = 0;
  for (size_t i = 0; i < t->len; i++) {
    if (i && s->len) {
      memcpy(out + at, s->bytes, s->len);
      at += s->len;
    }
    WAir *p = air_of(thr_at(t, i));
    memcpy(out + at, p->bytes, p->len);
    at += p->len;
  }
  return w_air(out, at);
}

Value wp_air(Value v) {
  size_t len = 0;
  char *bytes = w_render(v, &len);
  return w_air(bytes, len);
}

// ------------------------------------------------- reading a Power from text
//
// One verb per Power, named for it: `earth "42"` is `Held 42`, and `earth "x"`
// is `Stilled`. These replaced a single `parse` whose first argument was a
// type rather than a value — the one verb in the language that needed the
// checker and the code generator to know its name.

Value wp_earth(Value text) {
  WAir *a = air_of(text);
  char tmp[32];
  if (a->len == 0 || a->len >= sizeof(tmp))
    return w_stilled();
  memcpy(tmp, a->bytes, a->len);
  tmp[a->len] = 0;
  char *end = NULL;
  long long n = strtoll(tmp, &end, 10);
  if (end == tmp || *end != 0)
    return w_stilled();
  return w_held(w_earth(n));
}

Value wp_water(Value text) {
  WAir *a = air_of(text);
  char tmp[64];
  if (a->len == 0 || a->len >= sizeof(tmp))
    return w_stilled();
  memcpy(tmp, a->bytes, a->len);
  tmp[a->len] = 0;
  char *end = NULL;
  double d = strtod(tmp, &end);
  if (end == tmp || *end != 0)
    return w_stilled();
  return w_held(w_water(d));
}

Value wp_fire(Value text) {
  Value runes = wp_fires(text);
  if (w_thread_len(runes) != 1)
    return w_stilled();
  return w_held(w_thread_at(runes, 0));
}

// --------------------------------------------------------------------- hold

Value wp_otherwise(Value dflt, Value h) {
  return w_is_held(h) ? w_hold_inner(h) : dflt;
}
Value wp_holds(Value h) { return w_spirit(w_is_held(h)); }

// woven is `holds` for the other half of the pair. Without it, asking whether a
// Weaving succeeded is a two-armed `ward` — which is fine when you want the
// value and clumsy when you only want to know.
Value wp_woven(Value w) { return w_spirit(w_data_index(w) == 0); }

// A Weaving is a two-case sum type on the general data representation, so
// `Woven` is index 0 and `Gentled` index 1, exactly as they are written.
Value wp_rescue(Value dflt, Value w) {
  return w_data_index(w) == 0 ? w_data_field(w, 0) : dflt;
}

// snag is rescue from the other side: the value the weaving snagged on. A
// `gentle` that stopped early put its answer there, and a `harvest` that could
// not convert put the offending element there, so both are read out this way.
Value wp_snag(Value dflt, Value w) {
  return w_data_index(w) == 1 ? w_data_field(w, 0) : dflt;
}

// ------------------------------------------------------------------ threads

Value wp_bend(Value f, Value xs) {
  WThread *t = thr_of(xs);
  Value *out = (Value *)w_alloc(sizeof(Value) * (t->len ? t->len : 1));
  for (size_t i = 0; i < t->len; i++)
    out[i] = call1(f, thr_at(t, i));
  return w_thread(out, t->len);
}

Value wp_sift(Value p, Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  for (size_t i = 0; i < t->len; i++)
    if (call1(p, thr_at(t, i)).spirit)
      buf_push(&b, thr_at(t, i));
  return buf_thread(&b);
}

Value wp_braid(Value f, Value seed, Value xs) {
  WThread *t = thr_of(xs);
  Value acc = seed;
  for (size_t i = 0; i < t->len; i++)
    acc = call2(f, acc, thr_at(t, i));
  return acc;
}

Value wp_seek(Value p, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (call1(p, thr_at(t, i)).spirit)
      return w_held(thr_at(t, i));
  return w_stilled();
}

Value wp_span(Value lo, Value hi) {
  if (hi.earth < lo.earth)
    return w_thread_empty();
  size_t n = (size_t)(hi.earth - lo.earth + 1);
  Value *out = (Value *)w_alloc(sizeof(Value) * n);
  for (size_t i = 0; i < n; i++)
    out[i] = w_earth(lo.earth + (int64_t)i);
  return w_thread(out, n);
}

// under is `span 0 (sub n 1)` — the positions of n things rather than a range
// between two numbers. `span` is inclusive at both ends, which is what an input
// written `11-22` means, and that leaves "the first n whole numbers" spelled
// with a `sub` every time. It was written eight times across four of the twelve
// Advent of Code 2025 programs.
Value wp_under(Value n) {
  if (n.earth <= 0)
    return w_thread_empty();
  size_t len = (size_t)n.earth;
  Value *out = (Value *)w_alloc(sizeof(Value) * len);
  for (size_t i = 0; i < len; i++)
    out[i] = w_earth((int64_t)i);
  return w_thread(out, len);
}

// copies is `repeat` for a Thread rather than for text. A starting board, a row
// of zeroes, a counter per shape — all of them were `span 1 n | bend (i : 0)`,
// which says nothing about what it is for.
Value wp_copies(Value n, Value v) {
  if (n.earth <= 0)
    return w_thread_empty();
  size_t len = (size_t)n.earth;
  Value *out = (Value *)w_alloc(sizeof(Value) * len);
  for (size_t i = 0; i < len; i++)
    out[i] = v;
  return w_thread(out, len);
}

// size works on every collection. Values carry their tag, so one function
// serves the whole Bulk Talent.
Value wp_len(Value xs) {
  switch (xs.tag) {
  case W_THREAD:
    return w_earth((int64_t)w_thread_len(xs));
  case W_WEB:
  case W_CIRCLE:
    return w_earth((int64_t)w_web_size(xs));
  case W_TAVEREN:
    return w_earth((int64_t)w_taveren_size(xs));
  case W_PATTERN: {
    WPattern *p = (WPattern *)xs.obj;
    return w_earth((int64_t)(p->rows * p->cols));
  }
  case W_AIR: {
    // Runes, not bytes, so that `size` agrees with `runes | size`.
    WAir *a = air_of(xs);
    int64_t n = 0;
    for (size_t i = 0; i < a->len; i++)
      if (((unsigned char)a->bytes[i] & 0xC0) != 0x80)
        n++;
    return w_earth(n);
  }
  default:
    return w_earth(0);
  }
}

Value wp_count(Value p, Value xs) {
  WThread *t = thr_of(xs);
  int64_t n = 0;
  for (size_t i = 0; i < t->len; i++)
    if (call1(p, thr_at(t, i)).spirit)
      n++;
  return w_earth(n);
}

Value wp_sum(Value xs) {
  WThread *t = thr_of(xs);
  if (t->len == 0)
    return w_earth(0);
  Value acc = thr_at(t, 0);
  for (size_t i = 1; i < t->len; i++)
    acc = wp_add(acc, thr_at(t, i));
  return acc;
}

Value wp_prod(Value xs) {
  WThread *t = thr_of(xs);
  if (t->len == 0)
    return w_earth(1);
  Value acc = thr_at(t, 0);
  for (size_t i = 1; i < t->len; i++)
    acc = wp_mul(acc, thr_at(t, i));
  return acc;
}

// air_at is the byte offset of the nth rune, or the whole length when the text
// runs out first. Text is sliced by rune and not by byte, so that `take` agrees
// with `len` and never cuts a character in half.
static size_t air_at(const WAir *a, int64_t n) {
  if (n <= 0)
    return 0;
  size_t i = 0;
  int64_t seen = 0;
  while (i < a->len) {
    i++;
    while (i < a->len && ((unsigned char)a->bytes[i] & 0xC0) == 0x80)
      i++;
    if (++seen == n)
      break;
  }
  return i;
}

// air_rune decodes the rune at a byte offset. `wp_fires` walks the whole text
// and this reads one place in it, so the decoding is written out once here and
// used by both.
static uint32_t air_rune(const WAir *a, size_t i) {
  unsigned char c = (unsigned char)a->bytes[i];
  size_t n = rune_width(a, i);
  uint32_t r = c;
  if (n == 2)
    r = c & 0x1F;
  else if (n == 3)
    r = c & 0x0F;
  else if (n == 4)
    r = c & 0x07;
  for (size_t k = 1; k < n; k++)
    r = (r << 6) | ((unsigned char)a->bytes[i + k] & 0x3F);
  return r;
}

// take and drop share their source's storage, for a Thread and for text alike:
// an Air holds a pointer and a length, so a slice of one costs nothing.
Value wp_take(Value n, Value xs) {
  if (xs.tag == W_AIR) {
    WAir *a = air_of(xs);
    return w_air(a->bytes, air_at(a, n.earth));
  }
  WThread *t = thr_of(xs);
  size_t k = n.earth < 0 ? 0 : (size_t)n.earth;
  if (k > t->len)
    k = t->len;
  return thr_window(t, 0, k);
}

Value wp_drop(Value n, Value xs) {
  if (xs.tag == W_AIR) {
    WAir *a = air_of(xs);
    size_t at = air_at(a, n.earth);
    return w_air(a->bytes + at, a->len - at);
  }
  WThread *t = thr_of(xs);
  size_t k = n.earth < 0 ? 0 : (size_t)n.earth;
  if (k > t->len)
    k = t->len;
  return thr_window(t, k, t->len - k);
}

// weld puts one Thread on the end of another. `weld ys xs` is xs then ys, so
// `xs | weld ys` reads in the order it produces — and appending one element is
// `weld [x] xs`, prepending is `weld xs [x]`.
//
// It is the verb that was missing: until it existed a Thread could be read
// every way there is and built no way at all, short of rebuilding it from a
// fold.
// thread makes a Thread of a Twine. The signature only admits a pair whose
// halves agree, since a Twine of two different types has no element type to
// give a Thread — but the copy itself does not care how long the Twine is.
Value wp_thread(Value tw) {
  WTwine *t = (WTwine *)tw.obj;
  return w_thread_copy(t->items, t->len);
}

// w_around is the remainder that never goes negative, which is what a ring
// index wants: -1 of three is 2, not -1. C's own `%` truncates towards zero.
static int64_t w_around(int64_t at, int64_t len) {
  if (len <= 0)
    return 0;
  int64_t r = at % len;
  return r < 0 ? r + len : r;
}

// turn shifts a Thread round: what comes off the front goes on the back. A
// negative count turns the other way, and any count works however far past the
// length it is, since the index goes round.
//
// Text turns too, by rune, which is what `Ply` says: the verb never names the
// element type.
Value wp_turn(Value n, Value xs) {
  if (xs.tag == W_AIR) {
    WAir *a = air_of(xs);
    int64_t runes = 0;
    for (size_t i = 0; i < a->len; i++)
      if (((unsigned char)a->bytes[i] & 0xC0) != 0x80)
        runes++;
    if (runes == 0)
      return xs;
    size_t k = (size_t)w_around(n.earth, runes);
    size_t at = air_at(a, (int64_t)k);
    char *out = (char *)w_alloc(a->len ? a->len : 1);
    memcpy(out, a->bytes + at, a->len - at);
    memcpy(out + (a->len - at), a->bytes, at);
    return w_air(out, a->len);
  }
  WThread *t = thr_of(xs);
  if (t->len == 0)
    return xs;
  size_t k = (size_t)w_around(n.earth, (int64_t)t->len);
  Value *out = (Value *)w_alloc(sizeof(Value) * t->len);
  Value *elems = thr_boxed(t);
  memcpy(out, elems + k, sizeof(Value) * (t->len - k));
  memcpy(out + (t->len - k), elems, sizeof(Value) * k);
  return w_thread(out, t->len);
}

// wrap is `nth` on a ring. Only an empty Thread has nothing to answer with.
Value wp_wrap(Value at, Value xs) {
  WThread *t = thr_of(xs);
  if (t->len == 0)
    return w_stilled();
  return w_held(thr_at(t, w_around(at.earth, (int64_t)t->len)));
}

Value wp_weld(Value extra, Value xs) {
  // Text welds too: the verb never names the element type, which is what lets
  // it carry the Ply Talent alongside `take`, `drop`, `sever` and `rev`.
  if (xs.tag == W_AIR || extra.tag == W_AIR) {
    WAir *a = air_of(xs), *b = air_of(extra);
    size_t n = a->len + b->len;
    char *out = (char *)w_alloc(n ? n : 1);
    memcpy(out, a->bytes, a->len);
    memcpy(out + a->len, b->bytes, b->len);
    return w_air(out, n);
  }
  WThread *e = thr_of(extra), *x = thr_of(xs);
  size_t n = x->len + e->len;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  memcpy(out, thr_boxed(x), sizeof(Value) * x->len);
  memcpy(out + x->len, thr_boxed(e), sizeof(Value) * e->len);
  return w_thread(out, n);
}

// mend replaces one element. An index nothing is at leaves the Thread alone,
// the way `set` does for a grid: a position that is not there is a question
// about the input, not a failure worth a Hold.
Value wp_mend(Value at, Value v, Value xs) {
  WThread *x = thr_of(xs);
  if (at.earth < 0 || (size_t)at.earth >= x->len)
    return xs;
  Value *out = (Value *)w_alloc(sizeof(Value) * (x->len ? x->len : 1));
  memcpy(out, thr_boxed(x), sizeof(Value) * x->len);
  out[at.earth] = v;
  return w_thread(out, x->len);
}

// sever cuts a Thread in two at a position, which is `take` and `drop` in one
// pass and one reading. Both halves share the original's storage.
// wp_mend_owned is `mend` for a Thread the compiler has proved is not shared,
// on exactly the terms wp_set_owned has: the first update in a loop copies and
// marks the copy owned, and every later one writes through.
//
// A Thread is the one ownable whose storage can be shared without a second
// Thread object — `take`, `drop`, `sever`, `strands` and the Thread patterns
// all hand back a window on the same buffer. That is why none of them may name
// an owned parameter; see the readOnly list in internal/codegen/inplace.go.
// The copy this makes has a buffer of its own, so the invariant holds from
// there on.
Value wp_mend_owned(Value at, Value v, Value xs) {
  WThread *t = thr_of(xs);
  if (at.earth < 0 || (size_t)at.earth >= t->len)
    return xs;
  if (t->obj.rc == W_OWNED) {
    thr_put(t, at.earth, v);
    return xs;
  }
  Value copy = wp_mend(at, v, xs);
  copy.obj->rc = W_OWNED;
  return copy;
}

// wind is mend that goes round, the way wrap is nth that goes round: the
// position wraps rather than falling off, so `wind (neg 1)` replaces the last
// strand. Only an empty Thread has nowhere to put anything, and it comes back
// as it was — which is what mend does for a position that is not there.
Value wp_wind(Value at, Value v, Value xs) {
  WThread *x = thr_of(xs);
  if (x->len == 0)
    return xs;
  size_t k = (size_t)w_around(at.earth, (int64_t)x->len);
  Value *out = (Value *)w_alloc(sizeof(Value) * x->len);
  memcpy(out, thr_boxed(x), sizeof(Value) * x->len);
  out[k] = v;
  return w_thread(out, x->len);
}

Value wp_wind_owned(Value at, Value v, Value xs) {
  WThread *t = thr_of(xs);
  if (t->len == 0)
    return xs;
  if (t->obj.rc == W_OWNED) {
    thr_put(t, w_around(at.earth, (int64_t)t->len), v);
    return xs;
  }
  Value copy = wp_wind(at, v, xs);
  copy.obj->rc = W_OWNED;
  return copy;
}

Value wp_sever(Value n, Value xs) {
  if (xs.tag == W_AIR) {
    Value pair[2] = {wp_take(n, xs), wp_drop(n, xs)};
    return w_twine_copy(pair, 2);
  }
  WThread *x = thr_of(xs);
  size_t k = n.earth < 0 ? 0 : (size_t)n.earth;
  if (k > x->len)
    k = x->len;
  Value pair[2];
  pair[0] = thr_window(x, 0, k);
  pair[1] = thr_window(x, k, x->len - k);
  return w_twine_copy(pair, 2);
}

// strands breaks a Thread into runs of adjacent equals, which is what
// counting repeats or reading a run-length encoding wants. The runs share the
// original's storage, so this costs one Thread per run and no copying.
Value wp_strands(Value key, Value xs) {
  WThread *x = thr_of(xs);
  Buf out = {0};
  size_t i = 0;
  while (i < x->len) {
    Value k = call1(key, thr_at(x, i));
    size_t start = i;
    i++;
    while (i < x->len && w_equal(call1(key, thr_at(x, i)), k))
      i++;
    buf_push(&out, thr_window(x, start, i - start));
  }
  return buf_thread(&out);
}

// plait takes from each Thread in turn and stops with the shorter. It is `zip`
// flattened — `plait as bs` is `zip as bs` with the Twines taken off — and it
// takes its arguments in the same order for that reason, so the first Thread
// leads in both.
Value wp_plait(Value as, Value bs) {
  WThread *a = thr_of(as), *b = thr_of(bs);
  size_t n = a->len < b->len ? a->len : b->len;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? 2 * n : 1));
  for (size_t i = 0; i < n; i++) {
    out[2 * i] = thr_at(a, i);
    out[2 * i + 1] = thr_at(b, i);
  }
  return w_thread(out, 2 * n);
}

Value wp_zip(Value as, Value bs) {
  WThread *x = thr_of(as), *y = thr_of(bs);
  size_t n = x->len < y->len ? x->len : y->len;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  for (size_t i = 0; i < n; i++) {
    Value *pair = (Value *)w_alloc(sizeof(Value) * 2);
    pair[0] = thr_at(x, i);
    pair[1] = thr_at(y, i);
    out[i] = w_twine(pair, 2);
  }
  return w_thread(out, n);
}

// ------------------------------------------------------------------ sorting
//
// A sort of Weave's own rather than libc's qsort, for three reasons.
//
// qsort compares through a function pointer, so every comparison is an indirect
// call that cannot be inlined and that the branch predictor learns nothing
// from; it moves elements through a generic byte-wise path; and it is not
// required to be stable. The third is the one that decides it: ordering by a
// derived key has to keep equal keys in the order they were given, or a program
// that sorts and then walks gets a different answer on a different libc.
// Advent of Code 2025 day 8 orders half a million edges by distance, and there
// are a great many ties.
//
// So: a bottom-up merge sort. Stable by construction — the merge takes from the
// left run whenever the right is not strictly smaller — with no recursion and
// one scratch buffer, ping-ponged so that no pass copies back.

// W_MERGE_SORT writes that sort out for one element type and one comparison.
// Three are wanted — over values, over key/item pairs, and over pairs whose key
// is an unboxed integer — and the merge is the same in all three, so writing it
// three times to avoid a macro would be the worse trade.
//
// `before(x, y)` must be *strictly* before: it is what makes the sort stable.
#define W_MERGE_SORT(name, type, before)                                       \
  static void name(type *a, type *tmp, size_t n) {                             \
    type *src = a, *dst = tmp;                                                 \
    for (size_t width = 1; width < n; width *= 2) {                            \
      for (size_t i = 0; i < n; i += 2 * width) {                              \
        size_t mid = i + width < n ? i + width : n;                            \
        size_t end = i + 2 * width < n ? i + 2 * width : n;                    \
        size_t l = i, r = mid, k = i;                                          \
        while (l < mid && r < end)                                             \
          dst[k++] = before(src[r], src[l]) ? src[r++] : src[l++];             \
        while (l < mid)                                                        \
          dst[k++] = src[l++];                                                 \
        while (r < end)                                                        \
          dst[k++] = src[r++];                                                 \
      }                                                                        \
      type *swap = src;                                                        \
      src = dst;                                                               \
      dst = swap;                                                              \
    }                                                                          \
    if (src != a)                                                              \
      memcpy(a, src, sizeof(type) * n);                                        \
  }

#define W_VALUE_BEFORE(x, y) (w_compare(x, y) < 0)
W_MERGE_SORT(sort_values, Value, W_VALUE_BEFORE)

// An Earth key is sorted by its digits rather than by comparing at all.
//
// A merge sort of half a million elements is nineteen passes over the array,
// and every comparison in them is a branch nothing can predict. A least
// significant digit radix sort is one pass per byte that actually varies —
// three, for the squared distances Advent of Code 2025 day 8 orders — with no
// branch in the inner loop and a counting pass that is stable by construction.
//
// w_rank turns a signed key into the unsigned one whose bit order matches:
// flipping the sign bit puts the negatives, in order, below the positives.
static inline uint64_t w_rank(int64_t x) {
  return (uint64_t)x ^ ((uint64_t)1 << 63);
}

// Below this the counting arrays cost more than the comparisons they save.
#define W_RADIX_MIN 256

// W_RADIX_SORT writes that sort out for one element type and one way of
// reading its key, the same way W_MERGE_SORT does for a comparison.
#define W_RADIX_SORT(name, type, rank)                                         \
  static void name(type *a, type *tmp, size_t n) {                             \
    uint64_t ones = 0, zeros = ~(uint64_t)0;                                   \
    for (size_t i = 0; i < n; i++) {                                           \
      uint64_t k = rank(a[i]);                                                 \
      ones |= k;                                                               \
      zeros &= k;                                                              \
    }                                                                          \
    uint64_t vary = ones ^ zeros;                                              \
    type *src = a, *dst = tmp;                                                 \
    for (int byte = 0; byte < 8; byte++) {                                     \
      if (((vary >> (byte * 8)) & 0xFF) == 0)                                  \
        continue;                                                              \
      size_t count[256];                                                       \
      memset(count, 0, sizeof count);                                          \
      for (size_t i = 0; i < n; i++)                                           \
        count[(rank(src[i]) >> (byte * 8)) & 0xFF]++;                          \
      size_t at = 0;                                                           \
      for (int d = 0; d < 256; d++) {                                          \
        size_t c = count[d];                                                   \
        count[d] = at;                                                         \
        at += c;                                                               \
      }                                                                        \
      for (size_t i = 0; i < n; i++)                                           \
        dst[count[(rank(src[i]) >> (byte * 8)) & 0xFF]++] = src[i];            \
      type *swap = src;                                                        \
      src = dst;                                                               \
      dst = swap;                                                              \
    }                                                                          \
    if (src != a)                                                              \
      memcpy(a, src, sizeof(type) * n);                                        \
  }

#define W_VALUE_RANK(x) w_rank((x).earth)
W_RADIX_SORT(radix_values, Value, W_VALUE_RANK)

// The Earths are worth a sort of their own. `w_compare` is a switch on the tag
// followed by a call; when every element is an Earth the whole comparison is
// one signed load and a branch, and the tag check is one pass over the array
// rather than one per comparison.
#define W_EARTH_BEFORE(x, y) ((x).earth < (y).earth)
W_MERGE_SORT(sort_earths, Value, W_EARTH_BEFORE)

static bool all_earths(const Value *xs, size_t n) {
  for (size_t i = 0; i < n; i++)
    if (xs[i].tag != W_EARTH)
      return false;
  return true;
}

// sort_in_place orders a buffer the caller owns, taking the Earth path when it
// can. The scratch buffer is given back before returning.
static void sort_in_place(Value *out, size_t n) {
  if (n < 2)
    return;
  Value *tmp = (Value *)w_alloc(sizeof(Value) * n);
  if (!all_earths(out, n))
    sort_values(out, tmp, n);
  else if (n >= W_RADIX_MIN)
    radix_values(out, tmp, n);
  else
    sort_earths(out, tmp, n);
  w_free(tmp, sizeof(Value) * n);
}

Value wp_sort(Value xs) {
  WThread *t = thr_of(xs);
  Value *out = (Value *)w_alloc(sizeof(Value) * (t->len ? t->len : 1));
  memcpy(out, thr_boxed(t), sizeof(Value) * t->len);
  sort_in_place(out, t->len);
  return w_thread(out, t->len);
}

Value wp_all(Value p, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (!call1(p, thr_at(t, i)).spirit)
      return W_SHADOW;
  return W_LIGHT;
}

Value wp_any(Value p, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (call1(p, thr_at(t, i)).spirit)
      return W_LIGHT;
  return W_SHADOW;
}

// first, last and nth answer with an *element*, and text holds Fires. What that
// costs the type system is a constraint of its own — see settleStrands in
// internal/check — and what it costs here is a branch.
Value wp_first(Value xs) {
  if (xs.tag == W_AIR) {
    WAir *a = air_of(xs);
    return a->len ? w_held(w_fire(air_rune(a, 0))) : w_stilled();
  }
  WThread *t = thr_of(xs);
  return t->len ? w_held(thr_at(t, 0)) : w_stilled();
}

Value wp_last(Value xs) {
  if (xs.tag == W_AIR) {
    WAir *a = air_of(xs);
    if (a->len == 0)
      return w_stilled();
    // Back up over the continuation bytes to the start of the last rune.
    size_t i = a->len - 1;
    while (i > 0 && ((unsigned char)a->bytes[i] & 0xC0) == 0x80)
      i--;
    return w_held(w_fire(air_rune(a, i)));
  }
  WThread *t = thr_of(xs);
  return t->len ? w_held(thr_at(t, t->len - 1)) : w_stilled();
}

Value wp_rev(Value xs) {
  if (xs.tag == W_AIR) {
    // Runes, back to front: the bytes of a character stay in their own order.
    WAir *a = air_of(xs);
    char *out = (char *)w_alloc(a->len ? a->len : 1);
    size_t at = a->len;
    size_t i = 0;
    while (i < a->len) {
      size_t start = i;
      i++;
      while (i < a->len && ((unsigned char)a->bytes[i] & 0xC0) == 0x80)
        i++;
      at -= i - start;
      memcpy(out + at, a->bytes + start, i - start);
    }
    return w_air(out, a->len);
  }
  WThread *t = thr_of(xs);
  size_t n = t->len;
  // A packed Thread reversed is a packed Thread. Going through Values here
  // would undo the producer's work on the way past, which on a program that
  // reverses what it has just built is most of what packing saved.
  if (t->elems == NULL) {
    int64_t *raw = (int64_t *)w_alloc(sizeof(int64_t) * (n ? n : 1));
    for (size_t i = 0; i < n; i++)
      raw[i] = t->raw[n - 1 - i];
    return w_thread_packed(raw, n, t->obj.kind & W_THR_TAG);
  }
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  for (size_t i = 0; i < n; i++)
    out[i] = thr_at(t, n - 1 - i);
  return w_thread(out, n);
}

Value wp_flat(Value xss) {
  WThread *outer = thr_of(xss);
  Buf b = {0};
  for (size_t i = 0; i < outer->len; i++) {
    WThread *inner = thr_of(thr_at(outer, i));
    for (size_t j = 0; j < inner->len; j++)
      buf_push(&b, thr_at(inner, j));
  }
  return buf_thread(&b);
}

// uniq keeps the first of every value and drops the rest, in the order they
// first appeared. What has been seen goes in a set rather than being scanned:
// the straight comparison against everything kept is quadratic, which nobody
// notices on a hand-written Thread and which costs forty seconds on the eighty
// thousand candidates of Advent of Code 2025 day 2. `Eq` is all the type asks
// for, and it is all a Circle key asks for too.
Value wp_uniq(Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  Value seen = w_circle_empty();
  for (size_t i = 0; i < t->len; i++) {
    if (w_web_has(seen, thr_at(t, i)))
      continue;
    seen = w_web_put_owned(seen, thr_at(t, i), W_LIGHT);
    buf_push(&b, thr_at(t, i));
  }
  return buf_thread(&b);
}

// index says where each value sits, which is the Web a program builds the
// moment it has a Thread it means to look things up in. The first position
// wins, so it agrees with `uniq` about which of a repeated value is the one.
Value wp_index(Value xs) {
  WThread *t = thr_of(xs);
  Value w = w_web_empty();
  w.obj->rc = W_OWNED;
  for (size_t i = 0; i < t->len; i++)
    if (!w_web_has(w, thr_at(t, i)))
      w = w_web_put_owned(w, thr_at(t, i), w_earth((int64_t)i));
  return w_disown(w);
}

// squeeze turns a sparse axis into a dense one: the sorted distinct values, and
// after each one that has a gap after it, a single stand-in for the whole run.
//
// It is what makes a plane too large to draw drawable. Nothing changes between
// one coordinate and the next, so a run of ten thousand columns behaves exactly
// as one column does, and five hundred coordinates become an axis of about a
// thousand lines.
//
// The stand-in is one past the value it follows, so it is a value the input
// cannot have held — there would have been no gap if it had. That is what lets
// `index` serve both: a real coordinate looks up its own line, and no stand-in
// can collide with one.
Value wp_squeeze(Value vs) {
  Value sorted = wp_uniq(wp_sort(vs));
  WThread *s = thr_of(sorted);
  Buf out = {0};
  for (size_t i = 0; i < s->len; i++) {
    int64_t v = thr_at(s, i).earth;
    buf_push(&out, w_earth(v));
    if (i + 1 < s->len && thr_at(s, i + 1).earth > v + 1)
      buf_push(&out, w_earth(v + 1));
  }
  return buf_thread(&out);
}

// ------------------------------------------------------------------ pattern

Value wp_pattern(Value text) {
  Value rows = wp_lines(text);
  size_t nrows = w_thread_len(rows), ncols = 0;
  for (size_t r = 0; r < nrows; r++) {
    size_t n = w_thread_len(wp_fires(w_thread_at(rows, r)));
    if (n > ncols)
      ncols = n;
  }
  Value *cells = (Value *)w_alloc(sizeof(Value) * (nrows * ncols ? nrows * ncols : 1));
  for (size_t r = 0; r < nrows; r++) {
    Value line = wp_fires(w_thread_at(rows, r));
    size_t n = w_thread_len(line);
    for (size_t c = 0; c < ncols; c++)
      cells[r * ncols + c] = c < n ? w_thread_at(line, c) : w_fire(' ');
  }
  WPattern *p = (WPattern *)w_alloc(sizeof(WPattern));
  p->obj.rc = W_SHARED;
  p->obj.kind = W_PATTERN;
  p->rows = nrows;
  p->cols = ncols;
  p->cells = cells;
  Value v;
  v.tag = W_PATTERN;
  v.obj = &p->obj;
  return v;
}

// weft builds a Pattern out of rows of anything, which is the only way to get
// a grid whose cells are not single characters: `Source | lines | bend earths
// | weft` is a grid of Earths. Short rows are padded with the fill value so
// the result stays rectangular.
Value wp_weft(Value fill, Value rows) {
  WThread *outer = thr_of(rows);
  size_t nrows = outer->len, ncols = 0;
  for (size_t r = 0; r < nrows; r++) {
    size_t n = w_thread_len(thr_at(outer, r));
    if (n > ncols)
      ncols = n;
  }
  Value *cells = (Value *)w_alloc(sizeof(Value) * (nrows * ncols ? nrows * ncols : 1));
  for (size_t r = 0; r < nrows; r++) {
    WThread *row = thr_of(thr_at(outer, r));
    for (size_t c = 0; c < ncols; c++)
      cells[r * ncols + c] = c < row->len ? thr_at(row, c) : fill;
  }
  WPattern *p = (WPattern *)w_alloc(sizeof(WPattern));
  p->obj.rc = W_SHARED;
  p->obj.kind = W_PATTERN;
  p->rows = nrows;
  p->cols = ncols;
  p->cells = cells;
  Value v;
  v.tag = W_PATTERN;
  v.obj = &p->obj;
  return v;
}

static WPattern *pat_of(Value v) { return (WPattern *)v.obj; }

static Value pattern_of(size_t rows, size_t cols, Value *cells) {
  WPattern *p = (WPattern *)w_alloc(sizeof(WPattern));
  p->obj.rc = W_SHARED;
  p->obj.kind = W_PATTERN;
  p->rows = rows;
  p->cols = cols;
  p->cells = cells;
  Value v;
  v.tag = W_PATTERN;
  v.obj = &p->obj;
  return v;
}

// warp is the grid constructor `weft` is not. `weft` weaves rows you already
// have; `warp` is given the shape and a function from a knot to what belongs
// there, which is how a board is laid out before anything is on it. Written by
// hand it is a `span` inside a `span` inside a `weft`, and the fill value the
// `weft` then needs is one nothing will ever use.
Value wp_warp(Value f, Value rows, Value cols) {
  int64_t r = rows.earth, c = cols.earth;
  if (r <= 0 || c <= 0)
    return pattern_of(0, 0, (Value *)w_alloc(sizeof(Value)));
  size_t nr = (size_t)r, nc = (size_t)c;
  Value *cells = (Value *)w_alloc(sizeof(Value) * nr * nc);
  for (size_t i = 0; i < nr; i++)
    for (size_t j = 0; j < nc; j++) {
      Value k = w_knot_make((int64_t)i, (int64_t)j);
      cells[i * nc + j] = w_call(f, &k, 1);
    }
  return pattern_of(nr, nc, cells);
}


// spin turns a Pattern a quarter turn clockwise, flip mirrors it left to right.
// Between them, and with repetition, they reach all eight orientations — which
// is what a jigsaw day needs and what nobody wants to write by hand twice.
Value wp_spin(Value g) {
  WPattern *p = pat_of(g);
  size_t rows = p->rows, cols = p->cols;
  Value *cells = (Value *)w_alloc(sizeof(Value) * (rows * cols ? rows * cols : 1));
  for (size_t r = 0; r < rows; r++)
    for (size_t c = 0; c < cols; c++)
      // The top-left cell becomes the top-right one.
      cells[c * rows + (rows - 1 - r)] = p->cells[r * cols + c];
  return pattern_of(cols, rows, cells);
}

Value wp_flip(Value g) {
  WPattern *p = pat_of(g);
  size_t rows = p->rows, cols = p->cols;
  Value *cells = (Value *)w_alloc(sizeof(Value) * (rows * cols ? rows * cols : 1));
  for (size_t r = 0; r < rows; r++)
    for (size_t c = 0; c < cols; c++)
      cells[r * cols + (cols - 1 - c)] = p->cells[r * cols + c];
  return pattern_of(rows, cols, cells);
}

// pattern_of builds a grid from cells the caller has already allocated.

// cellwise maps over every square of a grid, keeping its shape. Without it a
// grid transformation is `cells | bend f` and then a rebuild, which loses the
// shape and is the one thing `grid` cannot give back.
Value wp_cellwise(Value f, Value g) {
  WPattern *p = pat_of(g);
  size_t n = p->rows * p->cols;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  for (size_t i = 0; i < n; i++)
    out[i] = call1(f, p->cells[i]);
  return pattern_of(p->rows, p->cols, out);
}

// tallies is the running total over a grid: each cell replaced by the sum of
// the rectangle from the top left corner to it, both ends included.
//
// It is what makes "how much is inside this box" one subtraction instead of a
// walk, for as many boxes as are asked. Advent of Code 2025 day 9 asks it a
// hundred thousand times over a grid a thousand a side; written out by hand it
// is a `priors` along each row and another down the rows, plus a four-corner
// query at every use, and every one of the off-by-ones is in the padding that
// the corners need. `tallied` below owns those, so nothing has to be padded.
Value wp_tallies(Value g) {
  WPattern *p = pat_of(g);
  size_t rows = p->rows, cols = p->cols, n = rows * cols;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  for (size_t r = 0; r < rows; r++) {
    for (size_t c = 0; c < cols; c++) {
      Value acc = p->cells[r * cols + c];
      if (c)
        acc = wp_add(acc, out[r * cols + c - 1]);
      if (r)
        acc = wp_add(acc, out[(r - 1) * cols + c]);
      if (r && c)
        acc = wp_sub(acc, out[(r - 1) * cols + c - 1]);
      out[r * cols + c] = acc;
    }
  }
  return pattern_of(rows, cols, out);
}

// tallied reads a rectangle back out of a grid `tallies` made: the total over
// the inclusive box between two knots, in whichever order they are given, and
// clipped to the grid.
//
// The four corners are the whole reason this is a verb. Three of them sit one
// row or one column *before* the box, which is where the hand-written version
// needs a border of zeroes, and getting the sign of the fourth wrong gives an
// answer that is plausible everywhere except at the edges.
Value wp_tallied(Value g, Value a, Value b) {
  WPattern *p = pat_of(g);
  if (p->rows == 0 || p->cols == 0)
    return w_earth(0);

  int64_t r1 = a.knot.row, r2 = b.knot.row;
  int64_t c1 = a.knot.col, c2 = b.knot.col;
  if (r1 > r2) {
    int64_t t = r1;
    r1 = r2;
    r2 = t;
  }
  if (c1 > c2) {
    int64_t t = c1;
    c1 = c2;
    c2 = t;
  }
  if (r1 < 0)
    r1 = 0;
  if (c1 < 0)
    c1 = 0;
  if (r2 >= (int64_t)p->rows)
    r2 = (int64_t)p->rows - 1;
  if (c2 >= (int64_t)p->cols)
    c2 = (int64_t)p->cols - 1;
  if (r1 > r2 || c1 > c2)
    return w_earth(0); // the box lies wholly off the grid

  Value total = p->cells[(size_t)r2 * p->cols + (size_t)c2];
  if (r1 > 0)
    total = wp_sub(total, p->cells[(size_t)(r1 - 1) * p->cols + (size_t)c2]);
  if (c1 > 0)
    total = wp_sub(total, p->cells[(size_t)r2 * p->cols + (size_t)(c1 - 1)]);
  if (r1 > 0 && c1 > 0)
    total = wp_add(total, p->cells[(size_t)(r1 - 1) * p->cols + (size_t)(c1 - 1)]);
  return total;
}

// ------------------------------------------------------------ linear systems
//
// solve reads a Pattern of Earths as an augmented matrix — one row per
// equation, the last column the right-hand side — and answers with the integer
// solutions.
//
// It exists because forty lines of one Advent of Code program were this and
// nothing else, and because the same forty lines turn up every year. What made
// it a design question rather than a function is the null space: a system with
// more unknowns than it pins down has infinitely many answers, and each puzzle
// uses that differently. So the answer is a Weaving, and which side it comes
// back on is the question "was this pinned down":
//
//   Woven x                  one solution, and it is whole numbers
//   Gentled (point, basis)   more than one, over the rationals
//
// The Gentled side carries a point as well as the directions, because the
// answer is point + span(basis) and the directions alone cannot rebuild it. An
// *empty* point means no whole-number answer was found: either the equations
// contradict each other, or the one solution they have is fractional, or — with
// a basis beside it — the solution set is real but does not pass through the
// place where every free unknown is zero.
//
// The arithmetic is integer throughout: rows are combined by the multipliers
// their gcd leaves behind and divided by their content afterwards, which is
// what keeps the entries from growing the way plain elimination makes them.
// Anything that would leave an Earth stops the program rather than wrapping.

// A row operation, guarded. Elimination is where these numbers grow, and an
// answer computed out of a wrapped multiply looks exactly like a right one.
static int64_t solve_mul(int64_t a, int64_t b) {
  int64_t r;
  if (__builtin_mul_overflow(a, b, &r))
    w_fail("`solve` overflowed: these equations need numbers larger than an Earth");
  return r;
}

static int64_t solve_sub(int64_t a, int64_t b) {
  int64_t r;
  if (__builtin_sub_overflow(a, b, &r))
    w_fail("`solve` overflowed: these equations need numbers larger than an Earth");
  return r;
}

static int64_t solve_gcd(int64_t a, int64_t b) {
  if (a < 0)
    a = -a;
  if (b < 0)
    b = -b;
  while (b) {
    int64_t t = a % b;
    a = b;
    b = t;
  }
  return a;
}

// solve_reduce divides a row by the gcd of its entries, which is the whole
// reason integer elimination stays inside an Earth. Without it the entries
// multiply up every turn and a five-by-five system overflows.
static void solve_reduce(int64_t *row, size_t n) {
  int64_t g = 0;
  for (size_t i = 0; i < n; i++)
    g = solve_gcd(g, row[i]);
  if (g <= 1)
    return;
  for (size_t i = 0; i < n; i++)
    row[i] /= g;
}

// solve_done hands the working copy back. A puzzle that solves one system per
// line of its input calls this often enough for the free lists to matter.
static void solve_done(int64_t *a, size_t *pivotRow, size_t m, size_t w, size_t n) {
  w_free(a, sizeof(int64_t) * (m * w ? m * w : 1));
  w_free(pivotRow, sizeof(size_t) * (n ? n : 1));
}

Value wp_solve(Value grid) {
  WPattern *p = pat_of(grid);
  size_t m = p->rows, w = p->cols;
  if (w == 0)
    w_fail("`solve` needs a column for the right-hand side");
  size_t n = w - 1; // unknowns

  int64_t *a = (int64_t *)w_alloc(sizeof(int64_t) * (m * w ? m * w : 1));
  for (size_t i = 0; i < m * w; i++)
    a[i] = p->cells[i].earth;

  // Which row pins down which unknown, and which unknowns nothing pins down.
  size_t *pivotRow = (size_t *)w_alloc(sizeof(size_t) * (n ? n : 1));
  for (size_t c = 0; c < n; c++)
    pivotRow[c] = m; // m means "free"

  size_t rank = 0;
  for (size_t c = 0; c < n && rank < m; c++) {
    size_t piv = m;
    for (size_t r = rank; r < m; r++)
      if (a[r * w + c] != 0) {
        piv = r;
        break;
      }
    if (piv == m)
      continue; // nothing here pins this unknown down
    if (piv != rank)
      for (size_t k = 0; k < w; k++) {
        int64_t t = a[rank * w + k];
        a[rank * w + k] = a[piv * w + k];
        a[piv * w + k] = t;
      }
    // Every other row, not just the ones below: clearing the column both ways
    // is what leaves each pivot row saying one thing about one unknown, which
    // is what the point and the directions are both read out of.
    for (size_t r = 0; r < m; r++) {
      if (r == rank || a[r * w + c] == 0)
        continue;
      int64_t g = solve_gcd(a[rank * w + c], a[r * w + c]);
      int64_t keep = a[rank * w + c] / g, drop = a[r * w + c] / g;
      for (size_t k = 0; k < w; k++)
        a[r * w + k] =
            solve_sub(solve_mul(keep, a[r * w + k]), solve_mul(drop, a[rank * w + k]));
      solve_reduce(&a[r * w], w);
    }
    pivotRow[c] = rank;
    rank++;
  }

  // A row saying `0 = c` is two equations that cannot both hold.
  bool consistent = true;
  for (size_t r = 0; r < m && consistent; r++) {
    bool allZero = true;
    for (size_t c = 0; c < n && allZero; c++)
      allZero = a[r * w + c] == 0;
    if (allZero && a[r * w + n] != 0)
      consistent = false;
  }

  size_t free = 0;
  for (size_t c = 0; c < n; c++)
    if (pivotRow[c] == m)
      free++;

  // The point where every free unknown is zero. Each pivot row is now
  // `k * x = rhs` in its own unknown alone, so the answer is whole exactly
  // when it divides.
  EBuf point = {0};
  bool whole = consistent;
  for (size_t c = 0; c < n && whole; c++) {
    if (pivotRow[c] == m) {
      ebuf_push(&point, 0);
      continue;
    }
    size_t r = pivotRow[c];
    int64_t k = a[r * w + c], rhs = a[r * w + n];
    if (rhs % k != 0) {
      whole = false;
      break;
    }
    ebuf_push(&point, rhs / k);
  }
  Value pointT = whole ? ebuf_thread(&point, W_EARTH) : w_thread_empty();

  // The working copy is dead from here: everything answered with has been
  // built out of it and none of it points back in.
  if (consistent && free == 0 && whole) {
    solve_done(a, pivotRow, m, w, n);
    Value one = pointT;
    return w_data("Woven", 0, &one, 1);
  }
  if (!consistent || free == 0) {
    // Equations that contradict each other have no solutions to point at, and
    // one fractional solution has no whole ones. Both say so the same way: no
    // point, and no directions to move along from it.
    solve_done(a, pivotRow, m, w, n);
    Value pair[2] = {w_thread_empty(), w_thread_empty()};
    Value none = w_twine_copy(pair, 2);
    return w_data("Gentled", 1, &none, 1);
  }

  // One direction per unknown nothing pinned down: set that one to a step big
  // enough to leave the others whole, and read the rest off the pivot rows.
  Buf dirs = {0};
  for (size_t f = 0; f < n; f++) {
    if (pivotRow[f] != m)
      continue;
    // The step is the least one that clears every pivot's denominator.
    int64_t step = 1;
    for (size_t c = 0; c < n; c++) {
      if (pivotRow[c] == m)
        continue;
      size_t r = pivotRow[c];
      if (a[r * w + f] == 0)
        continue;
      int64_t k = a[r * w + c];
      if (k < 0)
        k = -k;
      step = solve_mul(step / solve_gcd(step, k), k);
    }
    EBuf v = {0};
    for (size_t c = 0; c < n; c++) {
      if (c == f) {
        ebuf_push(&v, step);
      } else if (pivotRow[c] == m) {
        ebuf_push(&v, 0);
      } else {
        size_t r = pivotRow[c];
        ebuf_push(&v, -solve_mul(a[r * w + f], step) / a[r * w + c]);
      }
    }
    solve_reduce(v.items, v.len);
    buf_push(&dirs, ebuf_thread(&v, W_EARTH));
  }

  solve_done(a, pivotRow, m, w, n);
  Value pair[2] = {pointT, buf_thread(&dirs)};
  Value both = w_twine_copy(pair, 2);
  return w_data("Gentled", 1, &both, 1);
}

static bool in_bounds(WPattern *p, Value k) {
  return k.knot.row >= 0 && k.knot.col >= 0 && (size_t)k.knot.row < p->rows &&
         (size_t)k.knot.col < p->cols;
}

// The grid producers the loop fuser generates rather than builds. The verbs
// below all walk the same two tables, and a fused loop walks them too, so they
// are named here once — the orthogonal four first, which is what lets `nb4`
// and `nb8` be the same loop with a different bound.
const int8_t w_grid_dr[8] = {-1, 1, 0, 0, -1, -1, 1, 1};
const int8_t w_grid_dc[8] = {0, 0, -1, 1, -1, 1, -1, 1};

size_t w_pattern_shape(Value g, size_t *rows, size_t *cols) {
  WPattern *p = pat_of(g);
  *rows = p->rows;
  *cols = p->cols;
  return p->rows * p->cols;
}

bool w_pattern_in(Value g, int64_t r, int64_t c) {
  WPattern *p = pat_of(g);
  return r >= 0 && c >= 0 && (size_t)r < p->rows && (size_t)c < p->cols;
}

Value w_pattern_cell(Value g, int64_t r, int64_t c) {
  WPattern *p = pat_of(g);
  return p->cells[(size_t)r * p->cols + (size_t)c];
}

Value wp_cell(Value g, Value k) {
  WPattern *p = pat_of(g);
  if (!in_bounds(p, k))
    return w_stilled();
  return w_held(p->cells[(size_t)k.knot.row * p->cols + (size_t)k.knot.col]);
}

Value wp_set(Value g, Value k, Value cell) {
  WPattern *p = pat_of(g);
  if (!in_bounds(p, k))
    return g;
  size_t n = p->rows * p->cols;
  Value *cells = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  memcpy(cells, p->cells, sizeof(Value) * n);
  cells[(size_t)k.knot.row * p->cols + (size_t)k.knot.col] = cell;
  WPattern *q = (WPattern *)w_alloc(sizeof(WPattern));
  q->obj.rc = W_SHARED;
  q->obj.kind = W_PATTERN;
  q->rows = p->rows;
  q->cols = p->cols;
  q->cells = cells;
  Value v;
  v.tag = W_PATTERN;
  v.obj = &q->obj;
  return v;
}

// wp_set_owned is `set` for a grid the compiler has proved is not shared: it
// writes through instead of copying. See internal/codegen/inplace.go for the
// conditions, and SPEC.md section 13.
//
// The object header's rc field carries the one bit this needs. A grid arrives
// from `grid` marked shared, because the caller may still hold it, so the first
// update in a loop copies and marks the copy as owned; every later update in
// that loop writes through. That turns a loop of updates from quadratic into
// linear without the compiler having to reason about the caller.
Value wp_set_owned(Value g, Value k, Value cell) {
  WPattern *p = pat_of(g);
  if (!in_bounds(p, k))
    return g;
  if (p->obj.rc == W_OWNED) {
    p->cells[(size_t)k.knot.row * p->cols + (size_t)k.knot.col] = cell;
    return g;
  }
  Value copy = wp_set(g, k, cell);
  copy.obj->rc = W_OWNED;
  return copy;
}

Value wp_knots(Value g) {
  WPattern *p = pat_of(g);
  size_t n = p->rows * p->cols;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  for (size_t r = 0; r < p->rows; r++)
    for (size_t c = 0; c < p->cols; c++)
      out[r * p->cols + c] = w_knot_make((int64_t)r, (int64_t)c);
  return w_thread(out, n);
}

// sited and sites are the other way round from `cell`: where a value is rather
// than what is at a knot. Finding the start of a maze, or every antenna of one
// frequency, is otherwise `knots g | seek (k : eq v (cell g k | otherwise ...))`
// — which every grid program writes and which has to invent a default for a
// lookup that cannot fail.
//
// Both halves, as everywhere else: the first, and all of them. Reading order,
// so the answer does not depend on how the grid is stored.
Value wp_sited(Value g, Value v) {
  WPattern *p = pat_of(g);
  size_t n = p->rows * p->cols;
  for (size_t i = 0; i < n; i++)
    if (w_equal(p->cells[i], v))
      return w_held(w_knot_make((int64_t)(i / p->cols), (int64_t)(i % p->cols)));
  return w_stilled();
}

Value wp_sites(Value g, Value v) {
  WPattern *p = pat_of(g);
  size_t n = p->rows * p->cols;
  Buf b = {0};
  for (size_t i = 0; i < n; i++)
    if (w_equal(p->cells[i], v))
      buf_push(&b, w_knot_make((int64_t)(i / p->cols), (int64_t)(i % p->cols)));
  return buf_thread(&b);
}

Value wp_cells(Value g) {
  WPattern *p = pat_of(g);
  // The Thread shares the grid's buffer, so the grid can no longer be written
  // through: whoever holds the Thread would see the change.
  p->obj.rc = W_SHARED;
  return w_thread(p->cells, p->rows * p->cols);
}

static Value neighbors_of(Value g, Value k, bool diagonals) {
  static const int dr[8] = {-1, 1, 0, 0, -1, -1, 1, 1};
  static const int dc[8] = {0, 0, -1, 1, -1, 1, -1, 1};
  WPattern *p = pat_of(g);
  Buf b = {0};
  int n = diagonals ? 8 : 4;
  for (int i = 0; i < n; i++) {
    Value nk = w_knot_make(k.knot.row + dr[i], k.knot.col + dc[i]);
    if (in_bounds(p, nk))
      buf_push(&b, p->cells[(size_t)nk.knot.row * p->cols + (size_t)nk.knot.col]);
  }
  return buf_thread(&b);
}

Value wp_nb4(Value g, Value k) { return neighbors_of(g, k, false); }
Value wp_nb8(Value g, Value k) { return neighbors_of(g, k, true); }
Value wp_rows(Value g) { return w_earth((int64_t)pat_of(g)->rows); }
Value wp_cols(Value g) { return w_earth((int64_t)pat_of(g)->cols); }

Value wp_knot(Value r, Value c) { return w_knot_make(r.earth, c.earth); }
Value wp_row(Value k) { return w_earth(k.knot.row); }
Value wp_col(Value k) { return w_earth(k.knot.col); }

// -------------------------------------------------------------- collections

// A verb that builds a whole Web or Circle of its own owns it while it does, so
// it uses the in-place insert and hands back a map nobody has written into
// twice. `sealed` clears the ownership bit on the way out, so the value the
// program receives is an ordinary shared one — whoever adds to it next copies
// first, exactly as if it had been built the persistent way.
static Value sealed(Value m) {
  m.obj->rc = W_SHARED;
  return m;
}

Value wp_web(Value pairs) {
  WThread *t = thr_of(pairs);
  Value web = w_web_empty();
  for (size_t i = 0; i < t->len; i++)
    web = w_web_put_owned(web, w_twine_at(thr_at(t, i), 0), w_twine_at(thr_at(t, i), 1));
  return sealed(web);
}

Value wp_get(Value web, Value key) { return w_web_get(web, key); }
Value wp_put(Value web, Value key, Value val) { return w_web_put(web, key, val); }
Value wp_known(Value web, Value key) { return w_spirit(w_web_has(web, key)); }
Value wp_forget(Value web, Value key) { return w_web_forget(web, key); }
Value wp_keys(Value web) { return w_web_keys(web); }
Value wp_vals(Value web) { return w_web_vals(web); }
Value wp_items(Value web) { return w_web_pairs(web); }

// merge prefers the second Web where both hold a key, so `merge old new`
// reads as an update.
Value wp_merge(Value a, Value b) {
  Value ps = w_web_pairs(b);
  WThread *t = thr_of(ps);
  Value out = a;
  for (size_t i = 0; i < t->len; i++)
    out = w_web_put_owned(out, w_twine_at(thr_at(t, i), 0), w_twine_at(thr_at(t, i), 1));
  return sealed(out);
}

Value wp_freq(Value xs) {
  WThread *t = thr_of(xs);
  Value web = w_web_empty();
  for (size_t i = 0; i < t->len; i++) {
    Value seen = w_web_get(web, thr_at(t, i));
    int64_t n = w_is_held(seen) ? w_hold_inner(seen).earth : 0;
    web = w_web_put_owned(web, thr_at(t, i), w_earth(n + 1));
  }
  return sealed(web);
}

Value wp_most(Value web) {
  Value ps = w_web_pairs(web);
  WThread *t = thr_of(ps);
  if (t->len == 0)
    return w_stilled();
  Value best = w_twine_at(thr_at(t, 0), 0);
  int64_t bestN = w_twine_at(thr_at(t, 0), 1).earth;
  for (size_t i = 1; i < t->len; i++) {
    int64_t n = w_twine_at(thr_at(t, i), 1).earth;
    if (n > bestN) {
      bestN = n;
      best = w_twine_at(thr_at(t, i), 0);
    }
  }
  return w_held(best);
}

Value wp_circle(Value xs) {
  WThread *t = thr_of(xs);
  Value c = w_circle_empty();
  for (size_t i = 0; i < t->len; i++)
    c = w_web_put_owned(c, thr_at(t, i), W_LIGHT);
  return sealed(c);
}

Value wp_member(Value circle, Value v) { return w_spirit(w_web_has(circle, v)); }
Value wp_insert(Value circle, Value v) { return w_web_put(circle, v, W_LIGHT); }

// The in-place forms, for a map or set the compiler has proved is threaded
// without ever being duplicated. See internal/codegen/inplace.go.
Value wp_put_owned(Value web, Value key, Value val) {
  return w_web_put_owned(web, key, val);
}
Value wp_insert_owned(Value circle, Value v) {
  return w_web_put_owned(circle, v, W_LIGHT);
}
Value wp_remove(Value circle, Value v) { return w_web_forget(circle, v); }
Value wp_forget_owned(Value web, Value key) { return w_web_forget_owned(web, key); }
Value wp_remove_owned(Value circle, Value v) { return w_web_forget_owned(circle, v); }
Value wp_members(Value circle) { return w_web_keys(circle); }

// ------------------------------------------------------------------- links
//
// A Link is who is joined to whom, and it is the one question `clumps` cannot
// answer. `clumps` is asked once, of a finished graph; a Link is asked *while*
// the joining happens — after this connection, are these two in one circle yet,
// and how big has each circle grown? Advent of Code 2025 day 8 walks half a
// million pairs in order of distance and watches the circuits form, which is
// Kruskal's algorithm and is the shape every minimum-spanning-tree puzzle has.
//
// Disjoint sets, with the two usual tricks. Union by rank keeps the forest
// shallow; path halving flattens what it walks, and is a benign mutation — it
// changes which parent a node answers to but never which circle it is in, so a
// shared Link may do it and every holder still sees the same answer.
//
// Binding is not benign, so a shared Link copies first. Only `parent` and
// `rank` are copied: which values exist never changes, so `slots` and `nodes`
// are shared by every Link derived from one. That is eight bytes a node, which
// is what makes the copying path cheap enough that a program that forgets to
// thread its Link single-threadedly is slow rather than hopeless.

static WLink *link_of(Value v) { return (WLink *)v.obj; }

static Value link_value(WLink *l) {
  Value v;
  v.tag = W_LINK;
  v.obj = &l->obj;
  return v;
}

// find, with path halving: every second node on the way up is pointed at its
// grandparent, which flattens the tree without a second pass.
static int32_t link_find(WLink *l, int32_t i) {
  while (l->parent[i] != i) {
    l->parent[i] = l->parent[l->parent[i]];
    i = l->parent[i];
  }
  return i;
}

// The position of a value, or -1 when the Link was not built with it. A value
// it does not know cannot be bound to anything, which is the honest answer:
// `link` is given the nodes, and a node that was left out does not exist.
static int32_t link_slot(WLink *l, Value v) {
  Value at = w_web_get(l->slots, v);
  return w_is_held(at) ? (int32_t)w_hold_inner(at).earth : -1;
}

Value wp_link(Value nodes) {
  WThread *t = thr_of(nodes);
  Value slots = w_web_empty();
  slots.obj->rc = W_OWNED;

  Value *vals = (Value *)w_alloc(sizeof(Value) * (t->len ? t->len : 1));
  size_t n = 0;
  for (size_t i = 0; i < t->len; i++) {
    if (w_web_has(slots, thr_at(t, i)))
      continue; // a value given twice is one node
    slots = w_web_put_owned(slots, thr_at(t, i), w_earth((int64_t)n));
    vals[n++] = thr_at(t, i);
  }

  WLink *l = (WLink *)w_alloc(sizeof(WLink));
  l->obj.rc = W_SHARED;
  l->obj.kind = W_LINK;
  l->slots = w_disown(slots);
  l->nodes = vals;
  l->len = n;
  l->parent = (int32_t *)w_alloc(sizeof(int32_t) * (n ? n : 1));
  l->rank = (int32_t *)w_alloc(sizeof(int32_t) * (n ? n : 1));
  for (size_t i = 0; i < n; i++) {
    l->parent[i] = (int32_t)i;
    l->rank[i] = 0;
  }
  return link_value(l);
}

// A copy sharing everything that cannot change.
static WLink *link_copy(WLink *from) {
  WLink *l = (WLink *)w_alloc(sizeof(WLink));
  *l = *from;
  l->obj.rc = W_OWNED;
  size_t n = from->len ? from->len : 1;
  l->parent = (int32_t *)w_alloc(sizeof(int32_t) * n);
  l->rank = (int32_t *)w_alloc(sizeof(int32_t) * n);
  memcpy(l->parent, from->parent, sizeof(int32_t) * n);
  memcpy(l->rank, from->rank, sizeof(int32_t) * n);
  return l;
}

// Union by rank, on a Link the caller is free to write through.
static void link_join(WLink *l, Value a, Value b) {
  int32_t x = link_slot(l, a), y = link_slot(l, b);
  if (x < 0 || y < 0)
    return;
  x = link_find(l, x);
  y = link_find(l, y);
  if (x == y)
    return;
  if (l->rank[x] < l->rank[y]) {
    int32_t t = x;
    x = y;
    y = t;
  }
  l->parent[y] = x;
  if (l->rank[x] == l->rank[y])
    l->rank[x]++;
}

Value wp_bind(Value v, Value a, Value b) {
  WLink *l = link_copy(link_of(v));
  link_join(l, a, b);
  l->obj.rc = W_SHARED;
  return link_value(l);
}

// The in-place form, for a Link the compiler has proved is threaded without
// ever being duplicated. See internal/codegen/inplace.go.
Value wp_bind_owned(Value v, Value a, Value b) {
  if (v.obj->rc == W_OWNED) {
    link_join(link_of(v), a, b);
    return v;
  }
  WLink *l = link_copy(link_of(v));
  link_join(l, a, b);
  return link_value(l);
}

Value wp_bound(Value v, Value a, Value b) {
  WLink *l = link_of(v);
  int32_t x = link_slot(l, a), y = link_slot(l, b);
  if (x < 0 || y < 0)
    return w_spirit(w_equal(a, b));
  return w_spirit(link_find(l, x) == link_find(l, y));
}

// The circles, each once, in the order their first member was given. A node
// that was never bound to anything is a circle of one, which is what makes the
// sizes add up to however many nodes there were.
// Three passes rather than one, so that no circle is ever rebuilt as it grows:
// where each node's circle will go, then how long each circle is, then the
// nodes written straight into arrays already the right size.
Value wp_clumped(Value v) {
  WLink *l = link_of(v);
  size_t n = l->len ? l->len : 1;
  size_t bytes = sizeof(int32_t) * n;

  int32_t *at = (int32_t *)w_alloc(bytes);   // root -> which circle
  int32_t *size = (int32_t *)w_alloc(bytes); // circle -> how many
  int32_t *fill = (int32_t *)w_alloc(bytes); // circle -> how many written
  for (size_t i = 0; i < n; i++)
    at[i] = -1;

  size_t circles = 0;
  for (size_t i = 0; i < l->len; i++) {
    int32_t r = link_find(l, (int32_t)i);
    if (at[r] < 0) {
      at[r] = (int32_t)circles;
      size[circles] = 0;
      fill[circles] = 0;
      circles++;
    }
    size[at[r]]++;
  }

  Value *out = (Value *)w_alloc(sizeof(Value) * (circles ? circles : 1));
  Value **cells = (Value **)w_alloc(sizeof(Value *) * (circles ? circles : 1));
  for (size_t c = 0; c < circles; c++) {
    cells[c] = (Value *)w_alloc(sizeof(Value) * (size_t)size[c]);
    out[c] = w_thread(cells[c], (size_t)size[c]);
  }
  for (size_t i = 0; i < l->len; i++) {
    int32_t c = at[link_find(l, (int32_t)i)];
    cells[c][fill[c]++] = l->nodes[i];
  }

  w_free(at, bytes);
  w_free(size, bytes);
  w_free(fill, bytes);
  w_free(cells, sizeof(Value *) * (circles ? circles : 1));
  return w_thread(out, circles);
}

Value wp_taveren(Value xs) {
  WThread *t = thr_of(xs);
  Value h = w_taveren_empty();
  for (size_t i = 0; i < t->len; i++)
    h = w_taveren_push(h, thr_at(t, i));
  return h;
}

// reach is everything the step function can get to from here.
//
// It is the operation an Advent of Code grid asks for most — a region, a
// connected area, a flood fill — and every program that wanted it wrote the
// same hand-rolled frontier and seen-set. Breadth first, so a Thread is enough
// for the frontier and nothing needs a priority queue.
Value wp_reach(Value step, Value start) {
  Value seen = w_circle_empty();
  seen.obj->rc = W_OWNED;
  seen = w_web_put_owned(seen, start, W_LIGHT);

  Buf frontier = {0};
  buf_push(&frontier, start);
  while (frontier.len > 0) {
    Buf next = {0};
    for (size_t i = 0; i < frontier.len; i++) {
      Value out = w_call(step, &frontier.items[i], 1);
      size_t n = w_thread_len(out);
      for (size_t j = 0; j < n; j++) {
        Value node = w_thread_at(out, j);
        if (w_web_has(seen, node))
          continue;
        seen = w_web_put_owned(seen, node, W_LIGHT);
        buf_push(&next, node);
      }
    }
    frontier = next;
  }
  return w_disown(seen);
}

// route is dijkstra when the path matters rather than the cost. It keeps, for
// every node it settles, the node it came from, and walks that back.
Value wp_route(Value step, Value start, Value goal) {
  Value from = w_web_empty();
  from.obj->rc = W_OWNED;
  Value settled = w_circle_empty();
  settled.obj->rc = W_OWNED;

  Value first[2] = {w_earth(0), start};
  Value frontier = w_taveren_push(w_taveren_empty(), w_twine_copy(first, 2));
  bool found = w_equal(start, goal);

  while (!found) {
    Value taken = w_taveren_pop(frontier);
    if (!w_is_held(taken))
      break;
    Value pair = w_hold_inner(taken);
    Value entry = w_twine_at(pair, 0);
    frontier = w_twine_at(pair, 1);

    int64_t cost = w_twine_at(entry, 0).earth;
    Value node = w_twine_at(entry, 1);
    if (w_web_has(settled, node))
      continue;
    settled = w_web_put_owned(settled, node, W_LIGHT);

    Value out = w_call(step, &node, 1);
    size_t n = w_thread_len(out);
    for (size_t i = 0; i < n; i++) {
      Value edge = w_thread_at(out, i);
      Value next = w_twine_at(edge, 1);
      if (w_web_has(settled, next))
        continue;
      if (!w_web_has(from, next))
        from = w_web_put_owned(from, next, node);
      if (w_equal(next, goal)) {
        found = true;
        break;
      }
      Value ahead[2] = {w_earth(cost + w_twine_at(edge, 0).earth), next};
      frontier = w_taveren_push(frontier, w_twine_copy(ahead, 2));
    }
  }
  if (!found)
    return w_stilled();

  // Walk the predecessors back, then turn the path the right way round.
  Buf back = {0};
  Value at = goal;
  for (;;) {
    buf_push(&back, at);
    if (w_equal(at, start))
      break;
    Value prev = w_web_get(from, at);
    if (!w_is_held(prev))
      return w_stilled();
    at = w_hold_inner(prev);
  }
  Buf path = {0};
  for (size_t i = back.len; i > 0; i--)
    buf_push(&path, back.items[i - 1]);
  return w_held(buf_thread(&path));
}

// toposort orders nodes so that every edge points forwards, or answers Stilled
// when a cycle makes that impossible. Kahn's algorithm, with the in-degrees
// counted from the step function.
Value wp_toposort(Value step, Value nodes) {
  WThread *t = thr_of(nodes);

  Value degree = w_web_empty();
  degree.obj->rc = W_OWNED;
  for (size_t i = 0; i < t->len; i++)
    if (!w_web_has(degree, thr_at(t, i)))
      degree = w_web_put_owned(degree, thr_at(t, i), w_earth(0));
  for (size_t i = 0; i < t->len; i++) {
    Value one = thr_at(t, i);
    Value out = w_call(step, &one, 1);
    size_t n = w_thread_len(out);
    for (size_t j = 0; j < n; j++) {
      Value to = w_thread_at(out, j);
      Value have = w_web_get(degree, to);
      int64_t d = w_is_held(have) ? w_hold_inner(have).earth : 0;
      degree = w_web_put_owned(degree, to, w_earth(d + 1));
    }
  }

  Buf ready = {0};
  for (size_t i = 0; i < t->len; i++) {
    Value have = w_web_get(degree, thr_at(t, i));
    if (w_is_held(have) && w_hold_inner(have).earth == 0)
      buf_push(&ready, thr_at(t, i));
  }

  Buf order = {0};
  size_t head = 0;
  while (head < ready.len) {
    Value node = ready.items[head++];
    buf_push(&order, node);
    Value out = w_call(step, &node, 1);
    size_t n = w_thread_len(out);
    for (size_t j = 0; j < n; j++) {
      Value to = w_thread_at(out, j);
      Value have = w_web_get(degree, to);
      if (!w_is_held(have))
        continue;
      int64_t d = w_hold_inner(have).earth - 1;
      degree = w_web_put_owned(degree, to, w_earth(d));
      if (d == 0)
        buf_push(&ready, to);
    }
  }
  if (order.len != w_web_size(degree))
    return w_stilled(); // a cycle: something never reached in-degree zero
  return w_held(buf_thread(&order));
}

// clumps is reach from every node rather than from one. Each node ends up in
// exactly one group — the nodes that can get to one another — and the groups
// come back in the order their first member appears.
//
// A program that wants this writes `reach` in a fold with a seen-set around it,
// which is what the runtime does here anyway; writing it out by hand is where
// the same mistake keeps happening, since the seen-set has to survive between
// the seeds and not just within one of them.
//
// Whatever the step function reaches is part of the group whether or not the
// Thread mentions it. That is what makes the grid case work: the nodes are the
// cells worth starting from, and the edges say what belongs with them.
Value wp_clumps(Value step, Value nodes) {
  WThread *t = thr_of(nodes);
  Value seen = w_circle_empty();
  seen.obj->rc = W_OWNED;

  Buf out = {0};
  for (size_t i = 0; i < t->len; i++) {
    if (w_web_has(seen, thr_at(t, i)))
      continue;
    seen = w_web_put_owned(seen, thr_at(t, i), W_LIGHT);

    // Breadth first from this seed, the group doubling as the frontier: every
    // entry past `head` is still to be stepped from.
    Buf group = {0};
    buf_push(&group, thr_at(t, i));
    for (size_t head = 0; head < group.len; head++) {
      Value here = group.items[head]; // a copy: the push below may move the array
      Value step_out = w_call(step, &here, 1);
      size_t n = w_thread_len(step_out);
      for (size_t j = 0; j < n; j++) {
        Value node = w_thread_at(step_out, j);
        if (w_web_has(seen, node))
          continue;
        seen = w_web_put_owned(seen, node, W_LIGHT);
        buf_push(&group, node);
      }
    }
    buf_push(&out, buf_thread(&group));
  }
  return buf_thread(&out);
}

// settle applies until nothing changes, and answers what nothing changed.
//
// It is the shape `flow` cannot express. `flow` is endless and the test for
// having arrived is about two elements rather than one, so saying it as
// `flow f x through pairs through seek ((a, b) gives eq a b)` walks an endless
// Thread that is never consumed. One loop here says it and stops.
//
// A step function that never settles never returns, exactly as a recursion that
// never bottoms out never returns. Nothing here can tell the two apart: the
// caller who is not sure is the one who should be counting rounds anyway.
Value wp_settle(Value f, Value x) {
  for (;;) {
    Value next = w_call(f, &x, 1);
    if (w_equal(x, next))
      return next;
    x = next;
  }
}

Value wp_push(Value heap, Value v) { return w_taveren_push(heap, v); }
Value wp_pop(Value heap) { return w_taveren_pop(heap); }

// -------------------------------------------------------- pairwise & deeper

// zipwith walks two Threads together, which is the thing `zip` then `bend`
// over a tuple was standing in for. It stops at the shorter of the two.
// cull is sift's other half: it keeps what the test turns down. Writing
// `sift (x : not (odd x))` says the same thing and reads like an apology.
Value wp_cull(Value p, Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  for (size_t i = 0; i < t->len; i++)
    if (!call1(p, thr_at(t, i)).spirit)
      buf_push(&b, thr_at(t, i));
  return buf_thread(&b);
}

Value wp_zipwith(Value f, Value a, Value b) {
  WThread *x = thr_of(a), *y = thr_of(b);
  size_t n = x->len < y->len ? x->len : y->len;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  for (size_t i = 0; i < n; i++)
    out[i] = call2(f, thr_at(x, i), thr_at(y, i));
  return w_thread(out, n);
}

// bendr, siftr and zipr are the same three verbs one level further in, for a
// Thread of Threads — the shape a grid read as rows takes.
//
// rask's versions of these descend to whatever depth the data happens to have,
// which it can do because it is dynamically typed. Weave's types say how deep
// the data is, so these say exactly one level: `bendr f` is `bend (bend f)`,
// written once.
Value wp_bendr(Value f, Value xs) {
  WThread *t = thr_of(xs);
  Value *out = (Value *)w_alloc(sizeof(Value) * (t->len ? t->len : 1));
  for (size_t i = 0; i < t->len; i++)
    out[i] = wp_bend(f, thr_at(t, i));
  return w_thread(out, t->len);
}

Value wp_siftr(Value p, Value xs) {
  WThread *t = thr_of(xs);
  Value *out = (Value *)w_alloc(sizeof(Value) * (t->len ? t->len : 1));
  for (size_t i = 0; i < t->len; i++)
    out[i] = wp_sift(p, thr_at(t, i));
  return w_thread(out, t->len);
}

Value wp_zipr(Value f, Value a, Value b) {
  WThread *x = thr_of(a), *y = thr_of(b);
  size_t n = x->len < y->len ? x->len : y->len;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  for (size_t i = 0; i < n; i++)
    out[i] = wp_zipwith(f, thr_at(x, i), thr_at(y, i));
  return w_thread(out, n);
}

// sums and prods are the running totals: `sums [1 2 3]` is `[1 3 6]`. A prefix
// scan turns up in Advent of Code as often as the total does — the first point
// a cumulative frequency repeats, the first depth that goes negative.
static Value running(Value xs, bool product) {
  WThread *t = thr_of(xs);
  Value *out = (Value *)w_alloc(sizeof(Value) * (t->len ? t->len : 1));
  Value acc = w_earth(0);
  for (size_t i = 0; i < t->len; i++) {
    acc = i == 0 ? thr_at(t, i)
                 : (product ? wp_mul(acc, thr_at(t, i)) : wp_add(acc, thr_at(t, i)));
    out[i] = acc;
  }
  return w_thread(out, t->len);
}

Value wp_sums(Value xs) { return running(xs, false); }
Value wp_prods(Value xs) { return running(xs, true); }

// --------------------------------------------------- extra sequence & text

// numbers pulls every integer out of some text, sign included. Most Advent of
// Code inputs are a shape wrapped around a handful of numbers, so this saves
// writing a parser per day.
// spans reads the inclusive ranges out of some text: a run of digits, a dash,
// and another run of digits. It exists because `earths` cannot do this and
// should not try — `earths` has to read `x=-5` as a negative number, and the
// same dash between two digits is a separator. Rather than guess from context,
// the two readings get a verb each.
Value wp_spans(Value text) {
  WAir *a = air_of(text);
  Buf b = {0};
  size_t i = 0;
  while (i < a->len) {
    if (!isdigit((unsigned char)a->bytes[i])) {
      i++;
      continue;
    }
    int64_t lo = 0;
    while (i < a->len && isdigit((unsigned char)a->bytes[i]))
      lo = lo * 10 + (a->bytes[i++] - '0');
    // A dash straight after the digits, with digits straight after it, is what
    // makes this a range rather than a number that happens to be followed by a
    // subtraction.
    if (i + 1 >= a->len || a->bytes[i] != '-' ||
        !isdigit((unsigned char)a->bytes[i + 1]))
      continue;
    i++;
    int64_t hi = 0;
    while (i < a->len && isdigit((unsigned char)a->bytes[i]))
      hi = hi * 10 + (a->bytes[i++] - '0');
    Value pair[2] = {w_earth(lo), w_earth(hi)};
    buf_push(&b, w_twine_copy(pair, 2));
  }
  return buf_thread(&b);
}

// earths is the first verb to build a packed Thread, and it is the one that
// matters most: `Source | lines | glean earth` — or `earths` outright — is how
// nearly every Advent of Code program starts, and what it produces is millions
// of elements that all say W_EARTH.
Value wp_earths(Value text) {
  WAir *a = air_of(text);
  EBuf b = {0};
  size_t i = 0;
  while (i < a->len) {
    bool neg = false;
    size_t start = i;
    if ((a->bytes[i] == '-' || a->bytes[i] == '+') && i + 1 < a->len &&
        isdigit((unsigned char)a->bytes[i + 1])) {
      neg = a->bytes[i] == '-';
      i++;
    }
    if (!isdigit((unsigned char)a->bytes[i])) {
      i = start + 1;
      continue;
    }
    int64_t n = 0;
    while (i < a->len && isdigit((unsigned char)a->bytes[i])) {
      n = n * 10 + (a->bytes[i] - '0');
      i++;
    }
    ebuf_push(&b, neg ? -n : n);
  }
  return ebuf_thread(&b, W_EARTH);
}

// waters is the same sweep for Waters. A run of digits with no point is still
// a number, so `waters "1 2.5"` is `[1.0 2.5]` — the point is optional here in
// the way it is not in a literal, because this reads input rather than source.
Value wp_waters(Value text) {
  WAir *a = air_of(text);
  Buf b = {0};
  size_t i = 0;
  while (i < a->len) {
    size_t start = i;
    if (a->bytes[i] == '-' || a->bytes[i] == '+')
      i++;
    size_t digits = i;
    while (i < a->len && isdigit((unsigned char)a->bytes[i]))
      i++;
    if (i == digits) {
      i = start + 1;
      continue;
    }
    if (i < a->len && a->bytes[i] == '.') {
      i++;
      while (i < a->len && isdigit((unsigned char)a->bytes[i]))
        i++;
    }
    // An exponent counts only when it is complete: the `e` of `2e` is the
    // start of a word, not a malformed number, so the sweep stops before it.
    if (i < a->len && (a->bytes[i] == 'e' || a->bytes[i] == 'E')) {
      size_t exp = i + 1;
      if (exp < a->len && (a->bytes[exp] == '-' || a->bytes[exp] == '+'))
        exp++;
      size_t expDigits = exp;
      while (exp < a->len && isdigit((unsigned char)a->bytes[exp]))
        exp++;
      if (exp > expDigits)
        i = exp;
    }
    char tmp[64];
    size_t n = i - start;
    if (n >= sizeof(tmp))
      continue;
    memcpy(tmp, a->bytes + start, n);
    tmp[n] = 0;
    buf_push(&b, w_water(strtod(tmp, NULL)));
  }
  return buf_thread(&b);
}

Value wp_chunk(Value n, Value xs) {
  WThread *t = thr_of(xs);
  int64_t k = n.earth;
  if (k <= 0)
    return w_thread_empty();
  Buf b = {0};
  for (size_t i = 0; i < t->len; i += (size_t)k) {
    size_t len = t->len - i < (size_t)k ? t->len - i : (size_t)k;
    buf_push(&b, thr_window(t, i, len));
  }
  return buf_thread(&b);
}

Value wp_windows(Value n, Value xs) {
  WThread *t = thr_of(xs);
  int64_t k = n.earth;
  if (k <= 0 || (size_t)k > t->len)
    return w_thread_empty();
  Buf b = {0};
  for (size_t i = 0; i + (size_t)k <= t->len; i++)
    buf_push(&b, thr_window(t, i, (size_t)k));
  return buf_thread(&b);
}

Value wp_pivot(Value xss) {
  WThread *outer = thr_of(xss);
  size_t cols = 0;
  for (size_t i = 0; i < outer->len; i++) {
    size_t n = w_thread_len(thr_at(outer, i));
    if (n > cols)
      cols = n;
  }
  Buf out = {0};
  for (size_t c = 0; c < cols; c++) {
    Buf row = {0};
    for (size_t r = 0; r < outer->len; r++) {
      WThread *line = thr_of(thr_at(outer, r));
      if (c < line->len)
        buf_push(&row, thr_at(line, c));
    }
    buf_push(&out, buf_thread(&row));
  }
  return buf_thread(&out);
}

Value wp_gcd(Value a, Value b) {
  int64_t x = a.earth < 0 ? -a.earth : a.earth;
  int64_t y = b.earth < 0 ? -b.earth : b.earth;
  while (y) {
    int64_t t = x % y;
    x = y;
    y = t;
  }
  return w_earth(x);
}

Value wp_lcm(Value a, Value b) {
  int64_t g = wp_gcd(a, b).earth;
  if (g == 0)
    return w_earth(0);
  int64_t x = a.earth / g * b.earth;
  return w_earth(x < 0 ? -x : x);
}

// sortBy orders by a derived key, which is how most Advent of Code sorting is
// actually phrased. The key is worked out once per element and then never
// again: that is the whole reason the verb exists rather than a comparison
// function, and it is what lets the sort below never call back into the
// program.
typedef struct {
  Value key;
  Value item;
} Keyed;

#define W_KEYED_BEFORE(x, y) (w_compare((x).key, (y).key) < 0)
W_MERGE_SORT(sort_keyed, Keyed, W_KEYED_BEFORE)

// An Earth key — a distance, a count, a position — is nearly every key anyone
// sorts by, and it needs neither the tag nor the second Value: twenty-four
// bytes an element instead of thirty-two, and a comparison that is a load and
// a branch.
typedef struct {
  int64_t ord;
  Value item;
} Ordered;

#define W_ORDERED_BEFORE(x, y) ((x).ord < (y).ord)
W_MERGE_SORT(sort_ordered, Ordered, W_ORDERED_BEFORE)

#define W_ORDERED_RANK(x) w_rank((x).ord)
W_RADIX_SORT(radix_ordered, Ordered, W_ORDERED_RANK)

Value wp_sortby(Value key, Value xs) {
  WThread *t = thr_of(xs);
  size_t n = t->len;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  if (n < 2) {
    // One element or none, so a copy rather than the array: unpacking a Thread
    // in order to leave it in the order it was already in would be the worst
    // possible reason to do it.
    for (size_t i = 0; i < n; i++)
      out[i] = thr_at(t, i);
    return w_thread(out, n);
  }

  // The keys go straight into the unboxed form, because that is what nearly
  // every one of them turns out to be. A key that is not an Earth sends the
  // whole thing down the general path, which works the keys out again: they are
  // pure, so asking twice is only slower, and it keeps the common case to two
  // arrays rather than three.
  Ordered *a = (Ordered *)w_alloc(sizeof(Ordered) * n);
  bool earths = true;
  for (size_t i = 0; i < n; i++) {
    Value k = call1(key, thr_at(t, i));
    earths = earths && k.tag == W_EARTH;
    a[i].ord = k.earth;
    a[i].item = thr_at(t, i);
  }

  if (earths) {
    Ordered *tmp = (Ordered *)w_alloc(sizeof(Ordered) * n);
    if (n >= W_RADIX_MIN)
      radix_ordered(a, tmp, n);
    else
      sort_ordered(a, tmp, n);
    for (size_t i = 0; i < n; i++)
      out[i] = a[i].item;
    w_free(tmp, sizeof(Ordered) * n);
    w_free(a, sizeof(Ordered) * n);
    return w_thread(out, n);
  }

  w_free(a, sizeof(Ordered) * n);
  Keyed *keyed = (Keyed *)w_alloc(sizeof(Keyed) * n);
  Keyed *tmp = (Keyed *)w_alloc(sizeof(Keyed) * n);
  for (size_t i = 0; i < n; i++) {
    keyed[i].key = call1(key, thr_at(t, i));
    keyed[i].item = thr_at(t, i);
  }
  sort_keyed(keyed, tmp, n);
  for (size_t i = 0; i < n; i++)
    out[i] = keyed[i].item;
  w_free(tmp, sizeof(Keyed) * n);
  w_free(keyed, sizeof(Keyed) * n);
  return w_thread(out, n);
}

// groupBy collects elements sharing a derived key into a Web of Threads.
Value wp_group(Value key, Value xs) {
  WThread *t = thr_of(xs);
  Value web = w_web_empty();
  for (size_t i = 0; i < t->len; i++) {
    Value k = call1(key, thr_at(t, i));
    Value seen = w_web_get(web, k);
    Buf b = {0};
    if (w_is_held(seen)) {
      WThread *prev = thr_of(w_hold_inner(seen));
      for (size_t j = 0; j < prev->len; j++)
        buf_push(&b, thr_at(prev, j));
    }
    buf_push(&b, thr_at(t, i));
    web = w_web_put_owned(web, k, buf_thread(&b));
  }
  return sealed(web);
}

Value wp_idx(Value needle, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (w_equal(thr_at(t, i), needle))
      return w_held(w_earth((int64_t)i));
  return w_stilled();
}

// nth is how an element is taken out of a Thread by position. A Thread is a
// strict array, so this is one load — but it is out of bounds for most indices
// most of the time, so it answers with a Hold rather than a value.
Value wp_nth(Value n, Value xs) {
  if (xs.tag == W_AIR) {
    WAir *a = air_of(xs);
    if (n.earth < 0)
      return w_stilled();
    size_t at = air_at(a, n.earth);
    // air_at answers the whole length when the text runs out first, which is
    // exactly the position that has no rune at it.
    if (at >= a->len)
      return w_stilled();
    return w_held(w_fire(air_rune(a, at)));
  }
  WThread *t = thr_of(xs);
  if (n.earth < 0 || (size_t)n.earth >= t->len)
    return w_stilled();
  return w_held(thr_at(t, n.earth));
}

Value wp_has(Value needle, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (w_equal(thr_at(t, i), needle))
      return W_LIGHT;
  return W_SHADOW;
}

// glean is bend and compact in one pass: the elements a function could turn
// into something, with the ones it could not dropped. It is what converting a
// Thread of text into a Thread of Earths looks like — `lines | glean earth`.
Value wp_glean(Value f, Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  for (size_t i = 0; i < t->len; i++) {
    Value h = call1(f, thr_at(t, i));
    if (w_is_held(h))
      buf_push(&b, w_hold_inner(h));
  }
  return buf_thread(&b);
}

// harvest is glean when a failure means the input was wrong rather than
// uninteresting: everything converted, or the first element that would not.
//
// `lines Source | glean earth` quietly skips a malformed line, which is right
// for scraping numbers out of prose and wrong for reading a file that is
// supposed to be all numbers. This one says which line.
Value wp_harvest(Value f, Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  for (size_t i = 0; i < t->len; i++) {
    Value h = call1(f, thr_at(t, i));
    if (!w_is_held(h)) {
      Value one = thr_at(t, i);
      return w_data("Gentled", 1, &one, 1);
    }
    buf_push(&b, w_hold_inner(h));
  }
  Value woven = buf_thread(&b);
  return w_data("Woven", 0, &woven, 1);
}

// perms is every ordering of a Thread, in lexicographic order of position, by
// the usual swap-and-recurse. It is only ever called on something small — the
// count is n!, which passes ten million at eleven elements.
static void perms_into(Buf *out, Value *work, size_t n, size_t k) {
  if (k == n) {
    buf_push(out, w_thread_copy(work, n));
    return;
  }
  for (size_t i = k; i < n; i++) {
    Value t = work[k];
    work[k] = work[i];
    work[i] = t;
    perms_into(out, work, n, k + 1);
    t = work[k];
    work[k] = work[i];
    work[i] = t;
  }
}

Value wp_perms(Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  if (t->len == 0) {
    buf_push(&b, w_thread_empty());
    return buf_thread(&b);
  }
  Value *work = (Value *)w_alloc(sizeof(Value) * t->len);
  for (size_t i = 0; i < t->len; i++)
    work[i] = thr_at(t, i);
  perms_into(&b, work, t->len, 0);
  return buf_thread(&b);
}

Value wp_contains(Value needle, Value text) {
  WAir *n = air_of(needle), *h = air_of(text);
  if (n->len == 0)
    return W_LIGHT;
  if (n->len > h->len)
    return W_SHADOW;
  for (size_t i = 0; i + n->len <= h->len; i++)
    if (memcmp(h->bytes + i, n->bytes, n->len) == 0)
      return W_LIGHT;
  return W_SHADOW;
}

// ------------------------------------------------- more sequence verbs


Value wp_second(Value xs) {
  WThread *t = thr_of(xs);
  return t->len > 1 ? w_held(thr_at(t, 1)) : w_stilled();
}

Value wp_none(Value p, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (call1(p, thr_at(t, i)).spirit)
      return W_SHADOW;
  return W_LIGHT;
}

// enum pairs each element with its position, which is how most indexed loops
// are written without an index.
Value wp_enum(Value xs) {
  WThread *t = thr_of(xs);
  Value *out = (Value *)w_alloc(sizeof(Value) * (t->len ? t->len : 1));
  for (size_t i = 0; i < t->len; i++) {
    Value *pair = (Value *)w_alloc(sizeof(Value) * 2);
    pair[0] = w_earth((int64_t)i);
    pair[1] = thr_at(t, i);
    out[i] = w_twine(pair, 2);
  }
  return w_thread(out, t->len);
}

// scan is braid keeping every intermediate, for running totals.
// scan is braid keeping every running total: one total per element, so it is
// a bend with a memory and composes as a pipeline stage does. The seed is
// where the running starts, not a total of its own, which is what makes
// `sums` exactly `scan add 0`.
Value wp_scan(Value f, Value seed, Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  Value acc = seed;
  for (size_t i = 0; i < t->len; i++) {
    acc = call2(f, acc, thr_at(t, i));
    buf_push(&b, acc);
  }
  return buf_thread(&b);
}

// priors is scan with the starting value kept, so the result is one longer
// than the Thread. That is the shape a prefix sum wants: with the seed in
// front, the total over positions i..j is one subtraction, and the empty range
// needs no special case. `scan` leaves it out because running totals — `sums`,
// `prods` — want one per element.
Value wp_priors(Value f, Value seed, Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  Value acc = seed;
  buf_push(&b, acc);
  for (size_t i = 0; i < t->len; i++) {
    acc = call2(f, acc, thr_at(t, i));
    buf_push(&b, acc);
  }
  return buf_thread(&b);
}

// dupe is the first element that has been seen before. It is a `seek` with a
// memory: the test is not about the element on its own, so no predicate can
// express it, and every Advent of Code cycle-detection puzzle wants it.
// dupe answers where the first repeat is, where that value was seen before,
// and what it is.
//
// The second position is free. The seen-set was a Circle, and a Circle is a Web
// whose values are all `Light`: the flat table's cell is two int64s either way,
// so the slot holding that constant can hold the position instead at no cost in
// memory and no extra probe. What it buys is the length of the cycle, which is
// what a program that detects one is nearly always after and which used to cost
// a second pass over a Thread this verb exists to walk once.
Value wp_dupe(Value xs) {
  WThread *t = thr_of(xs);
  Value seen = w_web_empty();
  for (size_t i = 0; i < t->len; i++) {
    Value x = thr_at(t, i), first;
    if (w_web_find(seen, x, &first)) {
      Value found[3] = {w_earth((int64_t)i), first, x};
      return w_held(w_twine_copy(found, 3));
    }
    seen = w_web_put_owned(seen, x, w_earth((int64_t)i));
  }
  return w_stilled();
}

// gentle is a braid that can stop. The step answers `Woven acc` to carry on
// or `Gentled answer` to end the fold there and then, and the fold answers
// whichever it ended on — `Woven` with the final accumulator if the Thread
// simply ran out.
Value wp_gentle(Value f, Value seed, Value xs) {
  WThread *t = thr_of(xs);
  Value acc = seed;
  for (size_t i = 0; i < t->len; i++) {
    Value step = call2(f, acc, thr_at(t, i));
    if (w_data_index(step) != 0)
      return step;
    acc = w_data_field(step, 0);
  }
  return w_data("Woven", 0, &acc, 1);
}

Value wp_top(Value n, Value xs) {
  Value sorted = wp_rev(wp_sort(xs));
  return wp_take(n, sorted);
}

Value wp_bot(Value n, Value xs) { return wp_take(n, wp_sort(xs)); }

// pairs walks adjacent elements, which is what most "compare each line with
// the next" puzzles want.
Value wp_pairs(Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  for (size_t i = 0; i + 1 < t->len; i++) {
    Value *pair = (Value *)w_alloc(sizeof(Value) * 2);
    pair[0] = thr_at(t, i);
    pair[1] = thr_at(t, i + 1);
    buf_push(&b, w_twine(pair, 2));
  }
  return buf_thread(&b);
}

// couples is every element with every element after it — the all-pairs both of
// Advent of Code 2025's two slowest programs wrote out longhand as a `span`
// inside a `mapcat`, because `combos 2` answers Threads where `pairs` and
// `cross` answer Twines and neither reached across the difference.
//
// A Twine is what the callers want: it destructures, and `former`/`latter`
// reach into it.
Value wp_couples(Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  for (size_t i = 0; i + 1 < t->len; i++)
    for (size_t j = i + 1; j < t->len; j++) {
      Value pair[2] = {thr_at(t, i), thr_at(t, j)};
      buf_push(&b, w_twine_copy(pair, 2));
    }
  return buf_thread(&b);
}

// cross is the cartesian product of two Threads.
Value wp_cross(Value as, Value bs) {
  WThread *x = thr_of(as), *y = thr_of(bs);
  Buf b = {0};
  for (size_t i = 0; i < x->len; i++)
    for (size_t j = 0; j < y->len; j++) {
      Value *pair = (Value *)w_alloc(sizeof(Value) * 2);
      pair[0] = thr_at(x, i);
      pair[1] = thr_at(y, j);
      buf_push(&b, w_twine(pair, 2));
    }
  return buf_thread(&b);
}

// combos yields every n-element combination, in order.
static void combos_into(Buf *out, WThread *t, size_t n, size_t start, Buf *cur) {
  if (cur->len == n) {
    buf_push(out, w_thread_copy(cur->items, cur->len));
    return;
  }
  for (size_t i = start; i < t->len; i++) {
    Buf next = {0};
    for (size_t j = 0; j < cur->len; j++)
      buf_push(&next, cur->items[j]);
    buf_push(&next, thr_at(t, i));
    combos_into(out, t, n, i + 1, &next);
  }
}

Value wp_combos(Value n, Value xs) {
  WThread *t = thr_of(xs);
  int64_t k = n.earth;
  Buf out = {0};
  if (k < 0 || (size_t)k > t->len)
    return buf_thread(&out);
  Buf cur = {0};
  combos_into(&out, t, (size_t)k, 0, &cur);
  return buf_thread(&out);
}

// compact drops the Stilled entries and unwraps the rest, which is the usual
// shape after parsing a column of text.
Value wp_compact(Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  for (size_t i = 0; i < t->len; i++)
    if (w_is_held(thr_at(t, i)))
      buf_push(&b, w_hold_inner(thr_at(t, i)));
  return buf_thread(&b);
}

Value wp_takewhile(Value p, Value xs) {
  WThread *t = thr_of(xs);
  size_t n = 0;
  while (n < t->len && call1(p, thr_at(t, n)).spirit)
    n++;
  return thr_window(t, 0, n);
}

Value wp_dropwhile(Value p, Value xs) {
  WThread *t = thr_of(xs);
  size_t n = 0;
  while (n < t->len && call1(p, thr_at(t, n)).spirit)
    n++;
  return thr_window(t, n, t->len - n);
}

// mapcat maps then flattens, for a step that yields several results.
Value wp_mapcat(Value f, Value xs) { return wp_flat(wp_bend(f, xs)); }

// col takes one column out of a Thread of Threads.
static Value extreme_by(Value key, Value xs, int want) {
  WThread *t = thr_of(xs);
  if (t->len == 0)
    return w_stilled();
  Value best = thr_at(t, 0), bestKey = call1(key, best);
  for (size_t i = 1; i < t->len; i++) {
    Value k = call1(key, thr_at(t, i));
    if (w_compare(k, bestKey) * want > 0) {
      best = thr_at(t, i);
      bestKey = k;
    }
  }
  return w_held(best);
}

Value wp_maxby(Value key, Value xs) { return extreme_by(key, xs, 1); }
Value wp_minby(Value key, Value xs) { return extreme_by(key, xs, -1); }

// high and low are maxby and minby with the element as its own key, which is
// what anyone means nine times in ten. wantIdx asks for the position instead
// of the element: `maxby` then `idx` is two passes and answers the wrong one
// when the value repeats.
static Value extreme(Value xs, int want, bool wantIdx) {
  WThread *t = thr_of(xs);
  if (t->len == 0)
    return w_stilled();
  size_t at = 0;
  for (size_t i = 1; i < t->len; i++)
    if (w_compare(thr_at(t, i), thr_at(t, at)) * want > 0)
      at = i;
  return w_held(wantIdx ? w_earth((int64_t)at) : thr_at(t, at));
}

Value wp_high(Value xs) { return extreme(xs, 1, false); }
Value wp_low(Value xs) { return extreme(xs, -1, false); }
Value wp_highidx(Value xs) { return extreme(xs, 1, true); }
Value wp_lowidx(Value xs) { return extreme(xs, -1, true); }

// seekidx answers where `seek` would have found something. The test alone
// cannot say where it matched, and hunting for the value afterwards needs Eq
// and finds the wrong one when it repeats.
Value wp_seekidx(Value p, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (call1(p, thr_at(t, i)).spirit)
      return w_held(w_earth((int64_t)i));
  return w_stilled();
}

// siftidx and idxs are the all-of-them twins of seekidx and idx. `seek` and
// `sift` are the only pair the language had both halves of; asking where
// rather than what left only the first half, and `enum` then `sift` then
// taking the former of each pair is three passes and a Twine per element for
// something one loop answers.
Value wp_siftidx(Value p, Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  for (size_t i = 0; i < t->len; i++)
    if (call1(p, thr_at(t, i)).spirit)
      buf_push(&b, w_earth((int64_t)i));
  return buf_thread(&b);
}

Value wp_idxs(Value v, Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  for (size_t i = 0; i < t->len; i++)
    if (w_equal(v, thr_at(t, i)))
      buf_push(&b, w_earth((int64_t)i));
  return buf_thread(&b);
}

// twist is mend for a value that depends on what is already there, which is
// most of them: `twist i inc counts` rather than reading, adding and writing
// back.
Value wp_twist(Value at, Value f, Value xs) {
  WThread *t = thr_of(xs);
  if (at.earth < 0 || (size_t)at.earth >= t->len)
    return xs;
  return wp_mend(at, call1(f, thr_at(t, at.earth)), xs);
}

// wp_twist_owned is twist on a Thread the compiler has proved is not shared;
// see wp_mend_owned.
Value wp_twist_owned(Value at, Value f, Value xs) {
  WThread *t = thr_of(xs);
  if (at.earth < 0 || (size_t)at.earth >= t->len)
    return xs;
  return wp_mend_owned(at, call1(f, thr_at(t, at.earth)), xs);
}

// ------------------------------------------------------------------ ranges
//
// A range is a Twine, inclusive at both ends, because that is how every input
// carrying one writes it: `2-4`, `6-8`. Nothing here is Earth-only except
// `width`, so a range of Waters — or of Fires, which is a range of letters —
// works the same way.

static Value range_lo(Value r) { return w_twine_at(r, 0); }
static Value range_hi(Value r) { return w_twine_at(r, 1); }

static Value make_range(Value lo, Value hi) {
  Value pair[2] = {lo, hi};
  return w_twine_copy(pair, 2);
}

static Value larger(Value a, Value b) { return w_compare(a, b) >= 0 ? a : b; }
static Value smaller(Value a, Value b) { return w_compare(a, b) <= 0 ? a : b; }

// Two inclusive ranges meet when neither ends before the other begins.
Value wp_overlaps(Value a, Value b) {
  return w_spirit(w_compare(range_lo(a), range_hi(b)) <= 0 &&
                  w_compare(range_lo(b), range_hi(a)) <= 0);
}

// The shared range, or Stilled — which is the whole reason both verbs exist,
// since a Twine cannot say "empty" and `overlaps` cannot say "how much".
Value wp_overlapping(Value a, Value b) {
  Value lo = larger(range_lo(a), range_lo(b));
  Value hi = smaller(range_hi(a), range_hi(b));
  if (w_compare(lo, hi) > 0)
    return w_stilled();
  return w_held(make_range(lo, hi));
}

Value wp_within(Value outer, Value inner) {
  return w_spirit(w_compare(range_lo(outer), range_lo(inner)) <= 0 &&
                  w_compare(range_hi(inner), range_hi(outer)) <= 0);
}

// The smallest range holding both. Gaps included: two ranges that do not meet
// still have one range around them, which is what a bounding box is.
Value wp_spanning(Value a, Value b) {
  return make_range(smaller(range_lo(a), range_lo(b)),
                    larger(range_hi(a), range_hi(b)));
}

Value wp_holding(Value r, Value v) {
  return w_spirit(w_compare(range_lo(r), v) <= 0 &&
                  w_compare(v, range_hi(r)) <= 0);
}

Value wp_width(Value r) {
  int64_t lo = range_lo(r).earth, hi = range_hi(r).earth;
  return w_earth(hi < lo ? 0 : hi - lo + 1);
}

// mesh is what the other range verbs could not say: they all take two ranges,
// and the thing every puzzle actually has is a Thread of them that overlap.
//
// Sorted by where they start, a range either reaches past the one being built —
// and extends it — or begins a new one. Ranges that merely touch are joined:
// these are Earths, so 1-3 and 4-6 leave nothing between them, and a caller who
// wanted them kept apart would not have asked for them merged.
// A range that ends before it begins holds nothing — `width` already says so —
// and is dropped rather than merged, which would spread it over its neighbours.
static int range_cmp(const void *a, const void *b) {
  int64_t la = range_lo(*(const Value *)a).earth;
  int64_t lb = range_lo(*(const Value *)b).earth;
  return la < lb ? -1 : la > lb ? 1 : 0;
}

Value wp_mesh(Value ranges) {
  WThread *t = thr_of(ranges);

  Buf live = {0};
  for (size_t i = 0; i < t->len; i++)
    if (range_lo(thr_at(t, i)).earth <= range_hi(thr_at(t, i)).earth)
      buf_push(&live, thr_at(t, i));
  if (live.len == 0)
    return buf_thread(&live);
  qsort(live.items, live.len, sizeof(Value), range_cmp);

  Buf out = {0};
  int64_t lo = range_lo(live.items[0]).earth;
  int64_t hi = range_hi(live.items[0]).earth;
  for (size_t i = 1; i < live.len; i++) {
    int64_t l = range_lo(live.items[i]).earth;
    int64_t h = range_hi(live.items[i]).earth;
    if (l > hi + 1) {
      buf_push(&out, make_range(w_earth(lo), w_earth(hi)));
      lo = l;
      hi = h;
    } else if (h > hi) {
      hi = h;
    }
  }
  buf_push(&out, make_range(w_earth(lo), w_earth(hi)));
  return buf_thread(&out);
}

// --------------------------------------------------------- more text verbs

// blocks splits on blank lines, which is how paragraph-shaped inputs arrive.
Value wp_blocks(Value text) {
  WAir *a = air_of(text);
  Buf b = {0};
  size_t start = 0;
  for (size_t i = 0; i + 1 < a->len; i++) {
    if (a->bytes[i] == '\n' && a->bytes[i + 1] == '\n') {
      buf_push(&b, w_air(a->bytes + start, i - start));
      i++;
      while (i + 1 < a->len && a->bytes[i + 1] == '\n')
        i++;
      start = i + 1;
    }
  }
  size_t end = a->len;
  while (end > start && a->bytes[end - 1] == '\n')
    end--;
  if (end > start)
    buf_push(&b, w_air(a->bytes + start, end - start));
  return buf_thread(&b);
}

static Value map_case(Value text, int upper) {
  WAir *a = air_of(text);
  char *out = (char *)w_alloc(a->len ? a->len : 1);
  for (size_t i = 0; i < a->len; i++) {
    unsigned char c = (unsigned char)a->bytes[i];
    out[i] = (char)(upper ? toupper(c) : tolower(c));
  }
  return w_air(out, a->len);
}

Value wp_upper(Value text) { return map_case(text, 1); }
Value wp_lower(Value text) { return map_case(text, 0); }

static Value pad(Value width, Value fill, Value text, int left) {
  WAir *a = air_of(text);
  int64_t want = width.earth;
  if (want <= 0 || (size_t)want <= a->len)
    return text;
  size_t extra = (size_t)want - a->len;
  char *out = (char *)w_alloc((size_t)want);
  if (left) {
    memset(out, (int)fill.fire, extra);
    memcpy(out + extra, a->bytes, a->len);
  } else {
    memcpy(out, a->bytes, a->len);
    memset(out + a->len, (int)fill.fire, extra);
  }
  return w_air(out, (size_t)want);
}

Value wp_padl(Value width, Value fill, Value text) { return pad(width, fill, text, 1); }
Value wp_padr(Value width, Value fill, Value text) { return pad(width, fill, text, 0); }

Value wp_starts(Value prefix, Value text) {
  WAir *p = air_of(prefix), *t = air_of(text);
  return w_spirit(p->len <= t->len && memcmp(t->bytes, p->bytes, p->len) == 0);
}

Value wp_ends(Value suffix, Value text) {
  WAir *s = air_of(suffix), *t = air_of(text);
  return w_spirit(s->len <= t->len &&
                  memcmp(t->bytes + t->len - s->len, s->bytes, s->len) == 0);
}

Value wp_cutstart(Value prefix, Value text) {
  if (!wp_starts(prefix, text).spirit)
    return text;
  WAir *p = air_of(prefix), *t = air_of(text);
  return w_air(t->bytes + p->len, t->len - p->len);
}

Value wp_cutend(Value suffix, Value text) {
  if (!wp_ends(suffix, text).spirit)
    return text;
  WAir *s = air_of(suffix), *t = air_of(text);
  return w_air(t->bytes, t->len - s->len);
}

Value wp_replace(Value needle, Value with, Value text) {
  WAir *n = air_of(needle), *w = air_of(with), *t = air_of(text);
  if (n->len == 0)
    return text;
  Buf parts = {0};
  size_t start = 0;
  for (size_t i = 0; i + n->len <= t->len;) {
    if (memcmp(t->bytes + i, n->bytes, n->len) == 0) {
      buf_push(&parts, w_air(t->bytes + start, i - start));
      buf_push(&parts, w_air(w->bytes, w->len));
      i += n->len;
      start = i;
    } else {
      i++;
    }
  }
  buf_push(&parts, w_air(t->bytes + start, t->len - start));
  return wp_join(w_air("", 0), buf_thread(&parts));
}

// delve takes a line apart against a shape.
//
// `{}` stands for a run to keep and everything else has to match exactly, so
// `delve "Game {}: {}" line` answers `Held ["11", "3 blue, 4 red"]` — or
// `Stilled`, because a line that does not have the shape you said is a thing
// worth noticing rather than a thing to guess at.
//
// A run stops at the first place the text after it appears, and the shape has
// to account for the whole line: a trailing `{}` is how you say "and the rest".
// That leaves one thing unsayable, a literal `{}` in the input, which no Advent
// of Code line has ever contained, and it buys a shape with nothing to escape.

// hole reports whether the shape has `{}` at i.
static bool hole_at(const char *p, size_t n, size_t i) {
  return i + 1 < n && p[i] == '{' && p[i + 1] == '}';
}

// find_from is the first place needle appears in text at or after from.
static bool find_from(const char *s, size_t sn, size_t from, const char *needle,
                      size_t nn, size_t *at) {
  if (nn == 0) {
    *at = from;
    return true;
  }
  for (size_t i = from; i + nn <= sn; i++) {
    if (memcmp(s + i, needle, nn) == 0) {
      *at = i;
      return true;
    }
  }
  return false;
}

Value wp_delve(Value shape, Value text) {
  WAir *sh = air_of(shape), *t = air_of(text);
  const char *p = sh->bytes, *s = t->bytes;
  size_t pn = sh->len, sn = t->len;

  Buf out = {0};
  size_t pi = 0, si = 0, start = 0;
  bool waiting = false; // a run has begun and is looking for its end

  while (pi < pn) {
    if (hole_at(p, pn, pi)) {
      // Two runs with nothing between them: the first can only be empty.
      if (waiting)
        buf_push(&out, w_air(s + start, 0));
      waiting = true;
      start = si;
      pi += 2;
      continue;
    }

    size_t lit = pi;
    while (pi < pn && !hole_at(p, pn, pi))
      pi++;
    size_t litn = pi - lit;

    if (!waiting) {
      // No run is open, so this has to match where we stand.
      if (sn - si < litn || memcmp(s + si, p + lit, litn) != 0)
        return w_stilled();
      si += litn;
      continue;
    }

    size_t at;
    if (pi == pn) {
      // The shape ends here, so this literal has to be the end of the line
      // too, and the run is everything before it.
      if (sn - si < litn || memcmp(s + sn - litn, p + lit, litn) != 0)
        return w_stilled();
      at = sn - litn;
    } else if (!find_from(s, sn, si, p + lit, litn, &at)) {
      return w_stilled();
    }
    buf_push(&out, w_air(s + start, at - start));
    si = at + litn;
    waiting = false;
  }

  if (waiting) {
    buf_push(&out, w_air(s + start, sn - start));
    si = sn;
  }
  if (si != sn)
    return w_stilled(); // the shape ran out before the line did
  return w_held(buf_thread(&out));
}

Value wp_ord(Value c) { return w_earth((int64_t)c.fire); }
Value wp_spark(Value n) { return w_fire((uint32_t)n.earth); }

// digit is the value of a decimal Fire, which is what a grid of numbers or a
// run of digits needs. `ord c | sub _ 48` is the same thing written out, and
// wrong for everything that is not a digit.
Value wp_digit(Value c) {
  if (c.fire < '0' || c.fire > '9')
    return w_stilled();
  return w_held(w_earth((int64_t)(c.fire - '0')));
}

// repeat lays a Thread or some text end to end n times. `copies` is the other
// question — n of the same *element* — and `copies n xs | flat` was how this
// had to be written for a Thread, which builds the outer Thread only to throw
// it away.
Value wp_repeat(Value n, Value xs) {
  int64_t k = n.earth < 0 ? 0 : n.earth;
  if (xs.tag == W_AIR) {
    WAir *a = air_of(xs);
    size_t total = a->len * (size_t)k;
    char *out = (char *)w_alloc(total ? total : 1);
    for (int64_t i = 0; i < k; i++)
      memcpy(out + a->len * (size_t)i, a->bytes, a->len);
    return w_air(out, total);
  }
  WThread *t = thr_of(xs);
  size_t total = t->len * (size_t)k;
  Value *out = (Value *)w_alloc(sizeof(Value) * (total ? total : 1));
  Value *elems = thr_boxed(t);
  for (int64_t i = 0; i < k; i++)
    memcpy(out + t->len * (size_t)i, elems, sizeof(Value) * t->len);
  return w_thread(out, total);
}

// -------------------------------------------------------- more number verbs

Value wp_sign(Value n) {
  if (n.tag == W_WATER)
    return w_earth(n.water < 0 ? -1 : (n.water > 0 ? 1 : 0));
  return w_earth(n.earth < 0 ? -1 : (n.earth > 0 ? 1 : 0));
}

Value wp_sqrt(Value n) { return w_water(sqrt(as_d(n))); }
Value wp_cbrt(Value n) { return w_water(cbrt(as_d(n))); }
Value wp_ceil(Value n) { return w_earth((int64_t)ceil(as_d(n))); }
Value wp_floor(Value n) { return w_earth((int64_t)floor(as_d(n))); }
Value wp_round(Value n) { return w_earth((int64_t)llround(as_d(n))); }

Value wp_clamp(Value lo, Value hi, Value x) {
  if (w_compare(x, lo) < 0)
    return lo;
  if (w_compare(x, hi) > 0)
    return hi;
  return x;
}

Value wp_pow(Value base, Value exp) {
  if (base.tag == W_WATER || exp.tag == W_WATER)
    return w_water(pow(as_d(base), as_d(exp)));
  int64_t r = 1, b = base.earth, e = exp.earth;
  if (e < 0)
    return w_water(pow((double)b, (double)e));
  while (e) {
    if (e & 1)
      r *= b;
    b *= b;
    e >>= 1;
  }
  return w_earth(r);
}

Value wp_bor(Value a, Value b) { return w_earth(a.earth | b.earth); }
Value wp_band(Value a, Value b) { return w_earth(a.earth & b.earth); }
Value wp_bxor(Value a, Value b) { return w_earth(a.earth ^ b.earth); }
Value wp_bnot(Value a) { return w_earth(~a.earth); }
Value wp_shl(Value n, Value x) { return w_earth(x.earth << n.earth); }
Value wp_shr(Value n, Value x) { return w_earth(x.earth >> n.earth); }

// base and unbase are the two halves of writing a number in something other
// than ten. This used to be `bin`, which wrote base two and had no reading half
// at all — so a puzzle that handed you binary could print it and never read it
// back. Neither half is harder to write for any base than for two.
//
// Digits above nine are lower-case letters, and `unbase` accepts either case.
// A base outside 2..36 answers "0" and Stilled rather than guessing.
static const char base_digits[] = "0123456789abcdefghijklmnopqrstuvwxyz";

Value wp_base(Value b, Value n) {
  int64_t radix = b.earth;
  if (radix < 2 || radix > 36)
    return w_air_cstr("0");

  char tmp[66];
  int i = 65;
  bool negative = n.earth < 0;
  // Read as unsigned so that the most negative Earth does not overflow when
  // its sign is taken off.
  uint64_t v = negative ? (uint64_t)(-(n.earth + 1)) + 1 : (uint64_t)n.earth;
  if (v == 0)
    return w_air_cstr("0");
  while (v) {
    tmp[--i] = base_digits[v % (uint64_t)radix];
    v /= (uint64_t)radix;
  }
  if (negative)
    tmp[--i] = '-';

  size_t len = (size_t)(65 - i);
  char *out = (char *)w_alloc(len);
  memcpy(out, tmp + i, len);
  return w_air(out, len);
}

Value wp_unbase(Value b, Value text) {
  int64_t radix = b.earth;
  if (radix < 2 || radix > 36)
    return w_stilled();

  WAir *a = air_of(text);
  size_t i = 0;
  while (i < a->len && isspace((unsigned char)a->bytes[i]))
    i++;
  bool negative = i < a->len && a->bytes[i] == '-';
  if (negative || (i < a->len && a->bytes[i] == '+'))
    i++;

  int64_t acc = 0;
  size_t digits = 0;
  for (; i < a->len; i++) {
    unsigned char c = (unsigned char)a->bytes[i];
    int d;
    if (c >= '0' && c <= '9')
      d = c - '0';
    else if (c >= 'a' && c <= 'z')
      d = c - 'a' + 10;
    else if (c >= 'A' && c <= 'Z')
      d = c - 'A' + 10;
    else
      return w_stilled(); // anything that is not a digit at all
    if (d >= radix)
      return w_stilled(); // a digit this base does not have
    acc = acc * radix + d;
    digits++;
  }
  if (digits == 0)
    return w_stilled();
  return w_held(w_earth(negative ? -acc : acc));
}

// mdist is the Manhattan distance, the metric most grid puzzles measure in.
Value wp_mdist(Value a, Value b) {
  int64_t dr = (int64_t)a.knot.row - b.knot.row;
  int64_t dc = (int64_t)a.knot.col - b.knot.col;
  return w_earth((dr < 0 ? -dr : dr) + (dc < 0 ? -dc : dc));
}

// ---------------------------------------------------------- more grid verbs

Value wp_inb(Value g, Value k) { return w_spirit(in_bounds(pat_of(g), k)); }

Value wp_shape(Value g) {
  WPattern *p = pat_of(g);
  Value *pair = (Value *)w_alloc(sizeof(Value) * 2);
  pair[0] = w_earth((int64_t)p->rows);
  pair[1] = w_earth((int64_t)p->cols);
  return w_twine(pair, 2);
}

static Value dirs(bool diagonals) {
  static const int dr[8] = {-1, 1, 0, 0, -1, -1, 1, 1};
  static const int dc[8] = {0, 0, -1, 1, -1, 1, -1, 1};
  int n = diagonals ? 8 : 4;
  Value *out = (Value *)w_alloc(sizeof(Value) * (size_t)n);
  for (int i = 0; i < n; i++)
    out[i] = w_knot_make(dr[i], dc[i]);
  return w_thread(out, (size_t)n);
}

Value wp_dirs4(void) { return dirs(false); }
Value wp_dirs8(void) { return dirs(true); }

// around gives the neighbouring coordinates rather than their contents, which
// is what a search needs.
static Value around(Value g, Value k, bool diagonals) {
  static const int dr[8] = {-1, 1, 0, 0, -1, -1, 1, 1};
  static const int dc[8] = {0, 0, -1, 1, -1, 1, -1, 1};
  WPattern *p = pat_of(g);
  Buf b = {0};
  int n = diagonals ? 8 : 4;
  for (int i = 0; i < n; i++) {
    Value nk = w_knot_make(k.knot.row + dr[i], k.knot.col + dc[i]);
    if (in_bounds(p, nk))
      buf_push(&b, nk);
  }
  return buf_thread(&b);
}

Value wp_around4(Value g, Value k) { return around(g, k, false); }
Value wp_around8(Value g, Value k) { return around(g, k, true); }

// ----------------------------------------------------- more Web/Circle verbs

Value wp_mapvals(Value f, Value web) {
  Value ps = w_web_pairs(web);
  WThread *t = thr_of(ps);
  Value out = w_web_empty();
  for (size_t i = 0; i < t->len; i++)
    out = w_web_put_owned(out, w_twine_at(thr_at(t, i), 0), call1(f, w_twine_at(thr_at(t, i), 1)));
  return sealed(out);
}

Value wp_union(Value a, Value b) {
  Value ks = w_web_keys(b);
  WThread *t = thr_of(ks);
  Value out = a;
  for (size_t i = 0; i < t->len; i++)
    out = w_web_put_owned(out, thr_at(t, i), W_LIGHT);
  return sealed(out);
}

Value wp_inter(Value a, Value b) {
  Value ks = w_web_keys(a);
  WThread *t = thr_of(ks);
  Value out = w_circle_empty();
  for (size_t i = 0; i < t->len; i++)
    if (w_web_has(b, thr_at(t, i)))
      out = w_web_put_owned(out, thr_at(t, i), W_LIGHT);
  return sealed(out);
}

Value wp_diff(Value a, Value b) {
  Value ks = w_web_keys(a);
  WThread *t = thr_of(ks);
  Value out = w_circle_empty();
  for (size_t i = 0; i < t->len; i++)
    if (!w_web_has(b, thr_at(t, i)))
      out = w_web_put_owned(out, thr_at(t, i), W_LIGHT);
  return sealed(out);
}

// covers is the containment `union`, `inter` and `diff` were missing. The
// ranges have `within`; the sets had three ways to combine two Circles and no
// way to ask whether one already held the other.
Value wp_covers(Value outer, Value inner) {
  Value ks = w_web_keys(inner);
  WThread *t = thr_of(ks);
  for (size_t i = 0; i < t->len; i++)
    if (!w_web_has(outer, thr_at(t, i)))
      return W_SHADOW;
  return W_LIGHT;
}

Value wp_pi(void) { return w_water(3.14159265358979323846); }
Value wp_e(void) { return w_water(2.71828182845904523536); }
Value wp_inf(void) { return w_water(INFINITY); }
