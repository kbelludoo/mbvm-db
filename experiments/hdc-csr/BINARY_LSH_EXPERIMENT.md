# Binary LSH over 8,192-bit HDC vectors — experimental index

Goal: measure whether binary locality-sensitive hashing can reduce the **global**
candidate-ranking cost for `HDC_Canonical_VSA_Unbinding`. This is independent
of Kaggle training. It does **not** replace the exact CSR traversal or the
original HDC full-scan implementation.

## Search semantics

The original `HDCVSAUnbindEngine` assigns:

```
score(id) = max(
   1 - hamming(source_semantic, orig[id])/8192,
   1 - hamming(unbind(hop2[source], call->call), orig[id])/8192
)
```

The indexed variant `HDCVSALSHIndexEngine` uses the same score for returned
candidates, but collects candidates from *both* an LSH lookup of
`source_semantic` and an LSH lookup of the unbound 2-Hop estimate. Indexing
only one of these would lose candidates that rank highly on the other term.
When the source has no context, only the semantic lookup is used.

The LSH index stores random bit-position signatures over **original** symbol
vectors. Each independent hash table has `2^BitsPerTable` buckets and a
deterministic signature sampled without repeating bit positions across
all tables. Radius 0 probes one bucket; radius 1 additionally probes every
single-bit variant, and radius 2 probes all two-bit variants. Candidate IDs are
deduplicated, rescored exactly, and ranked with a stable node-ID tie break.

Default evaluation parameters: 12 tables, 10 bits per table, signature probe
radius 1, fixed seed 2026. **This is approximate search**, not a guarantee
of sublinear worst-case complexity or accuracy. In degenerate data, postings
may contain nearly all N vectors.

## Metrics and comparison

```bash
go test -count=1 -v -run '^TestBinaryLSH' ./experiments/hdc-csr
go test -run '^$' -bench '^BenchmarkVSA_(FullScan|LSH)_1000nodes_Matched$' -benchmem -cpu=1 ./experiments/hdc-csr
```

`TestBinaryLSHQualityE2E` writes `lsh_quality.json`, which the official CI
parser merges into `binary_lsh_evaluation`. The experiment reports:

- **Structural Recall@10** of full-scan VSA and indexed VSA against identical
  exact `RelCall -> RelCall` destination ground truth;
- **Agreement@10 with the full-scan oracle** (overlap of top-10 sets);
- Unique candidates examined, peak candidates and postings in all tables;
- Paired `ns/op`, `B/op` and `allocs/op` for 100 identical queries.

Index construction and memory for buckets are currently **excluded** from
query benchmarks. Any production claim must also include build time, retained
RSS/memory and mutation/reindex cost. For updates to original vectors, buckets
are stale until rebuilt; concurrent mutation is unsupported.

Crucial: the synthetic fixture contains class-prototype hypervectors with
artificial noise. Its results are **not evidence of semantic generalization**.
A successful LSH smoke test proves only candidate generation/reranking and
quantifies a specific latency/recall trade-off.

### Acceptance criterion

Indexing is useful only if (a) it reduces query time with a reproducible
comparable workload; (b) Recall@10 remains acceptable for the intended
workload; and (c) index memory plus mutation costs fit the target machine.

At 1,000 sparse nodes the CSR may remain both faster and more accurate. This
is an experiment to measure, **not** a commitment that LSH will win.
