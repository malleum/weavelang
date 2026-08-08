// The same pipeline as raw/chain, written the way it reads in Weave: build a
// slice, map over it, filter it, then sum it.
//
// This is what the Weave source *says*, and what Go would have to do if you
// wrote it in the same style. Fusion is the difference between this program
// and raw/chain.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func span(lo, hi int) []int {
	out := make([]int, 0, hi-lo+1)
	for x := lo; x <= hi; x++ {
		out = append(out, x)
	}
	return out
}

func bend(xs []int, f func(int) int) []int {
	out := make([]int, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

func sift(xs []int, p func(int) bool) []int {
	var out []int
	for _, x := range xs {
		if p(x) {
			out = append(out, x)
		}
	}
	return out
}

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

func main() {
	src, _ := io.ReadAll(os.Stdin)
	n, _ := strconv.Atoi(strings.TrimSpace(string(src)))

	fmt.Println(sum(sift(bend(span(1, n), func(x int) int { return (x * x) % 1000003 }),
		func(x int) bool { return x%2 == 0 })))
}
