// weave.h — the runtime for compiled Weave programs.
//
// Values are 16 bytes: a tag plus either an immediate (the unboxed Powers —
// Earth, Water, Fire, Spirit) or a pointer to a heap object. Heap objects are
// immutable once built, which is what lets the generated code share them
// freely.
//
// Memory: this runtime bump-allocates from an arena and does not collect.
// Weave programs are batch jobs over one input, so an arena is both the fastest
// thing available and by far the simplest thing to get right. What it does do
// is reuse: the places that *know* a block is dead — a buffer that has just
// outgrown its array, a trie node an owned insert has just replaced, a hash
// table a grow has just rehashed out of — hand it back with w_free, and the
// next allocation of that size takes it. See the free lists in weave.c.

#ifndef WEAVE_H
#define WEAVE_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

// ------------------------------------------------------------------- values

typedef enum {
  W_EARTH = 0, // int64
  W_WATER,     // double
  W_FIRE,      // rune
  W_SPIRIT,    // bool
  W_AIR,       // text
  W_THREAD,    // sequence
  W_HOLD,      // Held x / Stilled
  W_TWINE,
  W_CLOSURE,
  W_PATTERN, // grid
  W_KNOT,    // grid coordinate
  W_WEB,     // map
  W_CIRCLE,  // set
  W_TAVEREN, // priority queue
  W_DATA,    // a constructor of a declared sum type
} WTag;

typedef struct WObj WObj;

// A tag, four bytes that used to be padding, and eight bytes of payload.
//
// The middle four are what `Hold` is built on. A `Held` of an immediate keeps
// the value where it stands and puts the inner tag in `aux`, so `cell g k |
// otherwise 0` — which is most of how Advent of Code reads a grid — allocates
// nothing at all. Only a `Held` of something already on the heap needs a WHold.
// Nothing else reads `aux`, and the struct is the same sixteen bytes it was.
typedef struct {
  uint32_t tag;
  uint32_t aux;
  union {
    int64_t earth;
    double water;
    uint32_t fire;
    bool spirit;
    WObj *obj;
    struct {
      int32_t row, col;
    } knot;
  };
} Value;

// The two values of `aux` that are not an inner tag.
#define W_HOLD_NONE 0xFFFFFFFFu  // Stilled
#define W_HOLD_BOXED 0xFFFFFFFEu // Held, and the inner value is in a WHold

// Object header. `rc` currently records only whether an object is uniquely
// owned, which is what in-place updates need; a full reference count fits the
// same field when the allocator grows one.
struct WObj {
  uint32_t rc;
  uint32_t kind;
};

// The two states of WObj.rc.
#define W_SHARED 0
#define W_OWNED 1

typedef struct {
  WObj obj;
  size_t len;
  const char *bytes;
} WAir;

typedef struct {
  WObj obj;
  size_t len;
  Value *items;
} WThread;

typedef struct {
  WObj obj;
  size_t len;
  Value *items;
} WTwine;

typedef struct {
  WObj obj;
  Value inner;
} WHold;

// WData is a value of a sum type the program declares. `index` is the
// constructor's position in its declaration, which is what patterns test and
// what gives the type its ordering; `name` points at a string literal in the
// generated code, so rendering costs nothing to carry.
typedef struct {
  WObj obj;
  const char *name;
  uint32_t index;
  uint32_t nfields;
  Value *fields;
} WData;

typedef struct {
  WObj obj;
  size_t rows, cols;
  Value *cells; // row-major, rows*cols entries
} WPattern;

typedef struct WClosure WClosure;
typedef Value (*WFn)(Value *env, Value *args);

struct WClosure {
  WObj obj;
  WFn fn;
  int32_t arity; // how many arguments fn needs
  int32_t nargs; // how many have been supplied so far
  int32_t nenv;  // captured values, stored before the arguments
  Value *slots;  // nenv captures, then arity argument slots
};

// ---------------------------------------------------------------- immediates

