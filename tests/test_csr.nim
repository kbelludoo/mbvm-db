import std/unittest
import ../src/csr

suite "CSR Graph Tests":
  test "Deterministic graph construction":
    var adj = newSeq[seq[GraphEdge]](4)
    adj[0] = @[GraphEdge(target: 1, rel: RelDependency), GraphEdge(target: 2, rel: RelCall)]
    adj[1] = @[GraphEdge(target: 2, rel: RelComposition)]
    adj[2] = @[GraphEdge(target: 3, rel: RelGeneralizes)]
    adj[3] = @[]

    let g = initCSRGraph(4, adj)
    check g.nodeCount == 4
    check g.edgeCount == 4
    check (g.rowOffsets[1] - g.rowOffsets[0]) == 2
    check (g.rowOffsets[2] - g.rowOffsets[1]) == 1
    check (g.rowOffsets[3] - g.rowOffsets[2]) == 1
    check (g.rowOffsets[4] - g.rowOffsets[3]) == 0

  test "Neighbors retrieval with scratch buffer":
    var adj = newSeq[seq[GraphEdge]](3)
    adj[0] = @[GraphEdge(target: 1, rel: RelDependency), GraphEdge(target: 2, rel: RelCall)]
    adj[1] = @[]
    adj[2] = @[]
    let g = initCSRGraph(3, adj)

    var neighborsBuf: seq[int32]
    g.neighbors(0, neighborsBuf)
    check neighborsBuf.len == 2
    check neighborsBuf[0] == 1
    check neighborsBuf[1] == 2

  test "Zero-alloc BFS traversal":
    var adj = newSeq[seq[GraphEdge]](5)
    adj[0] = @[GraphEdge(target: 1, rel: RelDependency), GraphEdge(target: 2, rel: RelDependency)]
    adj[1] = @[GraphEdge(target: 3, rel: RelDependency)]
    adj[2] = @[GraphEdge(target: 4, rel: RelDependency)]
    adj[3] = @[]
    adj[4] = @[]
    let g = initCSRGraph(5, adj)

    var scratch = initScratch(5)
    let reached = g.traverseBFS(0, scratch, 2)

    check reached.len == 5 # 0, 1, 2, 3, 4
    check reached[0] == 0
    check 1 in reached
    check 2 in reached
    check 3 in reached
    check 4 in reached
