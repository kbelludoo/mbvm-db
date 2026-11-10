# MBVM-HDC-001 Independent Audit Report
**Auditor**: Antigravity Autonomous Systems Engineering (Rigorous Independent Audit)  
**Date**: 2026-11-10  
**Subject**: Codebase and Artifacts of Experiment MBVM-HDC-001 (`f1be491`)  

---

## 1. Audit Methodology & Scope

Every claim and component of MBVM-HDC-001 was audited independently by inspecting the actual source code, compiling standalone verification oracles, and re-running the test suites without assuming the truth of previous reports.

---

## 2. Component-by-Component Audit Findings

### 2.1 WyHash v3 64-bit Core
- **Implementation**: `wymix`, `wyr8`, `wyr4`, `wyr3`, `wyhash64` in `tests/experiments/mbvm_hdc_001/encoder.nim`.
- **Finding**: Uses the standard 128-bit multiplication fold via 32-bit high/low cross-multiplication.
- **Audit Verification**: Verified against an independent arithmetic oracle in `tests/test_oracle.nim`.
- **Status**: **CONFIRMED VALID**.

### 2.2 SplitMix64 PRNG
- **Implementation**: `splitmix64Next` using constants `0x9E37_79B9_7F4A_7C15`, `0xBF58_476D_1CE4_E5B9`, and `0x94D0_49BB_1331_11EB`.
- **Finding**: Deterministic, bit-exact pseudorandom generator.
- **Audit Verification**: Tested across repeat calls and matched against the independent implementation.
- **Status**: **CONFIRMED VALID**.

### 2.3 Structural Binding & Permutation
- **Binding**: XOR operation $V_{\text{role}} \oplus V_{\text{identifier}}$.
- **Permutation**: `permuteCyclic` implements bit-exact cyclic rotation of 8,192 bits. Reversibility $\rho^{-k}(\rho^k(V)) = V$ was verified and passed.
- **Bundling**: Uses majority voting ($2 \cdot \text{ones} > N$). Tie-breaking is deterministic (bit parity rule).
- **Status**: **CONFIRMED VALID**.

### 2.4 Binary Container (`MBVMHDC1`) & CRC32C
- **Format**: 64-byte packed header, Little-Endian records, 1040 bytes/record.
- **CRC32C**: Castagnoli polynomial `0x82F6_3B78`.
- **Corruption Rejection**: Verified that truncated files, invalid magic bytes, header corruption, and payload truncation are rejected with `HdcCorruptError`.
- **Status**: **CONFIRMED VALID**.

### 2.5 Search & Linear Oracle
- **Distance**: CPU `popcount(a xor b)`.
- **Top-K**: Deterministic tie-breaking on `(distance ASC, id ASC)`.
- **MMap**: Successfully maps file without copying entire buffer to Nim heap.
- **Status**: **CONFIRMED VALID**.

---

## 3. Critical Investigation: The "100% Semantic Recall@1 and Recall@3" Claim

### 3.1 What was actually tested in MBVM-HDC-001?
In `run_experiment.nim` (lines 182–239):
- **Corpus size**: Exactly **8 functions** (2 per class across 4 classes).
- **Query size**: Exactly **4 queries** (1 per class).
- **Total sample space**: Only **12 code snippets**.

### 3.2 Audit Assessment & Flaws Identified:
1. **Sample Size Deficiency**: Measuring Recall@K on $N=4$ queries is statistically weak. A 4/4 score is an illustrative sanity check, NOT generalizable proof of semantic understanding.
2. **Lexical Leakage**: The queries, while renamed, shared distinctive keywords and structures with the corpus (e.g. `chan int`, `time.Second`, `sync.Mutex`, `[]int`).
3. **No Negative / Adversarial Counter-Examples**: The test had no distractor functions with similar syntax but opposite behavior (e.g. `sync.RWMutex` reader vs writer, or channels with buffering/draining).
4. **Conclusion**:
   > **The claim of "100% semantic recall" in the previous report was an overstatement based on a toy 8-item dataset. HDC 8K encoding provides structural and token similarity, but it DOES NOT guarantee semantic equivalence in real-world codebases without a more rigorous evaluation.**

---

## 4. Scalability & Memory Budget Finding (400 MB VPS Audit)

- In `run_experiment.nim`, at **1,000,000 vectors**, linear scan over a 991 MB file took **321 ms** and pushed the process working set (RSS) to **1,107 MB** (and peaked at 2,099 MB during repeated warm searches) because unindexed linear scanning forces the OS page cache to map all 1 GB into physical memory.
- Therefore: **MBVM-HDC-001 linear scan FAILS the 400 MB RAM budget at 1M scale.**
- **Corrective Directive for MBVM-HDC-002**: An Inverted File Index (IVF) is strictly necessary to partition the hypervectors into clusters, probing only a small fraction of centroids and candidate records, keeping both search latency under 20 ms and active memory well under 400 MB.
