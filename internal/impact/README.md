# MBVM-DB impact CLI (experimental feature branch)

The `impact` command is a first-class CLI subcommand backed by
`internal/impact`, written in the Go standard library (plus the Go
compiler itself). It analyzes **one build-selected Go package**, not an
entire module/workspace, and DOES NOT save mutations to source code.

## Run

```sh
go run ./cmd/mbvm-db impact -dir ./pkg/mbvmdb \
  -symbol '(*StringPool).Intern' -json

go run ./cmd/mbvm-db impact -dir ./pkg/mbvmdb \
  -symbol '(*StringPool).Intern' \
  -simulate-add-param int -json
```

The same examples apply to the compiled `mbvm-db` executable.
`-dir` selects a Go package directory, not a `.mbvmdb` data file.
`-symbol` accepts a fully qualified symbol, receiver-qualified method
or an unambiguous bare name. Ambiguous bare names FAIL CLOSED and
list the candidates instead of selecting one.

`-simulate-add-param` currently accepts **only `int`**. It appends
one required `__mbvm_contract_probe int` parameter to a nonvariadic
source function AST, runs a fresh `go/types` check of the **whole
selected package**, collects diagnostics and restores the AST. It
neither edits Go source files nor tests package consumers elsewhere.

The JSON top-level schema is `mbvm-impact-v1`. Properties include
`symbol`, `signature`, `source_sha256`,
`source_files_unmodified`, `direct_callers`,
`transitive_callers`, `transitive_candidates`,
`max_depth`, `local_static_call_sites`, and optional
`simulation` with `broken_direct_callers` and
`total_type_diagnostics`. Errors are printed to stderr and return
a nonzero process exit status. JSON is written only to stdout.

## Meaning of the evidence

- **Direct callers**: within-package, statically resolved Go calls
  whose callees are matched by `types.Func` object identity. Duplicate
  call sites count as one caller in the reachable list.
- **Transitive callers**: reverse-CSR reachability candidates. They
  are not presumed to have compile errors.
- **Broken direct callers**: enclosing functions that received actual
  `go/types` errors after an explicitly requested in-memory signature
  change. Multiple diagnostics may map to one function.
- The analyzed Go source files' contents and names are hashed before
  and after the operation; mismatch is a hard error.
- Function-value calls, interface dispatch, reflection, dynamic
  plugins, consumers in other packages, alternative build configurations
  and cgo are not resolved by this first release. The CLI does not
  promise a complete, zero-false-positive/no-false-negative call graph.
  A check of an artificially changed signature does not prove that
  this change has occurred in a real commit.

## Performance and resource interpretation

The reverse-CSR traversal uses caller-supplied scratch and is
allocation-free in the isolated hot path. Parsing, `go list`,
loading compiler export files, building the graph and running
`go/types` all allocate and are orders of magnitude slower.

Do not infer `<10ms` per simulation or `<10MB` RSS across
different architectures or project sizes from earlier small
benchmarks. Neither `B/op` nor `allocs/op` measures peak
resident memory or proves absence of memory leaks.

## Tests

```sh
CGO_ENABLED=0 go test -count=1 -v ./internal/impact ./cmd/mbvm-db
go test -count=1 -v ./experiments/hdc-csr
```

The new tests cover: method-name collisions, exact method identity,
bare-name ambiguity, interface calls not guessed as concrete methods,
direct/transitive distinction, SHA-256 before/after, repeated
in-memory mutations, a module dependency provided by offline
`replace`, and the actual `pkg/mbvmdb` package.

For a follow-up step, compare before/after checkouts of a real
change and compile all downstream packages; only after establishing
a stable impact result contract should `MBVM8K2` persistence be
considered.

## Compare real versions (Stage 2, initial scope)

The CLI also includes `impact-diff`. It compares **two independently
well-typed versions of the same Go package**, using the typed caller
identity and reverse graph from the old snapshot.

```bash
# Two existing checkouts/directories
go run ./cmd/mbvm-db impact-diff \
  -old ../previous/pkg/mbvmdb -new ./pkg/mbvmdb -json

# Git refs already present in the local repository, NO checkout mutation
go run ./cmd/mbvm-db impact-diff \
  -repo . -old-ref HEAD~1 -new-ref HEAD \
  -package pkg/mbvmdb -json
```

When a function's typed signature changes, it attempts a conservative
compatibility check: old consumers rechecked with the new signature
transplanted to an old-package AST **in memory**. Only those old direct
callers with actual `go/types` diagnostics are reported under
`broken_old_direct_callers`. It is not full cross-package compilation.

See [DIFF.md](DIFF.md) for scope, the failure/unknown boundary, Git
archive extraction limits and security considerations.
