# Dynamic composition: no pre-stored 3–5-step programs

This CPU-only Go test is separate from Kaggle training and from the stable
`pkg/mbvmdb/` package.

Run locally with Go configured in this repository:

```sh
go test -count=1 -run '^TestAlgebraLabDynamicUnstoredComposition$' -v ./experiments/hdc-csr
```

The test writes `experiments/hdc-csr/dynamic_composition_results.json`;
the experimental GitHub Actions parser copies it to
`official_benchmarks.json` as `dynamic_composition_evaluation`.

## What the four arms know

The four atomic functions are from `composition_lab_test.go`:

- `double` = 2x
- `add3` = x+3
- `negate` = -x
- `subtract4` = x-4

Each is fitted as a two-coefficient affine transformation with training
inputs -10 through 10. **None of the 3-, 4-, or 5-step composition outputs
are in training data.**

The 32 ordered programs at each length are sampled reproducibly from all
4^length sequences. The numeric test inputs are [11,14,17,23,30].
There are 160 numerical test instances per length and seed.

Three evaluation branches share the **same fitted operations** and the
**same explicit composition executor**:

1. **Exact symbolic list:** a program is supplied directly as its ordered
   list of 3–5 primitive IDs. No random corruption is applied to the list.
   Size = 3–5 bytes using uint8 opcodes, plus a shared atomic lookup table.
2. **HDC role-bound majority bundle:** the SAME supplied program is bound
   to position roles, superposed via Majority into one 8,192-bit vector,
   optionally bit-flipped, and decoded back into primitive IDs by Hamming
   nearest-neighbor codebook search. Size = 1,024 bytes per program,
   excluding the shared codebook and learned primitives.
3. **Separate HDC codeword per step (non-superposed ablation):** the SAME
   operation codewords are passed independently for each position, under
   the SAME bit-flip percentage, and each is decoded by nearest-neighbor
   Hamming search. Size = 3–5 KB per program.

Noise rates are 0%, 10%, 20%, and 40% of **each transmitted representation**.
The error model has the same corrupted bit fraction, but not the same
number of corrupted bits or memory footprint across arms. The exact-list
arm has no comparable bit corruption and is a correctness/reference
control, NOT a noise-matched channel baseline.

There are 3 random seeds (2026,2027,2028), 3 program lengths (3,4,5), and
4 corruption rates, totaling 36 reported configurations.

## Interpretation and necessary caveats

A program has not been stored in the index beforehand. However,
**the sequence of operator IDs is already specified by the query**.
Encoding and decoding a given program is not planning, program induction,
or discovery of previously unseen operations.

The executor that composes learned affine operations is explicitly
programmed in all three arms. The HDC path cannot be credited with
inventing function composition. HDC's incremental value would have to be
measured against a compact, error-correcting, non-HDC symbolic representation
at matched memory and error budgets — the ablations here are only the
first step.

**No advantage is assumed.** Possible adverse results (step decoding loss,
program errors, numeric errors) remain in the report. The clean HDC
representation is expected to retrieve all steps; any failure is a
legitimate test failure rather than a hidden metric.

Metrics are per-seed/per-length/per-noise:
exact rounded answer rate, mean absolute error, atomic-step recovery rate,
whole-program recovery count, sample count, and representation bytes.

Neither HDC codebooks nor per-query encoding are the real trained
MiniLM/8192 embeddings. This is controlled symbolic retrieval research,
not evidence of understanding arbitrary source code.
