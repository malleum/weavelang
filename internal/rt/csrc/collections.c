// collections.c — Web, Circle and Taveren.
//
// Web and Circle have two representations behind one set of verbs. By default
// they are hash array mapped tries: 32-way branching on successive 5-bit slices
// of a key's hash, with path copying on insert. That keeps them genuinely
// immutable while an insert touches only log32(n) nodes — about four for a
// million entries — so building a map inside a fold stays linear instead of
// degrading to the quadratic copying a flat table would cost.
//
// A map the compiler has proved unshared, with immediate keys, is instead a
// flat open-addressed table, which is faster and smaller and needs none of what
// the trie buys. It turns back into a trie the moment it is used persistently.
// See the flat table section below.
//
// Taveren is a leftist heap, the standard purely functional priority queue:
// merge is O(log n), and push and pop are both defined in terms of it.

#include "weave.h"

#include <stdlib.h>
#include <string.h>

// --------------------------------------------------------------------- hash

// FNV-1a over a value's structure. It must agree with w_equal: values that
// compare equal have to hash equal.
static void hash_bytes(uint64_t *h, const void *p, size_t n) {
  const unsigned char *b = (const unsigned char *)p;
  for (size_t i = 0; i < n; i++) {
    *h ^= b[i];
    *h *= 1099511628211ULL;
  }
}

static void hash_value(uint64_t *h, Value v) {
  hash_bytes(h, &v.tag, sizeof v.tag);
  switch (v.tag) {
  case W_EARTH:
    hash_bytes(h, &v.earth, sizeof v.earth);
    break;
  case W_WATER:
    hash_bytes(h, &v.water, sizeof v.water);
    break;
  case W_FIRE:
    hash_bytes(h, &v.fire, sizeof v.fire);
    break;
  case W_SPIRIT: {
    unsigned char b = v.spirit ? 1 : 0;
    hash_bytes(h, &b, 1);
    break;
  }
  case W_KNOT:
    hash_bytes(h, &v.knot, sizeof v.knot);
    break;
  case W_AIR: {
    WAir *a = (WAir *)v.obj;
    hash_bytes(h, a->bytes, a->len);
    break;
  }
  case W_HOLD:
    if (w_is_held(v))
      hash_value(h, w_hold_inner(v));
    break;
  case W_TWINE: {
    WTwine *t = (WTwine *)v.obj;
    for (size_t i = 0; i < t->len; i++)
      hash_value(h, t->items[i]);
    break;
  }
  case W_DATA: {
    WData *d = (WData *)v.obj;
    hash_bytes(h, &d->index, sizeof d->index);
    for (uint32_t i = 0; i < d->nfields; i++)
      hash_value(h, d->fields[i]);
    break;
  }
  case W_THREAD: {
    WThread *t = (WThread *)v.obj;
    for (size_t i = 0; i < t->len; i++)
      hash_value(h, t->items[i]);
    break;
  }
  default:
    break;
  }
}

uint64_t w_hash(Value v) {
  uint64_t h = 14695981039346656037ULL;
  hash_value(&h, v);
  return h;
}

// --------------------------------------------------------------------- HAMT

#define HAMT_BITS 5
#define HAMT_WIDTH 32
#define HAMT_MASK (HAMT_WIDTH - 1)
// Past this shift the hash is exhausted and equal-hash keys share a node.
#define HAMT_MAX_SHIFT 60

typedef struct Hamt Hamt;

// A slot holds either a key/value pair or a child node.
typedef struct {
  bool is_node;
  union {
    struct {
      Value key, val;
    } leaf;
    Hamt *node;
  };
} Slot;

struct Hamt {
  uint32_t bitmap; // which of the 32 positions are filled
  uint32_t count;  // entries in this subtree
  bool collision;  // a bucket of equal-hash keys, stored linearly
  // owned means exactly one node points at this one, so an update that has
  // proved the whole map unshared may write through it rather than copy it.
  // See the in-place section below.
  bool owned;
  uint32_t nslots; // slots in use
  uint32_t cap;    // slots allocated; never less than nslots
  Slot slots[];
};

// A node is born owned: whoever allocated it is its only parent.
static Hamt *hamt_new_cap(uint32_t nslots, uint32_t cap) {
  if (cap < nslots)
    cap = nslots;
  Hamt *h = (Hamt *)w_alloc(sizeof(Hamt) + sizeof(Slot) * cap);
  h->bitmap = 0;
  h->count = 0;
  h->collision = false;
  h->owned = true;
  h->nslots = nslots;
  h->cap = cap;
  return h;
}

static Hamt *hamt_new(uint32_t nslots) { return hamt_new_cap(nslots, nslots); }

static size_t hamt_bytes(const Hamt *h) {
  return sizeof(Hamt) + sizeof(Slot) * h->cap;
}

// share_children is called wherever a node's slots are duplicated while the
// original stays reachable: its children have gained a second parent, so
// neither version may write through them any more. Marking one level is
// enough, since an in-place update stops at the first shared node and copies
// from there down, and copying marks that node's children in turn.
static void share_children(Hamt *h) {
  for (uint32_t i = 0; i < h->nslots; i++) {
    if (h->slots[i].is_node) {
      h->slots[i].node->owned = false;
    }
  }
}

static uint32_t popcount32(uint32_t x) {
  x = x - ((x >> 1) & 0x55555555u);
  x = (x & 0x33333333u) + ((x >> 2) & 0x33333333u);
  x = (x + (x >> 4)) & 0x0F0F0F0Fu;
  return (x * 0x01010101u) >> 24;
}

static uint32_t slot_index(uint32_t bitmap, uint32_t bit) {
  return popcount32(bitmap & (bit - 1));
}

// hamt_move copies a node whose original is about to become unreachable, so
// the children keep whatever ownership they had.
static Hamt *hamt_move(const Hamt *src, uint32_t nslots) {
  Hamt *h = hamt_new(nslots);
  h->bitmap = src->bitmap;
  h->count = src->count;
  h->collision = src->collision;
  uint32_t n = src->nslots < nslots ? src->nslots : nslots;
  memcpy(h->slots, src->slots, sizeof(Slot) * n);
  return h;
}

// hamt_copy copies a node whose original stays reachable, which is what path
// copying does, so the children become shared.
static Hamt *hamt_copy(const Hamt *src, uint32_t nslots) {
  Hamt *h = hamt_move(src, nslots);
  share_children(h);
  return h;
}

static Hamt *hamt_insert(const Hamt *node, Value key, Value val, uint64_t hash,
                         int shift, bool *replaced);

