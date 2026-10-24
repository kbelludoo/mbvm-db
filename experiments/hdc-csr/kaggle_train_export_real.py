#!/usr/bin/env python3
"""Train a REAL-AST MiniLM->8K projection on Kaggle and export MBVM8K1 v2.

Requires: torch, numpy, sentence-transformers.
This uses the actual Go declaration texts exported by cmd/export_real_ast.
It does NOT silently substitute synthetic data or an untrained projection.
"""
import argparse
import hashlib
import json
import random
import struct
from pathlib import Path

import numpy as np
import torch
from sentence_transformers import SentenceTransformer

MAGIC = b"MBVM8K1\x00"
D = 8192


def sha256(path):
    digest = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


class BitProjection8K(torch.nn.Module):
    def __init__(self, input_dim):
        super().__init__()
        self.proj = torch.nn.Linear(input_dim, D, bias=False)
        torch.nn.init.orthogonal_(self.proj.weight)

    def forward(self, x):
        z = self.proj(x)
        signed = torch.where(z >= 0, 1.0, -1.0)
        soft = torch.tanh(z)
        return signed - soft.detach() + soft


def validate_manifest(path):
    payload = json.loads(path.read_text(encoding="utf-8"))
    if payload.get("schema") != "mbvm-real-ast-v1":
        raise ValueError("real AST manifest required: run export_real_ast first")
    symbols = payload["symbols"]
    n = len(symbols)
    if n < 10 or n > 65536:
        raise ValueError(f"expect 10..65536 real symbols; got {n}")
    if len(payload["offsets"]) != n + 1 or len(payload["edges"]) != len(payload["relations"]):
        raise ValueError("CSR manifest invalid")
    for i, row in enumerate(symbols):
        if row["id"] != i:
            raise ValueError(f"AST symbol ID mismatch at {i}")
        content = row["text"].encode("utf-8")
        if hashlib.sha256(content).hexdigest() != row["text_sha256"]:
            raise ValueError(f"tampered AST text at {i}")
    if len(payload.get("source_digest", "")) != 64:
        raise ValueError("source digest missing")
    return payload


