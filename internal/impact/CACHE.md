# Phase 3.2 — Opt-in in-process Go module impact cache

`impact.NewModuleCache(capacity)` is a **bounded**, **per-process**
cache for repeated, read-only module impact queries. It caches the
Go-typed `ModuleGraph` privately and returns fresh `ModuleImpact`
report slices. The graph/AST and importer are never handed to clients,
and a mutex serializes lookup and construction.

**Existing public behavior remains unchanged**:
`LoadModule(root)`, `LoadModuleWithStats(root)`,
`mbvm-db impact-module`, `impact-diff`, JSON schemas and compiler
witnesses continue to load fresh graphs. There is no automatic global
cache, no on-disk serialization and no cross-run CLI speedup in this
release. To benefit from caching, the same application/agent process
must reuse the same cache object.

## Usage in a persistent Go process

```go
cache := impact.NewModuleCache(2)

impactReport, stats, err := cache.Impact(
    repoRoot,
    "github.com/kbelludoo/mbvm-db/pkg/mbvmdb.New",
)
if err != nil {
    return err
}
// stats.CacheHit = false, stats.GoListCalls = 1 on a cold read.

impactReport, stats, err = cache.Impact(
    repoRoot,
    "github.com/kbelludoo/mbvm-db/pkg/mbvmdb.New",
)
// stats.CacheHit = true, stats.GoListCalls = 0 on an unchanged hit.
// The returned report can be edited without changing the cached graph.

if err := cache.Invalidate(repoRoot); err != nil { return err }
cache.Clear() // free all cached module graphs
```

The `ModuleCacheQueryStats` fields include `cache_hit`,
`cache_bypassed_reason`, `fingerprint_ms`, `impact_ms`,
`total_ms`, `go_list_calls`, `packages`, `symbols`,
and `edges`. The `impact_ms` field measures ONLY the typed
CSR reachability/report construction; `total_ms - fingerprint_ms`
is **not** a reliable direct measurement of traversal cost.
A cache **hit** makes no `go list` subprocess; it still reads and
SHA-256 hashes local source files on every query. A cold read delegates
to the existing single-go-list shared importer from Phase 3.1.

## Invalidation and safety contract

The fingerprint contains:

- Full relative file paths and content hashes of all `.go` sources,
  `go.mod`, `go.sum`, and Go workspace metadata found inside the
  selected module, independent of modification timestamp.
- The active Go runtime/toolchain, OS/architecture and the installed
  `go` executable identity (path, length, modification timestamp).
- Relevant Go compiler environment variables, including build tags,
  GOOS, GOARCH, CGO, toolchain and module search settings. The default
  or explicit `GOENV` config file's **contents** are fingerprinted.
- The root's absolute path (cache entry identity).

Unknown inputs **disable caching**, falling back to the normal fresh
`LoadModuleWithStats`, including:

- Any `replace` in `go.mod`: a local target may change outside the
  selected repository without modifying `go.mod`.
- A detected or explicitly selected Go workspace.
- A `vendor` tree, an external `-modfile` / `-overlay` in GOFLAGS,
  a symlink in the scanned module tree, or a `//go:embed` directive
  (external asset changes are not included in the source-only hash).

The graph is rehashed **after** a cold load, before publication, to
reject source/config changes during construction. This is not a
transactional filesystem snapshot: concurrent external file writes
during fingerprinting may still race and should be coordinated with
the caller. External tampering with module download-cache files or
Go toolchain internals is not exhaustively tracked; call `Invalidate`
or `Clear` after such operations.

Only the bounded immutable-report query API is cached; mutating
operations and hypothetical compiler overlays are **never cached**.
There is no promise of automatic incremental per-package rebuilding:
when the fingerprint changes, the entire module is reloaded.
Maximum entries are bounded by the configured capacity; memory usage
per entry depends on the analyzed module.

## Verification and benchmarks

```powershell
$env:CGO_ENABLED = "0"
go test -count=1 -timeout=300s -v -run '^TestModuleCache' ./internal/impact
go test -race -count=1 -timeout=300s -run '^TestModuleCache' ./internal/impact

go test -run '^$' -bench '^BenchmarkModuleCacheWarmImpact$' `
    -benchtime=20x -count=5 -benchmem ./internal/impact