// hamt_widen returns a node with one more slot, holding a new leaf at pos. The
// caller decides what the children's ownership becomes.
static Hamt *hamt_widen(const Hamt *node, uint32_t pos, uint32_t bit, Value key,
                        Value val) {
  uint32_t n = popcount32(node->bitmap);
  Hamt *h = hamt_new(n + 1);
  h->bitmap = node->bitmap | bit;
  h->collision = false;
  h->count = node->count + 1;
  memcpy(h->slots, node->slots, sizeof(Slot) * pos);
  h->slots[pos].is_node = false;
  h->slots[pos].leaf.key = key;
  h->slots[pos].leaf.val = val;
  memcpy(h->slots + pos + 1, node->slots + pos, sizeof(Slot) * (n - pos));
  return h;
}

// grown picks the capacity a node should be reallocated at when it is being
// added to and nothing else can see it. Doubling keeps the number of
// reallocations logarithmic in the node's final width instead of linear.
static uint32_t grown(uint32_t n) {
  uint32_t cap = n < 2 ? n + 1 : n * 2;
  return cap > HAMT_WIDTH ? HAMT_WIDTH : cap;
}

// hamt_widen_owned is hamt_widen for a node whose original becomes
// unreachable: the children keep their ownership, and the replacement is
// allocated with room to grow.
static Hamt *hamt_widen_owned(const Hamt *node, uint32_t pos, uint32_t bit,
                              Value key, Value val) {
  uint32_t n = popcount32(node->bitmap);
  Hamt *h = hamt_new_cap(n + 1, grown(n + 1));
  h->bitmap = node->bitmap | bit;
  h->collision = false;
  h->count = node->count + 1;
  memcpy(h->slots, node->slots, sizeof(Slot) * pos);
  h->slots[pos].is_node = false;
  h->slots[pos].leaf.key = key;
  h->slots[pos].leaf.val = val;
  memcpy(h->slots + pos + 1, node->slots + pos, sizeof(Slot) * (n - pos));
  return h;
}

// merge_leaves builds the subtree holding two entries whose hashes agree up to
// shift.
static Hamt *merge_leaves(Value k1, Value v1, uint64_t h1, Value k2, Value v2,
                          uint64_t h2, int shift) {
  if (shift > HAMT_MAX_SHIFT) {
    // The hashes are equal the whole way down; keep both in a linear bucket.
    Hamt *h = hamt_new(2);
    h->collision = true;
    h->count = 2;
    h->nslots = 2;
    h->slots[0].is_node = false;
    h->slots[0].leaf.key = k1;
    h->slots[0].leaf.val = v1;
    h->slots[1].is_node = false;
    h->slots[1].leaf.key = k2;
    h->slots[1].leaf.val = v2;
    return h;
  }

  uint32_t i1 = (uint32_t)((h1 >> shift) & HAMT_MASK);
  uint32_t i2 = (uint32_t)((h2 >> shift) & HAMT_MASK);

  if (i1 == i2) {
    Hamt *child = merge_leaves(k1, v1, h1, k2, v2, h2, shift + HAMT_BITS);
    Hamt *h = hamt_new(1);
    h->bitmap = 1u << i1;
    h->count = child->count;
    h->slots[0].is_node = true;
    h->slots[0].node = child;
    return h;
  }

  Hamt *h = hamt_new(2);
  h->bitmap = (1u << i1) | (1u << i2);
  h->count = 2;
  uint32_t first = i1 < i2 ? 0 : 1;
  h->slots[first].is_node = false;
  h->slots[first].leaf.key = k1;
  h->slots[first].leaf.val = v1;
  h->slots[1 - first].is_node = false;
  h->slots[1 - first].leaf.key = k2;
  h->slots[1 - first].leaf.val = v2;
  return h;
}

static Hamt *insert_collision(const Hamt *node, Value key, Value val, bool *replaced) {
  for (uint32_t i = 0; i < node->nslots; i++) {
    if (w_equal(node->slots[i].leaf.key, key)) {
      Hamt *h = hamt_copy(node, node->nslots);
      h->slots[i].leaf.val = val;
      *replaced = true;
      return h;
    }
  }
  Hamt *h = hamt_copy(node, node->nslots + 1);
  h->slots[node->nslots].is_node = false;
  h->slots[node->nslots].leaf.key = key;
  h->slots[node->nslots].leaf.val = val;
  h->count = node->count + 1;
  return h;
}

static Hamt *hamt_insert(const Hamt *node, Value key, Value val, uint64_t hash,
                         int shift, bool *replaced) {
  if (node->collision)
    return insert_collision(node, key, val, replaced);

  uint32_t idx = (uint32_t)((hash >> shift) & HAMT_MASK);
  uint32_t bit = 1u << idx;
  uint32_t pos = slot_index(node->bitmap, bit);

  if (!(node->bitmap & bit)) {
    // Free position: widen this node by one slot.
    Hamt *h = hamt_widen(node, pos, bit, key, val);
    share_children(h);
    return h;
  }

  Slot slot = node->slots[pos];
  if (slot.is_node) {
    Hamt *child = hamt_insert(slot.node, key, val, hash, shift + HAMT_BITS, replaced);
    Hamt *h = hamt_copy(node, node->nslots);
    h->slots[pos].node = child;
    h->count = node->count + (*replaced ? 0 : 1);
    return h;
  }

  if (w_equal(slot.leaf.key, key)) {
    Hamt *h = hamt_copy(node, node->nslots);
    h->slots[pos].leaf.val = val;
    *replaced = true;
    return h;
  }

  // Two different keys want the same position: push both down a level.
  Hamt *child = merge_leaves(slot.leaf.key, slot.leaf.val, w_hash(slot.leaf.key),
                             key, val, hash, shift + HAMT_BITS);
  Hamt *h = hamt_copy(node, node->nslots);
  h->slots[pos].is_node = true;
  h->slots[pos].node = child;
  h->count = node->count + 1;
  return h;
}

// ------------------------------------------------------- in-place insertion
//
// A map threaded through a loop and added to on each turn is path-copied on
// every insert, which is where an Advent of Code program spends its memory: a
// seen-set or a frequency map built over a million steps allocates a million
// paths and keeps every one of them, since the arena never frees.
//
// The fix is the one already used for grids (SPEC.md section 13), one level
// finer. The compiler proves the map is never duplicated inside the loop; the
// runtime tracks, per node, whether anything else points at it. An insert
// writes through the owned prefix of the path and copies from the first shared
// node down — so the first turn copies, later turns mostly do not, and each
// node is copied at most once however long the loop runs.

