# Incremental HDC 2-Hop — exactness and reverse CSR cache

This change addresses the terminal-leaf bug in the old
`RecomputeNodeHop2`: when node A has outgoing edges but no valid second-hop
path, the canonical `PropagateHop2` falls back to its **current 1-Hop
context**. The incremental version now reproduces that fallback instead of
returning an all-zero vector.

## Supported mutation contract

- `BuildReverseCSR(graph)` creates a reverse CSR in **O(V+E)** time and
  **O(V+E)** space. Build it **once** for a static topology.
- `UpdateNodeIncremental2HopWithReverse(graph, reverse, mem, symbolID, newVec)`
  updates a vector only. It enumerates incoming predecessors within two hops
  via the cached reverse graph and recalculates affected contexts once each.
- The old `UpdateNodeIncremental2Hop(graph, mem, symbolID, newVec)` API
  remains available but rebuilds the reverse index **on every call**. Its
  complexity is **not** sublinear in E; do not use it for low-latency edits.
- Updates are in-place, and therefore require **exclusive access**; the API
  currently has no synchronization with concurrent queries.
- When adding/removing/retyping edges, **rebuild the reverse index and the
  contextual memory**. An incremental edge-mutation API is not implemented.
- Graph edge IDs, offsets, relation types and node IDs must be valid. Input
  validation is not part of the incremental hot path.

## Verification and benchmarks

`TestIncrementalTerminalLeafFallback` covers A->B with B terminal.

`TestIncrementalMultiGraphFuzz` runs a **deterministic property sweep**
(20 seeds, 5 consecutive vector mutations per seed) with random mixed-role
graphs, terminal nodes, high-degree nodes and an explicit bidirectional
cycle. Every mutation compares `Original`, `Contextual` and
`HasContext` for **every node** against a fresh full reconstruction.

It is not Go's built-in randomized `Fuzz*` testing.

To reproduce:

```bash
go test -count=1 -v -run 'TestIncremental' ./experiments/hdc-csr
go test -count=1 -run '^$' -bench 'Benchmark(PropagateHop2|IncrementalUpdate)_1000Nodes_Paired$' -benchmem -cpu=1 ./experiments/hdc-csr
```

The paired benchmarks use **the same synthetic 1,000-node fixture** and
**identical cyclic sequences of new original vectors**. The full rebuild
recomputes Hop1 and Hop2; the incremental benchmark uses a previously built
context and cached reverse CSR. They measure different legitimate operational
paths under equal graph topology. Preprocessing (initial Hop1/Hop2 and reverse
CSR) is excluded from the timed incremental region and must be accounted for
in amortization separately.

The CI parser stores the **paired** times under
`paired_full_rebuild_1000nodes_ms` and
`paired_incremental_cached_reverse_1000nodes_ms` with
`paired_incremental_speedup_x`. They do **not** overwrite legacy,
non-paired 2-Hop metrics.

Do not claim an improvement factor until a fresh GitHub Actions run passes
the property tests and publishes the paired measurements.
