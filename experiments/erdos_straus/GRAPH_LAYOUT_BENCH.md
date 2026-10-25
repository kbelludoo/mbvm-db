# CSR versus adjacency lists — reproducible engineering benchmark

**Status:** public synthetic performance experiment, not a mathematical
reasoning benchmark or evidence of a unique CSR intelligence advantage.

The benchmark uses production types `MathCSRGraph`, `GraphEdge`, `RelationType`
and the production `MathCSRGraph.Neighbors()` implementation. It does **not**
run the expensive `NewMathCSRGraph` rule-index constructor; both edge layouts
are populated from exactly the same synthetic directed edges before timing.

## Three representations

- **AdjacencyList:** `[][]GraphEdge` and direct iteration.
- **CSRRawArrays:** direct iteration over `RowOffsets`, `ColIndices`,
  `RelTypes`, without an intermediate edge slice.
- **CSRPublicNeighbors:** calls the actual production `Neighbors(node)` API,
  which allocates a `[]GraphEdge` for each nonempty row.

`CSRRawArrays` is a **measurement kernel**, not a public optimizer API.
The benchmark measures both single-row scanning and two-hop breadth-first
traversal with identical visited-set and queue reuse logic.

## Scales and reproduction

| Nodes | Out-degree | Directed edges |
|---:|---:|---:|
| 1,000 | 10 | 10,000 |
| 10,000 | 10 | 100,000 |
| 50,000 | 20 | 1,000,000 |

Targets and relation types are deterministic; both representations see the
same edges and query roots. Query roots cycle over 1,024 generated positions,
which may favor the hardware cache. This is NOT production traffic.

Run on the actual project toolchain (Go 1.27):

```sh
go test -race -count=1 -run '^TestGraphLayout' ./experiments/erdos_straus
go vet ./experiments/...
go test -run '^$' -bench '^BenchmarkGraphLayout' -benchmem -count=5 -benchtime=300ms -cpu=1 ./experiments/erdos_straus
```

Save the **full** raw benchmark output; report operating system, CPU, Go
version, benchmark flags and process load. Compare repeat medians with
uncertainty/benchstat, not a single run.

## Reading the results

- `ns/op`: scan or two-hop traversal, with graph build excluded.
- `B/op`, `allocs/op`: dynamic allocations per **measured traversal**.
- `csr-struct-B` and `list-struct-B`: payload capacities computed using
  `cap` and `unsafe.Sizeof`. Includes CSR offset/column/relation arrays
  and adjacency row headers/edge storage.
  These are **not** live heap or resident memory measurements and exclude
  common node/ID indexes, allocator rounding and object headers.

Production `NewMathCSRGraph` tests pairs of modular rules for generalization
(quadratic worst-case in the number of such rules), and `AddEdge` rebuilds
the CSR arrays. Those **construction and mutation costs are not measured**
by these traversal benchmarks; test them separately before making
production architecture claims.

## Preliminary isolated result (not a repository benchmark)

A Go 1.23 stand-in with equivalent data layout on Linux found:

| Scale | Adjacency scan | Raw CSR scan | Public CSR scan |
|---|---:|---:|---:|
| 10,000 edges | `11.9 ns/op | `15.0 ns/op | `74.5 ns/op |
| 100,000 edges | `18.6 ns/op | `18.5 ns/op | `75.5 ns/op |

The public accessor caused one `160 B` allocation per 10-edge query in that
isolated run. **These are exploratory figures; they are not verified Go 1.27
production benchmark measurements.** Repeated, fair testing may show
different results.

This benchmark makes no claim that CSR has superior mathematical reasoning
or better semantic pruning than an index with equivalent dependency data.