static inline Value w_earth(int64_t n) {
  Value v;
  v.tag = W_EARTH;
  v.earth = n;
  return v;
}
static inline Value w_water(double d) {
  Value v;
  v.tag = W_WATER;
  v.water = d;
  return v;
}
static inline Value w_fire(uint32_t r) {
  Value v;
  v.tag = W_FIRE;
  v.fire = r;
  return v;
}
static inline Value w_spirit(bool b) {
  Value v;
  v.tag = W_SPIRIT;
  v.spirit = b;
  return v;
}
static inline Value w_knot_make(int64_t row, int64_t col) {
  Value v;
  v.tag = W_KNOT;
  v.knot.row = (int32_t)row;
  v.knot.col = (int32_t)col;
  return v;
}

#define W_LIGHT w_spirit(true)
#define W_SHADOW w_spirit(false)

// ------------------------------------------------------------------ memory

// Allocation is a bump pointer with free lists in front of it, and the whole
// fast path is here rather than in weave.c: it is a handful of instructions,
// and a program that allocates per element spends more on the call than on the
// work. See weave.c for what happens when neither has room.
#define W_SMALL_CLASSES 512
extern void *w_small[W_SMALL_CLASSES];
extern char *w_bump, *w_bump_end;
void *w_alloc_slow(size_t bytes);

static inline void *w_alloc(size_t bytes) {
  bytes = (bytes + 15) & ~(size_t)15; // keep 16-byte alignment
  if (bytes == 0)
    bytes = 16;
  size_t words = bytes >> 4;
  if (words < W_SMALL_CLASSES) {
    void *p = w_small[words];
    if (p) {
      w_small[words] = *(void **)p;
      return p;
    }
    if ((size_t)(w_bump_end - w_bump) >= bytes) {
      void *p = w_bump;
      w_bump += bytes;
      return p;
    }
  }
  return w_alloc_slow(bytes);
}

// w_free hands a block back for the next allocation of the same size to reuse.
// Only call it where nothing else can reach the block. See weave.c.
void w_free(void *p, size_t bytes);
void w_init(void);

// -------------------------------------------------------------------- text

Value w_air(const char *bytes, size_t len);
Value w_air_cstr(const char *s);
Value w_source(void); // the program input, read from stdin

// ------------------------------------------------------------------ threads

// w_thread adopts items: the caller must have allocated them with w_alloc, and
// the Thread then shares that memory. Generated code holds its literals in C
// locals instead, so it uses w_thread_copy, which takes its own arena copy and
// is safe once the frame is gone.
Value w_thread(Value *items, size_t len);
Value w_thread_copy(const Value *items, size_t len);

// w_regrow reallocates an arena buffer, copying the first n values across, and
// hands the old one back to the free list. A fused loop over an endless `flow`
// has no length to size against, so it collects into a buffer that doubles.
Value *w_regrow(Value *items, size_t n, size_t cap);

// w_thread_fit adopts a buffer that has room to spare, handing the unused tail
// back to the allocator at its own size so a later request can match it.
Value w_thread_fit(Value *items, size_t len, size_t cap);

Value w_thread_empty(void);
// w_thread_release gives a Thread's storage back. Generated code calls it where
// the compiler has proved the Thread cannot outlive the call that built it; the
// elements are left alone, since they may.
void w_thread_release(Value t);

// Reading a Thread is a load, and a fused loop does it once or twice per
// element, so it has to be inline: left as calls these were 6% of one
// benchmark, all of it call overhead around a single dereference.
static inline size_t w_thread_len(Value t) { return ((WThread *)t.obj)->len; }
static inline Value w_thread_at(Value t, size_t i) {
  return ((WThread *)t.obj)->items[i];
}

// --------------------------------------------------------------------- hold

// A Held of one of the Powers is that value with its tag moved aside; anything
// else keeps a WHold. Both are read back through w_hold_inner, and neither is
// distinguishable from inside the language.
static inline bool w_hold_fits(uint32_t tag) {
  return tag == W_EARTH || tag == W_WATER || tag == W_FIRE || tag == W_SPIRIT ||
         tag == W_KNOT;
}

Value w_held_boxed(Value inner); // the WHold case, out of line

static inline Value w_held(Value inner) {
  if (w_hold_fits(inner.tag)) {
    Value v = inner;
    v.aux = inner.tag;
    v.tag = W_HOLD;
    return v;
  }
  return w_held_boxed(inner);
}

