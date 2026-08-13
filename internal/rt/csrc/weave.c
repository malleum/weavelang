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
  uint64_t serial; // when it was taken, so a region knows what is its own
  char data[];
} Chunk;

static Chunk *g_chunk = NULL;
static const size_t CHUNK_MIN = 1u << 20;
static uint64_t g_serial;
// How many blocks have been handed back. A region only has to empty the free
// lists if something was freed while it was open — a block freed before it was
// opened lives outside it and is still good.
static uint64_t g_frees;

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

// A ceiling on what the program may take from the operating system, in bytes,
// and what it has taken so far. Zero is no ceiling, which is what every
// ordinary run has: a batch job that is given an input is entitled to whatever
// that input costs.
//
// `weave trace` is the exception, because it runs a program nobody asked to
// run, on a file that is being typed into. A definition that means to allocate
// for ever should cost that file its own line's ghost text and nothing else —
// certainly not the machine the editor is running on. So the ceiling comes in
// through the environment rather than through the program, and the tracer sets
// it. See W_EXIT_OVER_MEMORY.
//
// A chunk is never handed back, so what has been taken is also the high-water
// mark, and one comparison per megabyte is not a cost worth measuring.
static size_t g_mem_cap;
static size_t g_mem_taken;

void w_init(void) {
  const char *cap = getenv("WEAVE_MEM_CAP");
  if (cap && *cap) {
    g_mem_cap = (size_t)strtoull(cap, NULL, 10);
  }
#ifdef WEAVE_TALLY
  w_tally_start();
#endif
}

// w_over_memory stops a program that has gone past the ceiling. Whatever it
// reported before it did stands, which is the whole point, so the records go
// out before the door closes.
static _Noreturn void w_over_memory(size_t want) {
  fflush(stdout);
  fprintf(stderr, "weave: past the %zu byte ceiling, asking for %zu more\n",
          g_mem_cap, want);
  fflush(stderr);
  _Exit(W_EXIT_OVER_MEMORY);
}

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
  // Every byte the program takes from the operating system passes through
  // here, so this is the only place the ceiling has to be checked.
  g_mem_taken += sizeof(Chunk) + cap;
  if (g_mem_cap && g_mem_taken > g_mem_cap)
    w_over_memory(sizeof(Chunk) + cap);

  Chunk *c = (Chunk *)malloc(sizeof(Chunk) + cap);
  if (!c)
    w_fail("out of memory");
  c->cap = cap;
#ifdef WEAVE_TALLY
  w_tally_chunk(sizeof(Chunk) + cap);
