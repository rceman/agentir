package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/rceman/agentir/internal/ir"
	"github.com/rceman/agentir/internal/patch"
	"github.com/rceman/agentir/internal/project"
	"github.com/rceman/agentir/internal/rewrite"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "agentir:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return errors.New("command required")
	}
	switch args[0] {
	case "project":
		return runProject(args[1:], stdout, stderr)
	case "apply":
		return runApply(args[1:], stdout, stderr)
	case "apply-ir":
		return runApplyIR(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runProject(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("project", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "compact", "output format: compact or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: agentir project [--format compact|json] <file.go>")
	}

	path := fs.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	projection, err := project.Project(path, data)
	if err != nil {
		return err
	}
	switch *format {
	case "compact":
		return ir.WriteCompact(stdout, projection.Document)
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(projection.Document)
	default:
		return fmt.Errorf("unsupported format %q; want compact or json", *format)
	}
}

func runApply(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(stderr)
	patchPath := fs.String("patch", "", "path to raw-Go node patch JSON")
	write := fs.Bool("write", false, "write the patched source back to the input file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *patchPath == "" {
		return errors.New("usage: agentir apply --patch <patch.json> [--write] <file.go>")
	}
	return applyFile(fs.Arg(0), *patchPath, *write, stdout, patch.Parse, patch.Apply)
}

func runApplyIR(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("apply-ir", flag.ContinueOnError)
	fs.SetOutput(stderr)
	patchPath := fs.String("patch", "", "path to AgentIR operation patch JSON")
	write := fs.Bool("write", false, "write the rewritten source back to the input file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *patchPath == "" {
		return errors.New("usage: agentir apply-ir --patch <patch.json> [--write] <file.go>")
	}

	path := fs.Arg(0)
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	patchData, err := os.ReadFile(*patchPath)
	if err != nil {
		return fmt.Errorf("read patch: %w", err)
	}
	p, err := rewrite.Parse(patchData)
	if err != nil {
		return err
	}
	out, err := rewrite.Apply(path, src, p)
	if err != nil {
		return err
	}
	return finishApply(path, out, *write, stdout)
}

func applyFile[P any](path, patchPath string, write bool, stdout io.Writer, parse func([]byte) (P, error), apply func(string, []byte, P) ([]byte, error)) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	patchData, err := os.ReadFile(patchPath)
	if err != nil {
		return fmt.Errorf("read patch: %w", err)
	}
	p, err := parse(patchData)
	if err != nil {
		return err
	}
	out, err := apply(path, src, p)
	if err != nil {
		return err
	}
	return finishApply(path, out, write, stdout)
}

func finishApply(path string, out []byte, write bool, stdout io.Writer) error {
	if write {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat source: %w", err)
		}
		if err := os.WriteFile(path, out, info.Mode().Perm()); err != nil {
			return fmt.Errorf("write source: %w", err)
		}
		return nil
	}
	_, err := stdout.Write(out)
	return err
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `AgentIR experimental CLI

Usage:
  agentir project [--format compact|json] <file.go>
  agentir apply --patch <patch.json> [--write] <file.go>
  agentir apply-ir --patch <patch.json> [--write] <file.go>

project creates an ephemeral agent-facing view.
apply is the raw-Go replacement baseline.
apply-ir translates edited AgentIR operation text back to minimal Go source spans.`)
}
