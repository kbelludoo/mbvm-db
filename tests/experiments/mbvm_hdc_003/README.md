# MBVM-HDC-003: Independent Correctness and Generalization Audit

## 1. Overview
This directory contains the independent audit, hardened IVF3 index, and large-scale 200-query generalization evaluation for **MBVM-HDC-003**.

## 2. Directory Structure
```
tests/experiments/mbvm_hdc_003/
  ├── ivf3.nim                # Hardened IVF3 index with post-update assignment pass and CRC32C
  ├── hybrid3.nim             # CSR integration bridge with cycle handling and explicit KeyError
  ├── eval_generalization.nim # 200-query benchmark across 400 functions and 10 categories
  ├── run_experiment.nim      # Master runner with p50/p95/p99 latency and peak RSS tracking
  ├── tests/
  │   ├── test_ivf_audit.nim    # P0 clustering consistency & P2 security tests
  │   └── test_hybrid_audit.nim # P3 CSR integration & error handling tests
  └── results/
      ├── benchmarks/
      │   └── mbvm_hdc_003_results.json # Raw benchmark measurements
      └── reports/
          └── MBVM-HDC-003-REPORT.md   # Final technical report
```

## 3. Reproduction Commands

### Run Correctness & Security Tests:
```powershell
nim c -r tests/experiments/mbvm_hdc_003/tests/test_ivf_audit.nim
nim c -r tests/experiments/mbvm_hdc_003/tests/test_hybrid_audit.nim
```

### Run Master Benchmark & Memory Certification:
```powershell
nim c -d:release --passC:"-O3 -march=native -flto -mavx2" -r tests/experiments/mbvm_hdc_003/run_experiment.nim
```
