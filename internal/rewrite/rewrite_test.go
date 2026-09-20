package rewrite

import (
	"strings"
	"testing"

	"github.com/rceman/agentir/internal/ir"
	"github.com/rceman/agentir/internal/project"
)

func TestApplyIRExpandsTemporaryBackToSource(t *testing.T) {
	src := []byte(`package demo
import "strings"
func f(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
`)
	projection, err := project.Project("demo.go", src)
	if err != nil {
		t.Fatal(err)
	}
	fn := projection.Document.Functions[0]
	if len(fn.Ops) < 2 {
		t.Fatalf("unexpected ops: %+v", fn.Ops)
	}
	inner := fn.Ops[0]
	ret := fn.Ops[1]
	if inner.Result != "$1" || ret.Kind != "return" {
		t.Fatalf("unexpected ops: %+v", fn.Ops)
	}

	p := ir.IRPatch{
		Version:      ir.IRPatchVersion,
		SourceSHA256: projection.Document.Source.SHA256,
		Edits: []ir.IREdit{{
			NodeID: ret.ID,
			Text:   `return strings.ToUpper($1)`,
		}},
	}
	out, err := Apply("demo.go", src, p)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, `return strings.ToUpper((strings.TrimSpace(name)))`) {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestApplyIRCanEditStatementUsingTemporary(t *testing.T) {
	src := []byte(`package demo
func wrap(v int) int { return v }
func f(a, b int) int {
	x := wrap(a + b)
	return x
}
`)
	projection, err := project.Project("demo.go", src)
	if err != nil {
		t.Fatal(err)
	}
	var assign ir.Op
	for _, fn := range projection.Document.Functions {
		if fn.Name != "f" {
			continue
		}
		for _, op := range fn.Ops {
			if op.Kind == "assign" {
				assign = op
			}
		}
	}
	if assign.ID == "" {
		t.Fatal("assignment not found")
	}
	p := ir.IRPatch{
		Version:      ir.IRPatchVersion,
		SourceSHA256: projection.Document.Source.SHA256,
		Edits:        []ir.IREdit{{NodeID: assign.ID, Text: "x := wrap($1) * 2"}},
	}
	out, err := Apply("demo.go", src, p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "x := wrap((a + b)) * 2") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestExpandTempsIgnoresQuotedDollar(t *testing.T) {
	src := []byte("abc")
	span := ir.Span{StartOffset: 0, EndOffset: 3}
	got, err := expandTemps(`fmt.Sprintf("$1", $1)`, src, map[string]ir.Span{"$1": span})
	if err != nil {
		t.Fatal(err)
	}
	if got != `fmt.Sprintf("$1", (abc))` {
		t.Fatalf("got %q", got)
	}
}

func TestApplyIRRejectsUnknownTemporary(t *testing.T) {
	src := []byte("package demo\nfunc f() int { return wrap(1 + 2) }\n")
	projection, err := project.Project("demo.go", src)
	if err != nil {
		t.Fatal(err)
	}
	var ret ir.Op
	for _, op := range projection.Document.Functions[0].Ops {
		if op.Kind == "return" {
			ret = op
		}
	}
	if ret.ID == "" {
		t.Fatal("return node not found")
	}
	p := ir.IRPatch{Version: ir.IRPatchVersion, SourceSHA256: projection.Document.Source.SHA256, Edits: []ir.IREdit{{NodeID: ret.ID, Text: "return $999"}}}
	_, err = Apply("demo.go", src, p)
	if err == nil || !strings.Contains(err.Error(), "unknown temporary $999") {
		t.Fatalf("error=%v", err)
	}
}
