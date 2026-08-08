// Mirror of bench/weave/raw/text.weave: split the input into words and count
// the long ones.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	src, _ := io.ReadAll(os.Stdin)
	text := string(src)

	long := 0
	for _, w := range strings.Fields(text) {
		if len(w) > 5 {
			long++
		}
	}

	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")

	fmt.Println(long)
	fmt.Println(len(lines))
}