static Hamt *hamt_insert_owned(Hamt *node, Value key, Value val, uint64_t hash,
                               int shift, bool *replaced) {
  if (!node->owned) {
    // Somebody else can see this subtree, so from here down it is the ordinary
    // copying insert. What comes back is fresh, and therefore owned.
    return hamt_insert(node, key, val, hash, shift, replaced);
  }

  if (node->collision) {
    for (uint32_t i = 0; i < node->nslots; i++) {
      if (w_equal(node->slots[i].leaf.key, key)) {
        node->slots[i].leaf.val = val;
        *replaced = true;
        return node;
      }
    }
    // A bucket has no children, so widening it moves nothing.
    Hamt *h = hamt_move(node, node->nslots + 1);
    h->slots[node->nslots].is_node = false;
    h->slots[node->nslots].leaf.key = key;
    h->slots[node->nslots].leaf.val = val;
    h->count = node->count + 1;
    h->nslots = node->nslots + 1;
    w_free(node, hamt_bytes(node));
    return h;
  }

  uint32_t idx = (uint32_t)((hash >> shift) & HAMT_MASK);
  uint32_t bit = 1u << idx;
  uint32_t pos = slot_index(node->bitmap, bit);

  if (!(node->bitmap & bit)) {
    if (node->cap > node->nslots) {
      // There is spare room, so the new leaf goes in where it belongs and the
      // node is not replaced at all. This is the case that makes building a
      // map in a loop cheap: without it every new key reallocates a node.
      memmove(node->slots + pos + 1, node->slots + pos,
              sizeof(Slot) * (node->nslots - pos));
      node->slots[pos].is_node = false;
      node->slots[pos].leaf.key = key;
      node->slots[pos].leaf.val = val;
      node->bitmap |= bit;
      node->count++;
      node->nslots++;
      return node;
    }
    // Out of room, so the node is replaced — but the old one becomes
    // unreachable, so its children keep the ownership they had, and the node
    // itself goes back to the allocator. It is grown with headroom, since a
    // node that has just been added to is likely to be added to again; a branch
    // node holds at most 32 slots, so the waste is bounded and small.
    Hamt *grown_node = hamt_widen_owned(node, pos, bit, key, val);
    w_free(node, hamt_bytes(node));
    return grown_node;
  }

  Slot slot = node->slots[pos];
  if (slot.is_node) {
    node->slots[pos].node =
        hamt_insert_owned(slot.node, key, val, hash, shift + HAMT_BITS, replaced);
    node->count += *replaced ? 0 : 1;
    return node;
  }

  if (w_equal(slot.leaf.key, key)) {
    node->slots[pos].leaf.val = val;
    *replaced = true;
    return node;
  }

  // Two different keys want the same position: push both down a level. The
  // subtree is fresh, so this node keeps writing through.
  node->slots[pos].is_node = true;
  node->slots[pos].node =
      merge_leaves(slot.leaf.key, slot.leaf.val, w_hash(slot.leaf.key), key, val,
                   hash, shift + HAMT_BITS);
  node->count++;
  return node;
}

static bool hamt_lookup(const Hamt *node, Value key, uint64_t hash, int shift,
                        Value *out) {
  if (node->collision) {
    for (uint32_t i = 0; i < node->nslots; i++) {
      if (w_equal(node->slots[i].leaf.key, key)) {
        *out = node->slots[i].leaf.val;
        return true;
      }
    }
    return false;
  }

  uint32_t idx = (uint32_t)((hash >> shift) & HAMT_MASK);
  uint32_t bit = 1u << idx;
  if (!(node->bitmap & bit))
    return false;

  Slot slot = node->slots[slot_index(node->bitmap, bit)];
  if (slot.is_node)
    return hamt_lookup(slot.node, key, hash, shift + HAMT_BITS, out);
  if (w_equal(slot.leaf.key, key)) {
    *out = slot.leaf.val;
    return true;
  }
  return false;
}

// hamt_remove returns a node without key. Removal keeps the node shape simple:
// an emptied slot is dropped, but nodes are not collapsed, which costs a
// little space and saves a great deal of complexity.
static Hamt *hamt_remove(const Hamt *node, Value key, uint64_t hash, int shift,
                         bool *removed) {
  if (node->collision) {
    for (uint32_t i = 0; i < node->nslots; i++) {
      if (w_equal(node->slots[i].leaf.key, key)) {
        Hamt *h = hamt_new(node->nslots - 1);
        h->collision = true;
        uint32_t at = 0;
        for (uint32_t j = 0; j < node->nslots; j++)
          if (j != i)
            h->slots[at++] = node->slots[j];
        h->count = node->count - 1;
        *removed = true;
        return h;
      }
    }
    return (Hamt *)node;
  }

  uint32_t idx = (uint32_t)((hash >> shift) & HAMT_MASK);
  uint32_t bit = 1u << idx;
  if (!(node->bitmap & bit))
    return (Hamt *)node;

  uint32_t pos = slot_index(node->bitmap, bit);
  Slot slot = node->slots[pos];

  if (slot.is_node) {
    Hamt *child = hamt_remove(slot.node, key, hash, shift + HAMT_BITS, removed);
    if (!*removed)
      return (Hamt *)node;
    Hamt *h = hamt_copy(node, node->nslots);
    h->slots[pos].node = child;
    h->count = node->count - 1;
    return h;
  }

  if (!w_equal(slot.leaf.key, key))
    return (Hamt *)node;

  uint32_t n = popcount32(node->bitmap);
  Hamt *h = hamt_new(n - 1);
  h->bitmap = node->bitmap & ~bit;
  memcpy(h->slots, node->slots, sizeof(Slot) * pos);
  memcpy(h->slots + pos, node->slots + pos + 1, sizeof(Slot) * (n - pos - 1));
  h->count = node->count - 1;
  // The node this came from stays reachable, so its children now have two
  // parents and neither version may write through them.
  share_children(h);
  *removed = true;
  return h;
}

// walk visits every entry in the trie.
typedef void (*EntryFn)(void *ctx, Value key, Value val);

static void hamt_walk(const Hamt *node, EntryFn fn, void *ctx) {
  uint32_t n = node->collision ? node->nslots : popcount32(node->bitmap);
  for (uint32_t i = 0; i < n; i++) {
    if (!node->collision && node->slots[i].is_node)
      hamt_walk(node->slots[i].node, fn, ctx);
    else
      fn(ctx, node->slots[i].leaf.key, node->slots[i].leaf.val);
  }
}