def train_projection(features, train_ids, validation_ids, epochs, batch_size, lr, device, seed):
    input_dim = int(features.shape[1])
    network = BitProjection8K(input_dim).to(device)
    optimizer = torch.optim.AdamW(network.parameters(), lr=lr, weight_decay=1e-4)
    # The teacher is the pretrained MiniLM embedding similarity. Graph edges
    # are intentionally NOT training labels to prevent structural-label leakage.
    generator = np.random.default_rng(seed)
    for epoch in range(epochs):
        network.train()
        shuffled = generator.permutation(train_ids)
        batch_losses = []
        for start in range(0, len(shuffled), batch_size):
            batch = shuffled[start:start + batch_size]
            if len(batch) < 2:
                continue
            x = torch.from_numpy(features[batch]).to(device)
            optimizer.zero_grad()
            pred = network(x)
            sim_pred = (pred @ pred.T) / float(D)
            sim_teacher = x @ x.T
            mask = ~torch.eye(len(batch), device=device, dtype=torch.bool)
            loss = torch.mean((sim_pred[mask] - sim_teacher[mask]) ** 2)
            if not torch.isfinite(loss):
                raise ValueError("non-finite training loss")
            loss.backward()
            optimizer.step()
            batch_losses.append(float(loss.detach().cpu()))
        if not batch_losses:
            raise ValueError("not enough train batches to learn projection")
        print(f"epoch {epoch + 1}/{epochs}: train_pair_mse={np.mean(batch_losses):.6f}", flush=True)
    if not validation_ids.size:
        raise ValueError("held-out symbols required")
    # Held-out reconstruction of teacher cosine similarities (not code retrieval accuracy).
    network.eval()
    with torch.no_grad():
        val = torch.from_numpy(features[validation_ids]).to(device)
        bits = network(val)
        pred = (bits @ bits.T) / float(D)
        teacher = val @ val.T
        mask = ~torch.eye(len(val), device=device, dtype=torch.bool)
        val_loss = float(torch.mean((pred[mask] - teacher[mask]) ** 2).cpu())
    return network, val_loss


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--manifest", type=Path, required=True)
    p.add_argument("--out-dir", type=Path, default=Path("output/real"))
    p.add_argument("--model", default="sentence-transformers/all-MiniLM-L6-v2")
    p.add_argument("--epochs", type=int, default=5)
    p.add_argument("--batch-size", type=int, default=32)
    p.add_argument("--lr", type=float, default=0.001)
    p.add_argument("--seed", type=int, default=42)
    a = p.parse_args()
    if a.epochs < 1 or a.batch_size < 2:
        raise SystemExit("epochs>=1 and batch-size>=2 required; random projection is not a trained result")
    manifest = validate_manifest(a.manifest)
    a.out_dir.mkdir(parents=True, exist_ok=True)
    random.seed(a.seed)
    np.random.seed(a.seed)
    torch.manual_seed(a.seed)
    if torch.cuda.is_available():
        torch.cuda.manual_seed_all(a.seed)
    device = "cuda" if torch.cuda.is_available() else "cpu"
    print("Device:", device, "Symbols:", len(manifest["symbols"]))
    encoder = SentenceTransformer(a.model, device=device)
    # Include the real Go function/type body; truncation is performed by the MiniLM tokenizer.
    texts = [
        f'{row["package"]} {row["kind"]} {row["name"]} {row.get("signature", "")}\n{row["text"]}'
        for row in manifest["symbols"]
    ]
    embs = encoder.encode(texts, batch_size=min(a.batch_size, 32),
                          show_progress_bar=True, normalize_embeddings=True,
                          convert_to_numpy=True).astype(np.float32)
    if embs.shape[0] != len(texts) or embs.shape[1] != 384:
        raise ValueError(f"MiniLM shape unexpected: {embs.shape}; expected N x 384")
    ids = np.random.default_rng(a.seed).permutation(len(texts))
    n_val = max(2, int(round(len(ids) * 0.2)))
    train_ids, validation_ids = ids[n_val:], ids[:n_val]
    if len(train_ids) < 2:
        raise ValueError("insufficient training symbols")
    projection, heldout_mse = train_projection(
        embs, train_ids, validation_ids, a.epochs, a.batch_size, a.lr, device, a.seed
    )
    checkpoint_path = a.out_dir / "real_ast_projection.pt"
    torch.save({
        "schema": "mbvm-projection-8k-v1",
        "input_dim": 384,
        "output_bits": D,
        "base_model": a.model,
        "source_digest": manifest["source_digest"],
        "seed": a.seed,
        "epochs": a.epochs,
        "state_dict": projection.cpu().state_dict(),
    }, checkpoint_path)
    index_path = a.out_dir / "knowledge_8k_real_ast.bin"
    with index_path.open("wb") as f:
        f.write(MAGIC)
        f.write(struct.pack("<IIH", 2, len(texts), D))
        f.write(bytes(46))  # v2: 64B header
        projection.eval()
        with torch.no_grad():
            for start in range(0, len(embs), a.batch_size):
                x = torch.from_numpy(embs[start:start + a.batch_size])
                binary = (projection(x).numpy() >= 0).astype(np.uint8)
                # uint64 little endian in Go: bit j corresponds to bit j%8 of byte j//8.
                packed = np.packbits(binary, axis=1, bitorder="little")
                if packed.shape[1] != 1024:
                    raise ValueError("expected exactly 1024 bytes per symbol")
                f.write(packed.tobytes())
    size = index_path.stat().st_size
    expected = 64 + len(texts) * 1024
    if size != expected:
        raise ValueError(f"unexpected export size {size} != {expected}")
    receipt = {
        "schema": "mbvm-real-vectors-v1",
        "source_digest": manifest["source_digest"],
        "manifest_sha256": sha256(a.manifest),
        "index_sha256": sha256(index_path),
        "checkpoint_sha256": sha256(checkpoint_path),
        "vector_count": len(texts),
        "vector_bits": D,
        "index_version": 2,
        "bit_order": "little",
        "base_model": a.model,
        "real_go_ast": True,
        "projection_trained": True,
        "train_symbols": len(train_ids),
        "heldout_symbols": len(validation_ids),
        "heldout_teacher_similarity_mse": heldout_mse,
        "evaluation_note": "Teacher similarity validation does not establish held-out structural retrieval accuracy.",
    }
    (a.out_dir / "real_export_receipt.json").write_text(
        json.dumps(receipt, indent=2, sort_keys=True), encoding="utf-8"
    )
    print(json.dumps({"index": str(index_path), "size_bytes": size,
                      "vectors": len(texts), "heldout_pair_mse": heldout_mse}, indent=2))


if __name__ == "__main__":
    main()
