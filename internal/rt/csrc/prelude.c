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
    total += air_of(t->items[i])->len;
  if (t->len > 1)
    total += s->len * (t->len - 1);
  char *out = (char *)w_alloc(total ? total : 1);
  size_t at = 0;
  for (size_t i = 0; i < t->len; i++) {
    if (i && s->len) {
      memcpy(out + at, s->bytes, s->len);
      at += s->len;
    }
    WAir *p = air_of(t->items[i]);
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
    out[i] = call1(f, t->items[i]);
  return w_thread(out, t->len);
}

Value wp_sift(Value p, Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  for (size_t i = 0; i < t->len; i++)
    if (call1(p, t->items[i]).spirit)
      buf_push(&b, t->items[i]);
  return buf_thread(&b);
}

Value wp_braid(Value f, Value seed, Value xs) {
  WThread *t = thr_of(xs);
  Value acc = seed;
  for (size_t i = 0; i < t->len; i++)
    acc = call2(f, acc, t->items[i]);
  return acc;
}

Value wp_seek(Value p, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (call1(p, t->items[i]).spirit)
      return w_held(t->items[i]);
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
    if (call1(p, t->items[i]).spirit)
      n++;
  return w_earth(n);
}

Value wp_sum(Value xs) {
  WThread *t = thr_of(xs);
  if (t->len == 0)
    return w_earth(0);
  Value acc = t->items[0];
  for (size_t i = 1; i < t->len; i++)
    acc = wp_add(acc, t->items[i]);
  return acc;
}

Value wp_prod(Value xs) {
  WThread *t = thr_of(xs);
  if (t->len == 0)
    return w_earth(1);
  Value acc = t->items[0];
  for (size_t i = 1; i < t->len; i++)
    acc = wp_mul(acc, t->items[i]);
  return acc;
}

Value wp_take(Value n, Value xs) {
  WThread *t = thr_of(xs);
  size_t k = n.earth < 0 ? 0 : (size_t)n.earth;
  if (k > t->len)
    k = t->len;
  return w_thread(t->items, k);
}

Value wp_drop(Value n, Value xs) {
  WThread *t = thr_of(xs);
  size_t k = n.earth < 0 ? 0 : (size_t)n.earth;
  if (k > t->len)
    k = t->len;
  return w_thread(t->items + k, t->len - k);
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

Value wp_weld(Value extra, Value xs) {
  WThread *e = thr_of(extra), *x = thr_of(xs);
  size_t n = x->len + e->len;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  memcpy(out, x->items, sizeof(Value) * x->len);
  memcpy(out + x->len, e->items, sizeof(Value) * e->len);
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
  memcpy(out, x->items, sizeof(Value) * x->len);
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
    t->items[at.earth] = v;
    return xs;
  }
  Value copy = wp_mend(at, v, xs);
  copy.obj->rc = W_OWNED;
  return copy;
}

Value wp_sever(Value n, Value xs) {
  WThread *x = thr_of(xs);
  size_t k = n.earth < 0 ? 0 : (size_t)n.earth;
  if (k > x->len)
    k = x->len;
  Value pair[2];
  pair[0] = w_thread(x->items, k);
  pair[1] = w_thread(x->items + k, x->len - k);
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
    Value k = call1(key, x->items[i]);
    size_t start = i;
    i++;
    while (i < x->len && w_equal(call1(key, x->items[i]), k))
      i++;
    buf_push(&out, w_thread(x->items + start, i - start));
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
    out[2 * i] = a->items[i];
    out[2 * i + 1] = b->items[i];
  }
  return w_thread(out, 2 * n);
}

Value wp_zip(Value as, Value bs) {
  WThread *x = thr_of(as), *y = thr_of(bs);
  size_t n = x->len < y->len ? x->len : y->len;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  for (size_t i = 0; i < n; i++) {
    Value *pair = (Value *)w_alloc(sizeof(Value) * 2);
    pair[0] = x->items[i];
    pair[1] = y->items[i];
    out[i] = w_twine(pair, 2);
  }
  return w_thread(out, n);
}

