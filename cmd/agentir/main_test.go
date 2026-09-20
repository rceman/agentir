package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectCommandCompact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.go")
	if err := os.WriteFile(path, []byte("package demo\nfunc f(x int) int { return x + 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := run([]string{"project", path}, &out, &errOut); err != nil {
		t.Fatalf("run: %v; stderr=%s", err, errOut.String())
	}
	for _, want := range []string{"agentir/v0 go", "fn f(x int) int", "n1 return x + 1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in:\n%s", want, out.String())
		}
	}
}

func TestProjectCommandJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.go")
	if err := os.WriteFile(path, []byte("package demo\nfunc f() int { return 1 + 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := run([]string{"project", "--format", "json", path}, &out, &errOut); err != nil {
		t.Fatalf("run: %v; stderr=%s", err, errOut.String())
	}
	for _, want := range []string{`"version": "agentir/v0"`, `"kind": "return"`, `"signature": "f() int"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in:\n%s", want, out.String())
		}
	}
}

func TestApplyIRCommand(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "demo.go")
	src := []byte("package demo\nfunc f(name string) string { return trim(name) }\n")
	if err := os.WriteFile(sourcePath, src, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(src)
	patchPath := filepath.Join(dir, "patch.json")
	patchJSON := `{"version":"agentir/irpatch-v0","source_sha256":"` + hex.EncodeToString(sum[:]) + `","edits":[{"node_id":"n1","text":"return normalize(name)"}]}`
	if err := os.WriteFile(patchPath, []byte(patchJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := run([]string{"apply-ir", "--patch", patchPath, sourcePath}, &out, &errOut); err != nil {
		t.Fatalf("run: %v; stderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "return normalize(name)") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}