static inline Value w_stilled(void) {
  Value v;
  v.tag = W_HOLD;
  v.aux = W_HOLD_NONE;
  v.obj = NULL;
  return v;
}

static inline bool w_is_held(Value v) { return v.aux != W_HOLD_NONE; }

Value w_hold_inner(Value v);

// --------------------------------------------------------------------- data

// w_data takes its own copy of fields, so generated code can pass a C local.
Value w_data(const char *name, uint32_t index, const Value *fields, size_t n);

static inline uint32_t w_data_index(Value v) { return ((WData *)v.obj)->index; }
static inline Value w_data_field(Value v, size_t i) {
  return ((WData *)v.obj)->fields[i];
}

// -------------------------------------------------------------------- tuple

Value w_twine(Value *items, size_t len);
Value w_twine_copy(const Value *items, size_t len);
Value w_twine_at(Value t, size_t i);

// ------------------------------------------------------------------ closure

Value w_closure(WFn fn, int arity, Value *env, int nenv);
// w_closure_value wraps a statically built closure. A function that captures
// nothing makes the same closure every time, and `w_apply` copies before it
// writes, so generated code declares one at file scope and points at it rather
// than allocating a fresh pair of blocks per call.
static inline Value w_closure_value(WClosure *c) {
  Value v;
  v.tag = W_CLOSURE;
  v.obj = &c->obj;
  return v;
}
void w_closure_env(Value v, const Value *env, int nenv);
Value w_apply(Value f, Value arg);
Value w_call(Value f, Value *args, int n);

// w_call1 and w_call2 are the shapes fused loops need. They inline the
// saturated-closure fast path so a call inside a loop is an indirect jump with
// no allocation, falling back to w_call for partially applied functions.
static inline Value w_call1(Value f, Value a) {
  if (f.tag == W_CLOSURE) {
    WClosure *c = (WClosure *)f.obj;
    if (c->nargs == 0 && c->arity == 1)
      return c->fn(c->slots, &a);
  }
  return w_call(f, &a, 1);
}

static inline Value w_call2(Value f, Value a, Value b) {
  Value args[2];
  args[0] = a;
  args[1] = b;
  if (f.tag == W_CLOSURE) {
    WClosure *c = (WClosure *)f.obj;
    if (c->nargs == 0 && c->arity == 2)
      return c->fn(c->slots, args);
  }
  return w_call(f, args, 2);
}

// -------------------------------------------------------------- collections

// Web and Circle are hash array mapped tries; Taveren is a leftist heap. See
// collections.c. A Circle is a Web whose values are ignored, so the two share
// one implementation.
uint64_t w_hash(Value v);

Value w_web_empty(void);
Value w_circle_empty(void);
Value w_web_put(Value web, Value key, Value val);
// See collections.c: `put` for a map the compiler has proved is not shared.
Value w_web_put_owned(Value web, Value key, Value val);
// w_web_reserve gives an empty map room for n entries and hands it back owned,
// so a fold that knows its length does not rehash its way up from sixteen.
Value w_web_reserve(Value web, size_t n);
Value w_web_forget(Value web, Value key);
Value w_web_forget_owned(Value web, Value key);
Value w_web_get(Value web, Value key);
bool w_web_has(Value web, Value key);
size_t w_web_size(Value web);
Value w_web_keys(Value web);
Value w_web_vals(Value web);
Value w_web_pairs(Value web);
bool w_web_equal(Value a, Value b);
// w_web_entries is `items` for a fused loop: the keys and the values as two
// parallel arrays in the same order, so no Twine is built for a pair that is
// about to be taken apart. Returns how many there are.
size_t w_web_entries(Value web, Value **keys, Value **vals);

// The table behind `remember`: private to one definition, never copied, never
// pruned, so it is a plain open-addressed table probed from the argument array
// rather than a Web. See collections.c.
typedef struct WMemo WMemo;
WMemo *w_memo_new(int arity);
bool w_memo_get(WMemo *m, const Value *args, Value *out);
void w_memo_put(WMemo *m, const Value *args, Value result);

