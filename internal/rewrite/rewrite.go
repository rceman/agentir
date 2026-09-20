package rewrite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"sort"
	"strings"

	"github.com/rceman/agentir/internal/ir"
	"github.com/rceman/agentir/internal/project"
	"github.com/rceman/agentir/internal/source"
)

func Parse(data []byte) (ir.IRPatch, error) {
	var p ir.IRPatch
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return ir.IRPatch{}, fmt.Errorf("decode IR patch: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return ir.IRPatch{}, errors.New("unexpected trailing JSON value")
		}
		return ir.IRPatch{}, fmt.Errorf("decode trailing JSON: %w", err)
	}
	if p.Version != ir.IRPatchVersion {
		return ir.IRPatch{}, fmt.Errorf("unsupported IR patch version %q", p.Version)
	}
	if p.SourceSHA256 == "" {
		return ir.IRPatch{}, errors.New("source_sha256 is required")
	}
	if len(p.Edits) == 0 {
		return ir.IRPatch{}, errors.New("at least one edit is required")
	}
	for i, edit := range p.Edits {
		if edit.NodeID == "" {
			return ir.IRPatch{}, fmt.Errorf("edits[%d].node_id is required", i)
		}
		if strings.TrimSpace(edit.Text) == "" {
			return ir.IRPatch{}, fmt.Errorf("edits[%d].text is required", i)
		}
	}
	return p, nil
}

type resolvedEdit struct {
	span        ir.Span
	replacement string
	nodeID      string
}

// Apply translates edited AgentIR operation text back to Go source. Temporary
// references ($1, $2, ...) resolve to the exact source expressions represented by
// the original projection. The original source remains canonical.
func Apply(path string, src []byte, p ir.IRPatch) ([]byte, error) {
	actualHash := source.SHA256(src)
	if actualHash != p.SourceSHA256 {
		return nil, fmt.Errorf("source hash mismatch: patch=%s source=%s", p.SourceSHA256, actualHash)
	}

	projection, err := project.Project(path, src)
	if err != nil {
		return nil, err
	}
	ops := indexOps(projection.Document)

	seen := make(map[string]struct{}, len(p.Edits))
	resolved := make([]resolvedEdit, 0, len(p.Edits))
	for _, edit := range p.Edits {
		if _, ok := seen[edit.NodeID]; ok {
			return nil, fmt.Errorf("duplicate node_id %s", edit.NodeID)
		}
		seen[edit.NodeID] = struct{}{}

		node, ok := projection.Nodes[edit.NodeID]
		if !ok {
			return nil, fmt.Errorf("unknown or non-addressable node_id %s", edit.NodeID)
		}
		op, ok := ops[edit.NodeID]
		if !ok {
			return nil, fmt.Errorf("projection invariant: missing operation %s", edit.NodeID)
		}

		expanded, err := expandTemps(edit.Text, src, projection.Temps)
		if err != nil {
			return nil, fmt.Errorf("edit %s: %w", edit.NodeID, err)
		}
		if err := validateReplacement(node.Kind, expanded); err != nil {
			return nil, fmt.Errorf("edit %s (%s): %w", edit.NodeID, op.Kind, err)
		}
		resolved = append(resolved, resolvedEdit{span: node.Span, replacement: expanded, nodeID: edit.NodeID})
	}

	return applyResolved(path, src, resolved)
}

func indexOps(doc ir.Document) map[string]ir.Op {
	out := make(map[string]ir.Op)
	for _, fn := range doc.Functions {
		for _, op := range fn.Ops {
			if op.ID != "" {
				out[op.ID] = op
			}
		}
	}
	return out
}

func expandTemps(text string, src []byte, temps map[string]ir.Span) (string, error) {
	var out strings.Builder
	for i := 0; i < len(text); {
		switch text[i] {
		case '"', '\'', '`':
			end, err := scanQuoted(text, i)
			if err != nil {
				return "", err
			}
			out.WriteString(text[i:end])
			i = end
			continue
		case '/':
			if i+1 < len(text) && text[i+1] == '/' {
				out.WriteString(text[i:])
				return out.String(), nil
			}
			if i+1 < len(text) && text[i+1] == '*' {
				end := strings.Index(text[i+2:], "*/")
				if end < 0 {
					return "", errors.New("unterminated block comment")
				}
				end = i + 2 + end + 2
				out.WriteString(text[i:end])
				i = end
				continue
			}
		case '$':
			j := i + 1
			for j < len(text) && text[j] >= '0' && text[j] <= '9' {
				j++
			}
			if j > i+1 {
				name := text[i:j]
				span, ok := temps[name]
				if !ok {
					return "", fmt.Errorf("unknown temporary %s", name)
				}
				if span.StartOffset < 0 || span.EndOffset < span.StartOffset || span.EndOffset > len(src) {
					return "", fmt.Errorf("invalid source span for temporary %s", name)
				}
				out.WriteByte('(')
				out.Write(src[span.StartOffset:span.EndOffset])
				out.WriteByte(')')
				i = j
				continue
			}
		}
		out.WriteByte(text[i])
		i++
	}
	return out.String(), nil
}

func scanQuoted(text string, start int) (int, error) {
	quote := text[start]
	for i := start + 1; i < len(text); i++ {
		if quote != '`' && text[i] == '\\' {
			i++
			continue
		}
		if text[i] == quote {
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated quoted literal starting at byte %d", start)
}

func validateReplacement(kind, replacement string) error {
	switch kind {
	case "call", "binary", "unary", "index", "slice", "type_assert", "deref":
		if _, err := parser.ParseExpr(replacement); err != nil {
			return fmt.Errorf("replacement is not a Go expression: %w", err)
		}
		return nil
	default:
		synthetic := "package p\nfunc _(){\n" + replacement + "\n}\n"
		if _, err := parser.ParseFile(token.NewFileSet(), "replacement.go", synthetic, parser.AllErrors); err != nil {
			return fmt.Errorf("replacement is not a Go statement: %w", err)
		}
		return nil
	}
}

func applyResolved(path string, src []byte, edits []resolvedEdit) ([]byte, error) {
	for _, edit := range edits {
		if edit.span.StartOffset < 0 || edit.span.EndOffset < edit.span.StartOffset || edit.span.EndOffset > len(src) {
			return nil, fmt.Errorf("invalid source span for %s", edit.nodeID)
		}
	}

	sort.Slice(edits, func(i, j int) bool {
		if edits[i].span.StartOffset == edits[j].span.StartOffset {
			return edits[i].span.EndOffset > edits[j].span.EndOffset
		}
		return edits[i].span.StartOffset < edits[j].span.StartOffset
	})
	for i := 1; i < len(edits); i++ {
		if edits[i].span.StartOffset < edits[i-1].span.EndOffset {
			return nil, fmt.Errorf("overlapping edits: %s and %s", edits[i-1].nodeID, edits[i].nodeID)
		}
	}

	out := append([]byte(nil), src...)
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		next := make([]byte, 0, len(out)-(e.span.EndOffset-e.span.StartOffset)+len(e.replacement))
		next = append(next, out[:e.span.StartOffset]...)
		next = append(next, e.replacement...)
		next = append(next, out[e.span.EndOffset:]...)
		out = next
	}
	if _, err := parser.ParseFile(token.NewFileSet(), path, out, parser.AllErrors); err != nil {
		return nil, fmt.Errorf("rewritten source does not parse: %w", err)
	}
	return out, nil
}
