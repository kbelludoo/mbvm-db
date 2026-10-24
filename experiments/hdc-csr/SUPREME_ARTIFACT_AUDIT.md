# Kaggle Supreme 8K artifact audit (Go-only, no re-training)

## Why this audit exists

The repository currently contains TWO distinct Kaggle training pipelines:

| Script | Training corpus | Loss | Export |
|---|---|---|---|
| root `build_code_graph_brain.py` | Synthetic descriptions of eight technical domains | Batch self-classification cross entropy (self-on-diagonal) | `output/knowledge_8k_supreme.bin`; as checked in: **v1, 18-byte header**, `np.packbits()` **big bit-endian**, NO saved projection weights |
| `experiments/hdc-csr/kaggle_train_export_real.py` | Actual Go AST manifest symbol texts | MiniLM teacher similarity MSE | `output/real/knowledge_8k_real_ast.bin` **v2, 64-byte header, little bit-endian**, `real_ast_projection.pt`, trained receipt |

**Do not conflate the above.** The root script defaults to 8.2 hours, not
exactly two; the reported two-hour job, 10,000 vectors, `manifest.json`
and v2 header may therefore reflect a differently packaged/saved or patched
script. Check the actual Kaggle notebook script/version and files, not only
the repository source at this ref.

`knowledge_8k_supreme.bin` contains N x 1024 B of PRE-COMPUTED bit
vectors. Neither header version embeds the trained `8192 x 384` weights.
You CANNOT reconstruct the original exact matrix W from the bit vectors
alone. To project unseen Go functions, recover a saved `.pt` checkpoint
or the original trainer's in-memory `projection_model.state_dict()`
from a still-running Kaggle process. Otherwise new training (or a
clearly labeled substitute, NOT the same model) is needed.

The root trainer's cross entropy labels compare each text embedding
against itself at the diagonal of the similarity matrix, without
separate positive augmented views or AST mutation labels. Its loss may
converge without proving learned code-equivalence or bug sensitivity.

## Audit the ACTUAL binary on the VPS

No external dependencies, Python or PyTorch are required:

```sh
go run ./experiments/hdc-csr/cmd/audit_supreme_index \
  -file /path/to/knowledge_8k_supreme.bin \
  -expected-sha256 29d947f45ab120e25ea91630c64a6f55916019d16e7477da5f1019640e154d1b \
  -queries 32 \
  -out supreme_index_audit.json
```

This command does not SSH into your VPS: run it in your repository checkout
**on the VPS**. `-expected-sha256` uses the checksum provided in the
conversation; ideally independently confirm it against the Kaggle upload
and/or authenticated downloaded artifact metadata.

The Go audit (JSON schema `mbvm-index-audit-v2`) strictly validates v1
and v2 headers, declared vector count, file size, dimensions, v2 padding and
optional expected SHA-256. It samples nearest nonself Hamming distances,
counts exact duplicate vectors, bit positions that never vary and observed
fraction of 1 bits.

**Important correction to v1:** `mean_random_pair_hamming` was misleadingly
named. The previous code compared vector `i` with
`(i + N/2 + 1) % N`, so its output (e.g., 2442.4375) depends on node
ordering and is **not** a representative uniformly random-pair mean.
v2 renames this legacy measure to `mean_fixed_offset_pair_hamming`.

**New exact metric:** `mean_all_distinct_pair_hamming_exact` is computed
over ALL unordered distinct pairs without quadratic work. For a dataset
with `N` vectors and `k_j` ones at bit position `j`:

```text
mean_hamming = sum_j [ 2 * k_j * (N - k_j) / (N * (N - 1)) ]
```

This formula counts all disagreements per bit and sums them, giving the
exact global pairwise mean in O(N*8192). It is not a sample.
`mean_marginal_bit_entropy_bits` gives the average per-position binary
entropy; it does **not** measure joint entropy or independence.
A global fraction of one bits close to 0.5 cannot establish maximum
information capacity when bits correlate.

The Hamming scan does NOT need bit-order metadata to calculate distances. The index
header does NOT encode bit endianness, so the tool explicitly reports it
as unknown; do not claim to have inferred `np.packbits` order from the
header. Provenance, source labels and real AST semantics are NOT
derivable from this file.

## Tests

```sh
go test -count=1 -v ./experiments/hdc-csr/cmd/audit_supreme_index
```

GitHub Actions runs synthetic version-1 and version-2 fixture tests,
including rejection of malformed headers, wrong file lengths, invalid
dimensions and nonzero version-2 padding.

## Required files for code-semantic mutation evaluation

1. The exact **trained weight checkpoint**, with `proj.weight`
   shape `[8192,384]` and matching normalization conventions.
2. MiniLM sentence embeddings of the five controlled Go-function triples,
   computed with the same text formatting and encoder as training.
3. Independent behavioral labels to evaluate whether
   `d_H(original,functionally_changed)` is greater than
   `d_H(original,equivalent)`.
4. A non-HDC baseline (MiniLM cosine similarity and Go AST mutation
   detection), plus negative controls.

The existing `export_projection_go.py` expects a
`mbvm-projection-8k-v1` checkpoint emitted by the **real-AST** trainer.
It cannot automatically convert a `supreme` index into the trained W.

Do not publish untrusted Kaggle API tokens, private manifest source
code, SSH keys or VPS credentials in the repository or audit output.
