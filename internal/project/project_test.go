package project

import (
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

	var runOps []string
	for _, fn := range projection.Document.Functions {
		if fn.Name != "run" {
			continue
		}
		for _, op := range fn.Ops {
			runOps = append(runOps, op.Text)
		}
	}

	want := []string{
		"call load(ctx)",
		"call filter($t1)",
		"users := $t2",
		"return users",
	}
	if len(runOps) != len(want) {
		t.Fatalf("ops=%q, want %q", runOps, want)
	}
	for i := range want {
		if runOps[i] != want[i] {
			t.Fatalf("op[%d]=%q, want %q", i, runOps[i], want[i])
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

	if a.Document.Source.SHA256 != b.Document.Source.SHA256 {
		t.Fatal("source hashes differ")
	}
	if len(a.Document.Functions) != 1 || len(b.Document.Functions) != 1 {
		t.Fatal("unexpected function count")
	}
	aOps := a.Document.Functions[0].Ops
	bOps := b.Document.Functions[0].Ops
	if len(aOps) != len(bOps) {
		t.Fatal("operation counts differ")
	}
	for i := range aOps {
		if aOps[i].ID != bOps[i].ID || aOps[i].Text != bOps[i].Text {
			t.Fatalf("projection differs at op %d", i)
		}
	}
}
