// weave.c — runtime support for compiled Weave programs. See weave.h.

#include "weave.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// ------------------------------------------------------------------ memory

// A bump allocator over chunks, with free lists in front of it.
//
// A Weave program is a batch job that runs once over one input and exits, so
// there is no collector and never will be: tracing would cost every program
// something to help the few that need it. But some of what a program allocates
// is *known* to be dead the instant it happens — the array a growing buffer has
// just outgrown, the trie node an owned insert has just replaced, the table a
// hash grow has just rehashed out of. Those places call w_free, and what they
// hand back is the first thing the next allocation of that size gets.
//
// The lists are exact-size, not rounded: a block only ever satisfies a request
// for the size it was allocated at. Sizes repeat far more than they vary — a
// loop that builds one Thread per turn asks for the same length every time — so
// exact fits reuse nearly everything without the internal waste that rounding
// up to a power of two would cost on the large blocks, which are the ones worth
// reusing.
// The bump pointer in weave.h is the truth about where the current chunk has
// got to; a Chunk only has to remember how to free itself and how big it is.
typedef struct Chunk {
  struct Chunk *prev;
  size_t cap;
  char data[];
} Chunk;

static Chunk *g_chunk = NULL;
static const size_t CHUNK_MIN = 1u << 20;

// Blocks up to 8 KiB are indexed directly by size/16. The list heads and the
// bump pointer live in weave.h so that the fast path can be inlined: left as a
// call, w_alloc was 30% of Advent of Code 2024 day 2, nearly all of it the call
// itself around a handful of instructions.
void *w_small[W_SMALL_CLASSES];
char *w_bump, *w_bump_end;

// Bigger ones live in an open-addressed table keyed by that same size/16, so a
// 32 000-byte Thread array does not have to share a class with a 65 000-byte
// one. A program has few distinct large sizes; if it somehow has more than this
// many, the excess simply is not reused.
#define LARGE_BINS 1024
typedef struct {
  size_t words; // size/16, or 0 for an empty bin
  void *head;
} LargeBin;
static LargeBin g_large[LARGE_BINS];

void w_init(void) {}

// large_bin finds the list for a size, claiming an empty bin if asked to.
static LargeBin *large_bin(size_t words, bool claim) {
  size_t i = (words * 2654435761u) & (LARGE_BINS - 1);
  for (size_t probe = 0; probe < LARGE_BINS; probe++) {
    LargeBin *b = &g_large[i];
    if (b->words == words)
      return b;
    if (b->words == 0) {
      if (!claim)
        return NULL;
      b->words = words;
      return b;
    }
    i = (i + 1) & (LARGE_BINS - 1);
  }
  return NULL;
}

static Chunk *new_chunk(size_t cap) {
  Chunk *c = (Chunk *)malloc(sizeof(Chunk) + cap);
  if (!c)
    w_fail("out of memory");
  c->cap = cap;
  return c;
}

// A block too big to sit inside a chunk gets one of its own, linked behind the
// current one so that the bump pointer keeps its remaining room. Rounding these
// up to the next chunk size instead would waste more than the block costs.
static void *w_alloc_own_chunk(size_t bytes) {
  Chunk *c = new_chunk(bytes);
  if (g_chunk) {
    c->prev = g_chunk->prev;
    g_chunk->prev = c;
  } else {
    c->prev = NULL;
    g_chunk = c;
  }
  return c->data;
}

