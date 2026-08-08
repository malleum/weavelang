// AoC 2024 day 2, in Go, for comparison with bench/weave/day02.weave.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func safe(r []int) bool {
	up, down := true, true
	for i := 0; i+1 < len(r); i++ {
		d := r[i+1] - r[i]
		if d < 1 || d > 3 {
			up = false
		}
		if d > -1 || d < -3 {
			down = false
		}
	}
	return up || down
}

func damped(r []int) bool {
	if safe(r) {
		return true
	}
	buf := make([]int, 0, len(r))
	for i := range r {
		buf = buf[:0]
		buf = append(buf, r[:i]...)
		buf = append(buf, r[i+1:]...)
		if safe(buf) {
			return true
		}
	}
	return false
}

func main() {
	var reports [][]int
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	for in.Scan() {
		f := strings.Fields(in.Text())
		if len(f) == 0 {
			continue
		}
		r := make([]int, len(f))
		for i, s := range f {
			r[i], _ = strconv.Atoi(s)
		}
		reports = append(reports, r)
	}

	p1, p2 := 0, 0
	for _, r := range reports {
		if safe(r) {
			p1++
		}
		if damped(r) {
			p2++
		}
	}
	fmt.Println(p1)
	fmt.Println(p2)
}
