// AoC 2024 day 22, in Go, for comparison with bench/weave/day22.weave.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func next(s int) int {
	s = (s ^ (s * 64)) % 16777216
	s = (s ^ (s / 32)) % 16777216
	s = (s ^ (s * 2048)) % 16777216
	return s
}

func main() {
	var buyers []int
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		buyers = append(buyers, n)
	}

	part1 := 0
	totals := map[int]int{}
	seen := map[int]int{} // key -> the buyer index that last claimed it

	for b, s := range buyers {
		prices := make([]int, 2001)
		for i := 0; i <= 2000; i++ {
			prices[i] = s % 10
			if i < 2000 {
				s = next(s)
			}
		}
		part1 += s

		for i := 0; i+4 < len(prices); i++ {
			key := 0
			for j := 0; j < 4; j++ {
				key = key*19 + (prices[i+j+1] - prices[i+j] + 9)
			}
			if last, ok := seen[key]; ok && last == b+1 {
				continue
			}
			seen[key] = b + 1
			totals[key] += prices[i+4]
		}
	}

	part2 := 0
	for _, v := range totals {
		if v > part2 {
			part2 = v
		}
	}

	fmt.Println(part1)
	fmt.Println(part2)
}
