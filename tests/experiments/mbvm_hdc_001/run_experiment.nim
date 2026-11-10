## MBVM-HDC-001 Master Benchmark and Scalability Runner
## Executes:
## - Scalability across D1 (1k), D2 (10k), D3 (100k), D4 (1M)
## - Timing: Serialization, Deserialization, MMap open, Cold search, Warm search
## - Collision analysis: min/mean/max Hamming distances and standard deviation
## - Associative Retrieval: Recall@K against labeled Go function variations
## - Generates JSON benchmark evidence for final report

import std/[monotimes, times, os, streams, strutils, math, stats]
import ./encoder
import ./storage
import ./search

# --- Windows Working Set (RSS) Measurement via K32 ---

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

proc runScaleDataset(count: int, name: string): string =
  echo "\n=========================================="
  echo ">>> Benchmark Dataset: ", name, " (", count, " vectors)"
  echo "=========================================="

  let baseDir = "tests/experiments/mbvm_hdc_001/fixtures"
  createDir(baseDir)
  let filePath = baseDir / (name & ".hdc")

  # 1. Generation
  echo "Generating ", count, " synthetic hypervectors..."
  let tGen0 = getMonoTime()
  var container: HdcContainer
  container.records = newSeq[HdcRecord](count)
  for i in 0 ..< count:
    let v = tokenToHypervector("scale_token_symbol_" & $i)
    container.records[i] = HdcRecord(id: uint64(i), symbolId: uint32(i mod 65536), flags: 0, vector: v)
  let tGenDurMs = inMilliseconds(getMonoTime() - tGen0)
  echo "Generation time: ", tGenDurMs, " ms"

  # 2. Serialization to disk
  echo "Serializing to disk: ", filePath
  let tSer0 = getMonoTime()
  var fs = newFileStream(filePath, fmWrite)
  serializeContainer(container, fs)
  fs.close()
  let tSerDurMs = inMilliseconds(getMonoTime() - tSer0)
  let fSize = getFileSize(filePath)
  echo "Serialization time: ", tSerDurMs, " ms (File size: ", fSize div (1024 * 1024), " MB)"

  # 3. MMap Open
  let tMMap0 = getMonoTime()
  var engine = openMMapEngine(filePath)
  let tMMapDurUs = inMicroseconds(getMonoTime() - tMMap0)
  echo "MMap open time: ", tMMapDurUs, " us"

  let rssMB = getResidentSetSizeMB()
  echo "Current Process Working Set (RSS): ", rssMB.formatFloat(ffDecimal, 2), " MB"

  # 4. Cold Search (Query index 0)
  var query = container.records[count div 2].vector
  # Flip 10 bits
  query[0] = query[0] xor 0x3FF'u64

  let tCold0 = getMonoTime()
  let coldRes = engine.searchMMapTopK(query, 10)
  let tColdDurMs = inMicroseconds(getMonoTime() - tCold0).float / 1000.0
  echo "Cold Top-10 search time: ", tColdDurMs.formatFloat(ffDecimal, 3), " ms"
  doAssert coldRes.len == 10

  # 5. Warm Searches (50 iterations)
  var warmTimes = newSeq[float]()
  let iters = if count >= 1000000: 5 else: 20
  for iter in 0 ..< iters:
    let qIdx = (iter * 17) mod count
    var q = container.records[qIdx].vector
    q[1] = q[1] xor 0xFF'u64
    let t0 = getMonoTime()
    let res = engine.searchMMapTopK(q, 10)
    let durMs = inMicroseconds(getMonoTime() - t0).float / 1000.0
    warmTimes.add(durMs)
    doAssert res.len == 10

  var runningStat: RunningStat
  for t in warmTimes:
    runningStat.push(t)

  let medianMs = runningStat.mean # approx
  let minMs = runningStat.min
  let maxMs = runningStat.max
  let qps = 1000.0 / medianMs

  echo "Warm Search (", iters, " runs):"
  echo "  Min:    ", minMs.formatFloat(ffDecimal, 3), " ms"
  echo "  Mean:   ", medianMs.formatFloat(ffDecimal, 3), " ms"
  echo "  Max:    ", maxMs.formatFloat(ffDecimal, 3), " ms"
  echo "  Throughput (QPS): ", qps.formatFloat(ffDecimal, 1), " queries/sec"
  echo "  Post-search RSS:  ", getResidentSetSizeMB().formatFloat(ffDecimal, 2), " MB"

  engine.close()

  result = "{\n" &
    "  \"dataset\": \"" & name & "\",\n" &
    "  \"count\": " & $count & ",\n" &
    "  \"file_size_bytes\": " & $fSize & ",\n" &
    "  \"gen_ms\": " & $tGenDurMs & ",\n" &
    "  \"ser_ms\": " & $tSerDurMs & ",\n" &
    "  \"cold_search_ms\": " & tColdDurMs.formatFloat(ffDecimal, 3) & ",\n" &
    "  \"warm_mean_ms\": " & medianMs.formatFloat(ffDecimal, 3) & ",\n" &
    "  \"qps\": " & qps.formatFloat(ffDecimal, 1) & ",\n" &
    "  \"rss_mb\": " & getResidentSetSizeMB().formatFloat(ffDecimal, 2) & "\n" &
    "}"