// --------------------------------------------------------------- flat table
//
// The trie is what makes a Web immutable cheaply, and that is exactly what an
// Advent of Code program building a seen-set does not need: the compiler has
// already proved, for the loops that matter, that nothing else can see the map.
// For an *owned* map whose keys are immediates — Earth, Knot, Fire, Spirit,
// which is very nearly every key anyone writes — an open-addressed table is
// strictly better than a trie: one array, no node per entry, no path to copy,
// one cache miss per lookup instead of four.
//
// So a map has two representations behind the same verbs. It becomes flat when
// an owned insert starts it off with an immediate key, and turns back into a
// trie the moment it is used persistently — a `put` on a map somebody else can
// still see, a `forget`, or a key that is not an immediate. That conversion is
// O(n) but happens at most once per map, so the trie's guarantees still hold:
// nothing degrades to quadratic, and the flat table is only ever written by the
// code path that proved it could be.
//
// Iteration order is ascending key (see web_collect), so the two
// representations are indistinguishable from inside the language.

typedef struct {
  Value key, val;
} Cell;

typedef struct {
  size_t count; // live entries
  size_t cap;   // slots, always a power of two
  size_t limit; // count at which the table grows
  Cell *cells;
} Flat;

// The smallest table worth allocating. A map built in a loop reaches this in
// eight inserts, and a map that never grows past it costs 512 bytes.
#define FLAT_MIN 16

// An empty slot is one whose key carries this tag, which no Value ever has.
#define FLAT_FREE 0xFFFFFFFFu

// immediate says whether a key is one of the Powers that fits in the Value
// itself and compares bitwise. Water is deliberately left out: 0.0 and -0.0 are
// equal but differ bitwise, so it would need its own comparison for no gain —
// nobody keys a map on a float.
static bool immediate(Value v) {
  return v.tag == W_EARTH || v.tag == W_KNOT || v.tag == W_FIRE ||
         v.tag == W_SPIRIT;
}

static bool imm_equal(Value a, Value b) {
  if (a.tag != b.tag)
    return false;
  switch (a.tag) {
  case W_EARTH:
    return a.earth == b.earth;
  case W_KNOT:
    return a.knot.row == b.knot.row && a.knot.col == b.knot.col;
  case W_FIRE:
    return a.fire == b.fire;
  default:
    return a.spirit == b.spirit;
  }
}

// imm_hash is splitmix64's finaliser over the key's bits. It does not have to
// agree with w_hash — the two representations never share a table — and being
// three multiplies rather than a byte loop is most of why a flat lookup is
// quick.
static uint64_t imm_hash(Value v) {
  uint64_t x;
  switch (v.tag) {
  case W_EARTH:
    x = (uint64_t)v.earth;
    break;
  case W_KNOT:
    x = ((uint64_t)(uint32_t)v.knot.row << 32) | (uint32_t)v.knot.col;
    break;
  case W_FIRE:
    x = v.fire;
    break;
  default:
    x = v.spirit ? 1 : 0;
    break;
  }
  x += (uint64_t)v.tag * 0x9E3779B97F4A7C15ULL;
  x ^= x >> 30;
  x *= 0xBF58476D1CE4E5B9ULL;
  x ^= x >> 27;
  x *= 0x94D049BB133111EBULL;
  return x ^ (x >> 31);
}

// The cell array is malloc'd rather than taken from the arena. A table that
// grows abandons its old array with no other owner, so its death is certain —
// but it doubles, and the arena's free lists are exact-size, so a freed array
// would sit there while the next request asked for twice as much. malloc is
// built for exactly this shape and reuses it; measured, going through the arena
// instead cost `mapbuild` a third of its memory for nothing.
static Flat *flat_new(size_t cap) {
  Flat *f = (Flat *)w_alloc(sizeof(Flat));
  f->count = 0;
  f->cap = cap;
  f->limit = cap - cap / 4; // grow at three quarters full
  f->cells = (Cell *)malloc(sizeof(Cell) * cap);
  for (size_t i = 0; i < cap; i++)
    f->cells[i].key.tag = FLAT_FREE;
  return f;
}

// flat_slot returns where a key lives, or where it would go. Linear probing:
// with a table three-quarters full at worst, the run is short and every step
// after the first is usually the same cache line.
static size_t flat_slot(const Flat *f, Value key, uint64_t hash) {
  size_t mask = f->cap - 1;
  size_t i = (size_t)hash & mask;
  for (;;) {
    Value k = f->cells[i].key;
    if (k.tag == FLAT_FREE || imm_equal(k, key))
      return i;
    i = (i + 1) & mask;
  }
}

static bool flat_lookup(const Flat *f, Value key, Value *out) {
  size_t i = flat_slot(f, key, imm_hash(key));
  if (f->cells[i].key.tag == FLAT_FREE)
    return false;
  *out = f->cells[i].val;
  return true;
}

static void flat_grow(Flat *f) {
  size_t cap = f->cap * 2, mask = cap - 1;
  Cell *cells = (Cell *)malloc(sizeof(Cell) * cap);
  for (size_t i = 0; i < cap; i++)
    cells[i].key.tag = FLAT_FREE;
  for (size_t i = 0; i < f->cap; i++) {
    Value k = f->cells[i].key;
    if (k.tag == FLAT_FREE)
      continue;
    size_t j = (size_t)imm_hash(k) & mask;
    while (cells[j].key.tag != FLAT_FREE)
      j = (j + 1) & mask;
    cells[j] = f->cells[i];
  }
  free(f->cells);
  f->cells = cells;
  f->cap = cap;
  f->limit = cap - cap / 4;
}

// flat_put writes through the table, which is only ever done to one the
// compiler has proved unshared.
static void flat_put(Flat *f, Value key, Value val) {
  size_t i = flat_slot(f, key, imm_hash(key));
  if (f->cells[i].key.tag != FLAT_FREE) {
    f->cells[i].val = val;
    return;
  }
  f->cells[i].key = key;
  f->cells[i].val = val;
  if (++f->count >= f->limit)
    flat_grow(f);
}

// flat_remove takes a key out of an open-addressed table without leaving a
// tombstone behind, by shifting back the entries that were displaced past the
// hole. A tombstone would be simpler and would degrade every later probe; this
// leaves the table exactly as if the key had never been inserted.
//
// The test is the standard one for linear probing: an entry belongs at or
// before the hole when the distance from its home to where it sits is at least
// the distance from the hole to where it sits, measured the long way round the
// table so that a run wrapping past the end is handled by the arithmetic
// rather than by a case.
static bool flat_remove(Flat *f, Value key) {
  size_t mask = f->cap - 1;
  size_t hole = flat_slot(f, key, imm_hash(key));
  if (f->cells[hole].key.tag == FLAT_FREE)
    return false;

  size_t i = hole;
  for (;;) {
    i = (i + 1) & mask;
    Value k = f->cells[i].key;
    if (k.tag == FLAT_FREE)
      break;
    size_t home = (size_t)imm_hash(k) & mask;
    if (((i - home) & mask) >= ((i - hole) & mask)) {
      f->cells[hole] = f->cells[i];
      hole = i;
    }
  }
  f->cells[hole].key.tag = FLAT_FREE;
  f->count--;
  return true;
}

