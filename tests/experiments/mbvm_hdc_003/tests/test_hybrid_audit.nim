import std/[unittest, tables]
import ../hybrid3
import ../ivf3
import ../../../../src/csr
import ../../mbvm_hdc_001/encoder
import ../../mbvm_hdc_001/storage

suite "MBVM-HDC-003: P3 CSR Integration & Error Handling Audit":

  test "Handling of cyclic graphs, disconnected nodes, and multi-hop depth":
    # 0 -> 1 -> 2 -> 0 (Cycle)
    # 3 (Disconnected)
    var adj = newSeq[seq[GraphEdge]](4)
    adj[0] = @[GraphEdge(target: 1, rel: RelCall)]
    adj[1] = @[GraphEdge(target: 2, rel: RelCall)]
    adj[2] = @[GraphEdge(target: 0, rel: RelCall)]
    adj[3] = @[]

    let graph = initCSRGraph(4, adj)
    let symbols = @[
      SymbolMetadata(nodeId: 0, name: "NodeA", filePath: "a.go", line: 10),
      SymbolMetadata(nodeId: 1, name: "NodeB", filePath: "b.go", line: 20),
      SymbolMetadata(nodeId: 2, name: "NodeC", filePath: "c.go", line: 30),
      SymbolMetadata(nodeId: 3, name: "NodeIsolated", filePath: "d.go", line: 40)
    ]

    let hdcRecords = @[
      HdcRecord(id: 0, symbolId: 0, flags: 0, vector: tokenToHypervector("code_node_a")),
      HdcRecord(id: 1, symbolId: 1, flags: 0, vector: tokenToHypervector("code_node_b")),
      HdcRecord(id: 2, symbolId: 2, flags: 0, vector: tokenToHypervector("code_node_c")),
      HdcRecord(id: 3, symbolId: 3, flags: 0, vector: tokenToHypervector("code_node_isolated"))
    ]

    let engine = initHybridEngine3(graph, symbols, hdcRecords, clusterCount = 2)

    # 1. Test cyclic expansion bounded by depth 1
    var scratch = initScratch(4)
    let d1 = engine.expandSymbolicAtDepth(0, depth = 1, scratch)
    check d1.len == 2 # 0 and 1

    # 2. Test cyclic expansion bounded by depth 2
    let d2 = engine.expandSymbolicAtDepth(0, depth = 2, scratch)
    check d2.len == 3 # 0, 1, 2

    # 3. Test cyclic expansion bounded by depth 10: does NOT infinite loop!
    let d10 = engine.expandSymbolicAtDepth(0, depth = 10, scratch)
    check d10.len == 3 # Traversal terminates cleanly on visited cycle

    # 4. Test disconnected node
    let dIso = engine.expandSymbolicAtDepth(3, depth = 5, scratch)
    check dIso.len == 1
    check dIso[0].name == "NodeIsolated"

    # 5. Missing / out-of-bounds symbol ID must fail explicitly with KeyError
    expect KeyError:
      discard engine.expandSymbolicAtDepth(999, depth = 1, scratch)