proc runCollisionAnalysis(): string =
  echo "\n=========================================="
  echo ">>> Collision & Orthogonality Analysis"
  echo "=========================================="
  const SampleSize = 2000
  var vecs = newSeq[Hypervector](SampleSize)
  for i in 0 ..< SampleSize:
    vecs[i] = tokenToHypervector("unique_token_id_" & $i)

  var distances = newSeq[int]()
  var minD = 8192
  var maxD = 0
  var exactCollisions = 0

  # Sample 50,000 pairs
  for i in 0 ..< 1000:
    for j in i + 1 ..< i + 51:
      let d = hammingDistance(vecs[i], vecs[j mod SampleSize])
      distances.add(d)
      if d == 0: inc exactCollisions
      if d < minD: minD = d
      if d > maxD: maxD = d

  var stat: RunningStat
  for d in distances:
    stat.push(d.float)

  echo "Sampled distance pairs: ", distances.len
  echo "Exact collisions:       ", exactCollisions
  echo "Min Hamming distance:   ", minD, " bits"
  echo "Max Hamming distance:   ", maxD, " bits"
  echo "Mean Hamming distance:  ", stat.mean.formatFloat(ffDecimal, 2), " bits (Theoretical: 4096.0)"
  echo "Standard Deviation:     ", stat.standardDeviation.formatFloat(ffDecimal, 2), " (Theoretical sqrt(N*p*q) = 45.25)"

  result = "{\n" &
    "  \"pairs_tested\": " & $distances.len & ",\n" &
    "  \"exact_collisions\": " & $exactCollisions & ",\n" &
    "  \"min_distance\": " & $minD & ",\n" &
    "  \"max_distance\": " & $maxD & ",\n" &
    "  \"mean_distance\": " & stat.mean.formatFloat(ffDecimal, 2) & ",\n" &
    "  \"std_dev\": " & stat.standardDeviation.formatFloat(ffDecimal, 2) & "\n" &
    "}"