// w_alloc_slow is everything the inline fast path in weave.h does not handle: a
// block too big for the direct-indexed lists, and a bump pointer with no room
// left. `bytes` arrives already rounded.
void *w_alloc_slow(size_t bytes) {
  size_t words = bytes >> 4;
  if (words >= W_SMALL_CLASSES) {
    // A block this size has no direct-indexed list, so the inline path did not
    // look at the bins or at the bump pointer. Both are still worth trying.
    LargeBin *b = large_bin(words, false);
    if (b && b->head) {
      void *p = b->head;
      b->head = *(void **)p;
      return p;
    }
    if ((size_t)(w_bump_end - w_bump) >= bytes) {
      void *p = w_bump;
      w_bump += bytes;
      return p;
    }
  }
  if (bytes >= CHUNK_MIN)
    return w_alloc_own_chunk(bytes);

  // A fresh chunk. Whatever is left of the old one is abandoned: it is under
  // one allocation's worth, and the bookkeeping to keep it would cost more
  // than it saves.
  Chunk *c = new_chunk(CHUNK_MIN);
  c->prev = g_chunk;
  g_chunk = c;
  w_bump = c->data + bytes;
  w_bump_end = c->data + c->cap;
  return c->data;
}

// w_free takes a block back. The caller is asserting that nothing else can
// reach it — every call site is somewhere the runtime has just replaced a block
// with another one and dropped the only pointer to it.
void w_free(void *p, size_t bytes) {
  if (!p)
    return;
  bytes = (bytes + 15) & ~(size_t)15;
  if (bytes == 0)
    return;
  size_t words = bytes >> 4;
  if (words < W_SMALL_CLASSES) {
    *(void **)p = w_small[words];
    w_small[words] = p;
    return;
  }
  LargeBin *b = large_bin(words, true);
  if (!b)
    return; // more distinct large sizes than bins: keep the old behaviour
  *(void **)p = b->head;
  b->head = p;
}

_Noreturn void w_fail(const char *msg) {
  fprintf(stderr, "weave: %s\n", msg);
  exit(1);
}

// -------------------------------------------------------------------- text

Value w_air(const char *bytes, size_t len) {
  WAir *a = (WAir *)w_alloc(sizeof(WAir));
  a->obj.rc = W_SHARED;
  a->obj.kind = W_AIR;
  a->len = len;
  a->bytes = bytes;
  Value v;
  v.tag = W_AIR;
  v.obj = &a->obj;
  return v;
}

Value w_air_cstr(const char *s) { return w_air(s, strlen(s)); }

static WAir *air_of(Value v) { return (WAir *)v.obj; }

Value w_source(void) {
  size_t cap = 1 << 16, len = 0;
  char *buf = (char *)malloc(cap);
  if (!buf)
    w_fail("out of memory reading Source");
  for (;;) {
    if (len == cap) {
      cap *= 2;
      buf = (char *)realloc(buf, cap);
      if (!buf)
        w_fail("out of memory reading Source");
    }
    size_t n = fread(buf + len, 1, cap - len, stdin);
    len += n;
    if (n == 0)
      break;
  }
  return w_air(buf, len);
}

// ------------------------------------------------------------------ threads

Value w_thread(Value *items, size_t len) {
  WThread *t = (WThread *)w_alloc(sizeof(WThread));
  t->obj.rc = W_SHARED;
  t->obj.kind = W_THREAD;
  t->len = len;
  t->items = items;
  Value v;
  v.tag = W_THREAD;
  v.obj = &t->obj;
  return v;
}

Value w_thread_copy(const Value *items, size_t len) {
  if (len == 0)
    return w_thread(NULL, 0);
  Value *copy = (Value *)w_alloc(sizeof(Value) * len);
  memcpy(copy, items, sizeof(Value) * len);
  return w_thread(copy, len);
}

// w_thread_fit makes a Thread out of a buffer that was allocated with room to
// spare, handing the unused tail back. A buffer that doubles, or one sized
// against a source a `sift` then thinned out, ends up holding rather more than
// the Thread needs; the tail is a block like any other, and giving it back at
// its true size is what lets the free lists match it to a later request.
Value w_thread_fit(Value *items, size_t len, size_t cap) {
  if (cap > len)
    w_free(items + len, sizeof(Value) * (cap - len));
  return w_thread(items, len);
}

Value w_thread_empty(void) { return w_thread(NULL, 0); }

