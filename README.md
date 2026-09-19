# AgentIR

**AgentIR is an experiment in giving coding agents a deterministic intermediate representation of source code while preserving human-written source as the source of truth.**

The working hypothesis is simple: compact source is often pleasant for humans but unnecessarily difficult for agents to search, reason about, and patch. AgentIR projects a selected source file into a more explicit operation stream, lets an agent address semantic nodes, and maps guarded edits back to the smallest original source spans.

This repository starts with **Go** because its standard library provides a strong parser/AST/token toolchain and a deliberately small language surface. That lets the first experiments test the representation instead of parser engineering.

## Current experiment: v0

The v0 prototype supports two operations:

```text
human Go source
      |
      v
agentir project
      |
      v
AgentIR projection (ephemeral view)
      |
      v
structured patch {node_id -> replacement}
      |
      v
agentir apply
      |
      v
minimal source replacement
```

The original `.go` file is never replaced by a generated canonical copy. The projection is disposable and can always be regenerated.

### Example

Human source:

```go
users := filterUsers(loadUsers(ctx))
```

AgentIR exposes the nested computation as explicit operations similar to:

```text
$t1 = call loadUsers(ctx)
$t2 = call filterUsers($t1)
users := $t2
```

Every projected node gets a deterministic snapshot-local ID (for example `n7`) in projection order. IDs only need to be stable for that exact source snapshot because the projection also includes the source SHA-256. Patches must target known node IDs and the exact source hash they were generated from, so stale projections fail closed.

## Try it

```bash
go run ./cmd/agentir project ./fixtures/nested.go
```

The default output is a compact, line-oriented agent view. It deliberately omits byte/line spans because the node ID is sufficient for patch targeting. JSON retains the full source map when a machine-readable projection is useful:

```bash
go run ./cmd/agentir project --format json ./fixtures/nested.go
```

Build the CLI:

```bash
go build ./cmd/agentir
./agentir project ./fixtures/nested.go
```

A patch document has this shape:

```json
{
  "version": "agentir/patch-v0",
  "source_sha256": "<sha256 from projection>",
  "edits": [
    {
      "node_id": "n7",
      "replacement": "normalizeName(name)"
    }
  ]
}
```

Preview the mapped edit without modifying the source:

```bash
./agentir apply --patch patch.json ./example.go
```

Write only when explicitly requested:

```bash
./agentir apply --patch patch.json --write ./example.go
```

`apply` rejects stale hashes, missing node IDs, duplicate node IDs, overlapping edits, and edits that make the Go file syntactically invalid. It intentionally does **not** run `gofmt`; preserving untouched human formatting is part of the experiment.

## What v0 is testing

The first benchmark should compare the same coding tasks under three modes:

1. raw source + text patching;
2. raw source + AST-addressed patching;
3. AgentIR projection + node-addressed patching.

Useful measurements:

- task success and test pass rate;
- compile/parse success;
- retries and failed patches;
- accidental/unrelated edits;
- input/output tokens;
- search/read/patch tool calls;
- patch size and wall-clock time.

The important question is not whether expanded code looks cleaner. It is whether an ephemeral agent-oriented representation produces **more reliable edits at acceptable token and latency cost**.

## Non-goals for v0

- replacing Go syntax with a new programming language;
- making AgentIR the repository source of truth;
- preserving a second generated codebase;
- solving whole-repository symbol resolution;
- supporting every Go AST node before measuring anything.

The next step after a useful Go result is likely TypeScript, where chained expressions, callbacks, JSX/TSX, and a larger syntax surface should stress the hypothesis much harder.
