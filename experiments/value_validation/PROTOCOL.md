# Value validation v1 — frozen before measurement

Question: does the existing MBVM-DB typed impact API provide correct, compact
answers and useful repeated-query savings on real Go projects?

This evaluates engineering utility. It does not reinterpret the negative
closed-loop neural result as a neural success or establish market superiority.

## Fixed cases and workload

First evaluation, in this order: the current MBVM-DB checkout, local
`twitch-miner-go`, and local `kick-miner-go`. Preserve errors and cache bypasses;
do not silently replace cases that fail to load. Analyze sources read-only;
never run these applications, download model data, or call paid services.

For each loaded module:

1. Load all build-selected packages through `impact.LoadModuleWithStats`.
   Record the first load, package/function/edge counts and source hashes.
2. Build a conventional reverse adjacency index, `map[uint32][]uint32`, with
   an O(1) symbol lookup. Its BFS and output are independently implemented,
   using exactly the same typed edge input. Compare every symbol against the
   existing CSR impact API, including empty neighborhoods and cycles.
3. Pick up to six symbols with callers and six without callers, ordered by
   SHA-256 of `20261026:<identity>`. Do not select by timing or code name.
4. Run seven paired trials: full fresh module load plus query versus the existing
   `ModuleCache.Impact`, alternating execution order. Compiler exports are warm
   for both; the graph cache initialization is separately reported. A warm
   graph-cache query still includes full source/config fingerprinting.
5. Measure 100,000 pre-indexed queries for both CSR API and conventional adjacency
   reference, alternating order across five batches. Include identical output
   construction, fresh BFS scratch arrays and sorted report names. The reference
   keeps an O(1) target lookup; the current CSR API's lookup is measured as-is.
6. Compare report JSON bytes with a complete typed symbol/edge JSON inventory.
   This is context selection for the precise impact question, not lossless
   source compression, exact tokenizer measurement, or general task sufficiency.
7. For up to three package-local, nonvariadic called functions, selected by
   the same fixed hash order, run the existing in-memory added-int-parameter
   simulation. Require the selected package to typecheck originally and actual
   Go/types errors in its statically identified direct callers; preserve
   mismatches/unsupported cases. Check source hashes before and after.

## Interpretation

- Algorithm parity proves equivalence on the loaded typed graph, not complete
  modeling of runtime dispatch, reflection or every Go build variant.
- The compiler witnesses provide additional finite-case evidence, not a formal
  proof of the Go type checker or of all possible future mutations.
- Fresh versus graph-cache timings isolate the benefit of retaining an index.
  Conventional tools can also cache indexes: do not call this unique to MBVM-DB.
- CSR versus pre-indexed adjacency is the competent layout/query comparison.
  Keep slower, tied, zero-gain and cache-bypassed results in the report.
- Report median/p95 timing, parent-process allocations, context bytes and initial
  costs separately. Allocation bytes are not peak RAM or subprocess memory.
- No minimum speedup or positive outcome is required for program success.
- These are public/local development projects and one machine. General claims
  need independent datasets, machines and competing product implementations.

## Measurement amendment v2

The first report in `output/value-validation-2026-10-26/` is preserved. Its
1,000-query micro batches returned zero-duration samples on small projects under
the host clock, so those micro timing ratios are unresolved and must not be used
as wins. Increase BOTH arms to the same fixed 100,000 queries per batch and mark
timing validity explicitly. This changes measurement resolution, not the
algorithms, selected datasets or outcome criterion. Preserve the original report
and rerun after adding the stdio client for the existing cache; source graphs
and their hashes are recorded anew.

## Additional real-build check

After timing, additionally exercise the existing compile-overlay command on
the three public constructors `mbvmdb.New`, `twitch-miner-go/pkg/miner.NewManager`
and `kick-miner-go/pkg/miner.NewManager`. Keep all three outcomes. Compile the
original provider/consumers, compile the self-consistent virtual provider and
attribute errors only at known consumer call sites. This is separate from the
nine package-local Go/types checks and does not change timing samples.
