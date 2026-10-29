# Batched CSR edge updates — engineering experiment

## Scope

`MathCSRGraph.AddEdges` appends multiple `CSRNamedEdge` relations in one
CSR rebuild. Legacy `AddEdge` is retained and delegates to the same
validated path.

Previously each `AddEdge` call reconstructed every adjacency row, materialized
temporary `[]GraphEdge` slices, and repacked CSR arrays. After multiple
CRT compositions, `RuleComposer.ComposeCRT` would also restore each old
`RelComposition` edge one at a time, incurring repeated full rebuilds.

With a batch of B edges on an existing graph containing V nodes and E edges:

- **Repeated single updates:** approximately O(B(V+E+B)) copied data.
- **One batch:** O(V+E+B) work and transient storage, plus ID lookup costs.

These bounds describe this edge-update phase **only**. The unchanged
`NewMathCSRGraph` builder still scans pairs of modular rules, quadratic in
the number of modular rules even if the final edge set is sparse. The
Composer still constructs the complete rule graph when adding a new rule.

## Guarantees

- Existing edge order is preserved in each adjacency row.
- New edges for a given source are appended in input order.
- Duplicate edges remain allowed, matching repeated `AddEdge`.
- All source/target IDs and CSR offsets are checked *before publication*.
  On an error, the graph retains its original arrays.
- Existing `RelComposition` edges survive subsequent `ComposeCRT` calls.
- Neither the mathematical solving functions nor proof verification changed.
- Concurrent readers and mutators require the same external synchronization
  as before. This is not a lock-free update API.

## Integration validation on Go 1.27

```sh
go test -race -count=1 -v -run '^TestCSRAddEdges|^TestComposeCRT' ./experiments/erdos_straus
go test -race -count=1 ./experiments/real_eval/...
go vet ./experiments/...
go test -run '^$' -bench '^BenchmarkCSR(MutationAddEdges|BuildModularPairs)' -benchmem -count=5 -benchtime=300ms -cpu=1 ./experiments/erdos_straus
```

The first benchmark compares the **same 32 insertions** using 32 calls to
`AddEdge` versus one `AddEdges` call, starting from exactly the same CSR
snapshot on each iteration (10K and 100K existing edges). The graph fixture
is built before timing. Keep raw logs to evaluate variance and allocations.

The second benchmark separates dependency-only graph construction from
graphs with distinct modular residues. All residue classes are distinct,
thus no generalization edge is emitted, but the current implementation
still performs pairwise tests. This diagnoses construction complexity;
it is not a production benchmark or a wall-clock forecast.

## Independent validation available

The batch-update algorithm and atomicity/parity tests passed in an
isolated Go 1.23 module with `go test -race -count=1` and `go vet`.
**Go 1.27 integrated test and benchmark results are pending.**

Do not conflate faster edge insertion with faster construction of all
modular subsumption edges, reduced total process RSS, or enhanced
mathematical reasoning. These are separate performance and science claims.
