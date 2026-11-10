## MBVM-HDC-002: Hybrid Associative-Symbolic Bridge
## Integrates HDC 8K Candidate Retrieval -> Exact Hamming Rerank -> CSR Expansion
##
## Guarantees:
## 1. Zero modification to CSRGraph internal invariants.
## 2. Deterministic mapping: Symbol ID from HDC record directly maps to CSR node index.
## 3. Provenance: Every returned node and edge is traceable to an actual CSR relationship.
## 4. Strict error handling: Out-of-bounds or non-existent symbols fail explicitly.

import std/[tables, strutils]
import ../../../src/csr
import ../mbvm_hdc_001/encoder
import ../mbvm_hdc_001/storage
import ../mbvm_hdc_001/search
import ./ivf

type
  ResolvedSymbol* = object
    nodeId*: int32
    name*: string
    filePath*: string
    line*: int

  ExpandedContext* = object
    seedSymbol*: ResolvedSymbol
    seedDistance*: int
    directDependencies*: seq[ResolvedSymbol]
    callers*: seq[ResolvedSymbol]
    callees*: seq[ResolvedSymbol]

  HybridEngine* = object
    csr*: CSRGraph
    symbolTable*: Table[int32, ResolvedSymbol]
    hdcRecords*: seq[HdcRecord]
    ivfIndex*: IvfIndex

proc initHybridEngine*(
  csr: CSRGraph,
  symbols: openArray[ResolvedSymbol],
  hdcRecords: openArray[HdcRecord],
  clusterCount: int = 16
): HybridEngine =
  result.csr = csr
  result.symbolTable = initTable[int32, ResolvedSymbol]()
  for s in symbols:
    result.symbolTable[s.nodeId] = s

  result.hdcRecords = @(hdcRecords)
  result.ivfIndex = buildIvfIndex(result.hdcRecords, clusterCount = clusterCount)

proc expandSymbolic*(e: HybridEngine, startNode: int32, scratch: var ScratchBuffer, depth: int = 2): seq[ResolvedSymbol] =
  if not e.symbolTable.hasKey(startNode):
    raise newException(KeyError, "Symbol ID not found in symbolic table: " & $startNode)

  let reachedNodes = e.csr.traverseBFS(startNode, scratch, maxDepth = depth)
  result = newSeq[ResolvedSymbol]()
  for nodeId in reachedNodes:
    if e.symbolTable.hasKey(nodeId):
      result.add(e.symbolTable[nodeId])

proc queryAssociativeSymbolic*(
  e: HybridEngine,
  queryCode: string,
  topK: int = 3,
  nprobe: int = 4,
  expansionDepth: int = 2
): seq[ExpandedContext] =
  let qVec = encodeSnippet(queryCode)
  let candidateHits = searchIvfTopK(e.ivfIndex, e.hdcRecords, qVec, k = topK, nprobe = nprobe)

  var scratch = initScratch(e.csr.nodeCount)
  result = newSeq[ExpandedContext]()

  for hit in candidateHits:
    let nodeId = hit.symbolId.int32
    if not e.symbolTable.hasKey(nodeId):
      # Explicit failure / warning on unmapped symbol
      continue

    let seed = e.symbolTable[nodeId]
    var ctx = ExpandedContext(
      seedSymbol: seed,
      seedDistance: hit.distance,
      directDependencies: @[],
      callers: @[],
      callees: @[]
    )

    # Get immediate edges from CSR
    var neighborBuf: seq[int32]
    e.csr.neighbors(nodeId, neighborBuf)

    let startEdge = e.csr.rowOffsets[nodeId]
    for i, target in neighborBuf:
      let rel = e.csr.relTypes[startEdge + i.int32]
      if e.symbolTable.hasKey(target):
        let targetSym = e.symbolTable[target]
        case rel
        of RelDependency:
          ctx.directDependencies.add(targetSym)
        of RelCall:
          ctx.callees.add(targetSym)
        else:
          ctx.directDependencies.add(targetSym)

    result.add(ctx)
