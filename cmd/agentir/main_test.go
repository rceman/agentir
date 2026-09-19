package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectCommandCompact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.go")
	if err := os.WriteFile(path, []byte("package demo\nfunc f() int { return 1 + 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	if err := run([]string{"project", path}, &out, &errOut); err != nil {
		t.Fatalf("run: %v; stderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "agentir/v0 go") {
		t.Fatalf("unexpected output: %s", out.String())
	}
	if !strings.Contains(out.String(), "binary") {
		t.Fatalf("binary operation missing: %s", out.String())
	}
}

func TestProjectCommandJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.go")
	if err := os.WriteFile(path, []byte("package demo\nfunc f() int { return 1 + 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	if err := run([]string{"project", "--format", "json", path}, &out, &errOut); err != nil {
		t.Fatalf("run: %v; stderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), `"version": "agentir/v0"`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
	if !strings.Contains(out.String(), `"kind": "binary"`) {
		t.Fatalf("binary operation missing: %s", out.String())
	}
}