// w_thread_release hands a Thread's storage back to the allocator. Only
// generated code calls it, and only where the compiler has proved the Thread
// dies with the call that built it — see internal/codegen/escape.go. The
// elements are not touched: whatever they point at may well outlive this.
void w_thread_release(Value t) {
  if (t.tag != W_THREAD || !t.obj)
    return;
  WThread *th = (WThread *)t.obj;
  if (th->items)
    w_free(th->items, sizeof(Value) * th->len);
  w_free(th, sizeof(WThread));
}

// --------------------------------------------------------------------- hold

// w_held_boxed is the case w_held cannot keep in the Value itself: a Thread, a
// Web, some text — anything already on the heap.
Value w_held_boxed(Value inner) {
  WHold *h = (WHold *)w_alloc(sizeof(WHold));
  h->obj.rc = W_SHARED;
  h->obj.kind = W_HOLD;
  h->inner = inner;
  Value v;
  v.tag = W_HOLD;
  v.aux = W_HOLD_BOXED;
  v.obj = &h->obj;
  return v;
}

Value w_hold_inner(Value v) {
  if (v.aux == W_HOLD_NONE)
    w_fail("tried to read a Stilled value");
  if (v.aux == W_HOLD_BOXED)
    return ((WHold *)v.obj)->inner;
  Value inner = v;
  inner.tag = v.aux;
  inner.aux = 0;
  return inner;
}

// -------------------------------------------------------------------- tuple

Value w_twine(Value *items, size_t len) {
  WTwine *t = (WTwine *)w_alloc(sizeof(WTwine));
  t->obj.rc = W_SHARED;
  t->obj.kind = W_TWINE;
  t->len = len;
  t->items = items;
  Value v;
  v.tag = W_TWINE;
  v.obj = &t->obj;
  return v;
}

Value *w_regrow(Value *items, size_t n, size_t cap) {
  Value *bigger = (Value *)w_alloc(sizeof(Value) * (cap ? cap : 1));
  if (n) {
    memcpy(bigger, items, sizeof(Value) * n);
    // n is the old capacity as well as the length — the buffer is regrown only
    // when it is full — and the fused loop that owns it has no other pointer.
    w_free(items, sizeof(Value) * n);
  }
  return bigger;
}

Value w_data(const char *name, uint32_t index, const Value *fields, size_t n) {
  WData *d = (WData *)w_alloc(sizeof(WData));
  d->obj.rc = W_SHARED;
  d->obj.kind = W_DATA;
  d->name = name;
  d->index = index;
  d->nfields = (uint32_t)n;
  if (n) {
    d->fields = (Value *)w_alloc(sizeof(Value) * n);
    memcpy(d->fields, fields, sizeof(Value) * n);
  } else {
    d->fields = NULL;
  }
  Value v;
  v.tag = W_DATA;
  v.obj = &d->obj;
  return v;
}

Value w_twine_copy(const Value *items, size_t len) {
  Value *copy = (Value *)w_alloc(sizeof(Value) * (len ? len : 1));
  memcpy(copy, items, sizeof(Value) * len);
  return w_twine(copy, len);
}

Value w_twine_at(Value t, size_t i) { return ((WTwine *)t.obj)->items[i]; }

// ------------------------------------------------------------------ closure

Value w_closure(WFn fn, int arity, Value *env, int nenv) {
  WClosure *c = (WClosure *)w_alloc(sizeof(WClosure));
  c->obj.rc = W_SHARED;
  c->obj.kind = W_CLOSURE;
  c->fn = fn;
  c->arity = arity;
  c->nargs = 0;
  c->nenv = nenv;
  c->slots = (Value *)w_alloc(sizeof(Value) * (size_t)(nenv + arity));
  for (int i = 0; i < nenv; i++)
    c->slots[i] = env[i];
  Value v;
  v.tag = W_CLOSURE;
  v.obj = &c->obj;
  return v;
}

