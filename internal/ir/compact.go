package ir

import (
	"fmt"
	"io"
	"strings"
)

// WriteCompact emits the representation intended for an agent to read.
func WriteCompact(w io.Writer, doc Document) error {
	if _, err := fmt.Fprintf(w, "%s %s %s sha256=%s\n", doc.Version, doc.Language, doc.Source.Path, doc.Source.SHA256); err != nil {
		return err
	}
	for _, fn := range doc.Functions {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if err := WriteCompactFunction(w, fn); err != nil {
			return err
		}
	}
	return nil
}

// WriteCompactFunction emits one function without a document header. It is also
// used by the density lab so measurements match the real agent view.
func WriteCompactFunction(w io.Writer, fn Function) error {
	signature := fn.Signature
	if signature == "" {
		signature = fn.Name
	}
	if _, err := fmt.Fprintf(w, "fn %s\n", signature); err != nil {
		return err
	}
	for _, op := range fn.Ops {
		text := op.Text
		if op.Result != "" {
			text = op.Result + "=" + text
		}
		prefix := ""
		if op.ID != "" {
			prefix = op.ID + " "
		}
		if _, err := fmt.Fprintf(w, "%s%s%s\n", strings.Repeat(" ", op.Depth+1), prefix, text); err != nil {
			return err
		}
	}
	return nil
}