# For comparison: no persisted graph, even with Go compiler exports warm.
go test -run '^$' -bench '^BenchmarkLoadModuleFixture$' `
    -benchtime=5x -count=3 -benchmem ./internal/impact
```

The warm benchmark measures **query + full fingerprint** in a
dependency-free two-package module. To measure the full MBVM-DB source
tree with its real package and symbol counts:

```powershell
$env:MBVM_BENCH_REAL_MODULE = "1"
go test -run '^
Test outcomes and benchmarks are not presumed successful until run in
Go 1.27 on the target environment.

## Relation to MBVM8K2

This cache does NOT define a persistent graph binary format. Keeping
compiled Go AST/type objects inside a process is distinct from safely
serializing stable symbol identities, edges and CSR. MBVM8K2 remains
a later, separately versioned storage design.
 -bench '^BenchmarkModuleCacheWarmImpactReal
Test outcomes and benchmarks are not presumed successful until run in
Go 1.27 on the target environment.

## Relation to MBVM8K2

This cache does NOT define a persistent graph binary format. Keeping
compiled Go AST/type objects inside a process is distinct from safely
serializing stable symbol identities, edges and CSR. MBVM8K2 remains
a later, separately versioned storage design.
 `
    -benchtime=20x -count=5 -benchmem ./internal/impact
Remove-Item Env:MBVM_BENCH_REAL_MODULE
```

The real benchmark is opt-in because it initializes a full module load.
To isolate the *fingerprint alone*, run:

```powershell
$env:MBVM_BENCH_REAL_MODULE = "1"
go test -run '^
Test outcomes and benchmarks are not presumed successful until run in
Go 1.27 on the target environment.

## Relation to MBVM8K2

This cache does NOT define a persistent graph binary format. Keeping
compiled Go AST/type objects inside a process is distinct from safely
serializing stable symbol identities, edges and CSR. MBVM8K2 remains
a later, separately versioned storage design.
 `
  -bench '^BenchmarkModule(FingerprintOnlyReal|CacheWarmImpactReal)
Test outcomes and benchmarks are not presumed successful until run in
Go 1.27 on the target environment.

## Relation to MBVM8K2

This cache does NOT define a persistent graph binary format. Keeping
compiled Go AST/type objects inside a process is distinct from safely
serializing stable symbol identities, edges and CSR. MBVM8K2 remains
a later, separately versioned storage design.
 `
  -benchtime=20x -count=5 -benchmem ./internal/impact
Remove-Item Env:MBVM_BENCH_REAL_MODULE
```

### Streaming fingerprint optimization (follow-up)

The Windows baseline supplied for the previous implementation was
25.27 ms/op on warm full-module queries, including 19.08 ms reported
by the `fingerprintModule` stage and around 3.96 MB/op allocations.
Those are measurements from the **previous** `io.ReadAll` design.

The new fingerprint streams source bytes directly into SHA-256 using a
reusable 32 KiB buffer. For Go files, a scanner verifies actual
`go:embed` directive lines while a `TeeReader` hashes exact input
bytes, including CRLF. Large unsupported scanner tokens safely
disable caching, preserving the ordinary fresh loader. This should
reduce temporary allocations, but the directory walk and physical
source reads remain on each cache hit.

The 6.19 ms difference between previous total and fingerprint timings
was *residual overhead*, **not** a directly measured CSR traversal.
Use the new `impact_ms` field and the standalone fingerprint
benchmark for exact attribution.

Compare against `BenchmarkLoadModuleReal` on the same Go build-cache
state. A <10 ms warm target is a hypothesis, not a performance
guarantee, until measured on the actual Windows machine.
Test outcomes and benchmarks are not presumed successful until run in
Go 1.27 on the target environment.

## Relation to MBVM8K2

This cache does NOT define a persistent graph binary format. Keeping
compiled Go AST/type objects inside a process is distinct from safely
serializing stable symbol identities, edges and CSR. MBVM8K2 remains
a later, separately versioned storage design.
