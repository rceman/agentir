package ir

// Version identifies the current experimental projection format.
const Version = "agentir/v0"

// PatchVersion identifies the current experimental patch format.
const PatchVersion = "agentir/patch-v0"

// SourceRef identifies the source file projected into AgentIR.
type SourceRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Span is a half-open byte range [StartOffset, EndOffset) plus human-readable positions.
type Span struct {
	StartOffset int `json:"start_offset"`
	EndOffset   int `json:"end_offset"`
	StartLine   int `json:"start_line"`
	StartColumn int `json:"start_column"`
	EndLine     int `json:"end_line"`
	EndColumn   int `json:"end_column"`
}

// Op is one explicit operation in the agent-facing projection.
type Op struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Result string `json:"result,omitempty"`
	Text   string `json:"text"`
	Depth  int    `json:"depth,omitempty"`
	Source Span   `json:"source"`
}

// Function contains the projected operations for one Go function or method.
type Function struct {
	Name   string `json:"name"`
	Source Span   `json:"source"`
	Ops    []Op   `json:"ops"`
}

// Document is the complete AgentIR projection for one source file.
type Document struct {
	Version   string     `json:"version"`
	Language  string     `json:"language"`
	Source    SourceRef  `json:"source"`
	Functions []Function `json:"functions"`
}

// Patch is a guarded set of source replacements addressed by AgentIR node IDs.
type Patch struct {
	Version      string `json:"version"`
	SourceSHA256 string `json:"source_sha256"`
	Edits        []Edit `json:"edits"`
}

// Edit replaces the exact source span corresponding to NodeID.
type Edit struct {
	NodeID      string `json:"node_id"`
	Replacement string `json:"replacement"`
}
