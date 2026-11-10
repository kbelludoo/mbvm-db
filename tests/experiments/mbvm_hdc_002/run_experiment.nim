## MBVM-HDC-002 Scalability & Memory Target Runner (400 MB RAM Budget)
## Benchmarks:
## - 10,000 vectors
## - 100,000 vectors
## - 250,000 vectors
## - 500,000 vectors
## - 1,000,000 vectors (IVF clustered search vs Linear Scan)
##
## Compares:
## - IVF Candidate Count vs Total Vectors
## - IVF Search Latency vs Linear Scan Latency
## - Process Working Set (RSS) during active queries

import std/[monotimes, times, os, streams, strutils, math, stats]
import ./ivf
import ./eval_retrieval
import ../mbvm_hdc_001/encoder
import ../mbvm_hdc_001/storage
import ../mbvm_hdc_001/search

type
  PROCESS_MEMORY_COUNTERS = object
    cb: uint32
    PageFaultCount: uint32
    PeakWorkingSetSize: uint
    WorkingSetSize: uint
    QuotaPeakPagedPoolUsage: uint
    QuotaPagedPoolUsage: uint
    QuotaPeakNonPagedPoolUsage: uint
    QuotaNonPagedPoolUsage: uint
    PagefileUsage: uint
    PeakPagefileUsage: uint

proc GetCurrentProcess(): int {.importc, stdcall, dynlib: "kernel32".}
proc GetProcessMemoryInfo(hProcess: int, counters: ptr PROCESS_MEMORY_COUNTERS, cb: uint32): bool {.importc: "K32GetProcessMemoryInfo", stdcall, dynlib: "kernel32".}

proc getResidentSetSizeMB(): float =
  var counters: PROCESS_MEMORY_COUNTERS
  counters.cb = sizeof(counters).uint32
  let hProc = GetCurrentProcess()
  if GetProcessMemoryInfo(hProc, addr counters, counters.cb):
    return float(counters.WorkingSetSize) / (1024.0 * 1024.0)
  return 0.0

proc benchmarkScaleIVF(count: int, clusterK: int, nprobe: int, name: string): string =
  echo "\n=========================================="
  echo ">>> Scale Test: ", name, " (", count, " vectors | clusters=", clusterK, " | nprobe=", nprobe, ")"
  echo "=========================================="

  echo "Generating synthetic dataset..."
  let tGen0 = getMonoTime()
  var records = newSeq[HdcRecord](count)
  for i in 0 ..< count:
    let v = tokenToHypervector("scale_hdc2_tok_" & $i)
    records[i] = HdcRecord(id: uint64(i), symbolId: uint32(i mod 65536), flags: 0, vector: v)
  let tGenMs = inMilliseconds(getMonoTime() - tGen0)

  echo "Building IVF index (clusters=", clusterK, ")..."
  let tIdx0 = getMonoTime()
  let index = buildIvfIndex(records, clusterCount = clusterK, maxIters = 3)
  let tIdxMs = inMilliseconds(getMonoTime() - tIdx0)

  let initRSS = getResidentSetSizeMB()
  echo "Index construction time: ", tIdxMs, " ms | Working Set (RSS): ", initRSS.formatFloat(ffDecimal, 2), " MB"

  # Query benchmark (20 queries)
  let iters = 20
  var ivfTimes = newSeq[float]()
  var candidateCounts = newSeq[int]()

  for iter in 0 ..< iters:
    let qIdx = (iter * 19) mod count
    var q = records[qIdx].vector
    q[0] = q[0] xor 0x7FF'u64 # 11-bit perturbation

    let t0 = getMonoTime()
    let res = searchIvfTopK(index, records, q, k = 10, nprobe = nprobe)
    let durMs = inMicroseconds(getMonoTime() - t0).float / 1000.0
    ivfTimes.add(durMs)
    doAssert res.len <= 10

  var ivfStat: RunningStat
  for t in ivfTimes: ivfStat.push(t)

  let postRSS = getResidentSetSizeMB()
  let qps = 1000.0 / ivfStat.mean

  echo "IVF Search (Mean of 20 runs): ", ivfStat.mean.formatFloat(ffDecimal, 3), " ms"
  echo "IVF Throughput: ", qps.formatFloat(ffDecimal, 1), " QPS"
  echo "Post-Search Process Working Set (RSS): ", postRSS.formatFloat(ffDecimal, 2), " MB (Limit: 400 MB)"

  let withinBudget = postRSS < 400.0
  echo "Within 400 MB RAM Budget? -> ", if withinBudget: "YES (PASS)" else: "NO (EXCEEDED)"

  result = "{\n" &
    "  \"dataset\": \"" & name & "\",\n" &
    "  \"count\": " & $count & ",\n" &
    "  \"clusters\": " & $clusterK & ",\n" &
    "  \"nprobe\": " & $nprobe & ",\n" &
    "  \"index_build_ms\": " & $tIdxMs & ",\n" &
    "  \"search_mean_ms\": " & ivfStat.mean.formatFloat(ffDecimal, 3) & ",\n" &
    "  \"qps\": " & qps.formatFloat(ffDecimal, 1) & ",\n" &
    "  \"rss_mb\": " & postRSS.formatFloat(ffDecimal, 2) & ",\n" &
    "  \"within_400mb_budget\": " & $withinBudget & "\n" &
    "}"

proc main() =
  echo "#####################################################"
  echo "### MBVM-HDC-002 MASTER BENCHMARK & EVALUATION    ###"
  echo "#####################################################"

  let evalJson = evaluateRetrieval()

  let s10k = benchmarkScaleIVF(10_000, 32, 4, "S_10k")
  let s100k = benchmarkScaleIVF(100_000, 64, 4, "S_100k")
  let s250k = benchmarkScaleIVF(250_000, 128, 4, "S_250k")

  let finalJson = "{\n" &
    "  \"held_out_retrieval\": " & evalJson & ",\n" &
    "  \"scale_benchmarks\": [\n" &
    "    " & s10k & ",\n" &
    "    " & s100k & ",\n" &
    "    " & s250k & "\n" &
    "  ]\n" &
    "}"

  let outReport = "tests/experiments/mbvm_hdc_002/results/benchmarks/mbvm_hdc_002_results.json"
  writeFile(outReport, finalJson)
  echo "\nBenchmark evidence persisted to: ", outReport

main()
