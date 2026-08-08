// Mirror of bench/weave/raw/mapbuild.weave.
//
// Weave's Web is a persistent hash-array-mapped trie; Go's map is a mutable
// hash table. Weave's in-place optimisation removes the copying when the old
// map is dead, which is what this measures.
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

	w := map[int]int{}
	for ; n > 0; n-- {
		w[(n*2654435761)%1000003] = n
	}
	fmt.Println(len(w))
}
