# HDC superposition capacity stress (Go)

The 1,024-byte hypervector **has fixed physical size**, not guaranteed
lossless information capacity for arbitrarily long ordered instruction lists.
This experiment specifically tests where retrieval degrades.

Run:
```bash
go test -count=1 -v -run '^TestHDCSuperpositionCapacityStress$' ./experiments/hdc-csr
```

The test produces `hdc_capacity_results.json` and the GitHub Actions
`official_benchmarks.json` parser publishes it under `hdc_capacity_evaluation`.

## Design

The four primitive affine operations are learned from atomic examples
in the training range [-10,10]. Program compositions are **never**
used as numerical training labels. Ordered programs of lengths
3, 5, 8, 10, 12, 16, 24, 32, 48 and 64 are sampled reproducibly,
12 distinct programs per length. Random HDC role and item vectors
use three seeds (2026, 2027, 2028). Each one of the same program
sequences is tested under exact bit-flip fractions of 0%, 10%, 20%, 40%.
This yields 120 parameter combinations, 1,440 whole-program recovery
attempts and 4,320 numeric evaluations **per approach**.

Each given sequence is encoded anew by position-role binding, Majority
superposition, and recovered by XOR unbinding followed by nearest-Hamming
operation codebook lookup.

Three essential nonlearning controls:
- Symbolic byte list, `length` bytes, assumed uncorrupted. A trivial
  exact reference for the explicitly supplied sequence.
- **Equal-budget classical repetition error code**, also **1024 bytes**:
  because there are 4 possible operations, each operation is exactly
  2 logical bits. The ordered logical bitstream is divided into equal-size
  contiguous repetition blocks spanning 8192 physical bits; each bit is
  recovered by majority vote. Exactly the **same number and positions**
  of bit flips are applied to the HDC and repetition-code representation
  for each program/noise seed.
- Separate HDC codewords, `1024*length` bytes. Each carries an individual
  codeword with the SAME fraction of bits flipped; no superposition.

The fixed HDC bundle is 1,024 bytes per sequence at every length,
excluding shared role/item codebooks, intermediate encoding buffers,
retrieval machinery and runtime heap. That excludes streaming/online
memory updates. The symbolic list occupies 3–64 bytes, which is vastly
smaller than even one HDC bundle.

## Metrics

For each (seed,length,noise), record separately:
1. **Step accuracy:** fraction of original ordered operation IDs recovered.
2. **Exact-program rate:** fraction of full ordered sequences recovered
   without any wrong steps. This is the crucial capacity metric.
3. **Equal-budget ECC vs HDC:** compare whole-program recovery with
   classical repetition at the SAME 1024-byte representation budget and
   matched bit-flip positions. The ECC is specialized for a known,
   fixed-length 4-symbol alphabet; it is a legitimate minimal baseline
   for these pre-specified symbolic programs.
4. **Rounded numerical accuracy:** fraction of evaluated inputs
   (11, 17, 30) that produce exact output. This may exceed exact-program
   recovery because wrong operations can accidentally cancel or
   yield the same answer.

**Do not interpret 100% accuracy at length 3–5 as evidence of unlimited
representational capacity.** At fixed 8,192 bits, no distinct encoding
can uniquely represent every sequence in an infinite program family,
and even much shorter lengths may fail under superposition noise.

The codebook is 4 random operation codewords and 64 independent role
vectors; these must be counted for resident memory comparisons.
No program is pre-stored in HDC. Instead, the exact ordered operator
sequence is given to the encoder, so this test does not examine
natural-language ambiguity, planning or rule induction.

Use future held-out instruction-following tasks to test **active rule
selection**, with a fair symbolic lookup/classifier baseline, fully
separate from this data-channel capacity experiment.