static int sort_cmp(const void *a, const void *b) {
  return w_compare(*(const Value *)a, *(const Value *)b);
}

Value wp_sort(Value xs) {
  WThread *t = thr_of(xs);
  Value *out = (Value *)w_alloc(sizeof(Value) * (t->len ? t->len : 1));
  memcpy(out, t->items, sizeof(Value) * t->len);
  qsort(out, t->len, sizeof(Value), sort_cmp);
  return w_thread(out, t->len);
}

Value wp_all(Value p, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (!call1(p, t->items[i]).spirit)
      return W_SHADOW;
  return W_LIGHT;
}

Value wp_any(Value p, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (call1(p, t->items[i]).spirit)
      return W_LIGHT;
  return W_SHADOW;
}

Value wp_first(Value xs) {
  WThread *t = thr_of(xs);
  return t->len ? w_held(t->items[0]) : w_stilled();
}

Value wp_last(Value xs) {
  WThread *t = thr_of(xs);
  return t->len ? w_held(t->items[t->len - 1]) : w_stilled();
}

Value wp_rev(Value xs) {
  WThread *t = thr_of(xs);
  Value *out = (Value *)w_alloc(sizeof(Value) * (t->len ? t->len : 1));
  for (size_t i = 0; i < t->len; i++)
    out[i] = t->items[t->len - 1 - i];
  return w_thread(out, t->len);
}

Value wp_flat(Value xss) {
  WThread *outer = thr_of(xss);
  Buf b = {0};
  for (size_t i = 0; i < outer->len; i++) {
    WThread *inner = thr_of(outer->items[i]);
    for (size_t j = 0; j < inner->len; j++)
      buf_push(&b, inner->items[j]);
  }
  return buf_thread(&b);
}

