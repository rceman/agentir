package patch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"sort"

	"github.com/rceman/agentir/internal/ir"
	"github.com/rceman/agentir/internal/project"
	"github.com/rceman/agentir/internal/source"
)

func Parse(data []byte) (ir.Patch, error) {
	var p ir.Patch
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return ir.Patch{}, fmt.Errorf("decode patch: %w", err)
	}
	if err := ensureEOF(dec); err != nil {
		return ir.Patch{}, err
	}
	if p.Version != ir.PatchVersion {
		return ir.Patch{}, fmt.Errorf("unsupported patch version %q", p.Version)
	}
	if p.SourceSHA256 == "" {
		return ir.Patch{}, errors.New("source_sha256 is required")
	}
	if len(p.Edits) == 0 {
		return ir.Patch{}, errors.New("at least one edit is required")
	}
	for i, edit := range p.Edits {
		if edit.NodeID == "" {
			return ir.Patch{}, fmt.Errorf("edits[%d].node_id is required", i)
		}
	}
	return p, nil
}

func ensureEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return errors.New("unexpected trailing JSON value")
}

type resolvedEdit struct {
	span        ir.Span
	replacement string
	nodeID      string
}

// Apply is the raw-source baseline: replacements are ordinary Go source snippets.
func Apply(path string, src []byte, p ir.Patch) ([]byte, error) {
	actualHash := source.SHA256(src)
	if actualHash != p.SourceSHA256 {
		return nil, fmt.Errorf("source hash mismatch: patch=%s source=%s", p.SourceSHA256, actualHash)
	}

	projection, err := project.Project(path, src)
	if err != nil {
		return nil, err
	}

	resolved := make([]resolvedEdit, 0, len(p.Edits))
	seen := make(map[string]struct{}, len(p.Edits))
	for _, edit := range p.Edits {
		if _, ok := seen[edit.NodeID]; ok {
			return nil, fmt.Errorf("duplicate node_id %s", edit.NodeID)
		}
		seen[edit.NodeID] = struct{}{}
		node, ok := projection.Nodes[edit.NodeID]
		if !ok {
			return nil, fmt.Errorf("unknown or non-addressable node_id %s", edit.NodeID)
		}
		resolved = append(resolved, resolvedEdit{span: node.Span, replacement: edit.Replacement, nodeID: edit.NodeID})
	}
	return applyResolved(path, src, resolved)
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
		return nil, fmt.Errorf("patched source does not parse: %w", err)
	}
	return out, nil
}
