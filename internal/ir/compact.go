package ir

import (
	"fmt"
	"io"
	"strings"
)

// WriteCompact emits the agent-facing textual projection. JSON remains available
// for machine consumers, but this format intentionally minimizes representation overhead.
func WriteCompact(w io.Writer, doc Document) error {
	if _, err := fmt.Fprintf(w, "%s %s %s sha256=%s\n", doc.Version, doc.Language, doc.Source.Path, doc.Source.SHA256); err != nil {
		return err
	}
	for _, fn := range doc.Functions {
		if _, err := fmt.Fprintf(w, "\nfn %s\n", fn.Name); err != nil {
			return err
		}
		for _, op := range fn.Ops {
			text := op.Text
			if op.Result != "" {
				text = op.Result + " = " + text
			}
			if _, err := fmt.Fprintf(
				w,
				"%s%s %-10s %s\n",
				strings.Repeat("  ", op.Depth+1),
				op.ID,
				op.Kind,
				text,
			); err != nil {
				return err
			}
		}
	}
	return nil
}
