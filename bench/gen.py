#!/usr/bin/env python3
"""Generate inputs for the benchmark programs.

Advent of Code inputs are per-user and not redistributable, so these are
generated to the same shape and size as the real ones. The sizes are taken from
the 2024 puzzles they imitate.
"""
import random, sys, os

here = os.path.dirname(os.path.abspath(__file__))
out = os.path.join(here, "input")
random.seed(20241225)

# --- day01: two columns of location ids, 1000 rows.
with open(f"{out}/day01.txt", "w") as f:
    left = [random.randint(10000, 99999) for _ in range(1000)]
    right = [random.randint(10000, 99999) for _ in range(1000)]
    for a, b in zip(left, right):
        f.write(f"{a}   {b}\n")

# --- day02: reports of 5-8 levels, 1000 rows.
with open(f"{out}/day02.txt", "w") as f:
    for _ in range(1000):
        n = random.randint(5, 8)
        start = random.randint(1, 60)
        step = random.choice([1, -1])
        lv, cur = [], start
        for _ in range(n):
            lv.append(max(1, cur))
            cur += step * random.randint(1, 4)
        if random.random() < 0.35:                 # perturb some
            i = random.randrange(n)
            lv[i] = max(1, lv[i] + random.randint(-9, 9))
        f.write(" ".join(map(str, lv)) + "\n")

# --- day09: a disk map, 20000 digits.
with open(f"{out}/day09.txt", "w") as f:
    f.write("".join(str(random.randint(1, 9)) for _ in range(19999)) + "\n")

# --- day11: 8 starting stones.
with open(f"{out}/day11.txt", "w") as f:
    f.write(" ".join(str(random.randint(0, 999999)) for _ in range(8)) + "\n")

# --- day10: a topographic map, 55x55 of heights 0-9.
#
# Interpolated value noise rather than uniform digits: adjacent cells have to
# mostly differ by 0 or 1 or there are no trails at all, which is what the real
# inputs look like and what makes the search worth timing.
SIDE = 55
def smooth_heights(side, cell=5):
    coarse = side // cell + 2
    grid = [[random.uniform(0, 9) for _ in range(coarse)] for _ in range(coarse)]
    out = []
    for r in range(side):
        row = []
        for c in range(side):
            gr, gc = r / cell, c / cell
            r0, c0 = int(gr), int(gc)
            fr, fc = gr - r0, gc - c0
            v = (grid[r0][c0] * (1 - fr) * (1 - fc) + grid[r0 + 1][c0] * fr * (1 - fc)
                 + grid[r0][c0 + 1] * (1 - fr) * fc + grid[r0 + 1][c0 + 1] * fr * fc)
            row.append(max(0, min(9, int(round(v)))))
        out.append(row)
    return out

# Smooth terrain alone rarely contains a run that climbs by exactly one nine
# times over, so trails are carved in explicitly — which is what makes the real
# inputs searchable, and what this benchmark is timing.
def carve(grid, side, trails=140):
    for _ in range(trails):
        r, c = random.randrange(side), random.randrange(side)
        path = [(r, c)]
        for _ in range(9):
            moves = [(r - 1, c), (r + 1, c), (r, c - 1), (r, c + 1)]
            moves = [(a, b) for a, b in moves
                     if 0 <= a < side and 0 <= b < side and (a, b) not in path]
            if not moves:
                break
            r, c = random.choice(moves)
            path.append((r, c))
        if len(path) == 10:
            for h, (a, b) in enumerate(path):
                grid[a][b] = h
    return grid

with open(f"{out}/day10.txt", "w") as f:
    for row in carve(smooth_heights(SIDE), SIDE):
        f.write("".join(str(v) for v in row) + "\n")

# --- day22: 2000 buyer secrets. Generated last so that adding it did not
# shift the random stream the inputs above were drawn from.
with open(f"{out}/day22.txt", "w") as f:
    for _ in range(2000):
        f.write(f"{random.randint(1, 99999999)}\n")

for name in sorted(os.listdir(out)):
    p = os.path.join(out, name)
    print(f"{name:14s} {os.path.getsize(p):>8} bytes")