Value w_taveren_empty(void);
Value w_taveren_push(Value heap, Value item);
Value w_taveren_pop(Value heap);
size_t w_taveren_size(Value heap);

// ------------------------------------------------------------------ prelude

// Comparison and equality work structurally across every type.
bool w_equal(Value a, Value b);
int w_compare(Value a, Value b);

// Output. w_render writes a value's text into a freshly allocated buffer,
// which both printing and `say` are built on, so there is exactly one
// definition of how each type looks.
char *w_render(Value v, size_t *len);
void w_show(Value v);
void w_print_result(Value v);

// Tracing: one tab-separated record per definition, for `weave trace`.
void w_trace(int64_t line, const char *name, Value v);
void w_trace_text(int64_t line, const char *name, const char *text);

// Errors abort with a message; they signal a bug in the compiler or a
// genuinely impossible situation, never ordinary program failure.
_Noreturn void w_fail(const char *msg);


// ------------------------------------------------------- prelude verbs
//
// These mirror the signatures declared in internal/prelude/prelude.go, in the
// same argument order. Generated code calls them directly when a call is
// saturated, and through a closure wrapper otherwise.

Value wp_abs(Value a);
Value wp_inc(Value a);
Value wp_dec(Value a);
Value wp_cells(Value a);
Value wp_col(Value a);
Value wp_cols(Value a);
Value wp_even(Value a);
Value wp_first(Value a);
Value wp_flat(Value a);
Value wp_pattern(Value a);
Value wp_holds(Value a);
Value wp_isAlpha(Value a);
Value wp_isDigit(Value a);
Value wp_isSpace(Value a);
Value wp_knots(Value a);
Value wp_last(Value a);
Value wp_lines(Value a);
Value wp_neg(Value a);
Value wp_not(Value a);
Value wp_odd(Value a);
Value wp_earth(Value a);
Value wp_fire(Value a);
Value wp_water(Value a);
Value wp_prod(Value a);
Value wp_rev(Value a);
Value wp_row(Value a);
Value wp_rows(Value a);
Value wp_fires(Value a);
Value wp_air(Value a);
Value wp_len(Value a);
Value wp_sort(Value a);
Value wp_sum(Value a);
Value wp_strip(Value a);
Value wp_uniq(Value a);
Value wp_words(Value a);
Value wp_add(Value a, Value b);
Value wp_all(Value a, Value b);
Value wp_and(Value a, Value b);
Value wp_any(Value a, Value b);
Value wp_cell(Value a, Value b);
Value wp_bend(Value a, Value b);
Value wp_count(Value a, Value b);
Value wp_div(Value a, Value b);
Value wp_divBy(Value a, Value b);
Value wp_drop(Value a, Value b);
Value wp_eq(Value a, Value b);
Value wp_gt(Value a, Value b);
Value wp_gte(Value a, Value b);
Value wp_knot(Value a, Value b);
Value wp_lt(Value a, Value b);
Value wp_lte(Value a, Value b);
Value wp_max(Value a, Value b);
Value wp_min(Value a, Value b);
Value wp_mod(Value a, Value b);
Value wp_mul(Value a, Value b);
Value wp_nb4(Value a, Value b);
Value wp_nb8(Value a, Value b);
Value wp_neq(Value a, Value b);
Value wp_or(Value a, Value b);
Value wp_otherwise(Value a, Value b);
Value wp_rescue(Value a, Value b);
Value wp_snag(Value a, Value b);
Value wp_dijkstra(Value a, Value b);
Value wp_reach(Value step, Value start);
Value wp_route(Value step, Value start, Value goal);
Value wp_toposort(Value step, Value nodes);
Value wp_cellwise(Value a, Value b);
Value wp_zipwith(Value a, Value b, Value c);
// Building and editing a Thread, which until these existed could be read every
// way there is and built no way at all.
Value wp_thread(Value tw);
Value wp_weld(Value extra, Value xs);
Value wp_mend(Value at, Value v, Value xs);
Value wp_mend_owned(Value at, Value v, Value xs);
Value wp_sever(Value n, Value xs);
Value wp_strands(Value key, Value xs);
Value wp_plait(Value as, Value bs);
Value wp_cull(Value p, Value xs);
Value wp_bendr(Value a, Value b);
Value wp_siftr(Value a, Value b);
Value wp_zipr(Value a, Value b, Value c);
Value wp_sums(Value a);
Value wp_prods(Value a);
Value wp_seek(Value a, Value b);
Value wp_sift(Value a, Value b);
Value wp_span(Value a, Value b);
Value wp_split(Value a, Value b);
Value wp_join(Value a, Value b);
Value wp_sub(Value a, Value b);
Value wp_take(Value a, Value b);
Value wp_zip(Value a, Value b);
Value wp_braid(Value a, Value b, Value c);
Value wp_pick(Value a, Value b, Value c);
Value wp_set(Value a, Value b, Value c);
Value wp_set_owned(Value a, Value b, Value c);
Value wp_put_owned(Value a, Value b, Value c);
Value wp_insert_owned(Value a, Value b);
Value wp_forget_owned(Value web, Value key);
Value wp_remove_owned(Value circle, Value v);


