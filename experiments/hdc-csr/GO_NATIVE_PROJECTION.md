# Go-native MiniLM384 -> HDC8192 projection and semantic-sensitivity evaluator

## Scope

The runtime code `projection_native.go` is pure Go (stdlib) and performs:
- Load a versioned **384 x 8192 float32** trained projection, verify **file size**
  and **SHA-256 checksum** of row-major weight bytes.
- Take **384 normalized float32** features, compute each row's dot product,
  threshold at `z >= 0` and pack 8192 output bits (128 uint64, little-endian
  bit numbering).
- Compare exact Hamming distances on the five function triples.

**Integrity checksum != cryptographic authentication:** SHA-256 detects
accidental corruption; an adversary who can rewrite both data and checksum
can forge a file. Compare the full binary's hash to a trusted artifact
receipt/download channel when authenticity is important.

The trained projection has **8,192 x 384 x 4 = 12,582,912 bytes (12 MiB)**
of weights, before Go runtime, index data, MiniLM and memory overhead.
The loader keeps only its float32 weight slice plus a small read buffer.
A complete raw-Go-text to embedding pipeline **cannot** meet a 15 MB RAM
claim based on this alone. Performance is empirically reported via
`BenchmarkNativeProjection8K_384`.

## Most important limitation: Where do 384 features come from?

The training uses `sentence-transformers/all-MiniLM-L6-v2`, which is a
substantial pretrained language encoder. These weights are NOT in the
12 MiB linear projection file. Go inference below expects the same
MiniLM-produced **normalized** 384D embeddings.

The `knowledge_8k_real_ast.bin` artifact is a packed index of vectors for
existing symbols; it is NOT the 384x8192 projection matrix and cannot
evaluate arbitrary new Go source.

The goal here is Python-free **projection + scoring**, not an unfounded claim
of end-to-end Go-only text encoding. For arbitrary new text on the VPS, a
separately validated encoder is still needed (e.g., a Go implementation
with exactly compatible MiniLM tokenizer/weights, or a trusted service).

## After the CURRENT Kaggle job completes (do not restart training)

Run the two OFFLINE conversion/export scripts ON KAGGLE or a PC that already
has torch/sentence-transformers installed. Do not run Python on the VPS.

```sh
python experiments/hdc-csr/export_projection_go.py \
  --checkpoint output/real/real_ast_projection.pt \
  --out output/real/projection_go_v1.bin

python experiments/hdc-csr/export_semantic_features.py \
  --receipt output/real/real_export_receipt.json \
  --cases experiments/hdc-csr/semantic_sensitivity_cases.json \
  --out output/real/semantic_features.f32le
```

Copy only the generated binary projection, feature matrix, feature JSON
metadata and code case JSON to a Go machine. Then run entirely with Go:

```sh
go run ./experiments/hdc-csr/cmd/semantic_sensitivity_go \
  -cases experiments/hdc-csr/semantic_sensitivity_cases.json \
  -features output/real/semantic_features.f32le \
  -metadata output/real/semantic_features.json \
  -projection output/real/projection_go_v1.bin \
  -out output/real/semantic_sensitivity_go.json
```

The evaluation checks SHA-256 of fixtures and feature bytes, compatible
`source_digest` from the trained checkpoint and receipt, and the floating
point weight format; it never silently substitutes fabricated embeddings.

## Continuous tests and benchmark

```sh
go test -count=1 -run '^TestNativeProjection' -v ./experiments/hdc-csr
go test -run '^$' -bench '^BenchmarkNativeProjection8K_384$' \
  -benchmem -cpu=1 ./experiments/hdc-csr
```

The CI parser publishes the optional
`metrics.native_projection_384_to_8192_ns` and allocations if the
benchmark completed; overall quality still depends on the separately
labeled semantic evaluation. We make **no latency or accuracy promise**
until these have actually been measured.

The evaluation records each
`distance(original,equivalent)`,
`distance(original,changed)`,
and `changed - equivalent` distance, plus pairwise accuracy over five
curated examples. These functions were checked over 225 integer inputs,
not formally proved equivalent over all possible Go programs.

**Train objective caution:** the Kaggle projection minimizes disparity
between the MiniLM teacher cosine similarities and projected pairwise
similarities. It is not trained with an explicit 'ignore variable rename
but separate +/-' semantic invariance objective. Better Hamming
separation cannot be presumed before evaluating the actual export.

**Numerical caution:** floating point summation order, SIMD and fused
multiply-add can differ between Go and PyTorch. Bits where projected
logits are near zero may disagree; to claim byte-for-byte parity,
compare Go-packed vectors against checkpoint reference for identical
384D inputs, and report mismatched bit positions.
