# Phase 3.1 — Shared module exporter and measured load stages

`LoadModule(root)` now invokes `LoadModuleWithStats(root)`. The module
graph and the CLI's default `mbvm-module-impact-v1` output retain the
same schema and typed call-site semantics as before.

## Change

Before this change the module loader called `go list ./...` once, then
launched `go list -deps -export -json .` separately inside the typecheck
of every import-bearing package. On a 24-package repository this could
launch approximately 25 subprocesses.

Now `loadModuleExports` makes a single call:

```bash
go list -mod=readonly -deps -export -json ./...
```

It reads the JSON stream and:

- Collects only explicitly selected packages (`DepOnly == false`)
  into the local graph.
- Resolves the compiler export archive path of all dependencies into
  `ImportPath -> Export` without fabricating types or interfaces.
- Creates one `go/importer.ForCompiler` with one shared
  `token.FileSet`, reusing them **sequentially** across all selected
  local packages. Call site and receiver identities remain type-based.
- Leaves single-package `Load(dir)` on its existing independent path.
- Does **not** reuse file/export caches across separate `LoadModule`
  calls. Source and dependency changes are observed on every load;
  cross-invocation memoization/invalidation belongs to Phase 3.2.

The importer is *not* thread-safe: analyses using the shared importer
must remain sequential. No source files are rewritten.

## Profile one real module load

```powershell
git pull origin feature/hdc-csr-experimental

go run ./cmd/mbvm-db impact-module `
  -root . `
  -symbol 'github.com/kbelludoo/mbvm-db/pkg/mbvmdb.New' `
  -json -profile
```

The optional `load_profile` JSON object includes:

| Field | Interpretation |
|---|---|
| `go_list_calls` | One external `go list` invocation by this loader |
| `export_archives` | Number of resolved compiler archive paths |
| `packages` / `symbols` / `edges` | Actual graph sizes for the current revision |
| `discovery_ms` | Go list subprocess + JSON decode + importer setup |
| `typecheck_ms` | Selection, AST parsing, Go type checking and per-package graph creation |
| `link_ms` | Inter-package identity matching and reverse CSR construction |
| `total_ms` | Complete `LoadModule` wall time |

Without `-profile`, default JSON output remains unchanged.

The old audit observed 24 packages, 308 typed symbols and 615 edges
at a **past revision**. The current repository may have more symbols
and edges after feature additions. Compare the same revision using
both loaders to test parity rather than hardcoding obsolete counts.

## Benchmarks

Run the small offline fixture:

```powershell
$env:CGO_ENABLED = "0"
go test -count=1 -run '^TestSharedModuleImporter' -v ./internal/impact
go test -run '^$' -bench '^BenchmarkLoadModuleFixture$' `
  -benchtime=5x -count=3 -benchmem ./internal/impact
```

For the real MBVM-DB repository:

```powershell
$env:MBVM_BENCH_REAL_MODULE = "1"
go test -run '^$' -bench '^BenchmarkLoadModuleReal$' `
  -benchtime=3x -count=3 -benchmem ./internal/impact
Remove-Item Env:MBVM_BENCH_REAL_MODULE
```

Both benchmarks prime Go's compiler **export cache** before collecting
measurements, but rebuild the complete MBVM module graph on each
iteration. They are measurements of warm-Go-cache *uncached graph
load*, not claims about the first-ever Go toolchain invocation.
`-profile` is best for individual breakdowns. Use clean or
independently sampled command executions to compare warm versus cold
Go build-cache conditions.

## Regression acceptance

- `TestSharedModuleImporterMatchesLegacyGraphs` compares qualified
  symbols, typed call sites, imported call identities, exclusions and
  cross-package CSR reachability against the independent per-package
  loader.
- `TestSharedModuleImporterReloadSeesSourceChange` ensures changing
  a source file is reflected in a subsequent load, since **there is no
  persistent graph cache yet**.
- The existing overlay/Git diff tests must still pass unchanged.
- The `impact-module -profile` CLI test requires exactly one list
  invocation, while ordinary JSON must not emit `load_profile`.
- Windows and Linux benchmark results should include both latency and
  allocated bytes. A 10x speedup is a hypothesis until measured, not a
  promised outcome.

The next Phase 3.2 should introduce an optional immutable snapshot
cache only after defining precise invalidation across Go sources,
`go.mod`/`go.sum`, `GOOS`, `GOARCH`, build tags, toolchain version,
CGO and workspace context.

## Phase 3.2 — Explicit immutable-report graph cache

`NewModuleCache(capacity).Impact(root, symbol)` caches a typed
module graph **only for repeated read-only queries within one process**.
On a warm cache hit it fingerprints current Go sources/configuration
using SHA-256 and does not invoke `go list`; on a miss it delegates to
the shared-importer `LoadModuleWithStats`.

The existing `LoadModule` and one-shot `mbvm-db impact-module`
remain fresh and uncached. A cache hit still scans all selected Go
source files. External workspaces/replaces/vendor/embed source
dependencies intentionally bypass caching.

See [CACHE.md](CACHE.md) for source and environment invalidation,
concurrency, Windows regression tests and small/real module benchmarks.
