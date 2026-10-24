#!/usr/bin/env python3
"""OFFLINE MiniLM feature extraction. This runs with the existing Kaggle/PC
transformers environment, NOT in the Go inference process on the VPS.

Output order: each JSON case's original, equivalent, changed.
All 384D features are L2-normalized float32 little-endian.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path
import numpy as np
from sentence_transformers import SentenceTransformer

def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()

def format_source(src: str) -> str:
    m = re.match(r"\s*func\s+([A-Za-z_]\w*)\s*\(", src)
    if m is None:
        raise ValueError("expected free Go function snippet")
    name = m.group(1)
    signature = src.split("{", 1)[0].strip()
    # Mirrors training's field order package/kind/name/signature/body, but
    # snippets in this lab are NOT actual manifest declarations.
    return f"semanticlab func {name} {signature}\n{src}"

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--cases", type=Path, default=Path("experiments/hdc-csr/semantic_sensitivity_cases.json"))
    ap.add_argument("--receipt", type=Path, required=True,
                    help="real_export_receipt.json from the SAME Kaggle run as the projection")
    ap.add_argument("--out", type=Path, default=Path("output/real/semantic_features.f32le"))
    args = ap.parse_args()
    raw_cases=args.cases.read_bytes()
    fixtures=json.loads(raw_cases)
    if fixtures.get("schema") != "mbvm-semantic-sensitivity-v1":
        raise SystemExit("unsupported fixture schema")
    rows=fixtures["cases"]
    if len(rows)!=5 or any(not all(x in row for x in ("original","equivalent","changed")) for row in rows):
        raise SystemExit("expected five complete ordered cases")
    receipt=json.loads(args.receipt.read_text(encoding="utf-8"))
    if receipt.get("schema") != "mbvm-real-vectors-v1" or not receipt.get("projection_trained"):
        raise SystemExit("invalid trained receipt")
    model=receipt["base_model"]
    texts=[format_source(row[field]) for row in rows for field in ("original","equivalent","changed")]
    encoder=SentenceTransformer(model,device="cpu")
    features=encoder.encode(
        texts,batch_size=15,show_progress_bar=False,
        normalize_embeddings=True,convert_to_numpy=True).astype("<f4",copy=False)
    if features.shape!=(15,384) or not np.isfinite(features).all():
        raise SystemExit(f"invalid MiniLM embedding dimensions/values: {features.shape}")
    data=features.tobytes(order="C")
    args.out.parent.mkdir(parents=True,exist_ok=True)
    args.out.write_bytes(data)
    meta={
        "schema":"mbvm-semantic-features-v1",
        "rows":len(texts),
        "dimensions":384,
        "ordered_fields":["original","equivalent","changed"],
        "fixture_sha256":sha256(raw_cases),
        "features_sha256":sha256(data),
        "source_digest":receipt["source_digest"],
        "encoder_model":model,
        "text_format":"semanticlab func {name} {signature}\\n{raw Go function}",
        "note":"Features are extracted offline by MiniLM; downstream 8K projection and distance evaluation run in pure Go.",
    }
    args.out.with_suffix(".json").write_text(json.dumps(meta,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    print(json.dumps({"feature_file":str(args.out),"feature_bytes":len(data),"rows":len(texts)},indent=2))

if __name__=="__main__":
    main()