Value wp_uniq(Value xs) {
  WThread *t = thr_of(xs);
  Buf b = {0};
  for (size_t i = 0; i < t->len; i++) {
    bool seen = false;
    for (size_t j = 0; j < b.len && !seen; j++)
      seen = w_equal(b.items[j], t->items[i]);
    if (!seen)
      buf_push(&b, t->items[i]);
  }
  return buf_thread(&b);
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
    size_t n = w_thread_len(outer->items[r]);
    if (n > ncols)
      ncols = n;
  }
  Value *cells = (Value *)w_alloc(sizeof(Value) * (nrows * ncols ? nrows * ncols : 1));
  for (size_t r = 0; r < nrows; r++) {
    WThread *row = thr_of(outer->items[r]);
    for (size_t c = 0; c < ncols; c++)
      cells[r * ncols + c] = c < row->len ? row->items[c] : fill;
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

static bool in_bounds(WPattern *p, Value k) {
  return k.knot.row >= 0 && k.knot.col >= 0 && (size_t)k.knot.row < p->rows &&
         (size_t)k.knot.col < p->cols;
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
    web = w_web_put_owned(web, w_twine_at(t->items[i], 0), w_twine_at(t->items[i], 1));
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
    out = w_web_put_owned(out, w_twine_at(t->items[i], 0), w_twine_at(t->items[i], 1));
  return sealed(out);
}

Value wp_freq(Value xs) {
  WThread *t = thr_of(xs);
  Value web = w_web_empty();
  for (size_t i = 0; i < t->len; i++) {
    Value seen = w_web_get(web, t->items[i]);
    int64_t n = w_is_held(seen) ? w_hold_inner(seen).earth : 0;
    web = w_web_put_owned(web, t->items[i], w_earth(n + 1));
  }
  return sealed(web);
}

Value wp_most(Value web) {
  Value ps = w_web_pairs(web);
  WThread *t = thr_of(ps);
  if (t->len == 0)
    return w_stilled();
  Value best = w_twine_at(t->items[0], 0);
  int64_t bestN = w_twine_at(t->items[0], 1).earth;
  for (size_t i = 1; i < t->len; i++) {
    int64_t n = w_twine_at(t->items[i], 1).earth;
    if (n > bestN) {
      bestN = n;
      best = w_twine_at(t->items[i], 0);
    }
  }
  return w_held(best);
}

Value wp_circle(Value xs) {
  WThread *t = thr_of(xs);
  Value c = w_circle_empty();
  for (size_t i = 0; i < t->len; i++)
    c = w_web_put_owned(c, t->items[i], W_LIGHT);
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

Value wp_taveren(Value xs) {
  WThread *t = thr_of(xs);
  Value h = w_taveren_empty();
  for (size_t i = 0; i < t->len; i++)
    h = w_taveren_push(h, t->items[i]);
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
    if (!w_web_has(degree, t->items[i]))
      degree = w_web_put_owned(degree, t->items[i], w_earth(0));
  for (size_t i = 0; i < t->len; i++) {
    Value out = w_call(step, &t->items[i], 1);
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
    Value have = w_web_get(degree, t->items[i]);
    if (w_is_held(have) && w_hold_inner(have).earth == 0)
      buf_push(&ready, t->items[i]);
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
    if (!call1(p, t->items[i]).spirit)
      buf_push(&b, t->items[i]);
  return buf_thread(&b);
}

Value wp_zipwith(Value f, Value a, Value b) {
  WThread *x = thr_of(a), *y = thr_of(b);
  size_t n = x->len < y->len ? x->len : y->len;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  for (size_t i = 0; i < n; i++)
    out[i] = call2(f, x->items[i], y->items[i]);
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
    out[i] = wp_bend(f, t->items[i]);
  return w_thread(out, t->len);
}

Value wp_siftr(Value p, Value xs) {
  WThread *t = thr_of(xs);
  Value *out = (Value *)w_alloc(sizeof(Value) * (t->len ? t->len : 1));
  for (size_t i = 0; i < t->len; i++)
    out[i] = wp_sift(p, t->items[i]);
  return w_thread(out, t->len);
}

Value wp_zipr(Value f, Value a, Value b) {
  WThread *x = thr_of(a), *y = thr_of(b);
  size_t n = x->len < y->len ? x->len : y->len;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  for (size_t i = 0; i < n; i++)
    out[i] = wp_zipwith(f, x->items[i], y->items[i]);
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
    acc = i == 0 ? t->items[i]
                 : (product ? wp_mul(acc, t->items[i]) : wp_add(acc, t->items[i]));
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
Value wp_earths(Value text) {
  WAir *a = air_of(text);
  Buf b = {0};
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
    buf_push(&b, w_earth(neg ? -n : n));
  }
  return buf_thread(&b);
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
    buf_push(&b, w_thread(t->items + i, len));
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
    buf_push(&b, w_thread(t->items + i, (size_t)k));
  return buf_thread(&b);
}

Value wp_pivot(Value xss) {
  WThread *outer = thr_of(xss);
  size_t cols = 0;
  for (size_t i = 0; i < outer->len; i++) {
    size_t n = w_thread_len(outer->items[i]);
    if (n > cols)
      cols = n;
  }
  Buf out = {0};
  for (size_t c = 0; c < cols; c++) {
    Buf row = {0};
    for (size_t r = 0; r < outer->len; r++) {
      WThread *line = thr_of(outer->items[r]);
      if (c < line->len)
        buf_push(&row, line->items[c]);
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
// actually phrased.
typedef struct {
  Value key;
  Value item;
} Keyed;

static int keyed_cmp(const void *a, const void *b) {
  return w_compare(((const Keyed *)a)->key, ((const Keyed *)b)->key);
}

Value wp_sortby(Value key, Value xs) {
  WThread *t = thr_of(xs);
  Keyed *tmp = (Keyed *)w_alloc(sizeof(Keyed) * (t->len ? t->len : 1));
  for (size_t i = 0; i < t->len; i++) {
    tmp[i].key = call1(key, t->items[i]);
    tmp[i].item = t->items[i];
  }
  qsort(tmp, t->len, sizeof(Keyed), keyed_cmp);
  Value *out = (Value *)w_alloc(sizeof(Value) * (t->len ? t->len : 1));
  for (size_t i = 0; i < t->len; i++)
    out[i] = tmp[i].item;
  return w_thread(out, t->len);
}

// groupBy collects elements sharing a derived key into a Web of Threads.
Value wp_group(Value key, Value xs) {
  WThread *t = thr_of(xs);
  Value web = w_web_empty();
  for (size_t i = 0; i < t->len; i++) {
    Value k = call1(key, t->items[i]);
    Value seen = w_web_get(web, k);
    Buf b = {0};
    if (w_is_held(seen)) {
      WThread *prev = thr_of(w_hold_inner(seen));
      for (size_t j = 0; j < prev->len; j++)
        buf_push(&b, prev->items[j]);
    }
    buf_push(&b, t->items[i]);
    web = w_web_put_owned(web, k, buf_thread(&b));
  }
  return sealed(web);
}

Value wp_idx(Value needle, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (w_equal(t->items[i], needle))
      return w_held(w_earth((int64_t)i));
  return w_stilled();
}

// nth is how an element is taken out of a Thread by position. A Thread is a
// strict array, so this is one load — but it is out of bounds for most indices
// most of the time, so it answers with a Hold rather than a value.
Value wp_nth(Value n, Value xs) {
  WThread *t = thr_of(xs);
  if (n.earth < 0 || (size_t)n.earth >= t->len)
    return w_stilled();
  return w_held(t->items[n.earth]);
}

Value wp_has(Value needle, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (w_equal(t->items[i], needle))
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
    Value h = call1(f, t->items[i]);
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
    Value h = call1(f, t->items[i]);
    if (!w_is_held(h))
      return w_data("Gentled", 1, &t->items[i], 1);
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
    work[i] = t->items[i];
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

Value wp_head(Value xs) { return wp_first(xs); }

Value wp_tail(Value xs) {
  WThread *t = thr_of(xs);
  if (t->len == 0)
    return w_thread_empty();
  return w_thread(t->items + 1, t->len - 1);
}

Value wp_second(Value xs) {
  WThread *t = thr_of(xs);
  return t->len > 1 ? w_held(t->items[1]) : w_stilled();
}

Value wp_none(Value p, Value xs) {
  WThread *t = thr_of(xs);
  for (size_t i = 0; i < t->len; i++)
    if (call1(p, t->items[i]).spirit)
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
    pair[1] = t->items[i];
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
    acc = call2(f, acc, t->items[i]);
    buf_push(&b, acc);
  }
  return buf_thread(&b);
}

// dupe is the first element that has been seen before. It is a `seek` with a
// memory: the test is not about the element on its own, so no predicate can
// express it, and every Advent of Code cycle-detection puzzle wants it.
Value wp_dupe(Value xs) {
  WThread *t = thr_of(xs);
  Value seen = w_circle_empty();
  for (size_t i = 0; i < t->len; i++) {
    if (w_web_has(seen, t->items[i])) {
      Value pair[2] = {w_earth((int64_t)i), t->items[i]};
      return w_held(w_twine_copy(pair, 2));
    }
    seen = wp_insert_owned(seen, t->items[i]);
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
    Value step = call2(f, acc, t->items[i]);
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
    pair[0] = t->items[i];
    pair[1] = t->items[i + 1];
    buf_push(&b, w_twine(pair, 2));
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
      pair[0] = x->items[i];
      pair[1] = y->items[j];
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
    buf_push(&next, t->items[i]);
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
    if (w_is_held(t->items[i]))
      buf_push(&b, w_hold_inner(t->items[i]));
  return buf_thread(&b);
}

Value wp_takewhile(Value p, Value xs) {
  WThread *t = thr_of(xs);
  size_t n = 0;
  while (n < t->len && call1(p, t->items[n]).spirit)
    n++;
  return w_thread(t->items, n);
}

Value wp_dropwhile(Value p, Value xs) {
  WThread *t = thr_of(xs);
  size_t n = 0;
  while (n < t->len && call1(p, t->items[n]).spirit)
    n++;
  return w_thread(t->items + n, t->len - n);
}

// mapcat maps then flattens, for a step that yields several results.
Value wp_mapcat(Value f, Value xs) { return wp_flat(wp_bend(f, xs)); }

// col takes one column out of a Thread of Threads.
static Value extreme_by(Value key, Value xs, int want) {
  WThread *t = thr_of(xs);
  if (t->len == 0)
    return w_stilled();
  Value best = t->items[0], bestKey = call1(key, best);
  for (size_t i = 1; i < t->len; i++) {
    Value k = call1(key, t->items[i]);
    if (w_compare(k, bestKey) * want > 0) {
      best = t->items[i];
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
    if (w_compare(t->items[i], t->items[at]) * want > 0)
      at = i;
  return w_held(wantIdx ? w_earth((int64_t)at) : t->items[at]);
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
    if (call1(p, t->items[i]).spirit)
      return w_held(w_earth((int64_t)i));
  return w_stilled();
}

// twist is mend for a value that depends on what is already there, which is
// most of them: `twist i inc counts` rather than reading, adding and writing
// back.
Value wp_twist(Value at, Value f, Value xs) {
  WThread *t = thr_of(xs);
  if (at.earth < 0 || (size_t)at.earth >= t->len)
    return xs;
  return wp_mend(at, call1(f, t->items[at.earth]), xs);
}

// wp_twist_owned is twist on a Thread the compiler has proved is not shared;
// see wp_mend_owned.
Value wp_twist_owned(Value at, Value f, Value xs) {
  WThread *t = thr_of(xs);
  if (at.earth < 0 || (size_t)at.earth >= t->len)
    return xs;
  return wp_mend_owned(at, call1(f, t->items[at.earth]), xs);
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

Value wp_repeat(Value n, Value text) {
  WAir *a = air_of(text);
  int64_t k = n.earth < 0 ? 0 : n.earth;
  size_t total = a->len * (size_t)k;
  char *out = (char *)w_alloc(total ? total : 1);
  for (int64_t i = 0; i < k; i++)
    memcpy(out + a->len * (size_t)i, a->bytes, a->len);
  return w_air(out, total);
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

Value wp_bin(Value n) {
  char tmp[65];
  uint64_t v = (uint64_t)n.earth;
  int i = 64;
  if (v == 0)
    return w_air_cstr("0");
  while (v) {
    tmp[--i] = (char)('0' + (v & 1));
    v >>= 1;
  }
  char *out = (char *)w_alloc((size_t)(64 - i));
  memcpy(out, tmp + i, (size_t)(64 - i));
  return w_air(out, (size_t)(64 - i));
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
    out = w_web_put_owned(out, w_twine_at(t->items[i], 0), call1(f, w_twine_at(t->items[i], 1)));
  return sealed(out);
}

Value wp_union(Value a, Value b) {
  Value ks = w_web_keys(b);
  WThread *t = thr_of(ks);
  Value out = a;
  for (size_t i = 0; i < t->len; i++)
    out = w_web_put_owned(out, t->items[i], W_LIGHT);
  return sealed(out);
}

Value wp_inter(Value a, Value b) {
  Value ks = w_web_keys(a);
  WThread *t = thr_of(ks);
  Value out = w_circle_empty();
  for (size_t i = 0; i < t->len; i++)
    if (w_web_has(b, t->items[i]))
      out = w_web_put_owned(out, t->items[i], W_LIGHT);
  return sealed(out);
}

Value wp_diff(Value a, Value b) {
  Value ks = w_web_keys(a);
  WThread *t = thr_of(ks);
  Value out = w_circle_empty();
  for (size_t i = 0; i < t->len; i++)
    if (!w_web_has(b, t->items[i]))
      out = w_web_put_owned(out, t->items[i], W_LIGHT);
  return sealed(out);
}

Value wp_pi(void) { return w_water(3.14159265358979323846); }
Value wp_e(void) { return w_water(2.71828182845904523536); }
Value wp_inf(void) { return w_water(INFINITY); }
