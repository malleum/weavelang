// Mirror of bench/weave/raw/fib.weave: naive recursion, nothing to optimise.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func fib(n int) int {
	if n == 0 {
		return 0
	}
	if n == 1 {
		return 1
	}
	return fib(n-1) + fib(n-2)
}

func main() {
	src, _ := io.ReadAll(os.Stdin)
	n, _ := strconv.Atoi(strings.TrimSpace(string(src)))
	fmt.Println(fib(n))
}
