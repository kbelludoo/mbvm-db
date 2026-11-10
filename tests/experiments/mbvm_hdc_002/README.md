# MBVM-HDC-002: Inverted File Index (IVF) & CSR Symbolic Integration

## 1. Scope & Mission
This experiment continues from the audited baseline of **MBVM-HDC-001** (`f1be491`) and accomplishes:
1. **Independent Audit of MBVM-HDC-001**: Verification of primitives, identification of sample size limitations, and memory boundary diagnosis.
2. **Inverted File Index (IVF)**: Deterministic clustering with majority bundling centroids, compact serialization, and exact Hamming reranking over probed posting lists.
3. **CSR Symbolic Integration**: Zero-modification bridging where HDC candidates resolve directly into symbolic CSR graph nodes and traverse call/dependency relationships with strict provenance.
4. **Held-Out Semantic Evaluation**: 24-sample corpus across 6 classes, featuring negative distractor functions, syntax variations, and renamed identifiers.
5. **Strict 400 MB RAM Scale Testing**: Benchmarking across 10k, 100k, and 250k vectors verifying working set (RSS) limits on constrained hardware.

## 2. Directory Layout
```
tests/experiments/mbvm_hdc_002/
  ├── ivf.nim              # Phase 1: IVF index builder, centroid bundling, serialization & candidate reranker
  ├── hybrid.nim           # Phase 3: Hybrid bridge (HDC Candidate -> CSR BFS expansion & provenance)
  ├── eval_retrieval.nim   # Phase 4: Held-out semantic & negative distractor evaluation
  ├── run_experiment.nim   # Phase 5: Master benchmark & 400 MB RAM budget profiler
  ├── tests/
  │   ├── test_ivf.nim     # Unit & oracle tests for IVF clustering, serialization, and recall
  │   └── test_hybrid.nim  # End-to-end integration test with real CSR graph
  └── results/
      ├── benchmarks/      # JSON benchmark results (mbvm_hdc_002_results.json)
      └── reports/
          ├── MBVM-HDC-001-AUDIT.md   # Critical independent audit of previous claims
          └── MBVM-HDC-002-REPORT.md  # Definitive technical report for HDC-002
```

## 3. Reproduction Commands

### Run Unit Tests & Correctness Suites:
```powershell
nim c -r tests/experiments/mbvm_hdc_002/tests/test_ivf.nim
nim c -r tests/experiments/mbvm_hdc_002/tests/test_hybrid.nim
```

### Run Master Scale & Evaluation Runner:
```powershell
nim c -d:release --passC:"-O3 -march=native -flto -mavx2" -r tests/experiments/mbvm_hdc_002/run_experiment.nim
```
