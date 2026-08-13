// Package rt embeds the C runtime so the compiler stays a single self-contained
// binary: `weave build` writes these sources next to the generated C and hands
// the lot to the C compiler.
package rt

import (
	"embed"
	"io/fs"
)

//go:embed csrc
var sources embed.FS

// Files returns every runtime source file, keyed by name.
func Files() map[string][]byte {
	out := map[string][]byte{}
	entries, err := fs.ReadDir(sources, "csrc")
	if err != nil {
		panic(err) // the sources are compiled in; this cannot fail at runtime
	}
	for _, e := range entries {
		b, err := sources.ReadFile("csrc/" + e.Name())
		if err != nil {
			panic(err)
		}
		out[e.Name()] = b
	}
	return out
}

// CSources lists the runtime files that must be passed to the C compiler.
var CSources = []string{"weave.c", "prelude.c", "collections.c"}

// TallySource is the arena's bookkeeping, which `weave build -tally` adds to
// the list above. It is `#ifdef`ed away without the flag, so compiling it into
// every ordinary build would buy an empty object file and a slower compile —
// and the compile is most of what a small build costs.
const TallySource = "tally.c"
