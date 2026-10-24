# Typed Go call identities and in-memory compile-breakage lab

The prior `BenchmarkRealWorldASTImpact` was grounded in `pkg/mbvmdb`,
but the existing ingestion's `RelCall` relations resolve names
heuristically. The new experiment uses **real Go object identity**
from `go/types` and independently checks the *same package* after a
controlled function signature change in memory.

## Implementation

- `typed_real_impact.go` loads the build-selected non-test Go source
  files in `pkg/mbvmdb` (with `go/build`), parses them with
  `go/parser`, then type-checks the complete package using
  `types.Config.Check` and `types.Info.Defs/Uses/Selections`.
- Each declared function/method gets a stable index for that load,
  linked internally to the actual `*types.Func` object. Its name,
  declaring package path, and receiver type are exported for reports.
  Two different methods named `Apply` belonging to types `A` and `B`
  **never share a node**.
- Each statically resolved call whose target object belongs to a
  function declared in the same package creates a directed edge.
  Package-qualified external calls, interface dispatch, function
  values, reflection and unknown indirect calls are deliberately
  **not** guessed to be specific local implementations.
- The existing `RealReverseCallsCSR` and
  `NewRealImpactScratch` enable allocation-free cached reverse
  traversal of the typed graph, with cycles and duplicate call sites
  suppressed at visitation.
- `VerifyTypedSignatureBreakage` adds one required `int` parameter
  to a real nonvariadic function's parsed AST. It reruns the package
  type-checker while collecting `types.Error` diagnostics, associates
  each error with its enclosing function declaration, and restores
  the original `ast.FuncDecl` using `defer`.
  It **never writes** modified Go source to disk.
- Source provenance and build-selection environment matter.
  The current implementation handles one non-cgo Go package;
  `go/importer.Default` must be able to resolve dependencies.
  It is not a comprehensive module-wide Go build, `go/packages`
  workspace analysis, interface call graph or cross-package regression
  runner.

## Run tests and benchmarks

```bash
CGO_ENABLED=0 go test -count=1 -v \
  -run '^TestTypedReal' ./experiments/hdc-csr

CGO_ENABLED=0 GOMAXPROCS=1 go test -run '^$' \
  -bench '^BenchmarkTypedRealWorldImpact$' \
  -benchmem -cpu=1 -benchtime=2s ./experiments/hdc-csr
```

The fixture test validates:

1. `A.Apply` and `B.Apply`, distinct concrete methods with the
   same spelling, are mapped to different `types.Func` identities.
2. Mutating `A.Apply` causes a concrete `go/types` error at the
   statically resolved `CallA` caller, but not at unrelated `CallB`.
3. An interface-typed `Caller.Apply` invocation is **not**
   misattributed to either concrete implementation.
4. A second type-check after mutation still sees the original
   source (mutation restoration).

The real-source test type-checks `pkg/mbvmdb` and writes
`typed_real_impact_output.json`. The official CI benchmark parser
publishes it under `typed_real_breakage_evaluation`, including the
chosen function, number of typed local call sites, compile errors
observed after mutation, and direct callers whose enclosing functions
produced diagnostics. It explicitly records
`runtime_behavior_verified=false` and makes no cross-package claims.

## Performance boundary

The benchmark has three independent stages:

- `IngestTypecheckAndBuild`: file reads, AST parsing, baseline type
  check, typed graph construction and reverse CSR creation.
- `MutationAndFullPackageTypecheck`: in-memory change, complete
  re-typecheck, error collection, AST restoration (with prebuilt graph).
- `CachedTypedReverseCSR`: reverse graph traversal alone, with a
  preallocated queue/visited buffer, excluding type-check and ingest.

Results are stored under `metrics.typed_real_*_{ns,bytes_per_op,allocs_per_op}`.
The benchmark must not mislabel the nanosecond-scale *cached query*
as the total time to determine real compile breakage.

Go benchmark `B/op` is cumulative allocation per iteration, not peak
resident memory. Zero `allocs/op` does not prove absence of a memory
leak over time.

## Evidence ladder

1. Typed call graph identifies likely **direct callers within the
   checked package** by object identity.
2. An actual `go/types` diagnostic in a direct caller after a
   signature mutation is an **observed compile-time type breakage**
   under the selected build configuration.
3. Transitively reachable callers represent a possible **blast
   radius**. They are not automatically compile failures.
4. Runtime failures, all build-tag variants, external module
   consumers, reflection and interface dispatch need additional
   evidence, tests, and/or multi-package analysis.

The earlier name-resolved graph (e.g. 68 symbols, 83 call edges)
and the new typed graph do **not** necessarily have identical node
universes or edge counts, so comparing their raw counts directly
without matching symbols and build variants would be misleading.

No neural embeddings are required to establish compile breakage from
a changed Go function signature. HDC remains an optional ranking tool
for queries the compiler cannot resolve deterministically.
