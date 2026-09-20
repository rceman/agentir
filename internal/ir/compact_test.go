package ir

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteCompactLeanFormat(t *testing.T) {
	doc := Document{
		Version:  Version,
		Language: "go",
		Source:   SourceRef{Path: "demo.go", SHA256: "abc"},
		Functions: []Function{{
			Name:      "f",
			Signature: "f(x bool) int",
			Ops: []Op{
				{ID: "n1", Kind: "value", Result: "$1", Text: "x"},
				{Kind: "if", Text: "if $1"},
				{ID: "n2", Kind: "return", Text: "return 1", Depth: 1},
			},
		}},
	}

	var b bytes.Buffer
	if err := WriteCompact(&b, doc); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	for _, want := range []string{
		"agentir/v0 go demo.go sha256=abc",
		"fn f(x bool) int",
		" n1 $1=x",
		" if $1",
		"  n2 return 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "binary     ") || strings.Contains(got, "return     ") {
		t.Fatalf("compact output leaked verbose kind labels:\n%s", got)
	}
}
