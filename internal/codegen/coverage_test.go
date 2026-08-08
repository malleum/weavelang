package codegen

import (
	"sort"
	"testing"

	"github.com/malleum/weave/internal/prelude"
)

// specialForms are prelude names the code generator handles itself rather than
// through the builtins table.
var specialForms = map[string]bool{
	"Source": true, // the input, read once and memoised
	"pick":   true, // lazy: only the taken branch is evaluated
}

// notYetImplemented lists verbs that internal/prelude declares but the runtime
// does not provide. Using one is a clean compile error naming the verb. This
// list exists so that adding a signature without an implementation is a
// deliberate act, visible in a diff, rather than a surprise for whoever writes
// the program that first needs it.
var notYetImplemented = map[string]string{
	// `flow` is endless, so it has no runtime function: it exists only as the
	// fused loop that consumes it, which internal/codegen/fuse.go emits.
	"flow": "compiled as a fused loop, not a runtime call",
	// `cycle` is endless for the same reason, and fused the same way.
	"cycle": "compiled as a fused loop, not a runtime call",
}

// TestEveryPreludeVerbIsAccountedFor fails when a verb is declared with a type
// but is neither compiled nor knowingly deferred.
func TestEveryPreludeVerbIsAccountedFor(t *testing.T) {
	var missing []string
	for _, e := range prelude.Values {
		if _, ok := builtins[e.Name]; ok {
			continue
		}
		if specialForms[e.Name] || notYetImplemented[e.Name] != "" {
			continue
		}
		missing = append(missing, e.Name)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("declared in the prelude but not compiled: %v\n"+
			"add each to the builtins table, or to notYetImplemented with a reason", missing)
	}
}

// TestNoStaleBuiltins catches the reverse drift: a C implementation wired up
// for a verb the prelude no longer declares.
func TestNoStaleBuiltins(t *testing.T) {
	declared := map[string]bool{}
	for _, e := range prelude.Values {
		declared[e.Name] = true
	}
	// `knot` is both a constructor and an ordinary function, so it is declared
	// among the constructors rather than the values.
	for _, c := range prelude.Ctors {
		declared[c.Name] = true
	}
	var stale []string
	for name := range builtins {
		if !declared[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("compiled but not declared in the prelude: %v", stale)
	}
}

// TestDeferredVerbsAreStillDeclared keeps the deferral list from going stale
// once a verb is implemented or removed.
func TestDeferredVerbsAreStillDeclared(t *testing.T) {
	declared := map[string]bool{}
	for _, e := range prelude.Values {
		declared[e.Name] = true
	}
	for name := range notYetImplemented {
		if !declared[name] {
			t.Errorf("`%s` is listed as deferred but is no longer in the prelude", name)
		}
		if _, ok := builtins[name]; ok {
			t.Errorf("`%s` is listed as deferred but is now compiled; drop it from the list", name)
		}
	}
}

// ctorNotYetImplemented lists constructors with a type but no runtime
// representation, for the same reason as notYetImplemented above.
var ctorNotYetImplemented = map[string]string{}

// TestEveryConstructorCompiles checks the constructor table the same way.
func TestEveryConstructorCompiles(t *testing.T) {
	for _, c := range prelude.Ctors {
		arity, ok := ctorArity[c.Name]
		if !ok {
			if ctorNotYetImplemented[c.Name] == "" {
				t.Errorf("constructor `%s` has no code generation", c.Name)
			}
			continue
		}
		if arity != c.Arity {
			t.Errorf("constructor `%s`: prelude says arity %d, codegen says %d",
				c.Name, c.Arity, arity)
		}
	}
}
