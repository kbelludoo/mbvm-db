#!/usr/bin/env python3
"""OFFLINE checkpoint conversion on Kaggle/PC. Runtime projection stays Go-only.

A 384D MiniLM text embedding remains mandatory. Conversion does not turn a
MiniLM model into Go code; it converts ONLY its trained 384->8192 projection.
"""
import argparse
import hashlib
import json
import math
import struct
from pathlib import Path

import numpy as np
import torch

MAGIC = b"MBVMPJ1\x00"
VERSION = 1
IN_DIM = 384
OUT_BITS = 8192
FLOAT_FORMAT = 1


def export_projection(checkpoint_path: Path, output_path: Path) -> dict:
    # Load checkpoints ONLY from our trusted Kaggle training job.
    ckpt = torch.load(checkpoint_path, map_location="cpu", weights_only=True)
    if (ckpt.get("schema") != "mbvm-projection-8k-v1" or
        ckpt.get("input_dim") != IN_DIM or
        ckpt.get("output_bits") != OUT_BITS):
        raise ValueError("incompatible trained checkpoint schema/dimensions")
    source_digest = ckpt.get("source_digest", "")
    if (not isinstance(source_digest, str) or len(source_digest) != 64):
        raise ValueError("missing source_digest")
    source_bytes = bytes.fromhex(source_digest)
    state = ckpt.get("state_dict")
    if set(state.keys()) != {"proj.weight"}:
        raise ValueError("unexpected projection parameters or bias")
    weight = state["proj.weight"].detach().cpu().contiguous().numpy()
    if weight.shape != (OUT_BITS, IN_DIM):
        raise ValueError(f"unexpected weight shape {weight.shape}")
    # The Go runtime expects each output row of weights in little-endian
    # float32, no bias. Numpy 'C' order matches PyTorch Linear weight layout.
    weights_bytes = weight.astype("<f4", copy=False).tobytes(order="C")
    if not np.isfinite(weight).all():
        raise ValueError("non-finite weights")
    checksum = hashlib.sha256(weights_bytes).digest()
    header = (
        MAGIC +
        struct.pack("<4I", VERSION, IN_DIM, OUT_BITS, FLOAT_FORMAT) +
        checksum +
        source_bytes +
        bytes(8)
    )
    if len(header) != 96:
        raise AssertionError(f"expected 96-byte header, got {len(header)}")
    output_path.parent.mkdir(parents=True,exist_ok=True)
    output_path.write_bytes(header + weights_bytes)
    if output_path.stat().st_size != 96+OUT_BITS*IN_DIM*4:
        raise AssertionError("projection size mismatch")
    return {
        "schema": "mbvm-go-projection-v1",
        "source_digest": source_digest,
        "projection_sha256": hashlib.sha256(output_path.read_bytes()).hexdigest(),
        "weights_sha256": checksum.hex(),
        "source_checkpoint_sha256": hashlib.sha256(checkpoint_path.read_bytes()).hexdigest(),
        "bytes": output_path.stat().st_size,
        "input_dim": IN_DIM,
        "output_bits": OUT_BITS,
        "text_encoder": ckpt.get("base_model"),
        "important_limitation": "Go can run this projection from normalized MiniLM384 features; encoding raw text remains a separate requirement.",
    }


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--checkpoint",type=Path,required=True)
    p.add_argument("--out",type=Path,default=Path("output/real/projection_go_v1.bin"))
    a=p.parse_args()
    result=export_projection(a.checkpoint,a.out)
    meta_path=a.out.with_suffix(".json")
    meta_path.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    print(json.dumps(result,indent=2))


if __name__ == "__main__":
    main()
