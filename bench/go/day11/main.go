// AoC 2024 day 11, in Go, for comparison with bench/weave/day11.weave.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type key struct{ stone, blinks int }

var memo = map[key]int{}

func digits(n int) int {
	d := 1
	for n > 9 {
		n /= 10
		d++
	}
	return d
}

func pow10(k int) int {
	p := 1
	for ; k > 0; k-- {
		p *= 10
	}
	return p
}

func stones(s, n int) int {
	if n == 0 {
		return 1
	}
	k := key{s, n}
	if v, ok := memo[k]; ok {
		return v
	}
	var out int
	switch d := digits(s); {
	case s == 0:
		out = stones(1, n-1)
	case d%2 == 0:
		half := pow10(d / 2)
		out = stones(s/half, n-1) + stones(s%half, n-1)
	default:
		out = stones(s*2024, n-1)
	}
	memo[k] = out
	return out
}

func main() {
	raw, _ := io.ReadAll(bufio.NewReader(os.Stdin))
	var start []int
	for _, f := range strings.Fields(string(raw)) {
		if n, err := strconv.Atoi(f); err == nil {
			start = append(start, n)
		}
	}
	p1, p2 := 0, 0
	for _, s := range start {
		p1 += stones(s, 25)
		p2 += stones(s, 75)
	}
	fmt.Println(p1)
	fmt.Println(p2)
}