// w_closure_env fills in a closure's captured values after it has been made.
//
// A local function that calls itself has to appear in its own environment, and
// nothing can be in an array that does not exist yet — so the closure is built
// empty, and this puts the environment in once the value is available to go in
// it. Nothing has been applied at this point, so there are no arguments to
// preserve.
void w_closure_env(Value v, const Value *env, int nenv) {
  WClosure *c = (WClosure *)v.obj;
  c->slots = (Value *)w_alloc(sizeof(Value) * (size_t)(nenv + c->arity));
  for (int i = 0; i < nenv; i++)
    c->slots[i] = env[i];
  c->nenv = nenv;
}

// w_apply supplies one argument, running the function once it is saturated.
// A partially applied closure is copied so the original stays reusable.
Value w_apply(Value f, Value arg) {
  if (f.tag != W_CLOSURE)
    w_fail("tried to call something that is not a function");
  WClosure *c = (WClosure *)f.obj;

  WClosure *n = (WClosure *)w_alloc(sizeof(WClosure));
  *n = *c;
  n->slots = (Value *)w_alloc(sizeof(Value) * (size_t)(c->nenv + c->arity));
  memcpy(n->slots, c->slots, sizeof(Value) * (size_t)(c->nenv + c->nargs));
  n->slots[c->nenv + c->nargs] = arg;
  n->nargs = c->nargs + 1;

  if (n->nargs == n->arity)
    return n->fn(n->slots, n->slots + n->nenv);

  Value v;
  v.tag = W_CLOSURE;
  v.obj = &n->obj;
  return v;
}

Value w_call(Value f, Value *args, int n) {
  // Fast path: an unapplied closure of exactly the right arity runs without
  // any copying at all.
  if (f.tag == W_CLOSURE) {
    WClosure *c = (WClosure *)f.obj;
    if (c->nargs == 0 && c->arity == n)
      return c->fn(c->slots, args);
  }
  Value cur = f;
  for (int i = 0; i < n; i++)
    cur = w_apply(cur, args[i]);
  return cur;
}

// -------------------------------------------------------- equality & order

bool w_equal(Value a, Value b) { return w_compare(a, b) == 0; }

static int cmp_i64(int64_t x, int64_t y) { return x < y ? -1 : (x > y ? 1 : 0); }

int w_compare(Value a, Value b) {
  if (a.tag != b.tag)
    return cmp_i64(a.tag, b.tag);
  switch (a.tag) {
  case W_EARTH:
    return cmp_i64(a.earth, b.earth);
  case W_WATER:
    return a.water < b.water ? -1 : (a.water > b.water ? 1 : 0);
  case W_FIRE:
    return cmp_i64(a.fire, b.fire);
  case W_SPIRIT:
    return cmp_i64(a.spirit, b.spirit);
  case W_KNOT: {
    int r = cmp_i64(a.knot.row, b.knot.row);
    return r ? r : cmp_i64(a.knot.col, b.knot.col);
  }
  case W_AIR: {
    WAir *x = air_of(a), *y = air_of(b);
    size_t n = x->len < y->len ? x->len : y->len;
    int r = n ? memcmp(x->bytes, y->bytes, n) : 0;
    return r ? (r < 0 ? -1 : 1) : cmp_i64((int64_t)x->len, (int64_t)y->len);
  }
  case W_HOLD: {
    if (!w_is_held(a) || !w_is_held(b))
      return cmp_i64(w_is_held(a), w_is_held(b));
    return w_compare(w_hold_inner(a), w_hold_inner(b));
  }
  case W_WEB:
  case W_CIRCLE: {
    if (w_web_equal(a, b))
      return 0;
    return cmp_i64((int64_t)w_web_size(a), (int64_t)w_web_size(b)) ? cmp_i64((int64_t)w_web_size(a), (int64_t)w_web_size(b)) : 1;
  }
  case W_TAVEREN:
    return cmp_i64((int64_t)w_taveren_size(a), (int64_t)w_taveren_size(b));
  case W_DATA: {
    // Declaration order is the ordering, as it is for Held and Stilled.
    WData *x = (WData *)a.obj, *y = (WData *)b.obj;
    if (x->index != y->index)
      return cmp_i64(x->index, y->index);
    for (uint32_t i = 0; i < x->nfields && i < y->nfields; i++) {
      int r = w_compare(x->fields[i], y->fields[i]);
      if (r)
        return r;
    }
    return 0;
  }
  case W_THREAD:
  case W_TWINE: {
    size_t alen = a.tag == W_THREAD ? w_thread_len(a) : ((WTwine *)a.obj)->len;
    size_t blen = b.tag == W_THREAD ? w_thread_len(b) : ((WTwine *)b.obj)->len;
    Value *ai = a.tag == W_THREAD ? ((WThread *)a.obj)->items : ((WTwine *)a.obj)->items;
    Value *bi = b.tag == W_THREAD ? ((WThread *)b.obj)->items : ((WTwine *)b.obj)->items;
    size_t n = alen < blen ? alen : blen;
    for (size_t i = 0; i < n; i++) {
      int r = w_compare(ai[i], bi[i]);
      if (r)
        return r;
    }
    return cmp_i64((int64_t)alen, (int64_t)blen);
  }
  default:
    return 0;
  }
}