#endif
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
void w_free_impl(void *p, size_t bytes) {
  if (!p)
    return;
  g_frees++;
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

// ------------------------------------------------------------------ regions
//
// A backtracking search is the one shape no ownership analysis will ever help.
// It uses the collection it was handed once *per option*, not once — the old
// value has to survive for the next branch — so every branch it tries copies,
// and every copy is dead the moment the branch is abandoned. Advent of Code
// 2025 day 10 held 296 MB at exit and had freed none of it: 1.1 million copied
// rows, one per node of a search whose answer is a single number.
//
// What a search wants is not to free those one at a time but to forget them all
// at once. The arena is a bump pointer, so it can: mark where it has got to,
// let a turn of the loop allocate whatever it likes, and put the pointer back.
//
// The whole safety of it is one condition — nothing allocated inside the region
// may be reachable after it — and that is a question about the loop, not about
// the arena. See regions.go in the compiler for what it takes to answer yes.

WMark w_mark(void) {
  WMark m;
  m.chunk = g_chunk;
  m.bump = w_bump;
  m.bump_end = w_bump_end;
  m.serial = g_serial;
  m.frees = g_frees;
  return m;
}

void w_release(WMark m) {
  // Every chunk taken since the mark goes back, wherever it sits in the list:
  // a block too big for a chunk gets one of its own spliced in behind the
  // current one, so position says nothing about age and the serial does.
  Chunk **link = &g_chunk;
  while (*link) {
    Chunk *c = *link;
    if (c->serial > m.serial) {
      *link = c->prev;
      free(c);
      continue;
    }
    link = &c->prev;
  }
  g_chunk = m.chunk;
  w_bump = m.bump;
  w_bump_end = m.bump_end;

  // A free list holding a block inside the region would hand out storage that
  // has just been forgotten, and hand it out twice. Only worth emptying when
  // something was actually freed while the region was open.
  if (g_frees != m.frees) {
    memset(w_small, 0, sizeof(w_small));
    memset(g_large, 0, sizeof(g_large));
    g_frees = m.frees;
  }
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
  t->elems = items;
  t->raw = NULL;
  Value v;
  v.tag = W_THREAD;
  v.obj = &t->obj;
  return v;
}

Value w_thread_packed(int64_t *raw, size_t len, uint32_t elem) {
  WThread *t = (WThread *)w_alloc(sizeof(WThread));
  t->obj.rc = W_SHARED;
  t->obj.kind = elem;
  t->len = len;
  t->elems = NULL;
  t->raw = raw;
  Value v;
  v.tag = W_THREAD;
  v.obj = &t->obj;
  return v;
}

Value w_thread_packed_fit(int64_t *raw, size_t len, size_t cap, uint32_t elem) {
  // Values are sixteen bytes and payloads are eight, which is the one way this
  // is not w_thread_fit. A tail that starts on an odd element does not start on
  // a boundary the allocator knows about: it rounds a free up to sixteen, so
  // handing back `cap - len` payloads gives away eight bytes that are still the
  // Thread's, and the next thirty-two byte request is handed a block sitting on
  // top of the elements. So the kept part is rounded up to a pair and the tail
  // starts there. The rounding also makes the block exactly the size the frees
  // in thr_boxed and w_thread_release ask for it back at.
  size_t keep = (len + 1) & ~(size_t)1;
  if (cap > keep) {
#ifdef WEAVE_TALLY
    w_tally_shrink(raw, sizeof(int64_t) * cap, sizeof(int64_t) * keep);
    w_free_impl(raw + keep, sizeof(int64_t) * (cap - keep));
#else
    w_free(raw + keep, sizeof(int64_t) * (cap - keep));
#endif
  }
  return w_thread_packed(raw, len, elem);
}

// thr_boxed gives a bulk verb the array of Values it wants.
//
// A packed Thread is unpacked in place rather than copied out, so the cost is
// paid once however many verbs ask: the widened array replaces the payloads,
// and the Thread is an ordinary one from then on.
//
// The payloads are not handed back, and that is deliberate. `take` and `drop`
// give out windows on them, so a Thread that is unpacked may not be the only
// one pointing at what it was storing — and there is nothing in the header that
// would say. Freeing them would put a block that is still being read on a free
// list. Half the width of the array that replaced them, left in the arena, is
// the price of not needing to know.
Value *thr_boxed(WThread *t) {
  if (t->elems != NULL)
    return t->elems;
  size_t n = t->len;
  Value *out = (Value *)w_alloc(sizeof(Value) * (n ? n : 1));
  Value v;
  v.tag = t->obj.kind & W_THR_TAG;
  v.aux = 0;
  for (size_t i = 0; i < n; i++) {
    v.earth = t->raw[i];
    out[i] = v;
  }
  t->raw = NULL;
  t->obj.kind = W_THREAD;
  t->elems = out;
  return out;
}

Value thr_window(WThread *t, size_t at, size_t len) {
  if (t->elems != NULL)
    return w_thread(t->elems + at, len);
  return w_thread_packed(t->raw + at, len, (t->obj.kind & W_THR_TAG) | W_THR_BORROWED);
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
//
// This is the one place that frees part of a block rather than all of it, which
// is why the tally build needs telling: the tail's address is not something it
// ever recorded, so the free goes straight to the allocator and the shrink is
// reported against the block the tail came out of.
Value w_thread_fit(Value *items, size_t len, size_t cap) {
  if (cap > len) {
#ifdef WEAVE_TALLY
    w_tally_shrink(items, sizeof(Value) * cap, sizeof(Value) * len);
    w_free_impl(items + len, sizeof(Value) * (cap - len));
#else
    w_free(items + len, sizeof(Value) * (cap - len));
#endif
  }
  return w_thread(items, len);
}

// The empty Thread is one object for the whole program. A Thread of no
// elements has nothing to distinguish one from another, and `else []` is how
// every program says "nothing was there" — Advent of Code 2025 day 8 built six
// million of them, one per element it read out of a Thread of Threads, for 183
// MB of headers holding nothing.
//
// It is never owned, so an update to it copies like any other shared
// collection, and w_thread_release knows to leave it alone.
static WThread g_empty = {{W_SHARED, W_THREAD}, 0, NULL, NULL};

Value w_thread_empty(void) {
  Value v;
  v.tag = W_THREAD;
  v.aux = 0;
  v.obj = &g_empty.obj;
  return v;
}

// w_thread_release hands a Thread's storage back to the allocator. Only
// generated code calls it, and only where the compiler has proved the Thread
// dies with the call that built it — see internal/codegen/escape.go. The
// elements are not touched: whatever they point at may well outlive this.
void w_thread_release(Value t) {
  if (t.tag != W_THREAD || !t.obj || t.obj == &g_empty.obj)
    return;
  WThread *th = (WThread *)t.obj;
  if (th->elems)
    w_free(th->elems, sizeof(Value) * th->len);
  else if (th->raw && th->len && !(th->obj.kind & W_THR_BORROWED))
    w_free(th->raw, sizeof(int64_t) * th->len);
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

// w_regrow_packed is w_regrow for a buffer of payloads. The old capacity is
// even wherever this is used — a doubling buffer that started at sixteen — so
// the free asks for exactly the block that was taken.
int64_t *w_regrow_packed(int64_t *raw, size_t n, size_t cap) {
  int64_t *bigger = (int64_t *)w_alloc(sizeof(int64_t) * (cap ? cap : 1));
  if (n) {
    memcpy(bigger, raw, sizeof(int64_t) * n);
    w_free(raw, sizeof(int64_t) * n);
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

// The slots a saturating application can keep on the C stack. Sixteen covers
// the environment and the arguments of anything a program actually writes;
// past it the heap path below still works.
#define W_APPLY_SLOTS 16

// w_apply supplies one argument, running the function once it is saturated.
// A partially applied closure is copied so the original stays reusable.
//
// Unless this argument is the last one. Then nothing has to outlive the call:
// the slots go on the C stack and the closure is not copied at all. That is the
// common shape by a long way — a predicate given its captured argument and then
// called once per element, `knots g | sift (reachable g)` — and it used to cost
// two allocations *per element*, which was 87 MB of Advent of Code 2025 day 4's
// 105.
Value w_apply(Value f, Value arg) {
  if (f.tag != W_CLOSURE)
    w_fail("tried to call something that is not a function");
  WClosure *c = (WClosure *)f.obj;

  if (c->nargs + 1 == c->arity && c->nenv + c->arity <= W_APPLY_SLOTS) {
    Value slots[W_APPLY_SLOTS];
    memcpy(slots, c->slots, sizeof(Value) * (size_t)(c->nenv + c->nargs));
    slots[c->nenv + c->nargs] = arg;
    return c->fn(slots, slots + c->nenv);
  }

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
  case W_PATTERN: {
    // Reading order, which is the order `cells` and `knots` walk, so a Pattern
    // sorts the way the text it was read from sorts. Shape breaks a tie only
    // where one grid runs out, and rows before columns because a grid one row
    // shorter is the smaller thing however wide it is.
    WPattern *x = (WPattern *)a.obj, *y = (WPattern *)b.obj;
    size_t xn = x->rows * x->cols, yn = y->rows * y->cols;
    size_t n = xn < yn ? xn : yn;
    for (size_t i = 0; i < n; i++) {
      int r = w_compare(x->cells[i], y->cells[i]);
      if (r)
        return r;
    }
    int r = cmp_i64((int64_t)x->rows, (int64_t)y->rows);
    return r ? r : cmp_i64((int64_t)x->cols, (int64_t)y->cols);
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
    size_t n = alen < blen ? alen : blen;
    // Element at a time rather than array at a time, so that comparing a
    // packed Thread does not unpack it: an ordering is a question about the
    // elements and has no business changing how they are stored.
    for (size_t i = 0; i < n; i++) {
      Value x = a.tag == W_THREAD ? w_thread_at(a, i) : ((WTwine *)a.obj)->items[i];
      Value y = b.tag == W_THREAD ? w_thread_at(b, i) : ((WTwine *)b.obj)->items[i];
      int r = w_compare(x, y);
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

// quoted writes text the way it was written in the source: in double quotes,
// with the escapes the lexer accepts put back.
static void quoted(SBuf *b, Value v) {
  WAir *a = air_of(v);
  sb_char(b, '"');
  for (size_t i = 0; i < a->len; i++) {
    char c = a->bytes[i];
    switch (c) {
    case '"':
      sb_cstr(b, "\\\"");
      break;
    case '\\':
      sb_cstr(b, "\\\\");
      break;
    case '\n':
      sb_cstr(b, "\\n");
      break;
    case '\r':
      sb_cstr(b, "\\r");
      break;
    case '\t':
      sb_cstr(b, "\\t");
      break;
    case '\0':
      sb_cstr(b, "\\0");
      break;
    default:
      sb_char(b, c);
    }
  }
  sb_char(b, '"');
}

// render_item renders a value inside a collection, where text is quoted.
//
// Text is quoted. Without it `["a b", "c"]` and `["a", "b", "c"]` both come out
// as `[a b c]`, and there is no reading that recovers which one it was. At the
// top level the text *is* the answer, so it stays bare there — a program that
// prints a line wants the line, not the line in quotes.
//
// render_nested is that plus bracketing, for the positions where values are
// separated by nothing but a space: without it `[knot 1 2, knot 3 4]` comes
// back as `knot 1 2 knot 3 4`. A comma or a ` : ` keeps neighbours apart on its
// own, so a Twine and a Web quote without bracketing.
static void render_item(SBuf *b, Value v) {
  if (v.tag == W_AIR) {
    quoted(b, v);
    return;
  }
  render(b, v);
}

static void render_nested(SBuf *b, Value v) {
  bool wrap = v.tag != W_AIR && reads_as_application(v);
  if (wrap)
    sb_char(b, '(');
  render_item(b, v);
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
      render_item(b, t->items[i]);
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
      render_nested(b, thr_at(t, i));
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
      render_item(b, w_twine_at(thr_at(t, i), 0));
      sb_cstr(b, " : ");
      render_item(b, w_twine_at(thr_at(t, i), 1));
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
      render_nested(b, thr_at(t, i));
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
  case W_LINK: {
    // A Link is its circles: what it holds is who is joined to whom, and a
    // count of nodes would say nothing about that.
    Value cs = wp_clumped(v);
    size_t n = w_thread_len(cs);
    sb_cstr(b, "<link");
    for (size_t i = 0; i < n; i++) {
      sb_char(b, ' ');
      render_nested(b, w_thread_at(cs, i));
    }
    sb_char(b, '>');
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
//
// Every record is flushed as it is written. Tracing is run under a time limit —
// a definition that will not finish is not allowed to keep the rest of the file
// quiet — and a program that is killed part way through never returns from
// printf's buffer. Flushing means the lines that did report have already left
// the building when the axe falls.

void w_trace_text(int64_t line, const char *name, const char *text) {
  printf("%lld\t%s\t%s\n", (long long)line, name, text);
  fflush(stdout);
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
  fflush(stdout);
}

// --------------------------------------------------------- watching a call
//
// Ghost text answers "what does this line hold", and a line inside a function
// body does not hold one thing — it holds a different thing on every call. So
// the inside of a function is the one place in a program with no ghost text,
// and it is where a bug is most likely to be.
//
// `weave trace -watch f` records what f's names held on each call instead. The
// records go out on the same stream in the same shape as the others, marked
// with a leading `@` so that anything reading the by-line records skips them:
//
//	@LINE<TAB>CALL<TAB>NAME<TAB>VALUE
//
// A recursion that is being debugged runs millions of times and a window can
// show a few dozen, so what is kept is bounded at both ends: the first calls
// are written as they happen, and the last are held in a ring and written when
// the program stops. Between them is a count, because "it ran 27 763 113 times"
// is itself an answer. The head is where a base case that never fires shows up
// and the tail is where a loop that will not settle does, and neither is in the
// middle.

#define W_CALL_HEAD 24
#define W_CALL_TAIL 24

typedef struct {
  int64_t call;  // which call these belong to, or 0 for an unused slot
  char **texts;  // the rendered records, in the order they were made
  size_t n, cap;
} CallSlot;

static CallSlot g_call_ring[W_CALL_TAIL];
static int64_t g_call_no;

// w_watch_enter starts a call and hands back its number. The number is the
// caller's to keep, in a local, because a function that recurses is inside
// several calls at once: a global counter would file what the outermost call
// answered under whatever number the innermost had reached.
//
// The slot this call will use is emptied now rather than when it is
// overwritten, so the ring holds whole calls and never half of one.
int64_t w_watch_enter(void) {
  g_call_no++;
  if (g_call_no > W_CALL_HEAD) {
    CallSlot *slot = &g_call_ring[g_call_no % W_CALL_TAIL];
    for (size_t i = 0; i < slot->n; i++)
      free(slot->texts[i]);
    slot->n = 0;
    slot->call = g_call_no;
  }
  return g_call_no;
}

// call_record renders one record. The value's own newlines and tabs are
// escaped, exactly as w_trace escapes them, so a record is always one line.
static char *call_record(int64_t call, int64_t line, const char *name, Value v) {
  size_t n = 0;
  char *s = w_render(v, &n);
  size_t shown = n < W_TRACE_MAX ? n : W_TRACE_MAX;

  // Two bytes per shown byte covers every escape, plus the header and the
  // ellipsis and the newline.
  size_t cap = shown * 2 + strlen(name) + 64;
  char *out = (char *)malloc(cap);
  if (!out)
    return NULL;
  int head = snprintf(out, cap, "@%lld\t%lld\t%s\t", (long long)line,
                      (long long)call, name);
  size_t w = (size_t)head;
  for (size_t i = 0; i < shown; i++) {
    switch (s[i]) {
    case '\n':
      out[w++] = '\\';
      out[w++] = 'n';
      break;
    case '\t':
      out[w++] = '\\';
      out[w++] = 't';
      break;
    case '\\':
      out[w++] = '\\';
      out[w++] = '\\';
      break;
    default:
      out[w++] = s[i];
    }
  }
  if (shown < n) {
    memcpy(out + w, "...", 3);
    w += 3;
  }
  out[w] = '\0';
  return out;
}

void w_watch(int64_t call, int64_t line, const char *name, Value v) {
  if (call <= W_CALL_HEAD) {
    // Written as it happens, and flushed, so a run that is cut short by a
    // limit still shows how the first calls went.
    char *rec = call_record(call, line, name, v);
    if (!rec)
      return;
    puts(rec);
    fflush(stdout);
    free(rec);
    return;
  }
  CallSlot *slot = &g_call_ring[call % W_CALL_TAIL];
  // A recursion deep enough to wrap the ring while an outer call is still
  // running has had that call's slot taken by an inner one. What it went on to
  // answer is dropped rather than filed under somebody else's number.
  if (slot->call != call)
    return;
  if (slot->n == slot->cap) {
    size_t cap = slot->cap ? slot->cap * 2 : 8;
    char **grown = (char **)realloc(slot->texts, sizeof(char *) * cap);
    if (!grown)
      return;
    slot->texts = grown;
    slot->cap = cap;
  }
  char *rec = call_record(call, line, name, v);
  if (rec)
    slot->texts[slot->n++] = rec;
}

// w_watch_flush writes the count and then the calls the ring held, in order.
// It runs when the program stops of its own accord; a run cut short by a limit
// has already written its head, which is the half that matters most.
void w_watch_flush(void) {
  if (g_call_no == 0)
    return;
  printf("@0\t0\tcalls\t%lld\n", (long long)g_call_no);
  for (int64_t call = g_call_no - W_CALL_TAIL + 1; call <= g_call_no; call++) {
    if (call <= W_CALL_HEAD)
      continue;
    CallSlot *slot = &g_call_ring[call % W_CALL_TAIL];
    if (slot->call != call)
      continue;
    for (size_t i = 0; i < slot->n; i++)
      puts(slot->texts[i]);
  }
  fflush(stdout);
}

void w_print_result(Value v) {
  if (v.tag == W_THREAD) {
    WThread *t = (WThread *)v.obj;
    for (size_t i = 0; i < t->len; i++) {
      w_show(thr_at(t, i));
      putchar('\n');
    }
    return;
  }
  w_show(v);
  putchar('\n');
}
