# Semantic sensitivity four-way lab (Go + optional trained MiniLM/8K)

This lab intentionally separates *representation robustness* from
*functional understanding*. It does NOT prove that MBVM-DB has learned Go
program semantics.

## Immediate offline tests (no Kaggle, CPU, Go only)

```bash
go test -count=1 -v -run '^TestSemanticSensitivity' ./experiments/hdc-csr
```

Four conceptual conditions are evaluated via **two independent controls**:

1. **Bit flip:** XOR an exact number of uniformly sampled bit positions in the
   QUERY hypervector. No positions are disclosed to the distance function.
2. **Erasure:** the same positions are marked unknown in a separate 8192-bit
   validity mask. Hamming uses only known positions. Erasure is NOT zero-fill.
3. **Behavior-preserving Go code edits:** alpha-renaming, harmless parentheses,
   commutative arithmetic reorder and comparator reversal.
4. **Behavior-changing Go code edits:** operator/constant changes, comparator
   inversion, and operand reversal.

The bit-flip and erasure suite compares the first **10** Hamming nearest
neighbors under noise with the first **10** clean Hamming neighbors, using the
**same query IDs** and **same mask seeds** at every corruption level. Its metric
is **clean-exact-Top10 agreement**. It is NOT labeled semantic Recall@10 and
does not itself use HDC context or trained embeddings. The original
`pkg/mbvmdb/robustness_experiment_test.go` instead checked whether ten
clean-cosine neighbors appeared in the first **64** noisy Hamming candidates,
so its reported 95.2% was Recall@64 against a ten-item proxy ground truth.

The code-pair suite parses each Go function with `go/parser`, then interprets
a deliberately small, well-defined integer expression/if/return subset. Its
labels are verified over 15x15 = 225 fixed integer inputs including in-range
and out-of-distribution values. It does not prove formal equivalence over all
integers or arbitrary Go control flow. It verifies the **behavior labels**,
but does not yet evaluate representation distances.

## Trained model inference (only AFTER current Kaggle baseline finishes)

Save the `real_ast_projection.pt` from `kaggle_train_export_real.py`. This
uses the checkpoint and MiniLM, without retraining the model:

```bash
python experiments/hdc-csr/semantic_sensitivity_encode.py \
  --fixtures experiments/hdc-csr/semantic_sensitivity_cases.json \
  --checkpoint output/real/real_ast_projection.pt \
  --out output/real/semantic_sensitivity_neural.json \
  --cpu-only
```

The report contains absolute Hamming bit distances and
`trained_functional_change_bits - trained_equivalent_bits` for each pair,
plus the same baseline from a reproducibly initialized **UNTRAINED**
384-to-8192 linear projection. The measured question is: **does an
independently labeled behavior change induce a larger bit distance than
behavior-preserving edits, and does training improve this separation?**

The test functions are not training data. Do not assign thresholds like
`<500 bits` or bug-prediction probabilities without empirical validation.
The MiniLM source encoding here uses the same field order as the current
training script but is not byte-identical to its AST-manifest inputs:
it adds a simple Go signature and uses newly written functions. Report
this domain difference explicitly.

For causal evidence about HDC, compare identical trained embeddings
with and without HDC context under the **same retrieval task** and verify
that HDC itself provides an improvement. Inspect separate tests for
practical effects of input-bit corruption and unknown-bit masking.

## Current interpretation

- **4-way experimental design:** prepared.
- **Noise and behavior-label controls:** Go tests published; require CI results.
- **Trained MiniLM bit distances:** not available until Kaggle artifact arrives.
- **No alteration to `pkg/mbvmdb/`:** core DB implementation unchanged by
  these experimental files.
