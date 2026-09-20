# AgentIR

**AgentIR is an experiment in giving coding agents a deterministic intermediate representation of source code while keeping human-written source as the only source of truth.**

The hypothesis: compact/nested source is often good for humans but creates poor search and patch targets for coding agents. AgentIR projects a selected Go file into an ephemeral, more linear view, gives exact source fragments short node addresses, and can translate edits to that view back into minimal source changes.

Go is the first target because the standard parser/AST/token packages let the experiment focus on the representation rather than parser engineering.

## Example

Human source:

```go
result = append(result, strings.ToLower(strings.TrimSpace(user.Name)))
```

AgentIR:

```text
n4 $2=strings.TrimSpace(user.Name)
n5 $3=strings.ToLower($2)
n6 result = append(result, $3)
```

Control flow stays visible but is deliberately not patch-addressable when a row does not correspond to one exact source fragment:

```text
if !user.Active
 n3 continue
```

Only rows with an `nN` address are direct edit targets.

## CLI

Project Go source:

```bash
go run ./cmd/agentir project ./fixtures/nested.go
```

Machine-readable projection with kinds and exact source spans:

```bash
go run ./cmd/agentir project --format json ./fixtures/nested.go
```

### Edit AgentIR and project it back

An IR patch edits the operation text rather than supplying the original Go span directly:

```json
{
  "version": "agentir/irpatch-v0",
  "source_sha256": "<hash from project output>",
  "edits": [
    {
      "node_id": "n5",
      "text": "strings.ToUpper($2)"
    }
  ]
}
```

Preview:

```bash
go run ./cmd/agentir apply-ir --patch patch.json ./fixtures/nested.go
```

Write explicitly:

```bash
go run ./cmd/agentir apply-ir --patch patch.json --write ./fixtures/nested.go
```

`$N` values are ephemeral AgentIR temporaries. During projection back to Go they expand to the exact source expression represented by that temporary. For example, `strings.ToUpper($2)` can become `strings.ToUpper((strings.TrimSpace(user.Name)))` without the agent needing to reproduce the nested source text.

`apply` also exists as a lower-level baseline where the replacement payload is raw Go source. This lets experiments separate the value of node addressing from the value of editing the IR itself.

## Safety properties in v0

- Human source remains canonical; AgentIR is disposable.
- Every projection is bound to the exact source SHA-256.
- Node IDs are deterministic and local to that source snapshot.
- Structural rows such as `if`, `for`, `switch`, and `select` are non-addressable when they span nested bodies.
- Unknown, duplicate, stale, and overlapping edits fail closed.
- Rewritten output must parse as Go before it is returned.
- Untouched source is not globally reformatted.

## Representation density

Run the local density lab:

```bash
go run ./cmd/agentir-lab /usr/local/go/src
```

On the Go 1.23.2 source tree in the development sandbox, excluding tests, `vendor`, and `testdata`:

- 3,480 files
- 40,390 functions
- 0 projection failures
- 23,464,339 raw function bytes
- 27,900,472 compact AgentIR bytes
- **1.189x weighted byte expansion**
- 1.141x median
- 1.365x p90
- 1.758x p99
- 169 functions above 2x

This is a byte-size sanity check, **not a token benchmark** and not evidence that agents perform better. It already changed the design: folding the outermost expression into its containing statement reduced weighted expansion from roughly 1.37x to 1.19x while retaining explicit nested operations.

## What still has to be proven

The useful benchmark compares identical coding tasks under controlled modes:

1. raw source + textual patching;
2. raw source + AST/node-addressed raw-Go patching;
3. AgentIR + AgentIR-addressed patching.

Measure task/test success, retries, rejected patches, unrelated edits, context/output tokens, tool calls, patch size, and wall-clock time. If AgentIR does not improve reliability or cost after its representation overhead is included, the hypothesis fails.

See [`docs/EXPERIMENTS.md`](docs/EXPERIMENTS.md) for the experiment protocol and current observations.
