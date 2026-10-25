# Indexed modular subsumption — build benchmark

## Why

The PR #10 Go 1.27 benchmark on Windows/Xeon reported 1.27 ms for 256
modular rules, 20.3 ms for 1,024, and 87.7 ms for 2,048.
The old builder checked **every rule pair** even for disjoint singleton
residues with the same modulus.

## Algorithm

- Index by `modulus -> residue -> rule indices`.
- For each source rule, examine only target moduli dividing its modulus.
- Use the first source residue to find candidate posting lists.
- Check **all source residues** against each candidate using the exact
  previous `slices.Contains` condition, not a new approximate criterion.
- Sort target indices to retain exact original CSR ordering.
- Preserve duplicate/negative/out-of-range and empty-residue behavior.

Cost depends on N rules, D distinct positive moduli and posting list
candidate hits: roughly O(N*D + totalResidues + candidateChecks +
acceptedEdges*log(acceptedEdges)). This is **not always O(N)**:
D ~= N or quadratic output can still cause quadratic work.
The extra temporary posting index also allocates heap memory.

## Independent correctness tests

`legacyCSRPairwiseReference` reproduces the original nested loop.
Tests compare all CSR arrays and rule IDs under:
- Mixed modular factors, repeated/negative and empty residues.
- Duplicate/unknown dependency IDs.
- 2,048 distinct residues, same modulus.
- 64 identical residues, same modulus (dense actual edge set).
- 200 deterministic randomized catalogs.

## Go 1.27 integrated commands

~~~sh
go test -race -count=1 -run '^TestCSRModularIndex|^TestComposeCRT|^TestGraphLayout' ./experiments/erdos_straus
go vet ./experiments/...
go test -run '^$' -bench '^BenchmarkCSR(ModularIndexVsPairwise|BuildModularPairs)' -benchmem -count=5 -benchtime=300ms -cpu=1 ./experiments/erdos_straus
~~~

Benchmark the old oracle and new index on the same inputs in the same
binary. Compare sparse (same modulus, distinct residues), dense (same
residue), and many-distinct-modulus adversarial cases. Keep all raw
repetitions including B/op and allocs/op.

## Isolated exploratory run (not a Go 1.27 repo benchmark)

A standalone equivalent Go 1.23 module passed 1,000 randomized
comparisons, go test -race and go vet. With 2,048 distinct singleton
residues of the same modulus:
- Indexed: 311,547 / 311,584 ns/op; approximately 390 KB allocated/op.
- Legacy pairwise: 16,713,966 / 19,079,101 ns/op;
  approximately 49 KB allocated/op.

This synthetic improvement comes with **extra temporary index memory**.
It must not be extrapolated to all rule catalogs or to the Windows
Go 1.27 build. It is not evidence of new mathematical proof discovery,
nor CSR-specific semantic advantage over an equivalent indexed graph.
