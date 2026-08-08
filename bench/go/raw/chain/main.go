// Mirror of bench/weave/raw/chain.weave.
//
// The Weave version is written as a pipeline. Fusion turns it into one loop
// with no intermediate Thread, so the honest Go counterpart is the loop a Go
// programmer would write by hand.
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

	sum := 0
	for x := 1; x <= n; x++ {
		sq := (x * x) % 1000003
		if sq%2 == 0 {
			sum += sq
		}
	}
	fmt.Println(sum)
}