proc runAssociativeRetrievalTest(): string =
  echo "\n=========================================="
  echo ">>> Associative Semantic Retrieval & Recall@K"
  echo "=========================================="

  # Labeled code repository of Go functions
  let corpus = @[
    # Class 0: Goroutine worker with leakage (chan without close/drain)
    (0, "func workerLeaky(ch chan int) { go func() { for v := range ch { println(v) } }() }"),
    (0, "func backgroundRoutine(in chan int) { go func() { for x := range in { println(x) } }() }"), # Variant of 0
    # Class 1: Safe HTTP Handler with Timeout
    (1, "func handleRequest(w http.ResponseWriter, r *http.Request) { ctx, cancel := context.WithTimeout(r.Context(), time.Second); defer cancel(); req.WithContext(ctx) }"),
    (1, "func serveHTTP(res http.ResponseWriter, req *http.Request) { ctx, cancel := context.WithTimeout(req.Context(), time.Second); defer cancel(); req.WithContext(ctx) }"), # Variant of 1
    # Class 2: Binary Search in Slice
    (2, "func binarySearch(arr []int, target int) int { low := 0; high := len(arr) - 1; for low <= high { mid := (low + high) / 2; return mid }; return -1 }"),
    (2, "func findIndex(list []int, val int) int { start := 0; end := len(list) - 1; for start <= end { m := (start + end) / 2; return m }; return -1 }"), # Variant of 2
    # Class 3: Mutex Lock Critical Section
    (3, "func updateCounter(mu *sync.Mutex, val *int) { mu.Lock(); defer mu.Unlock(); *val = *val + 1 }"),
    (3, "func increment(lock *sync.Mutex, count *int) { lock.Lock(); defer lock.Unlock(); *count = *count + 1 }")  # Variant of 3
  ]

  var db = newSeq[HdcRecord](corpus.len)
  for i in 0 ..< corpus.len:
    db[i] = HdcRecord(
      id: uint64(i),
      symbolId: uint32(corpus[i][0]), # class label
      flags: 0,
      vector: encodeSnippet(corpus[i][1])
    )

  # Queries: Modified / perturbed versions
  let queries = @[
    (0, "func leakedGoroutine(c chan int) { go func() { for n := range c { println(n) } }() }"),
    (1, "func routerEndpoint(w http.ResponseWriter, r *http.Request) { ctx, cancel := context.WithTimeout(r.Context(), time.Second); defer cancel() }"),
    (2, "func bsearch(elems []int, target int) int { l := 0; r := len(elems)-1; for l <= r { return l }; return -1 }"),
    (3, "func add(m *sync.Mutex, v *int) { m.Lock(); defer m.Unlock(); *v++ }")
  ]

  var recall1 = 0
  var recall3 = 0

  for q in queries:
    let qVec = encodeSnippet(q[1])
    let results = searchLinearTopK(qVec, db, 3)

    let top1Class = results[0].symbolId
    var top3Classes: seq[uint32] = @[]
    for r in results: top3Classes.add(r.symbolId)

    if top1Class == uint32(q[0]): inc recall1
    if uint32(q[0]) in top3Classes: inc recall3

    echo "Query (Class ", q[0], ") -> Top-1 distance: ", results[0].distance, " (Matched Class ", top1Class, ")"

  let r1Pct = (recall1.float / queries.len.float) * 100.0
  let r3Pct = (recall3.float / queries.len.float) * 100.0
  echo "Recall@1: ", r1Pct.formatFloat(ffDecimal, 1), "%"
  echo "Recall@3: ", r3Pct.formatFloat(ffDecimal, 1), "%"

  result = "{\n" &
    "  \"queries_tested\": " & $queries.len & ",\n" &
    "  \"recall_at_1_pct\": " & r1Pct.formatFloat(ffDecimal, 1) & ",\n" &
    "  \"recall_at_3_pct\": " & r3Pct.formatFloat(ffDecimal, 1) & "\n" &
    "}"

proc main() =
  echo "#####################################################"
  echo "### MBVM-HDC-001 BENCHMARK & EXPERIMENT SUITE     ###"
  echo "#####################################################"

  let collJson = runCollisionAnalysis()
  let assocJson = runAssociativeRetrievalTest()

  let d1Json = runScaleDataset(1_000, "D1_1k")
  let d2Json = runScaleDataset(10_000, "D2_10k")
  let d3Json = runScaleDataset(100_000, "D3_100k")
  let d4Json = runScaleDataset(1_000_000, "D4_1M")

  let finalJson = "{\n" &
    "  \"collision_analysis\": " & collJson & ",\n" &
    "  \"associative_retrieval\": " & assocJson & ",\n" &
    "  \"datasets\": [\n" &
    "    " & d1Json & ",\n" &
    "    " & d2Json & ",\n" &
    "    " & d3Json & ",\n" &
    "    " & d4Json & "\n" &
    "  ]\n" &
    "}"

  let outReport = "tests/experiments/mbvm_hdc_001/results/benchmarks/benchmark_results.json"
  writeFile(outReport, finalJson)
  echo "\nBenchmark evidence persisted to: ", outReport

main()
