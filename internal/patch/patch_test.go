package patch

import (
	"strings"
	"testing"

	"github.com/rceman/agentir/internal/ir"
	"github.com/rceman/agentir/internal/project"
)

func TestApplyReplacesExactExpressionAndPreservesSurroundingSource(t *testing.T) {
	src := []byte(`package demo

func run(name string) string {
	// keep this comment and formatting exactly
	return formatName(name)
}
`)
	projection, err := project.Project("demo.go", src)
	if err != nil {
		t.Fatal(err)
	}

	var callID string
	for _, fn := range projection.Document.Functions {
		for _, op := range fn.Ops {
			if op.Kind == "call" && strings.Contains(op.Text, "formatName") {
				callID = op.ID
			}
		}
	}
	if callID == "" {
		t.Fatal("call node not found")
	}

	p := ir.Patch{
		Version:      ir.PatchVersion,
		SourceSHA256: projection.Document.Source.SHA256,
		Edits: []ir.Edit{{
			NodeID:      callID,
			Replacement: "normalizeName(name)",
		}},
	}
	out, err := Apply("demo.go", src, p)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "// keep this comment and formatting exactly\n\treturn normalizeName(name)") {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestApplyRejectsStaleSource(t *testing.T) {
	src := []byte("package demo\nfunc f() int { return 1 }\n")
	p := ir.Patch{
		Version:      ir.PatchVersion,
		SourceSHA256: "stale",
		Edits:        []ir.Edit{{NodeID: "n_missing", Replacement: "2"}},
	}
	_, err := Apply("demo.go", src, p)
	if err == nil || !strings.Contains(err.Error(), "source hash mismatch") {
		t.Fatalf("error=%v, want source hash mismatch", err)
	}
}
