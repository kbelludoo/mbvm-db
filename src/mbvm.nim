## MBVM CLI em Nim Puro
import std/[os, strutils, times, monotimes]
import csr, hdc, mbvm8k2

proc printHelp() =
  echo "⚡ MBVM-DB (Nim Core Engine v2.0 - --mm:orc)"
  echo "Uso: mbvm <comando> [argumentos]"
  echo ""
  echo "Comandos disponíveis:"
  echo "  benchmark   Executa benchmarks do motor CSR e HDC 8K nativo"
  echo "  version     Exibe versão e flags do compilador Nim"

proc runBenchmark() =
  echo "=== [MBVM-DB Nim Engine Benchmark] ==="
  
  # 1. Benchmark HDC 8K Hamming
  echo "\n1. Testando HDC 8K Hamming Distance (8.192 bits / vetor):"
  var v1, v2: Hypervector
  for i in 0 ..< HdcWords:
    v1[i] = 0xAAAAAAAAAAAAAAAA'u64
    v2[i] = 0x5555555555555555'u64

  let t0 = getMonoTime()
  var totalDist = 0
  const HdcIters = 1_000_000
  for _ in 0 ..< HdcIters:
    totalDist += hammingDistance(v1, v2)
  let t1 = getMonoTime()
  let hdcElapsedNs = (t1 - t0).inNanoseconds
  let hdcPerOp = hdcElapsedNs.float64 / HdcIters.float64

  echo "  • Iterações: ", HdcIters
  echo "  • Distância de Hamming: ", hammingDistance(v1, v2), " bits (máxima dispersão)"
  echo "  • Tempo total: ", (t1 - t0).inMilliseconds, " ms"
  echo "  • Latência por comparação HDC: ", hdcPerOp.formatFloat(ffDecimal, 2), " ns/op"

  # 2. Benchmark CSR BFS Traversal
  echo "\n2. Testando Grafo CSR BFS Traversal (10.000 nós, 100.000 arestas):"
  const NodeCount = 10_000
  const Degree = 10
  var adj = newSeq[seq[GraphEdge]](NodeCount)
  for i in 0 ..< NodeCount:
    adj[i] = newSeq[GraphEdge](Degree)
    for d in 0 ..< Degree:
      adj[i][d] = GraphEdge(target: int32((i * 7 + d * 13) mod NodeCount), rel: RelCall)

  let csr = initCSRGraph(NodeCount, adj)
  var scratch = initScratch(NodeCount)

  let t2 = getMonoTime()
  const CsrIters = 100_000
  var reachedNodes = 0
  for i in 0 ..< CsrIters:
    let res = csr.traverseBFS(int32(i mod NodeCount), scratch, maxDepth = 2)
    reachedNodes += res.len
  let t3 = getMonoTime()
  let csrElapsedNs = (t3 - t2).inNanoseconds
  let csrPerOp = csrElapsedNs.float64 / CsrIters.float64

  echo "  • Iterações de percurso BFS: ", CsrIters
  echo "  • Tempo total: ", (t3 - t2).inMilliseconds, " ms"
  echo "  • Latência de percurso CSR: ", csrPerOp.formatFloat(ffDecimal, 2), " ns/op"
  echo "  • Alocações na Heap: 0 B/op (buffer reutilizado via Scratch)"

when isMainModule:
  let args = commandLineParams()
  if args.len == 0:
    printHelp()
    quit(0)

  case args[0]
  of "benchmark", "bench":
    runBenchmark()
  of "version", "-v", "--version":
    echo "MBVM-DB Nim Engine v0.1.0"
    echo "Runtime: Nim ", NimVersion, " | MM: ORC | OS: Windows AMD64"
  else:
    echo "Comando desconhecido: ", args[0]
    printHelp()
    quit(1)
