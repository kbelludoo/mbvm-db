# Real-world Go AST impact lab — MBVM-DB

This lab **reads real repository Go source** under `pkg/mbvmdb`, ingests
symbol texts and the existing MBVM-DB call graph via `ExportRealAST`,
builds a reverse `RelCall` CSR and evaluates the set of potential caller
symbols affected by a controlled, structural API contract change.

It is independent of Kaggle, Python, LLM inference, MiniLM and the
synthetic `knowledge_8k_supreme.bin` index.

## Run

From the root of the module:

```sh
CGO_ENABLED=0 go test -count=1 -v \
  -run '^TestReal(WorldASTImpactOnOwnPackage|ImpactReverseCSRCyclesAndDuplicateEdges|ContractMutationControls|ImpactCachedQueryAllocation)$' \
  ./experiments/hdc-csr

CGO_ENABLED=0 GOMAXPROCS=1 go test -run '^$' \
  -bench '^BenchmarkRealWorldASTImpact$' \
  -benchmem -cpu=1 -benchtime=1s ./experiments/hdc-csr
```

Note: `CGO_ENABLED=0` is convenient for a Go-only deployment but not
evidence that all dependencies or system calls are absent.

## Source and methodology

- **Real input:** `../../pkg/mbvmdb` when the package test is executed.
  `ExportRealAST` calls the project's own `mbvmdb.IngestGoAST`,
  extracts declarations and creates the same name-resolved outgoing CSR
  `RelCall` edges. The corpus is neither generated toy functions nor
  training templates.
- **Target selection:** select an actual, parseable function declaration
  with the largest number of resolved immediate incoming calls.
- **Benign refactor:** `go/format.Source` on the actual function
  declaration, checking the normalized function signature is unchanged.
  This is **not** a certified arbitrary alpha-renaming and does not
  establish behavioral identity for all possible refactorings.
- **Controlled contract change:** append the required
  `__mbvm_contract_probe int` argument to that actual function's signature.
  Verify the normalized signature changed. Reject variadic functions or
  reserved-name collisions. The actual checked-in files are NOT altered.
- **Reverse dependency query:** build incoming `RelCall` arrays and run
  cycle-safe, iterative BFS from the mutated symbol, reporting direct,
  indirect, total potential callers, and maximum observed call depth.
  All callers are *candidates for impact*, not proven runtime failures.
- **Behavioral witness:** NOT attempted on arbitrary `pkg/mbvmdb`
  functions. The independent five-fixture `TestASTBaseline...` suite
  supplies bounded input/output witnesses for its supported int subset.
  A real-package compile/test/integration harness is needed to prove a
  particular API change breaks a concrete caller.

## Static-call graph limitations

The existing exporter is **name-based**, not fully resolved through
`go/types`. Interfaces, function values, reflection, build tags,
cross-package calls, and same-name functions may produce missing or
incorrect edges. This is a **heuristic blast-radius candidate set**.
Do not interpret its reachability as a complete verified call graph or
a proof that every reached caller would fail.

For a production-grade graph, augment edges with `go/types` object
identity, imports and package paths, test variants, and independently
reviewed dynamic/integration tests before claiming complete impact.

## Benchmarks and allocation boundaries

`BenchmarkRealWorldASTImpact` has four distinct sub-benchmarks:

| Sub-benchmark | Measured work |
|---|---|
| `FullIngestMutationReverseCSR` | Reparse / ingest REAL source, build reverse CSR, select and mutate actual declaration, traverse impacts |
| `BuildReverseCSR` | Construct reverse graph from pre-ingested real symbol manifest |
| `RealSignatureMutation` | Parse and format the selected actual declaration; append required argument |
| `CachedReverseCSRQuery` | Traverse a prebuilt real reverse CSR using reusable visitation stamps and queue |

The cached traversal is designed for zero per-operation heap allocations,
but total resident memory includes ASTs, manifest, graph, buffers, Go
runtime and any other database structures. Do not claim a universal
`<10 MB` total system envelope from the 1,024-byte/vector index or a
single cached-query benchmark. Go allocation statistics do not certify
absence of memory leaks over long durations.

The real-package test writes `experiments/hdc-csr/real_world_impact_output.json`
with source digest, count of actual Go symbols, count of **name-resolved**
call edges, selected target symbol, immediate/indirect/total possible
callers, and explicit `functional_counterexample_confirmed=false`.
The CI parser requires this file and publishes it under
`real_ast_impact_evaluation` in `gh-pages/official_benchmarks.json`.
No private source text is copied to the published report.

The official `gh-pages/official_benchmarks.json` parser publishes:

- `metrics.real_ast_full_ingest_mutation_reverse_csr_ns`
- `metrics.real_ast_build_reverse_csr_ns`
- `metrics.real_ast_signature_mutation_ns`
- `metrics.real_ast_cached_reverse_csr_query_ns`

For each, `_bytes_per_op` and `_allocs_per_op` are also recorded where
present in Go benchmark output.

The tests run on the actual branch checkout and can be reproduced on
the low-memory VPS with `CGO_ENABLED=0`; compare results using
the same checkout, Go version, `GOMAXPROCS` and CPU. Measure production
resident memory with an OS-level RSS tool and separately monitor GC
behavior in a long-running process.

## Decision rule

A repository change that the AST baseline **proves equivalent** may
avoid warnings within its explicitly verified subset.

A signature change is an **API contract mutation**. Traverse actual
resolved callers as *candidates* for inspection. Validate compile/test
failure before reporting an actual breakage.

A case the static analyzer cannot prove either way is **unknown**,
not automatically a functional bug. HDC or neural retrieval can be
introduced as a *ranking heuristic* only after demonstrating incremental
benefit over deterministic CSR and ordinary lexical/static controls.
