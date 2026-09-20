# AgentIR experiments

## E0 — representation viability

Goal: determine whether a deterministic source-to-AgentIR projection can cover real Go code without becoming unreasonably verbose or losing a safe path back to source.

### Corpus

Local Go 1.23.2 source tree (`$GOROOT/src`). The density lab excludes:

- `*_test.go`
- `vendor/`
- `testdata/`
- hidden directories

The measurement is per function. Raw bytes are the AST function span. AgentIR bytes are exactly what `ir.WriteCompactFunction` emits.

### Current result

| Metric | Result |
| --- | ---: |
| Files | 3,480 |
| Functions | 40,390 |
| Projection failures | 0 |
| Raw function bytes | 23,464,339 |
| AgentIR bytes | 27,900,472 |
| Weighted byte ratio | **1.189x** |
| Median function ratio | 1.141x |
| p90 | 1.365x |
| p99 | 1.758x |
| Functions below 1.0x | 5,609 |
| Functions above 2.0x | 169 |

The first fully materialized form was around 1.37x weighted. The current form keeps the outermost expression in its containing statement and materializes nested operations. That dropped the cost to 1.19x.

Worst cases are mostly arithmetic/bit-manipulation-heavy functions. This suggests a future adaptive representation may be useful: explicit linearization has much less value when a function is already a flat arithmetic formula.

### Limitation

Bytes are only a rough proxy for model context cost. The next density test must use the tokenizer of the model under test. No claim should be made from byte ratios alone.

## E1 — round-trip editing

Goal: verify that an agent can edit the projected representation without having to reconstruct the original nested expression.

Example source:

```go
result = append(result, strings.ToLower(strings.TrimSpace(user.Name)))
```

Projection:

```text
n4 $2=strings.TrimSpace(user.Name)
n5 $3=strings.ToLower($2)
n6 result = append(result, $3)
```

IR edit:

```json
{
  "node_id": "n5",
  "text": "strings.ToUpper($2)"
}
```

Projected source:

```go
result = append(result, strings.ToUpper((strings.TrimSpace(user.Name))))
```

The extra parentheses are intentional in v0: temporary expansion prioritizes semantics and unambiguous precedence over source aesthetics. A later source-splice layer can remove redundant parentheses without globally formatting the file.

Current guards:

- exact source hash required;
- only addressable nodes can be edited;
- unknown temporary references fail;
- overlapping node edits fail;
- final Go syntax must parse.

## E2 — agent benchmark (next)

Use the same task corpus and model configuration in three modes:

### A. Raw text

The agent reads normal source and returns ordinary text patches.

### B. Addressed source

The agent reads normal source plus stable AST/node addresses and returns raw Go replacements by node ID. This isolates the benefit of reliable addressing.

### C. AgentIR

The agent reads compact AgentIR and returns `agentir/irpatch-v0` operations. This tests both addressing and the normalized representation.

### Required controls

- same model/version and reasoning setting;
- same task statement;
- same repository snapshot;
- same allowed tool budget;
- no hidden retry only for one mode;
- randomized mode order per task;
- multiple runs/seeds when the model is stochastic.

### Primary metrics

- tests passing after first patch;
- final task success;
- patch rejection rate;
- retries/tool calls;
- unrelated changed bytes/lines;
- input and output tokens;
- wall-clock time.

Secondary metrics: parse/compile failures, patch size, source reads, and search operations.

### Falsification criteria

AgentIR is not useful merely because the projection looks easier to read. It needs a measurable reliability or cost advantage after including projection overhead. If node addressing alone captures the improvement, the simpler addressed-source design should win.