static Flat *flat_copy(const Flat *src) {
  Flat *f = (Flat *)w_alloc(sizeof(Flat));
  *f = *src;
  f->cells = (Cell *)malloc(sizeof(Cell) * src->cap);
  memcpy(f->cells, src->cells, sizeof(Cell) * src->cap);
  return f;
}

// ---------------------------------------------------------------- memo table
//
// `remember` used to keep its results in an ordinary Web, which was the right
// first answer — hashing and equality came for free, so a remembered function
// could be keyed on anything. But a memo table is not a value: the program can
// never see it, it is never copied, and nothing is ever removed from it. All of
// the trie's guarantees are being paid for and none is being used, and for a
// function of two or more arguments the key was a Twine allocated on every
// call, hit or miss, only to be hashed and thrown away.
//
// So it is its own thing: one open-addressed table, rows of arity keys and a
// result, probed straight from the argument array.

struct WMemo {
  size_t count, cap, limit;
  int arity;
  int width;    // arity + 1, in Values
  Value *slots; // cap rows; a row whose first key is FLAT_FREE is empty
};

// Keys here can be any type, unlike a flat map's, so hashing goes through
// w_hash — except for the immediates, which are most of them and which the
// short version handles in three multiplies.
static uint64_t key_hash(Value v) {
  return immediate(v) ? imm_hash(v) : w_hash(v);
}

static bool key_equal(Value a, Value b) {
  return immediate(a) ? imm_equal(a, b) : w_equal(a, b);
}

static uint64_t args_hash(const Value *args, int n) {
  uint64_t h = key_hash(args[0]);
  for (int i = 1; i < n; i++)
    h = h * 1099511628211ULL + key_hash(args[i]);
  return h;
}

static void memo_alloc(WMemo *m, size_t cap) {
  m->cap = cap;
  m->limit = cap - cap / 4;
  m->slots = (Value *)malloc(sizeof(Value) * cap * (size_t)m->width);
  for (size_t i = 0; i < cap; i++)
    m->slots[i * (size_t)m->width].tag = FLAT_FREE;
}

WMemo *w_memo_new(int arity) {
  WMemo *m = (WMemo *)w_alloc(sizeof(WMemo));
  m->count = 0;
  m->arity = arity;
  m->width = arity + 1;
  memo_alloc(m, FLAT_MIN);
  return m;
}

static size_t memo_slot(const WMemo *m, const Value *args, uint64_t hash) {
  size_t mask = m->cap - 1;
  size_t i = (size_t)hash & mask;
  for (;;) {
    Value *row = m->slots + i * (size_t)m->width;
    if (row->tag == FLAT_FREE)
      return i;
    int k = 0;
    while (k < m->arity && key_equal(row[k], args[k]))
      k++;
    if (k == m->arity)
      return i;
    i = (i + 1) & mask;
  }
}

bool w_memo_get(WMemo *m, const Value *args, Value *out) {
  Value *row = m->slots + memo_slot(m, args, args_hash(args, m->arity)) *
                              (size_t)m->width;
  if (row->tag == FLAT_FREE)
    return false;
  *out = row[m->arity];
  return true;
}

static void memo_grow(WMemo *m) {
  size_t oldcap = m->cap;
  Value *old = m->slots;
  memo_alloc(m, oldcap * 2);
  size_t mask = m->cap - 1;
  for (size_t i = 0; i < oldcap; i++) {
    Value *row = old + i * (size_t)m->width;
    if (row->tag == FLAT_FREE)
      continue;
    size_t j = (size_t)args_hash(row, m->arity) & mask;
    while (m->slots[j * (size_t)m->width].tag != FLAT_FREE)
      j = (j + 1) & mask;
    memcpy(m->slots + j * (size_t)m->width, row, sizeof(Value) * (size_t)m->width);
  }
  free(old);
}

void w_memo_put(WMemo *m, const Value *args, Value result) {
  size_t i = memo_slot(m, args, args_hash(args, m->arity));
  Value *row = m->slots + i * (size_t)m->width;
  if (row->tag == FLAT_FREE) {
    memcpy(row, args, sizeof(Value) * (size_t)m->arity);
    m->count++;
  }
  row[m->arity] = result;
  if (m->count >= m->limit)
    memo_grow(m);
}

// ---------------------------------------------------------------- web object

typedef struct {
  WObj obj;
  Hamt *root; // the trie representation; NULL when the map is flat
  Flat *flat; // the flat one; NULL when the map is a trie
} WMap;

static Value map_value(Hamt *root, Flat *flat, uint32_t tag) {
  WMap *m = (WMap *)w_alloc(sizeof(WMap));
  m->obj.rc = W_SHARED;
  m->obj.kind = tag;
  m->root = root;
  m->flat = flat;
  Value v;
  v.tag = tag;
  v.obj = &m->obj;
  return v;
}

static WMap *map_of(Value v) { return (WMap *)v.obj; }

// to_trie gives a flat map its trie back. It rewrites the map object in place,
// which is invisible: the two representations hold the same entries and answer
// every verb the same way. It is what every persistent operation on a flat map
// does first, so the flat table stays confined to the owned path.
static Hamt *to_trie(WMap *m) {
  if (m->root)
    return m->root;
  Flat *f = m->flat;
  Hamt *root = hamt_new_cap(0, HAMT_WIDTH);
  bool replaced = false;
  for (size_t i = 0; i < f->cap; i++) {
    if (f->cells[i].key.tag == FLAT_FREE)
      continue;
    replaced = false;
    root = hamt_insert_owned(root, f->cells[i].key, f->cells[i].val,
                             w_hash(f->cells[i].key), 0, &replaced);
  }
  free(f->cells);
  f->cells = NULL;
  m->flat = NULL;
  m->root = root;
  return root;
}

Value w_web_empty(void) { return map_value(hamt_new(0), NULL, W_WEB); }
Value w_circle_empty(void) { return map_value(hamt_new(0), NULL, W_CIRCLE); }

Value w_web_put(Value web, Value key, Value val) {
  bool replaced = false;
  Hamt *root = hamt_insert(to_trie(map_of(web)), key, val, w_hash(key), 0, &replaced);
  return map_value(root, NULL, web.tag);
}