// ------------------------------------------------------------------- output

// A growable byte buffer for rendering values to text.
typedef struct {
  char *bytes;
  size_t len, cap;
} SBuf;

static void sb_grow(SBuf *b, size_t need) {
  if (b->len + need <= b->cap)
    return;
  size_t cap = b->cap ? b->cap : 64;
  while (cap < b->len + need)
    cap *= 2;
  char *next = (char *)w_alloc(cap);
  if (b->len)
    memcpy(next, b->bytes, b->len);
  b->bytes = next;
  b->cap = cap;
}

static void sb_put(SBuf *b, const char *s, size_t n) {
  sb_grow(b, n);
  memcpy(b->bytes + b->len, s, n);
  b->len += n;
}

static void sb_cstr(SBuf *b, const char *s) { sb_put(b, s, strlen(s)); }

static void sb_char(SBuf *b, char c) { sb_put(b, &c, 1); }

static void sb_rune(SBuf *b, uint32_t r) {
  char tmp[4];
  size_t n = 0;
  if (r < 0x80) {
    tmp[n++] = (char)r;
  } else if (r < 0x800) {
    tmp[n++] = (char)(0xC0 | (r >> 6));
    tmp[n++] = (char)(0x80 | (r & 0x3F));
  } else if (r < 0x10000) {
    tmp[n++] = (char)(0xE0 | (r >> 12));
    tmp[n++] = (char)(0x80 | ((r >> 6) & 0x3F));
    tmp[n++] = (char)(0x80 | (r & 0x3F));
  } else {
    tmp[n++] = (char)(0xF0 | (r >> 18));
    tmp[n++] = (char)(0x80 | ((r >> 12) & 0x3F));
    tmp[n++] = (char)(0x80 | ((r >> 6) & 0x3F));
    tmp[n++] = (char)(0x80 | (r & 0x3F));
  }
  sb_put(b, tmp, n);
}

// reads_as_application reports whether a value renders as a name followed by
// arguments, which needs bracketing wherever values sit side by side —
// otherwise `[knot 1 2, knot 3 4]` comes back as `knot 1 2 knot 3 4`, which is
// not a thing anyone can read.
static bool reads_as_application(Value v) {
  switch (v.tag) {
  case W_DATA:
    return ((WData *)v.obj)->nfields > 0;
  case W_HOLD:
    return w_is_held(v);
  case W_KNOT:
    return true;
  default:
    return false;
  }
}

