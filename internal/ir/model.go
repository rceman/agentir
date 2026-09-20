package ir

const Version = "agentir/v0"
const PatchVersion = "agentir/patch-v0"
const IRPatchVersion = "agentir/irpatch-v0"

type SourceRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Span struct {
	StartOffset int `json:"start_offset"`
	EndOffset   int `json:"end_offset"`
	StartLine   int `json:"start_line"`
	StartColumn int `json:"start_column"`
	EndLine     int `json:"end_line"`
	EndColumn   int `json:"end_column"`
}

// Op is one operation in the agent-facing projection. ID is empty for structural
// rows that are useful for reasoning but intentionally cannot be patched directly.
type Op struct {
	ID     string `json:"id,omitempty"`
	Kind   string `json:"kind"`
	Result string `json:"result,omitempty"`
	Text   string `json:"text"`
	Depth  int    `json:"depth,omitempty"`
	Source Span   `json:"source"`
}

type Function struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
	Source    Span   `json:"source"`
	Ops       []Op   `json:"ops"`
}

type Document struct {
	Version   string     `json:"version"`
	Language  string     `json:"language"`
	Source    SourceRef  `json:"source"`
	Functions []Function `json:"functions"`
}

// Patch is the raw-source baseline. Replacement contains Go source.
type Patch struct {
	Version      string `json:"version"`
	SourceSHA256 string `json:"source_sha256"`
	Edits        []Edit `json:"edits"`
}

type Edit struct {
	NodeID      string `json:"node_id"`
	Replacement string `json:"replacement"`
}

// IRPatch edits the AgentIR text of an addressable operation. Temporary values
// such as $3 are expanded back to the exact source expression they represent.
type IRPatch struct {
	Version      string   `json:"version"`
	SourceSHA256 string   `json:"source_sha256"`
	Edits        []IREdit `json:"edits"`
}

type IREdit struct {
	NodeID string `json:"node_id"`
	Text   string `json:"text"`
}
