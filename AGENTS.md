# AgentIR development rules

- Do not add or use GitHub Actions / hosted CI for this experiment.
- Validate changes in the working sandbox with `gofmt`, `go test ./...`, `go vet ./...`, and local builds.
- Keep human Go source canonical. AgentIR is an ephemeral projection and must be reproducible from source.
- Structural rows that do not map to one exact replaceable source fragment must not receive patch IDs.
- Prefer measurable experiments over representation changes justified only by aesthetics.
