# MBVM-HDC-001: Autonomous Experiment Documentation & Reproduction

## 1. Scope & Objective
This directory contains the isolated, autonomous engineering experiment **MBVM-HDC-001** for MBVM-DB.
The goal is to implement, test, benchmark, and independently verify an 8,192-bit Hyperdimensional Computing (HDC) Associative Memory Engine alongside the symbolic MBVM-DB engine.

## 2. Directory Layout
```
tests/experiments/mbvm_hdc_001/
  ├── encoder.nim          # P0: WyHash v3 64-bit + SplitMix64 + VSA operators + AST-aware Go tokenizer
  ├── storage.nim          # P1/P2: MBVMHDC1 binary container with CRC32C and corruption rejection
  ├── search.nim           # P3/P4: Linear scan Top-K and mmap (memfiles) zero-copy engine
  ├── run_experiment.nim   # P5: Master benchmark runner, RSS profiler, and evaluation suite
  ├── tests/
  │   ├── test_oracle.nim          # Independent ground-truth oracle (pure bit-by-bit calculation)
  │   ├── test_determinism.nim     # P0 verification against independent oracle
  │   ├── test_binding.nim         # P0 binding, reversibility, and bundling verification
  │   ├── test_storage.nim         # P1/P2 roundtrip, truncation, and corruption rejection tests
  │   └── test_search_oracle.nim   # P3 Top-K exact match against independent oracle
  ├── fixtures/            # Generated binary artifacts (.hdc)
  └── results/
      ├── benchmarks/      # Machine-readable JSON benchmark data
      └── reports/         # Detailed experimental audits
```

## 3. Reproduction Instructions

### Prerequisites
- **Nim Compiler**: >= 2.2.0 (Tested on Nim 2.2.12 AMD64)
- **C Compiler**: GCC MinGW-w64 with POSIX threads & UCRT (or Clang)

### Run Unit Tests & Independent Oracle Verification:
```powershell
nim c -r tests/experiments/mbvm_hdc_001/tests/test_determinism.nim
nim c -r tests/experiments/mbvm_hdc_001/tests/test_binding.nim
nim c -r tests/experiments/mbvm_hdc_001/tests/test_storage.nim
nim c -r tests/experiments/mbvm_hdc_001/tests/test_search_oracle.nim
```

### Run Full Scalability Benchmark (D1=1k, D2=10k, D3=100k, D4=1M):
```powershell
nim c -d:release --passC:"-O3 -march=native -flto -mavx2" -r tests/experiments/mbvm_hdc_001/run_experiment.nim
```