// w_web_put_owned is `put` for a map the compiler has proved is not shared. It
// mirrors wp_set_owned for grids: a map arrives shared, because the caller may
// still hold it, so the first insert in a loop copies and marks the copy owned,
// and every later one writes through.
Value w_web_put_owned(Value web, Value key, Value val) {
  WMap *m = map_of(web);
  bool replaced = false;

  if (web.obj->rc == W_OWNED) {
    if (m->flat) {
      if (immediate(key)) {
        flat_put(m->flat, key, val);
        return web;
      }
      to_trie(m); // a key the flat table cannot hold ends the experiment
    }
    m->root = hamt_insert_owned(m->root, key, val, w_hash(key), 0, &replaced);
    return web;
  }

  // The map arrived shared, so this is the first insert of a loop and it has to
  // copy. A flat map copies flat; an empty one becomes flat if the key allows,
  // which is where a map picks its representation.
  if (immediate(key) && (m->flat || m->root->count == 0)) {
    Flat *f = m->flat ? flat_copy(m->flat) : flat_new(FLAT_MIN);
    flat_put(f, key, val);
    Value out = map_value(NULL, f, web.tag);
    out.obj->rc = W_OWNED;
    return out;
  }
  Hamt *root = hamt_insert(to_trie(m), key, val, w_hash(key), 0, &replaced);
  Value out = map_value(root, NULL, web.tag);
  out.obj->rc = W_OWNED;
  return out;
}

// w_web_reserve makes room for n entries in a map that is about to be built.
// Without it a fold rehashes its way up from sixteen — eight grows to reach two
// thousand entries, which is as much work again as the inserts themselves.
//
// Only an empty map is re-shaped, and what comes back is owned: it is brand new,
// so the first insert has nothing to copy. The caller is the fused fold, which
// asks only when it knows both that it owns the accumulator and how many
// elements are coming.
Value w_web_reserve(Value web, size_t n) {
  WMap *m = map_of(web);
  if (m->flat || m->root->count != 0)
    return web;
  size_t cap = FLAT_MIN;
  while (cap - cap / 4 <= n)
    cap *= 2;
  Value out = map_value(NULL, flat_new(cap), web.tag);
  out.obj->rc = W_OWNED;
  return out;
}

Value w_web_forget(Value web, Value key) {
  WMap *m = map_of(web);
  // A flat map forgets flatly. Turning it into a trie to take one key out
  // would cost the whole map, and would leave every later lookup on the slow
  // side of the two representations.
  if (m->flat) {
    if (!immediate(key))
      return web; // a flat table holds only immediates, so it is not in there
    Value out;
    if (!flat_lookup(m->flat, key, &out))
      return web;
    Flat *f = flat_copy(m->flat);
    flat_remove(f, key);
    return map_value(NULL, f, web.tag);
  }
  bool removed = false;
  Hamt *root = hamt_remove(m->root, key, w_hash(key), 0, &removed);
  return removed ? map_value(root, NULL, web.tag) : web;
}

// w_web_forget_owned is `forget` for a map the compiler has proved is not
// shared, on the same terms w_web_put_owned has. Taking keys out of a map in a
// loop is as common as putting them in — a frontier shrinking, a set of
// candidates being narrowed — and without this it was the only half of the
// pair that copied.
Value w_web_forget_owned(Value web, Value key) {
  WMap *m = map_of(web);

  if (web.obj->rc == W_OWNED) {
    if (m->flat) {
      if (immediate(key))
        flat_remove(m->flat, key);
      return web;
    }
    bool removed = false;
    m->root = hamt_remove(m->root, key, w_hash(key), 0, &removed);
    return web;
  }

  // Shared, so this is the first turn of a loop and it has to copy. What it
  // copies into is owned, and every turn after this one writes through.
  Value out = w_web_forget(web, key);
  if (out.obj == web.obj) {
    // The key was not there, so nothing was copied and the map handed back is
    // still the caller's. Copying it here is what makes the next turn cheap.
    if (m->flat) {
      out = map_value(NULL, flat_copy(m->flat), web.tag);
    } else {
      return web;
    }
  }
  out.obj->rc = W_OWNED;
  return out;
}

static bool map_lookup(Value web, Value key, Value *out) {
  WMap *m = map_of(web);
  if (m->flat)
    // A flat table holds only immediates, so anything else is absent.
    return immediate(key) && flat_lookup(m->flat, key, out);
  return hamt_lookup(m->root, key, w_hash(key), 0, out);
}

Value w_web_get(Value web, Value key) {
  Value out;
  if (map_lookup(web, key, &out))
    return w_held(out);
  return w_stilled();
}

bool w_web_has(Value web, Value key) {
  Value out;
  return map_lookup(web, key, &out);
}

size_t w_web_size(Value web) {
  WMap *m = map_of(web);
  return m->flat ? m->flat->count : m->root->count;
}

// Collecting entries into Threads. The entries come out in ascending key
// order: a map has no order of its own, and picking one costs a sort but makes
// a program's output depend on what it put in rather than on how the runtime
// happened to store it — including which of the two representations it used.
typedef struct {
  Cell *cells;
  size_t len;
} Collect;

static void collect_entry(void *ctx, Value key, Value val) {
  Collect *c = (Collect *)ctx;
  c->cells[c->len].key = key;
  c->cells[c->len].val = val;
  c->len++;
}

static int cell_cmp(const void *a, const void *b) {
  return w_compare(((const Cell *)a)->key, ((const Cell *)b)->key);
}

// An immediate key is one machine word once biased, and a whole map's keys have
// one type, so entries can be ordered without a comparison function at all. It
// is worth the code: a program that reads back every map it builds spends real
// time here, and qsort's indirect call per comparison is most of it.
static uint64_t sort_key(Value v) {
  switch (v.tag) {
  case W_EARTH:
    // Flipping the sign bit makes unsigned order agree with signed.
    return (uint64_t)v.earth ^ 0x8000000000000000ULL;
  case W_KNOT:
    return ((uint64_t)(uint32_t)(v.knot.row ^ INT32_MIN) << 32) |
           (uint32_t)(v.knot.col ^ INT32_MIN);
  case W_FIRE:
    return v.fire;
  default:
    return v.spirit ? 1 : 0;
  }
}

// A rank is a key's ordering value and where its entry sits. The sort moves
// these and not the entries: sixteen bytes rather than thirty-two, four to a
// cache line rather than two, and a map's cells never move at all.
typedef struct {
  uint64_t key;
  uint32_t at;
} Rank;

