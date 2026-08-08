// AoC 2024 day 10, in Go, for comparison with bench/weave/day10.weave.
package main

import (
	"bufio"
	"fmt"
	"os"
)

type knot struct{ r, c int }

var (
	grid []string
	rows int
	cols int
)

func height(k knot) int {
	if k.r < 0 || k.c < 0 || k.r >= rows || k.c >= cols {
		return -1
	}
	return int(grid[k.r][k.c] - '0')
}

func up(k knot) []knot {
	want := height(k) + 1
	var out []knot
	for _, n := range []knot{{k.r - 1, k.c}, {k.r + 1, k.c}, {k.r, k.c - 1}, {k.r, k.c + 1}} {
		if height(n) == want {
			out = append(out, n)
		}
	}
	return out
}

func reach(k knot) map[knot]bool {
	if height(k) == 9 {
		return map[knot]bool{k: true}
	}
	out := map[knot]bool{}
	for _, n := range up(k) {
		for s := range reach(n) {
			out[s] = true
		}
	}
	return out
}

func rating(k knot) int {
	if height(k) == 9 {
		return 1
	}
	total := 0
	for _, n := range up(k) {
		total += rating(n)
	}
	return total
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	for in.Scan() {
		if line := in.Text(); line != "" {
			grid = append(grid, line)
		}
	}
	rows = len(grid)
	if rows > 0 {
		cols = len(grid[0])
	}

	p1, p2 := 0, 0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			k := knot{r, c}
			if height(k) != 0 {
				continue
			}
			p1 += len(reach(k))
			p2 += rating(k)
		}
	}
	fmt.Println(p1)
	fmt.Println(p2)
}
