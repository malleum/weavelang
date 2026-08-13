// Accounting for the arena, compiled in only by `weave build -tally`.
//
// The problem this solves: weave.c bump-allocates out of megabyte chunks, so
// every heap profiler sees the same thing — one 1 MB `malloc`, charged to
// whichever call happened to exhaust the previous chunk, and nothing at all
// about the thousands of objects that then went into it. On Advent of Code 2024
// day 22 that call is `wp_rev`, and massif duly reports 131 MB of live memory
// under `rev`, which reads as a leak in a function that allocates one array and
// hands it straight back. It is not a leak and it is not `rev`; it is 125
// chunks that `rev` happened to ask for first.
//
// So the arena keeps its own books instead. Every live block is recorded
// against the `file:line` that asked for it — the two entry points are macros
// in weave.h, which is the whole reason the real ones are named `_impl` — and
// the breakdown at the high-water mark goes to stderr when the program ends.
//
// This is a debugging build and makes no attempt to be cheap: a hash insert and
// a hash delete cost far more than the bump-pointer allocation they are
// measuring. Timings from a `-tally` build mean nothing. Sizes are exact.

#include "weave.h"

#ifdef WEAVE_TALLY

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// ------------------------------------------------------------------- sites
//
// A site is identified by the address of the `file ":" line` literal the macro
// pastes together, so two allocations from the same line share a site without
// anything having to compare strings. A translation unit that mentions the same
// line twice cannot happen; two units with coincidentally equal text get two
// entries, which is correct — they are different lines.

#define TALLY_SITES 1024

typedef struct {
  const char *site;
  size_t live;      // bytes outstanding right now
  size_t allocated; // bytes ever handed out
  size_t blocks;    // blocks ever handed out
  size_t at_peak;   // live bytes when the snapshot below was taken
} Site;

static Site g_sites[TALLY_SITES];
static size_t g_nsites;

static size_t site_index(const char *site) {
  size_t i = ((uintptr_t)site >> 4) * 2654435761u & (TALLY_SITES - 1);
  for (size_t probe = 0; probe < TALLY_SITES; probe++) {
    if (g_sites[i].site == site)
      return i;
    if (g_sites[i].site == NULL) {
      g_sites[i].site = site;
      g_nsites++;
      return i;
    }
    i = (i + 1) & (TALLY_SITES - 1);
  }
  return 0; // more distinct sites than the table holds: fold them together
}

// ------------------------------------------------------------------ blocks
//
// Pointer to owning site, so that a free can be charged back to the line that
// allocated it rather than to the line that released it. Tombstoned open
// addressing; the table only ever grows.

typedef struct {
  void *p; // NULL empty, TOMB deleted
  size_t site;
  size_t bytes;
  bool arena; // from the bump allocator, rather than straight from malloc
} Block;

#define TOMB ((void *)(uintptr_t)1)

static Block *g_blocks;
static size_t g_cap, g_used, g_dead;

static size_t block_slot(const Block *tab, size_t cap, void *p) {
  size_t i = ((uintptr_t)p >> 4) * 2654435761u & (cap - 1);
  while (tab[i].p && tab[i].p != p)
    i = (i + 1) & (cap - 1);
  return i;
}

static void blocks_grow(void) {
  size_t cap = g_cap ? g_cap * 2 : 1 << 16;
  Block *tab = (Block *)calloc(cap, sizeof(Block));
  if (!tab)
    w_fail("out of memory keeping the allocation tally");
  for (size_t i = 0; i < g_cap; i++) {
    if (!g_blocks[i].p || g_blocks[i].p == TOMB)
      continue;
    tab[block_slot(tab, cap, g_blocks[i].p)] = g_blocks[i];
  }
  free(g_blocks);
  g_blocks = tab;
  g_cap = cap;
  g_used -= g_dead;
  g_dead = 0;
}

// ------------------------------------------------------------------- totals

static size_t g_live, g_peak, g_snapshot;
static size_t g_arena;     // the part of g_live that came out of a chunk
static size_t g_unmatched; // frees of a block nobody recorded
static size_t g_chunks;    // what the arena has taken from the OS
static size_t g_reported;

