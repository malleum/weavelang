// Mirror of bench/weave/raw/collatz.weave: branch-heavy integer work.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func step(n int) int {
	if n%2 == 0 {
		return n / 2
	}
	return 3*n + 1
}

func length(n int) int {
	acc := 0
	for n != 1 {
		n = step(n)
		acc++
	}
	return acc
}

func main() {
	src, _ := io.ReadAll(os.Stdin)
	n, _ := strconv.Atoi(strings.TrimSpace(string(src)))

	best := 0
	for s := 1; s <= n; s++ {
		if l := length(s); l > best {
			best = l
		}
	}
	fmt.Println(best)
}
