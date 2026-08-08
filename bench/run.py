#!/usr/bin/env python3
"""Build and time every benchmark in both languages.

Usage:  python3 bench/run.py [-n runs] [filter ...]

For each benchmark this builds the Weave program with `weave build -opt -O3`
and the Go program with `go build`, runs both against the same input, checks
that they print the same answer, and reports the best wall time of N runs
along with the peak resident memory.

Best-of-N rather than mean: the fastest run is the one least disturbed by the
rest of the machine, and it is the number that reproduces.
"""

import argparse
import os
import resource
import subprocess
import sys
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BENCH = os.path.join(ROOT, "bench")
WORK = os.path.join(BENCH, "build")

AOC = ["day01", "day02", "day10", "day11", "day22"]
RAW = ["fib", "chain", "chainalloc", "loop", "collatz", "mapbuild", "text"]

# The scalar arguments the raw benchmarks are run with. Sized so every one
# takes a noticeable fraction of a second, and none takes minutes.
RAW_INPUT = {
    "fib": "32",
    "chain": "20000000",
    "chainalloc": "20000000",
    "loop": "100000000",
    "collatz": "300000",
    "mapbuild": "2000000",
}


def sh(cmd, **kw):
    return subprocess.run(cmd, check=True, capture_output=True, text=True, **kw)


def build_compiler():
    sh(["go", "build", "-o", os.path.join(WORK, "weave"), "./cmd/weave"], cwd=ROOT)
    return os.path.join(WORK, "weave")


def build_weave(weave, src, out):
    sh([weave, "build", "-opt", "-O3", "-o", out, src])
    return out


def build_go(pkg, out):
    sh(["go", "build", "-o", out, pkg], cwd=os.path.join(BENCH, "go"))
    return out


def once(exe, stdin_path, out_path):
    """Run exe with stdin from a file, returning (seconds, peak KiB).

    fork and wait4 rather than subprocess, because wait4 reports the child's
    own peak resident memory — which for these programs is half the story.
    """
    start = time.perf_counter()
    pid = os.fork()
    if pid == 0:
        fd = os.open(stdin_path, os.O_RDONLY)
        os.dup2(fd, 0)
        fo = os.open(out_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)
        os.dup2(fo, 1)
        os.execv(exe, [exe])
    _, status, ru = os.wait4(pid, 0)
    elapsed = time.perf_counter() - start
    if status != 0:
        raise SystemExit("%s exited with status %d" % (exe, status))
    return elapsed, ru.ru_maxrss


def best_of(exe, stdin_path, runs):
    out_path = os.path.join(WORK, "out.txt")
    best, peak = None, 0
    for _ in range(runs):
        t, rss = once(exe, stdin_path, out_path)
        best = t if best is None else min(best, t)
        peak = max(peak, rss)
    return best, peak, open(out_path).read().strip()


def scalar_input(name):
    path = os.path.join(WORK, name + ".in")
    with open(path, "w") as f:
        f.write(RAW_INPUT[name] + "\n")
    return path


def make_text_input():
    """A words-and-lines corpus for the `text` benchmark: 400k lines."""
    path = os.path.join(BENCH, "input", "text.txt")
    if os.path.exists(path):
        return path
    words = ["thread", "weave", "on", "the", "pattern", "of", "an", "age",
             "spun", "out", "wind", "rose", "in", "mountains", "mist"]
    with open(path, "w") as f:
        n = 0
        for _ in range(400000):
            line = []
            for _ in range(8):
                line.append(words[n % len(words)])
                n += 7
            f.write(" ".join(line) + "\n")
    return path


def mb(kib):
    return kib / 1024.0


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("-n", "--runs", type=int, default=5)
    ap.add_argument("filter", nargs="*")
    args = ap.parse_args()

    os.makedirs(WORK, exist_ok=True)
    weave = build_compiler()

    jobs = []
    for name in RAW:
        stem = "chain" if name == "chainalloc" else name
        weave_src = os.path.join(BENCH, "weave", "raw", stem + ".weave")
        stdin = make_text_input() if name == "text" else scalar_input(name)
        jobs.append(("raw/" + name, weave_src, "./raw/" + name, stdin))

    for day in AOC:
        jobs.append((day,
                     os.path.join(BENCH, "weave", day + ".weave"),
                     "./" + day,
                     os.path.join(BENCH, "input", day + ".txt")))

    if args.filter:
        jobs = [j for j in jobs if any(f in j[0] for f in args.filter)]

    head = "%-16s %10s %8s %10s %8s %7s   %s"
    print(head % ("benchmark", "weave", "rss", "go", "rss", "ratio", "answer"))
    print("-" * 86)
    disagreed = []
    for name, weave_src, go_pkg, stdin in jobs:
        tag = name.replace("/", "_")
        wexe = build_weave(weave, weave_src, os.path.join(WORK, "w_" + tag))
        gexe = build_go(go_pkg, os.path.join(WORK, "g_" + tag))

        wt, wrss, wout = best_of(wexe, stdin, args.runs)
        gt, grss, gout = best_of(gexe, stdin, args.runs)

        note = wout.replace("\n", " ")
        if wout != gout:
            note = "MISMATCH weave=%r go=%r" % (wout, gout)
            disagreed.append(name)
        print("%-16s %8.1fms %7.0fM %8.1fms %7.0fM %6.2fx   %s"
              % (name, wt * 1000, mb(wrss), gt * 1000, mb(grss), wt / gt, note))

    if disagreed:
        print("\ndisagreed: " + ", ".join(disagreed))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