// w_tally_chunk is the arena telling the tally what it took from the OS. The
// difference between that and the live total is the arena's own retention:
// blocks on a free list, and the tail of a chunk something outgrew. It is never
// given back, so a program whose peak and whose exit total are far apart still
// holds the peak.
void w_tally_chunk(size_t bytes) { g_chunks += bytes; }

// A fresh snapshot of every site's live bytes costs a walk of the site table,
// and the peak is approached in thousands of small steps, so take one only when
// the high-water mark has moved by more than about 1.5%. The exact peak is a
// scalar and is always precise; it is the *breakdown* that lags, and the report
// says by how much.
static void note_peak(void) {
  if (g_live <= g_peak)
    return;
  g_peak = g_live;
  if (g_live < g_snapshot + g_snapshot / 64 + 4096)
    return;
  g_snapshot = g_live;
  for (size_t i = 0; i < TALLY_SITES; i++)
    g_sites[i].at_peak = g_sites[i].live;
}

// Rounding has to match weave.h and weave.c exactly, or a block is recorded at
// one size and released at another.
static size_t round_up(size_t bytes) { return (bytes + 15) & ~(size_t)15; }

static void record(const char *where, void *p, size_t bytes, bool arena) {
  if (!g_cap || (g_used + 1) * 10 >= g_cap * 7)
    blocks_grow();
  size_t i = block_slot(g_blocks, g_cap, p);
  if (!g_blocks[i].p)
    g_used++;
  size_t s = site_index(where);
  g_blocks[i].p = p;
  g_blocks[i].site = s;
  g_blocks[i].bytes = bytes;
  g_blocks[i].arena = arena;

  g_sites[s].live += bytes;
  g_sites[s].allocated += bytes;
  g_sites[s].blocks++;
  g_live += bytes;
  if (arena)
    g_arena += bytes;
  note_peak();
}

// forget takes a block off the books, charging it back to the line that
// allocated it rather than the line that released it.
static void forget(void *p, size_t bytes) {
  if (g_cap) {
    size_t i = block_slot(g_blocks, g_cap, p);
    if (g_blocks[i].p == p) {
      g_sites[g_blocks[i].site].live -= g_blocks[i].bytes;
      g_live -= g_blocks[i].bytes;
      if (g_blocks[i].arena)
        g_arena -= g_blocks[i].bytes;
      g_blocks[i].p = TOMB;
      g_dead++;
      return;
    }
  }
  // Nothing recorded this pointer. The one legitimate way that happens is a
  // free of part of a block — see w_thread_fit, which hands back a buffer's
  // unused tail and calls w_tally_shrink instead. Anything reaching here is a
  // genuine mismatch and is worth seeing in the report.
  g_unmatched += bytes;
}

void *w_alloc_at(const char *where, size_t bytes) {
  bytes = round_up(bytes);
  if (bytes == 0)
    bytes = 16;
  void *p = w_alloc_impl(bytes);
  record(where, p, bytes, true);
  return p;
}

void w_free_at(const char *where, void *p, size_t bytes) {
  (void)where;
  bytes = round_up(bytes);
  if (p && bytes != 0)
    forget(p, bytes);
  w_free_impl(p, bytes);
}

// The two `malloc`-backed entry points. They record exactly as the arena ones
// do, so a flat table and a Thread array appear side by side in the report even
// though they come from different allocators.
void *w_raw_alloc_at(const char *where, size_t bytes) {
  void *p = malloc(bytes);
  if (!p)
    w_fail("out of memory");
  record(where, p, bytes, false);
  return p;
}

void w_raw_free_at(void *p, size_t bytes) {
  if (p)
    forget(p, bytes);
  free(p);
}