// ranks_sort orders by an eight-byte key, which a radix sort does in a fixed
// number of passes over the array instead of n log n comparisons.
//
// Only the bytes that actually differ get a pass. Keys agree on their high bits
// far more often than not — every Earth key a program builds is small and
// positive, so five of the eight bytes are the same in all of them — and
// skipping those is most of what makes this quick. A map read back after being
// built is a thing Advent of Code does constantly, and this was a quarter of
// one benchmark's running time.
//
// It returns where the sorted ranks ended up, which is either the array it was
// given or the scratch it borrowed, depending on how many passes ran. Handing
// the pointer back rather than copying saves a whole pass over the array on any
// key that needs an odd number of them, which is most of them. Whatever comes
// back in *scratch is the caller's to release.
static Rank *ranks_sort(Rank *a, size_t n, Rank **scratch) {
  *scratch = NULL;
  if (n < 2)
    return a;
  if (n <= 32) {
    for (size_t i = 1; i < n; i++) {
      Rank held = a[i];
      size_t j = i;
      while (j > 0 && held.key < a[j - 1].key) {
        a[j] = a[j - 1];
        j--;
      }
      a[j] = held;
    }
    return a;
  }

  uint64_t ones = 0, zeros = ~(uint64_t)0;
  for (size_t i = 0; i < n; i++) {
    ones |= a[i].key;
    zeros &= a[i].key;
  }
  uint64_t varying = ones ^ zeros; // a bit is set where the keys disagree
  if (varying == 0)
    return a;

  Rank *tmp = (Rank *)w_alloc(sizeof(Rank) * n);
  *scratch = tmp;
  Rank *src = a, *dst = tmp;
  for (int shift = 0; shift < 64; shift += 8) {
    if (((varying >> shift) & 0xff) == 0)
      continue;
    uint32_t count[257];
    memset(count, 0, sizeof count);
    for (size_t i = 0; i < n; i++)
      count[((src[i].key >> shift) & 0xff) + 1]++;
    for (int d = 0; d < 256; d++)
      count[d + 1] += count[d];
    for (size_t i = 0; i < n; i++)
      dst[count[(src[i].key >> shift) & 0xff]++] = src[i];
    Rank *swap = src;
    src = dst;
    dst = swap;
  }
  return src;
}

// Entries is a map's contents ready to be read in ascending key order.
//
// For a flat map the cells *are* the table — nothing is copied to get here, and
// the order says which slot to visit next. For a trie they have to be gathered
// first, since walking one is a callback. Either way the entries stay put and
// only the order is sorted.
typedef struct {
  const Cell *cells;
  const Rank *order; // NULL when the cells are already in the order wanted
  size_t len;
  Cell *cells_owned; // gathered entries, when they had to be gathered
  Rank *ranks;       // the two rank arrays the sort ping-ponged between
  Rank *ranks_spare;
} Entries;

static inline const Cell *entry_at(const Entries *e, size_t i) {
  return &e->cells[e->order ? e->order[i].at : i];
}

static void entries_release(Entries *e) {
  if (e->cells_owned)
    w_free(e->cells_owned, sizeof(Cell) * e->len);
  if (e->ranks)
    w_free(e->ranks, sizeof(Rank) * e->len);
  if (e->ranks_spare)
    w_free(e->ranks_spare, sizeof(Rank) * e->len);
}

// web_entries_of puts a map's contents in ascending key order. It is what every
// way of reading a whole map is built on.
static Entries web_entries_of(Value web) {
  WMap *m = map_of(web);
  size_t n = w_web_size(web);
  Entries e;
  e.cells = NULL;
  e.order = NULL;
  e.len = n;
  e.cells_owned = NULL;
  e.ranks = NULL;
  e.ranks_spare = NULL;
  if (n == 0)
    return e;

  if (m->flat) {
    // A flat table holds only immediate keys, and its slots are not dense, so
    // the ranks carry the slot numbers and the table is read where it lies.
    // Nothing is copied to get here.
    e.ranks = (Rank *)w_alloc(sizeof(Rank) * n);
    size_t j = 0;
    for (size_t i = 0; i < m->flat->cap; i++) {
      if (m->flat->cells[i].key.tag == FLAT_FREE)
        continue;
      e.ranks[j].key = sort_key(m->flat->cells[i].key);
      e.ranks[j].at = (uint32_t)i;
      j++;
    }
    e.order = ranks_sort(e.ranks, n, &e.ranks_spare);
    e.cells = m->flat->cells;
    return e;
  }

  // Walking a trie is a callback, so its entries have to be gathered first.
  e.cells_owned = (Cell *)w_alloc(sizeof(Cell) * n);
  Collect c;
  c.cells = e.cells_owned;
  c.len = 0;
  hamt_walk(m->root, collect_entry, &c);
  e.cells = e.cells_owned;

  // Every key in a map has the same type, so the first one decides. A key that
  // is not an immediate has no one-word ordering, so it keeps the comparison
  // sort — and pays for it, which is the argument for keying on Earths.
  if (immediate(e.cells_owned[0].key)) {
    e.ranks = (Rank *)w_alloc(sizeof(Rank) * n);
    for (size_t i = 0; i < n; i++) {
      e.ranks[i].key = sort_key(e.cells_owned[i].key);
      e.ranks[i].at = (uint32_t)i;
    }
    e.order = ranks_sort(e.ranks, n, &e.ranks_spare);
  } else if (n > 1) {
    qsort(e.cells_owned, n, sizeof(Cell), cell_cmp);
  }
  return e;
}

// w_web_entries hands back the keys and the values as two parallel arrays, in
// the same order `items` would give. A fused loop over `items` walks these
// instead of a Thread of Twines, so the pair a fold immediately takes apart is
// never built. See internal/codegen/fuse.go.
size_t w_web_entries(Value web, Value **keys, Value **vals) {
  Entries e = web_entries_of(web);
  size_t n = e.len ? e.len : 1;
  Value *ks = (Value *)w_alloc(sizeof(Value) * n);
  Value *vs = (Value *)w_alloc(sizeof(Value) * n);
  for (size_t i = 0; i < e.len; i++) {
    const Cell *c = entry_at(&e, i);
    ks[i] = c->key;
    vs[i] = c->val;
  }
  entries_release(&e);
  *keys = ks;
  *vals = vs;
  return e.len;
}

static Value web_collect(Value web, int which) {
  Entries e = web_entries_of(web);
  size_t n = e.len ? e.len : 1;
  Value *items = (Value *)w_alloc(sizeof(Value) * n);
  for (size_t i = 0; i < e.len; i++) {
    const Cell *c = entry_at(&e, i);
    switch (which) {
    case 0:
      items[i] = c->key;
      break;
    case 1:
      items[i] = c->val;
      break;
    default: {
      Value *pair = (Value *)w_alloc(sizeof(Value) * 2);
      pair[0] = c->key;
      pair[1] = c->val;
      items[i] = w_twine(pair, 2);
      break;
    }
    }
  }
  size_t len = e.len;
  entries_release(&e);
  return w_thread(items, len);
}