static void render(SBuf *b, Value v);

// render_nested renders a value where another value follows it, bracketing it
// if it would otherwise run into its neighbour.
static void render_nested(SBuf *b, Value v) {
  bool wrap = reads_as_application(v);
  if (wrap)
    sb_char(b, '(');
  render(b, v);
  if (wrap)
    sb_char(b, ')');
}

// water_text renders a Water so that reading it back gives the same number,
// and so that it cannot be mistaken for an Earth.
//
// `%g` was neither: it rounds to six significant digits, so a third printed as
// 0.333333 and a program that had computed carefully printed something that had
// not. What is printed here is the *shortest* text that still reads back as the
// same double — found by asking for one significant digit, then two, until one
// round-trips.
//
// Where to put the decimal point is a separate question, and `%g` ties it to
// the precision, which gives 1.2e+02 for a hundred and twenty. So the two are
// decided separately, on the rule Go uses: exponent form when the exponent is
// below -4 or at least 6, fixed otherwise. A whole number then gets `.0`
// appended, because `1` and `1.0` are different types in this language and
// printing them the same way hides that.
static size_t water_text(char *out, size_t cap, double d) {
  if (d != d)
    return (size_t)snprintf(out, cap, "NaN");
  if (d > 1.7976931348623157e308)
    return (size_t)snprintf(out, cap, "Infinity");
  if (d < -1.7976931348623157e308)
    return (size_t)snprintf(out, cap, "-Infinity");

  // The fewest significant digits that still name this number exactly.
  char sci[64];
  int digits = 0;
  for (; digits < 17; digits++) {
    snprintf(sci, sizeof sci, "%.*e", digits, d);
    if (strtod(sci, NULL) == d)
      break;
  }

  // %e always writes an exponent, so read the one it chose.
  int exp = 0;
  for (const char *p = sci; *p; p++) {
    if (*p == 'e') {
      exp = (int)strtol(p + 1, NULL, 10);
      break;
    }
  }

  size_t n;
  if (exp < -4 || exp >= 6) {
    n = strlen(sci);
    if (n < cap)
      memcpy(out, sci, n + 1);
    return n;
  }

  int decimals = digits - exp;
  if (decimals < 0)
    decimals = 0;
  n = (size_t)snprintf(out, cap, "%.*f", decimals, d);

  for (size_t i = 0; i < n; i++)
    if (out[i] == '.')
      return n;
  if (n + 2 < cap) {
    out[n++] = '.';
    out[n++] = '0';
    out[n] = 0;
  }
  return n;
}

