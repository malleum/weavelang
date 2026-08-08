// AoC 2024 day 1, in Go, for comparison with bench/weave/day01.weave.
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	var left, right []int
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	for in.Scan() {
		f := strings.Fields(in.Text())
		if len(f) < 2 {
			continue
		}
		a, _ := strconv.Atoi(f[0])
		b, _ := strconv.Atoi(f[1])
		left = append(left, a)
		right = append(right, b)
	}
	sort.Ints(left)
	sort.Ints(right)

	part1 := 0
	for i := range left {
		d := left[i] - right[i]
		if d < 0 {
			d = -d
		}
		part1 += d
	}

	counts := make(map[int]int, len(right))
	for _, r := range right {
		counts[r]++
	}
	part2 := 0
	for _, l := range left {
		part2 += l * counts[l]
	}

	fmt.Println(part1)
	fmt.Println(part2)
}
