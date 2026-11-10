import std/unittest
import ../hybrid
import ../ivf
import ../../../../src/csr
import ../../mbvm_hdc_001/encoder
import ../../mbvm_hdc_001/storage

suite "MBVM-HDC-002: Phase 3 CSR Integration & Expansion Verification":

  test "End-to-End Associative Retrieval -> CSR Expansion":
    # 1. Create a CSR graph with 4 nodes:
    # 0: HandleRequest -> calls 1: ValidateAuth, depends on 2: DatabaseConn
    # 1: ValidateAuth -> calls 3: TokenVerifier
    # 2: DatabaseConn
    # 3: TokenVerifier
    var adj = newSeq[seq[GraphEdge]](4)
    adj[0] = @[
      GraphEdge(target: 1, rel: RelCall),
      GraphEdge(target: 2, rel: RelDependency)
    ]
    adj[1] = @[GraphEdge(target: 3, rel: RelCall)]
    adj[2] = @[]
    adj[3] = @[]

    let graph = initCSRGraph(4, adj)

    let symbols = @[
      ResolvedSymbol(nodeId: 0, name: "HandleRequest", filePath: "server.go", line: 42),
      ResolvedSymbol(nodeId: 1, name: "ValidateAuth", filePath: "auth.go", line: 15),
      ResolvedSymbol(nodeId: 2, name: "DatabaseConn", filePath: "db.go", line: 100),
      ResolvedSymbol(nodeId: 3, name: "TokenVerifier", filePath: "jwt.go", line: 55)
    ]

    # 2. HDC records encoding the snippets
    let code0 = "func HandleRequest(w http.ResponseWriter, r *http.Request) { ValidateAuth(r); db := DatabaseConn() }"
    let code1 = "func ValidateAuth(r *http.Request) { TokenVerifier(r.Header) }"
    let code2 = "type DatabaseConn struct { conn net.Conn }"
    let code3 = "func TokenVerifier(header string) bool { return true }"

    let hdcRecords = @[
      HdcRecord(id: 0, symbolId: 0, flags: 0, vector: encodeSnippet(code0)),
      HdcRecord(id: 1, symbolId: 1, flags: 0, vector: encodeSnippet(code1)),
      HdcRecord(id: 2, symbolId: 2, flags: 0, vector: encodeSnippet(code2)),
      HdcRecord(id: 3, symbolId: 3, flags: 0, vector: encodeSnippet(code3))
    ]

    let engine = initHybridEngine(graph, symbols, hdcRecords, clusterCount = 2)

    # 3. Query with a perturbed version of HandleRequest
    let querySnippet = "func serveHTTP(res http.ResponseWriter, req *http.Request) { ValidateAuth(req); db := DatabaseConn() }"
    let contexts = engine.queryAssociativeSymbolic(querySnippet, topK = 1, nprobe = 2)

    check contexts.len == 1
    let seed = contexts[0].seedSymbol
    check seed.name == "HandleRequest"
    check seed.filePath == "server.go"

    # Check that CSR relations were expanded without fabrication
    check contexts[0].callees.len == 1
    check contexts[0].callees[0].name == "ValidateAuth"
    check contexts[0].directDependencies.len == 1
    check contexts[0].directDependencies[0].name == "DatabaseConn"