// Collections.
Value wp_web(Value pairs);
Value wp_get(Value web, Value key);
Value wp_put(Value web, Value key, Value val);
Value wp_known(Value web, Value key);
Value wp_forget(Value web, Value key);
Value wp_keys(Value web);
Value wp_vals(Value web);
Value wp_items(Value web);
Value wp_merge(Value a, Value b);
Value wp_freq(Value xs);
Value wp_most(Value web);
Value wp_circle(Value xs);
Value wp_member(Value circle, Value v);
Value wp_insert(Value circle, Value v);
Value wp_remove(Value circle, Value v);
Value wp_members(Value circle);
Value wp_taveren(Value xs);
Value wp_push(Value heap, Value v);
Value wp_pop(Value heap);

// Extra sequence and text verbs.
Value wp_earths(Value text);
Value wp_waters(Value text);
Value wp_chunk(Value n, Value xs);
Value wp_windows(Value n, Value xs);
Value wp_pivot(Value xss);
Value wp_gcd(Value a, Value b);
Value wp_lcm(Value a, Value b);
Value wp_sortby(Value key, Value xs);
Value wp_group(Value key, Value xs);
Value wp_idx(Value needle, Value xs);
Value wp_nth(Value n, Value xs);
Value wp_has(Value needle, Value xs);
Value wp_glean(Value f, Value xs);
Value wp_harvest(Value f, Value xs);
Value wp_weft(Value fill, Value rows);
Value wp_spin(Value g);
Value wp_flip(Value g);
Value wp_perms(Value xs);
Value wp_contains(Value needle, Value text);


// w_disown marks an object shared again. The compiler emits it where a grid a
// loop owns escapes to its caller, since the caller may keep it.
static inline Value w_disown(Value v) {
  if (v.obj && (v.tag == W_PATTERN || v.tag == W_WEB || v.tag == W_CIRCLE))
    v.obj->rc = W_SHARED;
  return v;
}

// ------------------------------------------------- typed primitive operations
//
// The checker knows the type of every expression, so the code generator can
// call these instead of the tag-dispatching verbs in prelude.c. That removes a
// branch per arithmetic operation and, for comparisons, replaces an
// out-of-line call to w_compare with a machine instruction. They are the same
// operations, specialised: `add` at Earth is w_add_e.
//
// Argument order matches the prelude, so the comparisons read `gt bound value`
// and answer "is value greater than bound".