Value w_web_keys(Value web) { return web_collect(web, 0); }
Value w_web_vals(Value web) { return web_collect(web, 1); }
Value w_web_pairs(Value web) { return web_collect(web, 2); }

// w_web_equal compares two maps by content, independently of insertion order.
typedef struct {
  Value other;
  bool same;
} Compare;

static void compare_entry(void *ctx, Value key, Value val) {
  Compare *c = (Compare *)ctx;
  if (!c->same)
    return;
  Value found;
  if (!map_lookup(c->other, key, &found) || !w_equal(val, found))
    c->same = false;
}

bool w_web_equal(Value a, Value b) {
  if (w_web_size(a) != w_web_size(b))
    return false;
  WMap *m = map_of(a);
  if (m->flat) {
    for (size_t i = 0; i < m->flat->cap; i++) {
      Value found;
      if (m->flat->cells[i].key.tag == FLAT_FREE)
        continue;
      if (!map_lookup(b, m->flat->cells[i].key, &found) ||
          !w_equal(m->flat->cells[i].val, found))
        return false;
    }
    return true;
  }
  Compare c;
  c.other = b;
  c.same = true;
  hamt_walk(m->root, compare_entry, &c);
  return c.same;
}

// ------------------------------------------------------------------ taveren

// A leftist heap: the right spine is kept short, so merging two heaps costs
// O(log n) and both push and pop are merges.
typedef struct Heap Heap;
struct Heap {
  Value item;
  Heap *left, *right;
  int rank; // length of the right spine
};

typedef struct {
  WObj obj;
  Heap *root;
  size_t count;
} WHeap;

static Value heap_value(Heap *root, size_t count) {
  WHeap *h = (WHeap *)w_alloc(sizeof(WHeap));
  h->obj.rc = W_SHARED;
  h->obj.kind = W_TAVEREN;
  h->root = root;
  h->count = count;
  Value v;
  v.tag = W_TAVEREN;
  v.obj = &h->obj;
  return v;
}

static int heap_rank(Heap *h) { return h ? h->rank : 0; }

static Heap *heap_merge(Heap *a, Heap *b) {
  if (!a)
    return b;
  if (!b)
    return a;
  if (w_compare(b->item, a->item) < 0) {
    Heap *tmp = a;
    a = b;
    b = tmp;
  }
  Heap *merged = heap_merge(a->right, b);

  Heap *out = (Heap *)w_alloc(sizeof(Heap));
  out->item = a->item;
  if (heap_rank(a->left) >= heap_rank(merged)) {
    out->left = a->left;
    out->right = merged;
  } else {
    out->left = merged;
    out->right = a->left;
  }
  out->rank = heap_rank(out->right) + 1;
  return out;
}

static Heap *heap_single(Value v) {
  Heap *h = (Heap *)w_alloc(sizeof(Heap));
  h->item = v;
  h->left = NULL;
  h->right = NULL;
  h->rank = 1;
  return h;
}

Value w_taveren_empty(void) { return heap_value(NULL, 0); }

Value w_taveren_push(Value heap, Value item) {
  WHeap *h = (WHeap *)heap.obj;
  return heap_value(heap_merge(h->root, heap_single(item)), h->count + 1);
}

// w_taveren_pop yields Held (smallest, rest), or Stilled when empty.
Value w_taveren_pop(Value heap) {
  WHeap *h = (WHeap *)heap.obj;
  if (!h->root)
    return w_stilled();
  Value rest = heap_value(heap_merge(h->root->left, h->root->right), h->count - 1);
  Value *pair = (Value *)w_alloc(sizeof(Value) * 2);
  pair[0] = h->root->item;
  pair[1] = rest;
  return w_held(w_twine(pair, 2));
}

size_t w_taveren_size(Value heap) { return ((WHeap *)heap.obj)->count; }

// heap_merge_owned is heap_merge for two heaps nothing else can see. It rewires
// the nodes it already has rather than allocating a copy of each one along the
// path, which is what makes a frontier of millions of entries cost the entries
// and nothing more.
static Heap *heap_merge_owned(Heap *a, Heap *b) {
  if (!a)
    return b;
  if (!b)
    return a;
  if (w_compare(b->item, a->item) < 0) {
    Heap *tmp = a;
    a = b;
    b = tmp;
  }
  Heap *merged = heap_merge_owned(a->right, b);
  if (heap_rank(a->left) >= heap_rank(merged)) {
    a->right = merged;
  } else {
    a->right = a->left;
    a->left = merged;
  }
  a->rank = heap_rank(a->right) + 1;
  return a;
}

// ------------------------------------------------------------------ dijkstra

// dijkstra walks outwards from a start node, cheapest first, and returns the
// cost of reaching every node it can. `step` answers with the (cost, node)
// pairs leading out of a node, so the caller describes the graph and never
// builds one.
//
// It lives here rather than with the other verbs because it is made entirely
// out of the two structures above, and because it owns both outright: nothing
// outside this function ever sees the frontier, and the distance map does not
// escape until it is returned. That is ownership by construction rather than
// by an analysis, so both are updated in place — the frontier is a bare heap
// with no wrapper allocated per push, and settling a node writes through the
// map instead of path-copying it. Over a 600x600 grid graph — 360,000 nodes —
// that is 263 MB against the 2.2 GB it took when both were persistent.
Value wp_dijkstra(Value step, Value start) {
  Value dist = w_web_empty();
  dist.obj->rc = W_OWNED;

  Value first[2] = {w_earth(0), start};
  Heap *frontier = heap_single(w_twine_copy(first, 2));

  while (frontier) {
    Value entry = frontier->item;
    frontier = heap_merge_owned(frontier->left, frontier->right);

    int64_t cost = w_twine_at(entry, 0).earth;
    Value node = w_twine_at(entry, 1);
    if (w_web_has(dist, node))
      continue;
    dist = w_web_put_owned(dist, node, w_earth(cost));

    Value out = w_call(step, &node, 1);
    size_t n = w_thread_len(out);
    for (size_t i = 0; i < n; i++) {
      Value edge = w_thread_at(out, i);
      Value next = w_twine_at(edge, 1);
      if (w_web_has(dist, next))
        continue;
      Value ahead[2] = {w_earth(cost + w_twine_at(edge, 0).earth), next};
      frontier = heap_merge_owned(frontier, heap_single(w_twine_copy(ahead, 2)));
    }
  }
  // It belongs to the caller now, who may keep it alongside anything they
  // build from it.
  return w_disown(dist);
}
