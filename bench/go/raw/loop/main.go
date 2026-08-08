// Mirror of bench/weave/raw/loop.weave: tail recursion, which Weave compiles
// to a jump. In Go that is a for loop.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func main() {
	src, _ := io.ReadAll(os.Stdin)
	n, _ := strconv.Atoi(strings.TrimSpace(string(src)))

	acc := 0
	for ; n > 0; n-- {
		acc += n % 7
	}
	fmt.Println(acc)
}