// An Earth is an int64, and int64 arithmetic wraps. Advent of Code produces
// numbers in the 10^14 range often enough that a wrapped answer is a real
// hazard — and a wrong answer that looks like an answer is the worst outcome
// there is. `weave build -overflow` compiles these with a check, which turns
// a silently wrong result into a stopped program that says which verb did it.
//
// It is off by default because it is not free: measured on the benchmark
// suite, a tight arithmetic loop pays 30-75% and a program that also touches
// memory pays under 5%. See TODO.md.
#ifdef WEAVE_CHECK_OVERFLOW
static inline Value w_add_e(Value a, Value b) {
  int64_t r;
  if (__builtin_add_overflow(a.earth, b.earth, &r))
    w_fail("`add` overflowed: an Earth holds numbers up to 9223372036854775807");
  return w_earth(r);
}
static inline Value w_sub_e(Value a, Value b) {
  int64_t r;
  if (__builtin_sub_overflow(a.earth, b.earth, &r))
    w_fail("`sub` overflowed: an Earth holds numbers down to -9223372036854775808");
  return w_earth(r);
}
static inline Value w_mul_e(Value a, Value b) {
  int64_t r;
  if (__builtin_mul_overflow(a.earth, b.earth, &r))
    w_fail("`mul` overflowed: an Earth holds numbers up to 9223372036854775807");
  return w_earth(r);
}
#else
static inline Value w_add_e(Value a, Value b) { return w_earth(a.earth + b.earth); }
static inline Value w_sub_e(Value a, Value b) { return w_earth(a.earth - b.earth); }
static inline Value w_mul_e(Value a, Value b) { return w_earth(a.earth * b.earth); }
#endif
static inline Value w_neg_e(Value a) { return w_earth(-a.earth); }
static inline Value w_abs_e(Value a) { return w_earth(a.earth < 0 ? -a.earth : a.earth); }

static inline Value w_div_e(Value a, Value b) {
  if (b.earth == 0)
    w_fail("divided by zero");
  return w_earth(a.earth / b.earth);
}

static inline Value w_mod_e(Value a, Value b) {
  if (b.earth == 0)
    w_fail("took a remainder by zero");
  return w_earth(a.earth % b.earth);
}

static inline Value w_even_e(Value a) { return w_spirit(a.earth % 2 == 0); }
static inline Value w_odd_e(Value a) { return w_spirit(a.earth % 2 != 0); }

static inline Value w_divby_e(Value d, Value n) {
  if (d.earth == 0)
    w_fail("divBy was given zero");
  return w_spirit(n.earth % d.earth == 0);
}

static inline Value w_add_w(Value a, Value b) { return w_water(a.water + b.water); }
static inline Value w_sub_w(Value a, Value b) { return w_water(a.water - b.water); }
static inline Value w_mul_w(Value a, Value b) { return w_water(a.water * b.water); }
static inline Value w_div_w(Value a, Value b) { return w_water(a.water / b.water); }
static inline Value w_neg_w(Value a) { return w_water(-a.water); }
static inline Value w_abs_w(Value a) { return w_water(a.water < 0 ? -a.water : a.water); }

static inline Value w_min_e(Value a, Value b) { return a.earth <= b.earth ? a : b; }
static inline Value w_max_e(Value a, Value b) { return a.earth >= b.earth ? a : b; }
static inline Value w_min_w(Value a, Value b) { return a.water <= b.water ? a : b; }
static inline Value w_max_w(Value a, Value b) { return a.water >= b.water ? a : b; }

static inline Value w_eq_e(Value a, Value b) { return w_spirit(a.earth == b.earth); }
static inline Value w_neq_e(Value a, Value b) { return w_spirit(a.earth != b.earth); }
static inline Value w_lt_e(Value b, Value a) { return w_spirit(a.earth < b.earth); }
static inline Value w_lte_e(Value b, Value a) { return w_spirit(a.earth <= b.earth); }
static inline Value w_gt_e(Value b, Value a) { return w_spirit(a.earth > b.earth); }
static inline Value w_gte_e(Value b, Value a) { return w_spirit(a.earth >= b.earth); }

static inline Value w_eq_w(Value a, Value b) { return w_spirit(a.water == b.water); }
static inline Value w_neq_w(Value a, Value b) { return w_spirit(a.water != b.water); }
static inline Value w_lt_w(Value b, Value a) { return w_spirit(a.water < b.water); }
static inline Value w_lte_w(Value b, Value a) { return w_spirit(a.water <= b.water); }
static inline Value w_gt_w(Value b, Value a) { return w_spirit(a.water > b.water); }
static inline Value w_gte_w(Value b, Value a) { return w_spirit(a.water >= b.water); }

