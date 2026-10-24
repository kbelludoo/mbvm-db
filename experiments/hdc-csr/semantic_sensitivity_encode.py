#!/usr/bin/env python3
"""Evaluate behavior-controlled Go function pairs with an EXISTING trained 8K projection.

This script does not train or modify checkpoints. Distances are observations,
not proof of equivalence; Go's independent functional fixture test supplies
the bounded behavioral labels.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path

import numpy as np
import torch
from sentence_transformers import SentenceTransformer


def model_input(src: str) -> str:
    # Same field ordering as kaggle_train_export_real.py.
    # The declared signature is approximate; we document this domain shift.
    match = re.match(r"\s*func\s+([A-Za-z_]\w*)\s*\(", src)
    if not match:
        raise ValueError("expected a Go free function")
    name = match.group(1)
    signature = src.split("{", 1)[0].strip()
    return f"semanticlab func {name} {signature}\n{src}"


def packed_bits(features: np.ndarray, layer: torch.nn.Module) -> np.ndarray:
    outputs = []
    with torch.no_grad():
        for start in range(0, len(features), 32):
            chunk = torch.from_numpy(features[start:start+32])
            logits = layer(chunk).numpy()
            bits = (logits >= 0).astype(np.uint8)
            outputs.append(np.packbits(bits, axis=1, bitorder="little"))
    return np.concatenate(outputs)


def distance(a: np.ndarray, b: np.ndarray) -> int:
    return int(np.unpackbits(np.bitwise_xor(a, b), bitorder="little").sum())


class Projection8K(torch.nn.Module):
    def __init__(self):
        super().__init__()
        self.proj = torch.nn.Linear(384, 8192, bias=False)

    def forward(self, x):
        return self.proj(x)


def run():
    parser = argparse.ArgumentParser()
    parser.add_argument("--fixtures", type=Path, default=Path(
        "experiments/hdc-csr/semantic_sensitivity_cases.json"))
    parser.add_argument("--checkpoint", type=Path, required=True)
    parser.add_argument("--out", type=Path, default=Path(
        "output/real/semantic_sensitivity_neural.json"))
    parser.add_argument("--cpu-only", action="store_true",
                        help="Run MiniLM inference on CPU even when CUDA exists")
    args = parser.parse_args()
    fixtures = json.loads(args.fixtures.read_text(encoding="utf-8"))
    if fixtures.get("schema") != "mbvm-semantic-sensitivity-v1":
        raise SystemExit("invalid fixture schema")
    cases = fixtures["cases"]
    if not cases:
        raise SystemExit("empty cases")

    # Checkpoints come from our trusted training job, never arbitrary downloads.
    ckpt = torch.load(args.checkpoint, map_location="cpu", weights_only=True)
    if ckpt.get("schema") != "mbvm-projection-8k-v1" or ckpt.get("input_dim") != 384 or ckpt.get("output_bits") != 8192:
        raise SystemExit("trained projection checkpoint schema mismatch")
    model_name = ckpt["base_model"]
    states = ckpt["state_dict"]
    trained = Projection8K()
    trained.load_state_dict(states, strict=True)
    trained.eval()
    torch.manual_seed(2026)
    random_baseline = Projection8K()
    random_baseline.eval()

    sources = []
    for case in cases:
        for kind in ("original", "equivalent", "changed"):
            sources.append(model_input(case[kind]))
    device = "cpu" if args.cpu_only or not torch.cuda.is_available() else "cuda"
    encoder = SentenceTransformer(model_name, device=device)
    embeddings = encoder.encode(
        sources, batch_size=16, show_progress_bar=False,
        normalize_embeddings=True, convert_to_numpy=True
    ).astype(np.float32)
    if embeddings.shape != (len(sources), 384):
        raise SystemExit(f"unexpected encoder output shape: {embeddings.shape}")

    trained_bits = packed_bits(embeddings, trained)
    random_bits = packed_bits(embeddings, random_baseline)

    results = []
    for i, case in enumerate(cases):
        base = i * 3
        d_eq = distance(trained_bits[base], trained_bits[base + 1])
        d_changed = distance(trained_bits[base], trained_bits[base + 2])
        rd_eq = distance(random_bits[base], random_bits[base + 1])
        rd_changed = distance(random_bits[base], random_bits[base + 2])
        results.append({
            "id": case["id"],
            "trained_equivalent_bits": d_eq,
            "trained_functional_change_bits": d_changed,
            "trained_separation_bits": d_changed - d_eq,
            "untrained_equivalent_bits": rd_eq,
            "untrained_functional_change_bits": rd_changed,
            "untrained_separation_bits": rd_changed - rd_eq,
            "trained_orders_pairs_correctly": d_eq < d_changed,
        })
    success = sum(1 for x in results if x["trained_orders_pairs_correctly"])
    report = {
        "schema": "mbvm-semantic-sensitivity-neural-v1",
        "fixture_sha256": hashlib.sha256(args.fixtures.read_bytes()).hexdigest(),
        "checkpoint_sha256": hashlib.sha256(args.checkpoint.read_bytes()).hexdigest(),
        "checkpoint_source_digest": ckpt.get("source_digest"),
        "encoder_model": model_name,
        "projection": "trained MiniLM 384-to-binary-8192 with STE; baseline random linear projection",
        "trained_correct_pair_order": success,
        "total_cases": len(cases),
        "trained_pair_order_accuracy": success / len(cases),
        "results": results,
        "limitations": [
            "Behavioral labels validated only by bounded Go AST interpreter input suite",
            "This is a small set of hand-designed functions, not a general code understanding benchmark",
            "Function body edits change tokenization; MiniLM may truncate longer source snippets",
            "Projection training objective preserved teacher cosine similarity rather than explicit functional equivalence",
            "Distances <500 bits or large operator-change gaps are hypotheses, not prerequisites",
        ],
    }
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
    print(f"Trained projection orders {success}/{len(cases)} functional pairs correctly.")
    print("Wrote", args.out)


if __name__ == "__main__":
    run()