static void render(SBuf *b, Value v) {
  char tmp[64];
  switch (v.tag) {
  case W_EARTH:
    sb_put(b, tmp, (size_t)snprintf(tmp, sizeof tmp, "%lld", (long long)v.earth));
    break;
  case W_WATER:
    sb_put(b, tmp, water_text(tmp, sizeof tmp, v.water));
    break;
  case W_FIRE:
    sb_rune(b, v.fire);
    break;
  case W_SPIRIT:
    sb_cstr(b, v.spirit ? "Light" : "Shadow");
    break;
  case W_AIR:
    sb_put(b, air_of(v)->bytes, air_of(v)->len);
    break;
  case W_KNOT:
    sb_put(b, tmp,
           (size_t)snprintf(tmp, sizeof tmp, "knot %d %d", v.knot.row, v.knot.col));
    break;
  case W_HOLD:
    if (w_is_held(v)) {
      sb_cstr(b, "Held ");
      render_nested(b, w_hold_inner(v));
    } else {
      sb_cstr(b, "Stilled");
    }
    break;
  case W_DATA: {
    WData *d = (WData *)v.obj;
    sb_cstr(b, d->name);
    for (uint32_t i = 0; i < d->nfields; i++) {
      sb_char(b, ' ');
      render_nested(b, d->fields[i]);
    }
    break;
  }
  case W_TWINE: {
    WTwine *t = (WTwine *)v.obj;
    sb_char(b, '(');
    for (size_t i = 0; i < t->len; i++) {
      if (i)
        sb_cstr(b, ", ");
      render(b, t->items[i]);
    }
    sb_char(b, ')');
    break;
  }
  case W_THREAD: {
    WThread *t = (WThread *)v.obj;
    sb_char(b, '[');
    for (size_t i = 0; i < t->len; i++) {
      if (i)
        sb_char(b, ' ');
      render_nested(b, t->items[i]);
    }
    sb_char(b, ']');
    break;
  }
  case W_PATTERN: {
    WPattern *p = (WPattern *)v.obj;
    for (size_t r = 0; r < p->rows; r++) {
      for (size_t c = 0; c < p->cols; c++)
        render(b, p->cells[r * p->cols + c]);
      sb_char(b, '\n');
    }
    break;
  }
  case W_WEB: {
    Value ps = w_web_pairs(v);
    WThread *t = (WThread *)ps.obj;
    sb_char(b, '{');
    for (size_t i = 0; i < t->len; i++) {
      if (i)
        sb_cstr(b, "  ");
      render(b, w_twine_at(t->items[i], 0));
      sb_cstr(b, " : ");
      render(b, w_twine_at(t->items[i], 1));
    }
    sb_char(b, '}');
    break;
  }
  case W_CIRCLE: {
    Value ks = w_web_keys(v);
    WThread *t = (WThread *)ks.obj;
    sb_char(b, '{');
    for (size_t i = 0; i < t->len; i++) {
      if (i)
        sb_char(b, ' ');
      render_nested(b, t->items[i]);
    }
    sb_char(b, '}');
    break;
  }
  case W_TAVEREN: {
    char tmp[48];
    sb_put(b, tmp,
           (size_t)snprintf(tmp, sizeof tmp, "<taveren %zu>", w_taveren_size(v)));
    break;
  }
  default:
    sb_cstr(b, "<function>");
  }
}

char *w_render(Value v, size_t *len) {
  SBuf b = {0};
  render(&b, v);
  *len = b.len;
  return b.bytes;
}

void w_show(Value v) {
  size_t n = 0;
  char *s = w_render(v, &n);
  if (n)
    fwrite(s, 1, n, stdout);
}

// w_print_result renders the program's output. A Thread prints one element per
// line, which is what a list of answers should look like on a terminal.
// ------------------------------------------------------------------- trace
//
// `weave trace` compiles a program that reports every top-level definition's
// value instead of only the output expression, so an editor can show each line's
// result beside it. One record per line, tab separated, with the value's own
// newlines and tabs escaped so a record is always one line.

void w_trace_text(int64_t line, const char *name, const char *text) {
  printf("%lld\t%s\t%s\n", (long long)line, name, text);
}

// Longer than this is not something an editor can show at the end of a line.
#define W_TRACE_MAX 400

void w_trace(int64_t line, const char *name, Value v) {
  size_t n = 0;
  char *s = w_render(v, &n);

  printf("%lld\t%s\t", (long long)line, name);
  size_t shown = n < W_TRACE_MAX ? n : W_TRACE_MAX;
  for (size_t i = 0; i < shown; i++) {
    switch (s[i]) {
    case '\n':
      fputs("\\n", stdout);
      break;
    case '\t':
      fputs("\\t", stdout);
      break;
    case '\\':
      fputs("\\\\", stdout);
      break;
    default:
      putchar(s[i]);
    }
  }
  if (shown < n) {
    fputs("...", stdout);
  }
  putchar('\n');
}

void w_print_result(Value v) {
  if (v.tag == W_THREAD) {
    WThread *t = (WThread *)v.obj;
    for (size_t i = 0; i < t->len; i++) {
      w_show(t->items[i]);
      putchar('\n');
    }
    return;
  }
  w_show(v);
  putchar('\n');
}
