package project

import (
	"strings"
	"testing"
)

func TestProjectExpandsNestedExpression(t *testing.T) {
	src := []byte(`package demo

func load(ctx string) []int { return nil }
func filter(v []int) []int { return v }
func run(ctx string) []int {
	users := filter(load(ctx))
	return users
}
`)

	projection, err := Project("demo.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Document.Functions) != 3 {
		t.Fatalf("functions=%d, want 3", len(projection.Document.Functions))
	}

	var got []string
	for _, fn := range projection.Document.Functions {
		if fn.Name != "run" {
			continue
		}
		if fn.Signature != "run(ctx string) []int" {
			t.Fatalf("signature=%q", fn.Signature)
		}
		for _, op := range fn.Ops {
			if op.Result != "" {
				got = append(got, op.Result+"="+op.Text)
			} else {
				got = append(got, op.Text)
			}
		}
	}

	want := []string{
		"$1=load(ctx)",
		"users := filter($1)",
		"return users",
	}
	if len(got) != len(want) {
		t.Fatalf("ops=%q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("op[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestStructuralControlRowsAreNotPatchable(t *testing.T) {
	src := []byte(`package demo
func f(x bool) int {
	if !x {
		return 1
	}
	return 2
}
`)
	projection, err := Project("demo.go", src)
	if err != nil {
		t.Fatal(err)
	}
	ops := projection.Document.Functions[0].Ops
	var sawIf bool
	for _, op := range ops {
		if op.Kind == "if" {
			sawIf = true
			if op.ID != "" {
				t.Fatalf("structural if unexpectedly addressable: %+v", op)
			}
		}
	}
	if !sawIf {
		t.Fatal("missing if row")
	}
	for id, node := range projection.Nodes {
		if node.Kind == "if" || id == "" {
			t.Fatalf("invalid patch node %q: %+v", id, node)
		}
	}
}

func TestCommentsRemainVisible(t *testing.T) {
	src := []byte(`package demo
func f(x int) int {
	// preserve semantic hint
	if x > 0 {
		// positive path
		return x
	}
	return 0
}
`)
	projection, err := Project("demo.go", src)
	if err != nil {
		t.Fatal(err)
	}
	var text []string
	for _, op := range projection.Document.Functions[0].Ops {
		text = append(text, op.Text)
	}
	joined := strings.Join(text, "
")
	for _, want := range []string{"// preserve semantic hint", "// positive path"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:
%s", want, joined)
		}
	}
}

func TestProjectDeterministic(t *testing.T) {
	src := []byte("package demo\nfunc f(x int) int { return x + 1 }\n")
	a, err := Project("demo.go", src)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Project("demo.go", src)
	if err != nil {
		t.Fatal(err)
	}

	aOps := a.Document.Functions[0].Ops
	bOps := b.Document.Functions[0].Ops
	if len(aOps) != len(bOps) {
		t.Fatal("operation counts differ")
	}
	for i := range aOps {
		if aOps[i].ID != bOps[i].ID || aOps[i].Text != bOps[i].Text || aOps[i].Result != bOps[i].Result {
			t.Fatalf("projection differs at op %d", i)
		}
	}
}
