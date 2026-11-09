## MBVM-DB CSR (Compressed Sparse Row) Core Engine in Nim
## Designed for 0 B/op and nanosecond BFS traversals under --mm:orc.

type
  RelationType* = enum
    RelDependency = 1
    RelGeneralizes = 2
    RelComposition = 3
    RelCall = 4

  GraphEdge* = object
    target*: int32
    rel*: RelationType

  CSRGraph* = object
    rowOffsets*: seq[int32]
    colIndices*: seq[int32]
    relTypes*: seq[RelationType]
    nodeCount*: int
    edgeCount*: int

  ScratchBuffer* = object
    visited*: seq[bool]
    queue*: seq[int32]

proc initScratch*(nodeCount: int): ScratchBuffer =
  result.visited = newSeq[bool](nodeCount)
  result.queue = newSeqOfCap[int32](nodeCount)

proc reset*(s: var ScratchBuffer) {.inline.} =
  for i in 0 ..< s.visited.len:
    s.visited[i] = false
  s.queue.setLen(0)

proc initCSRGraph*(nodeCount: int, adj: openArray[seq[GraphEdge]]): CSRGraph =
  result.nodeCount = nodeCount
  result.rowOffsets = newSeq[int32](nodeCount + 1)
  
  var totalEdges = 0
  for edges in adj:
    totalEdges += edges.len
  result.edgeCount = totalEdges
  result.colIndices = newSeq[int32](totalEdges)
  result.relTypes = newSeq[RelationType](totalEdges)

  var edgeIdx = 0
  for i in 0 ..< nodeCount:
    result.rowOffsets[i] = int32(edgeIdx)
    for edge in adj[i]:
      result.colIndices[edgeIdx] = edge.target
      result.relTypes[edgeIdx] = edge.rel
      inc edgeIdx
  result.rowOffsets[nodeCount] = int32(edgeIdx)

proc neighbors*(g: CSRGraph, node: int32, buffer: var seq[int32]) {.inline.} =
  buffer.setLen(0)
  if node < 0 or node >= g.nodeCount.int32:
    return
  let startIdx = g.rowOffsets[node]
  let endIdx = g.rowOffsets[node + 1]
  for i in startIdx ..< endIdx:
    buffer.add(g.colIndices[i])

proc traverseBFS*(g: CSRGraph, startNode: int32, scratch: var ScratchBuffer, maxDepth: int = 3): seq[int32] =
  scratch.reset()
  if startNode < 0 or startNode >= g.nodeCount.int32:
    return @[]

  scratch.visited[startNode] = true
  scratch.queue.add(startNode)

  var head = 0
  var currentDepth = 0

  while head < scratch.queue.len and currentDepth < maxDepth:
    let levelSize = scratch.queue.len - head
    for _ in 0 ..< levelSize:
      let curr = scratch.queue[head]
      inc head
      
      let startIdx = g.rowOffsets[curr]
      let endIdx = g.rowOffsets[curr + 1]
      for e in startIdx ..< endIdx:
        let neighbor = g.colIndices[e]
        if not scratch.visited[neighbor]:
          scratch.visited[neighbor] = true
          scratch.queue.add(neighbor)
    inc currentDepth

  result = scratch.queue