static inline Value w_eq_f(Value a, Value b) { return w_spirit(a.fire == b.fire); }
static inline Value w_neq_f(Value a, Value b) { return w_spirit(a.fire != b.fire); }
static inline Value w_lt_f(Value b, Value a) { return w_spirit(a.fire < b.fire); }
static inline Value w_lte_f(Value b, Value a) { return w_spirit(a.fire <= b.fire); }
static inline Value w_gt_f(Value b, Value a) { return w_spirit(a.fire > b.fire); }
static inline Value w_gte_f(Value b, Value a) { return w_spirit(a.fire >= b.fire); }

static inline Value w_eq_s(Value a, Value b) { return w_spirit(a.spirit == b.spirit); }
static inline Value w_neq_s(Value a, Value b) { return w_spirit(a.spirit != b.spirit); }

static inline Value w_and_s(Value a, Value b) { return w_spirit(a.spirit && b.spirit); }
static inline Value w_or_s(Value a, Value b) { return w_spirit(a.spirit || b.spirit); }
static inline Value w_not_s(Value a) { return w_spirit(!a.spirit); }


// Verbs added alongside the rask-aligned naming pass.

Value wp_bin(Value a);
Value wp_blocks(Value a);
Value wp_bnot(Value a);
Value wp_cbrt(Value a);
Value wp_ceil(Value a);
Value wp_spark(Value a);
Value wp_digit(Value a);
Value wp_compact(Value a);
Value wp_enum(Value a);
Value wp_floor(Value a);
Value wp_head(Value a);
Value wp_lower(Value a);
Value wp_ord(Value a);
Value wp_pairs(Value a);
Value wp_round(Value a);
Value wp_second(Value a);
Value wp_shape(Value a);
Value wp_sign(Value a);
Value wp_sqrt(Value a);
Value wp_tail(Value a);
Value wp_upper(Value a);
Value wp_around4(Value a, Value b);
Value wp_around8(Value a, Value b);
Value wp_band(Value a, Value b);
Value wp_bor(Value a, Value b);
Value wp_bot(Value a, Value b);
Value wp_bxor(Value a, Value b);

Value wp_combos(Value a, Value b);
Value wp_cross(Value a, Value b);
Value wp_cutend(Value a, Value b);
Value wp_cutstart(Value a, Value b);
Value wp_diff(Value a, Value b);
Value wp_dropwhile(Value a, Value b);
Value wp_ends(Value a, Value b);
Value wp_inb(Value a, Value b);
Value wp_inter(Value a, Value b);
Value wp_mapcat(Value a, Value b);
Value wp_mapvals(Value a, Value b);
Value wp_maxby(Value a, Value b);
Value wp_mdist(Value a, Value b);
Value wp_minby(Value a, Value b);
Value wp_none(Value a, Value b);
Value wp_pow(Value a, Value b);
Value wp_repeat(Value a, Value b);
Value wp_shl(Value a, Value b);
Value wp_shr(Value a, Value b);
Value wp_starts(Value a, Value b);
Value wp_takewhile(Value a, Value b);
Value wp_top(Value a, Value b);
Value wp_union(Value a, Value b);
Value wp_clamp(Value a, Value b, Value c);
Value wp_padl(Value a, Value b, Value c);
Value wp_padr(Value a, Value b, Value c);
Value wp_replace(Value a, Value b, Value c);
// delve takes a line apart against a shape: `{}` is a run to keep, everything
// else has to match exactly, and the shape has to account for the whole line.
Value wp_delve(Value shape, Value text);
Value wp_scan(Value a, Value b, Value c);
Value wp_dupe(Value xs);
Value wp_high(Value xs);
Value wp_low(Value xs);
Value wp_highidx(Value xs);
Value wp_lowidx(Value xs);
Value wp_seekidx(Value p, Value xs);
Value wp_twist(Value at, Value f, Value xs);
Value wp_twist_owned(Value at, Value f, Value xs);
Value wp_overlaps(Value a, Value b);
Value wp_overlapping(Value a, Value b);
Value wp_within(Value outer, Value inner);
Value wp_spanning(Value a, Value b);
Value wp_holding(Value r, Value v);
Value wp_width(Value r);
Value wp_gentle(Value f, Value seed, Value xs);
Value wp_dirs4(void);
Value wp_dirs8(void);
Value wp_e(void);
Value wp_inf(void);
Value wp_pi(void);
#endif // WEAVE_H
