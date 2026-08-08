package check

import "testing"

func TestSumTypeConstructorTypes(t *testing.T) {
	info := ok(t, `
Direction is North | South | East | West

Move is Step Direction Earth | Rest

go d is Step d 1
stay is Rest

(go North, stay)
`)
	if got := declType(t, info, "go"); got != "Direction -> Move" {
		t.Errorf("go :: %s", got)
	}
	if got := declType(t, info, "stay"); got != "Move" {
		t.Errorf("stay :: %s", got)
	}
}

func TestParameterisedSumType(t *testing.T) {
	info := ok(t, `
Tree a is Leaf | Node (Tree a) a (Tree a)

total Leaf is 0
total (Node l v r) is add v (add (total l) (total r))

depth Leaf is 0
depth (Node l _ r) is add 1 (max (depth l) (depth r))

(total (Node Leaf 1 Leaf), depth Leaf)
`)
	if got := declType(t, info, "total"); got != "Tree Earth -> Earth" {
		t.Errorf("total :: %s", got)
	}
	if got := declType(t, info, "depth"); got != "Tree a -> Earth" {
		t.Errorf("depth :: %s", got)
	}
}

func TestSumTypeIsPolymorphicAtEachUse(t *testing.T) {
	ok(t, `
Pair a is Both a a

nums is Both 1 2
words is Both "a" "b"

(nums, words)
`)
}

func TestMutuallyRecursiveSumTypes(t *testing.T) {
	info := ok(t, `
Expr is Lit Earth | Neg Expr | Sum Terms

Terms is End | More Expr Terms

value (Lit n) is n
value (Neg e) is neg (value e)
value (Sum ts) is terms ts

terms End is 0
terms (More e rest) is add (value e) (terms rest)

value (Sum (More (Lit 1) End))
`)
	if got := declType(t, info, "value"); got != "Expr -> Earth" {
		t.Errorf("value :: %s", got)
	}
}

func TestSumTypeExhaustiveness(t *testing.T) {
	fails(t, `
Direction is North | South | East | West

turn North is East
turn East is South
turn South is West

turn North
`, "`West` is unmatched")

	fails(t, `
Tree a is Leaf | Node (Tree a) a (Tree a)

first (Node l _ _) is l

first Leaf
`, "`Leaf` is unmatched")
}

func TestSumTypeUnreachableArm(t *testing.T) {
	fails(t, `
Colour is Red | Green | Blue

name c is
  ward c
    Red : "red"
    _ : "other"
    Blue : "blue"

name Red
`, "this arm can never match")
}

func TestSumTypeArityMismatch(t *testing.T) {
	fails(t, `
Move is Step Earth Earth | Rest

go (Step n) is n
go Rest is 0

go Rest
`, "carries 2 value(s), but this pattern binds 1")
}

func TestSumTypeTalentsAreDerived(t *testing.T) {
	// Eq, Ord and Show come for free when the fields have them.
	ok(t, `
Colour is Red | Green

Tree a is Leaf | Node (Tree a) a (Tree a)

sorted is sort [Green, Red]
seen is member (circle [Red]) Green
tree is Node Leaf 1 Leaf

(sorted, seen, tree)
`)
	// A field with no Show Talent takes it away from the whole type.
	fails(t, "Fn is Wrap (Earth -> Earth)\n\nWrap (x : add x 1)\n",
		"Fn has no Show Talent")
	// And arithmetic is never derived.
	fails(t, "Colour is Red | Green\n\nadd Red Green\n", "no Reckon Talent")
}

func TestSumTypeTalentsFollowTheirArguments(t *testing.T) {
	fails(t, `
Box a is Hidden a

show is Hidden (x : x)

show
`, "has no Show Talent")
}

func TestSumTypeErrors(t *testing.T) {
	fails(t, "Earth is Nope\n\n1\n", "`Earth` is already a type")
	fails(t, "Colour is Red\nColour is Blue\n\nRed\n", "`Colour` is already a type")
	fails(t, "A is Same\nB is Same\n\nSame\n", "`Same` is already a constructor")
	fails(t, "Hold2 is Held\n\n1\n", "`Held` is already a constructor")
	fails(t, "Bad a is Mk b\n\n1\n", "does not take a type parameter named `b`")
	fails(t, "Dup a a is Mk a\n\n1\n", "lists the type parameter `a` twice")
	fails(t, "In is Wrap Source\n\n1\n", "unknown type `Source`")
}

func TestSumTypeUsedInASignature(t *testing.T) {
	info := ok(t, `
Colour is Red | Green

pick2 :: Colour -> Air
pick2 Red is "red"
pick2 Green is "green"

pick2 Red
`)
	if got := declType(t, info, "pick2"); got != "Colour -> Air" {
		t.Errorf("pick2 :: %s", got)
	}
}
