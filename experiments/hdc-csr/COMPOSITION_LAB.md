# Compositional Rule Generalization Lab — MBVM-DB

This is a **toy CPU-only control** to test what is *learned*, what is
*provided by an executor*, and what is merely *recalled* from associative
memory. It does not run Kaggle, consume its 8K neural vectors or claim
mathematical reasoning in a general-purpose LLM.

Run:

```bash
go test -count=1 -v -run '^TestAlgebraLabCompositionalOODTriplet$' ./experiments/hdc-csr
```

A successful run produces `experiments/hdc-csr/composition_lab_results.json`.
The official CI parser embeds it as `algebra_composition_evaluation`.

## Hidden task and held-out split

Primitive functions (used only by the label-generating oracle):

- F0(x) = 2x;
- F1(x) = x+3;
- F2(x) = -x;
- F3(x) = x-4.

The **training set** consists of these operations applied *individually*
to integers -10 through 10. Each primitive is supplied in either first
or second program slot, with an explicit identity in the other slot.
**Zero composed output labels** are supplied to any trainable system.

The **held-out compositional set** includes all 16 ordered pairs (including
same-op pairs) and inputs 11 through 30, totaling 320 queries per run.
The oracle only scores the output after training. It is not a training input.

## Ablation arms

1. `neural_mlp_direct`: a 32-hidden-unit unconstrained tanh MLP trained
   on the identical atomic-label data in the two input slots. At evaluation
   it must directly predict outputs of held-out two-operation combinations.
   Its input design does not enforce function composition.
2. `learned_affine_with_explicit_composition`: each atomic operation is
   learned as `w*x+b` from numeric labels, then an **explicit programmed
   evaluator** sequentially applies the learned functions. The ability
   to compose is therefore **given by architecture**, not learned from
   composed examples.
3. `learned_affine_with_hdc_program_memory`: exactly the same affine
   models and sequential evaluator. For each program, the step IDs
   are encoded with independent HDC role vectors, Bundling via Majority,
   and an XOR-bound task key. A corrupted key is used to recover the
   program steps by Hamming nearest codebook match.
4. `learned_affine_with_nearest_key_map`: an essential non-HDC control.
   Given the same corrupted task key, nearest-Hamming lookup retrieves
   a conventional map entry containing the ordered program; it then uses
   exactly the same affine models and executor.

Programs are registered in memory **before** evaluation. Thus this lab
does not prove that HDC inferred an unseen program from a written question.
The held-out target consists of composed **numeric outputs**, not hidden
program descriptors. The HDC arm already knows the sequence it has to execute.
The nearest-key control ensures that successful noisy-key retrieval is not
attributed uniquely to HDC.

## Reproducibility and interpretation

Fixed seeds 2026, 2027, 2028; perturb the 8192-bit HDC task key with
0%, 20%, 40% exact random bit flips. Report:
rounded integer exact accuracy, mean absolute error, number of programs
recovered, and nearest-key comparison. Each seed/noise test evaluates
320 composed OOD problems. If an affine model and executor attain 100%,
the result is expected on these affine rules and **not a proof of acquired
abstract reasoning**.

The pure MLP and constrained-affine baseline differ structurally, so any
performance difference is a **comparison of inductive biases**, not an
isolated causal effect of the HDC. The HDC vs nearest-key map arms are
the more relevant control for memory representation, though indexing
and resident memory overhead are not measured here.

Future experiments should include non-affine rules, programs omitted from
memory, operations whose order changes the answer, lengths >=3, label and
training budget matching, noisy/partial task descriptions, and multiple
seeds with confidence intervals. Trained Kaggle code embeddings are a
separate domain and must not be presumed to generalize from this toy task.

The `f9a7a42` tests in `pkg/mbvmdb/` test XOR commutativity and
bit-corruption retrieval, which are different phenomena. This lab does
not change `pkg/mbvmdb/`.
