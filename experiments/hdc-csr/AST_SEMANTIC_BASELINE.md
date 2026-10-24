# Go-native AST equivalence baseline (no neural model)

This is an intentionally conservative, deterministic, Go-standard-library-only
reference for the five curated source-code transformation fixtures in
`semantic_sensitivity_cases.json`. It is NOT a full Go semantic analyzer.

## Run

```sh
go test -count=1 -v -run '^TestASTBaseline' ./experiments/hdc-csr
go test -run '^$' -bench '^BenchmarkASTCanonicalGoFunction$' -benchmem ./experiments/hdc-csr
```

Implemented by `ast_semantic_baseline.go` with tests in
`ast_semantic_baseline_test.go`. The ordinary Go CI tests in
`.github/workflows/hdc_benchmark_pages.yml` include these files
through `go test` of the entire `experiments/hdc-csr` package.

## Explicitly supported semantics

- Free functions with exactly **two named int parameters and one int return**.
- Integer literals, parameter reads, `+`, `-`, `*`, unary plus/minus,
  parentheses, up to degree 8 in polynomial arithmetic.
- One `if` with a binary integer comparison and a `return`, followed
  by an unconditional `return`. `<` and `>` and their reversed
  operands normalize to the same condition; `<=` and `>=` too.
- Algebraic normalization using **arbitrary-precision integer polynomial
  coefficients**, positional argument names and sorted monomials.

This recognizes parameter alpha-renaming, association/commutativity in
pure polynomial arithmetic, constant folding, reordering `2*b+b`,
and `a-b` versus `a+(-b)`. It recognizes `a>b` versus `b<a`
for identical then/fallback return expressions.

It **rejects** loops, function calls, division, bitwise operations,
assignments, effects, general Go control flow, non-`int` signatures and
large unsupported expressions. Unsupported inputs return an error.

### Soundness distinction

`ASTConservativeEquivalent(a,b)` returns:
- `true` only if both are supported and have identical canonical forms.
  This is a proof of the same polynomial/conditional structure for the
  recognized subset, subject to normal Go int operation semantics.
- `false` if the canonical forms differ: **NOT PROVEN EQUIVALENT**.
  False does **not** prove a functional difference. For example, two
  programs using different but equivalent conditional rewrites may be
  reported as `false` because the normalizer does not implement that rule.
- An error for unsupported constructs. Do not conflate this with an
  inference that the snippets are functionally distinct.

The test suite independently checks all five alleged functional mutations
with an existing bounded Go AST evaluator over **225 input pairs per case**.
That supplies a concrete behavioral counterexample where applicable;
it does NOT prove arbitrary Go functions equivalent over all inputs.

## Scientific result and fair next experiment

On the existing five curated cases, this baseline is designed to certify
**5/5 equivalent pairs** and reject equivalence certification for
**5/5 behavior-changing mutations**, with a separate behavioral witness
for every changed pair. That is a small hand-authored fixture, not a
large benchmark or an argument that symbolic normalization replaces all
code-language models.

The original V4 `knowledge_8k_supreme.bin` is a **synthetic template**
index; it contains no trained projection W. Therefore its 97.5%
synthetic-domain Top1 accuracy does not establish performance on these
function mutations. The Go lexical Jaccard baseline achieves 100%
on that separate synthetic-template task by construction.

To claim incremental neural value, evaluate one frozen encoder on a
large, held-out, non-template benchmark with real repositories, paired
behavioral tests, AST canonicalization and token-based baselines, aligned
quality/cost/latency metrics, and no train/test leakage. A neural model
is NOT logically required to identify `+` to `-` changes or harmless
alpha-renaming in the supported Go subset.
