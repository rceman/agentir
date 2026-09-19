package patch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"sort"

	"github.com/rceman/agentir/internal/ir"
	"github.com/rceman/agentir/internal/project"
	"github.com/rceman/agentir/internal/source"
)

// Parse decodes and validates the basic patch envelope.
func Parse(data []byte) (ir.Patch, error) {
	var p ir.Patch
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return ir.Patch{}, fmt.Errorf("decode patch: %w", err)
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

// Apply maps node IDs back to exact source spans and performs minimal replacements.
func Apply(path string, src []byte, p ir.Patch) ([]byte, error) {
	actualHash := source.SHA256(src)
	if actualHash != p.SourceSHA256 {
		return nil, fmt.Errorf("source hash mismatch: patch=%s source=%s", p.SourceSHA256, actualHash)
	}

	projection, err := project.Project(path, src)
	if err != nil {
		return nil, err
	}

	type resolvedEdit struct {
		span        ir.Span
		replacement string
		nodeID      string
	}

	seen := make(map[string]struct{}, len(p.Edits))
	resolved := make([]resolvedEdit, 0, len(p.Edits))
	for _, edit := range p.Edits {
		if _, ok := seen[edit.NodeID]; ok {
			return nil, fmt.Errorf("duplicate node_id %s", edit.NodeID)
		}
		seen[edit.NodeID] = struct{}{}

		span, ok := projection.Nodes[edit.NodeID]
		if !ok {
			return nil, fmt.Errorf("unknown node_id %s", edit.NodeID)
		}
		if span.StartOffset < 0 || span.EndOffset < span.StartOffset || span.EndOffset > len(src) {
			return nil, fmt.Errorf("invalid source span for %s", edit.NodeID)
		}
		resolved = append(resolved, resolvedEdit{span: span, replacement: edit.Replacement, nodeID: edit.NodeID})
	}

	sort.Slice(resolved, func(i, j int) bool {
		if resolved[i].span.StartOffset == resolved[j].span.StartOffset {
			return resolved[i].span.EndOffset > resolved[j].span.EndOffset
		}
		return resolved[i].span.StartOffset < resolved[j].span.StartOffset
	})

	for i := 1; i < len(resolved); i++ {
		if resolved[i].span.StartOffset < resolved[i-1].span.EndOffset {
			return nil, fmt.Errorf("overlapping edits: %s and %s", resolved[i-1].nodeID, resolved[i].nodeID)
		}
	}

	out := append([]byte(nil), src...)
	for i := len(resolved) - 1; i >= 0; i-- {
		e := resolved[i]
		next := make([]byte, 0, len(out)-(e.span.EndOffset-e.span.StartOffset)+len(e.replacement))
		next = append(next, out[:e.span.StartOffset]...)
		next = append(next, e.replacement...)
		next = append(next, out[e.span.EndOffset:]...)
		out = next
	}

	// A patch is only accepted if it still parses as Go. Formatting is intentionally not
	// automatic: preserving untouched human source is part of the experiment.
	if _, err := parser.ParseFile(token.NewFileSet(), path, out, parser.AllErrors); err != nil {
		return nil, fmt.Errorf("patched source does not parse: %w", err)
	}
	return out, nil
}