// w_tally_shrink records that a block has given back everything past `to`
// bytes. The caller frees the tail itself, bypassing the tally, because the
// tail's address is not a block this table knows.
void w_tally_shrink(void *p, size_t from, size_t to) {
  (void)from;
  to = round_up(to);
  if (!g_cap || !p)
    return;
  size_t i = block_slot(g_blocks, g_cap, p);
  if (g_blocks[i].p != p || g_blocks[i].bytes <= to)
    return;
  size_t given = g_blocks[i].bytes - to;
  g_blocks[i].bytes = to;
  g_sites[g_blocks[i].site].live -= given;
  g_live -= given;
  if (g_blocks[i].arena)
    g_arena -= given;
}

// ------------------------------------------------------------------- report

static const char *scaled(size_t bytes, char *buf, size_t n) {
  const char *unit[] = {"B", "KB", "MB", "GB", "TB"};
  double v = (double)bytes;
  size_t i = 0;
  while (v >= 1024.0 && i + 1 < sizeof(unit) / sizeof(unit[0])) {
    v /= 1024.0;
    i++;
  }
  snprintf(buf, n, "%.1f %s", v, unit[i]);
  return buf;
}

// The site strings are whatever path the C compiler was handed, which for a
// cached runtime is a long absolute one. The file name is the part that means
// anything: `collections.c:694` locates a line, and `program.c:202` says the
// generated code rather than the runtime.
static const char *shortened(const char *site) {
  const char *slash = strrchr(site, '/');
  return slash ? slash + 1 : site;
}

static int by_peak(const void *a, const void *b) {
  const Site *x = (const Site *)a, *y = (const Site *)b;
  if (x->at_peak != y->at_peak)
    return x->at_peak < y->at_peak ? 1 : -1;
  if (x->allocated != y->allocated)
    return x->allocated < y->allocated ? 1 : -1;
  return 0;
}

static void report(void) {
  if (g_reported)
    return;
  g_reported = 1;

  Site *rows = (Site *)calloc(TALLY_SITES, sizeof(Site));
  if (!rows)
    return;
  size_t n = 0;
  for (size_t i = 0; i < TALLY_SITES; i++)
    if (g_sites[i].site)
      rows[n++] = g_sites[i];
  qsort(rows, n, sizeof(Site), by_peak);

  char a[32], b[32], c[32], d[32];
  fprintf(stderr, "\nweave: heap high-water mark %s",
          scaled(g_peak, a, sizeof a));
  if (g_snapshot != g_peak)
    fprintf(stderr, " (breakdown taken at %s)", scaled(g_snapshot, b, sizeof b));
  fprintf(stderr, ", %s still live at exit\n", scaled(g_live, c, sizeof c));
  // The arena never gives a chunk back, so what it took from the OS and what is
  // live in it are two different numbers, and the gap is what the process is
  // holding for nothing: free-listed blocks, and the tail of every chunk that
  // something outgrew.
  fprintf(stderr,
          "weave: arena holds %s from the OS for %s live, %s of it idle; "
          "%s live outside it\n",
          scaled(g_chunks, a, sizeof a), scaled(g_arena, b, sizeof b),
          scaled(g_chunks > g_arena ? g_chunks - g_arena : 0, c, sizeof c),
          scaled(g_live - g_arena, d, sizeof d));
  if (g_unmatched)
    fprintf(stderr, "weave: %s freed from blocks nobody recorded\n",
            scaled(g_unmatched, a, sizeof a));
  fprintf(stderr, "\n  %12s  %12s  %12s  %10s  %s\n", "at peak", "at exit",
          "allocated", "blocks", "site");

  // Everything holding a twentieth of a percent of the peak, which on a real
  // program is a dozen lines and not a hundred.
  size_t floor = g_peak / 2000;
  for (size_t i = 0; i < n; i++) {
    if (rows[i].at_peak < floor && rows[i].live < floor)
      continue;
    fprintf(stderr, "  %12s  %12s  %12s  %10zu  %s\n",
            scaled(rows[i].at_peak, a, sizeof a),
            scaled(rows[i].live, b, sizeof b),
            scaled(rows[i].allocated, c, sizeof c), rows[i].blocks,
            shortened(rows[i].site));
  }
  fprintf(stderr, "\n");
  free(rows);
}

void w_tally_start(void) { atexit(report); }

#endif // WEAVE_TALLY
