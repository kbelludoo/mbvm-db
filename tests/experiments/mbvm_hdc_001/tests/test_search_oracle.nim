import std/[unittest, os, streams]
import ../search
import ../storage
import ../encoder
import ./test_oracle

suite "MBVM-HDC-001: P3 Search & Independent Oracle Matching":

  test "Exact match of Top-K results against independent linear oracle":
    # Construct a dataset of 500 diverse vectors
    var records = newSeq[HdcRecord](500)
    var oracleDataset = newSeq[array[128, uint64]](500)

    for i in 0 ..< 500:
      let v = tokenToHypervector("SymbolIdentifier_" & $i)
      records[i] = HdcRecord(id: uint64(i), symbolId: uint32(i * 10), flags: 0, vector: v)
      for w in 0 ..< 128:
        oracleDataset[i][w] = v[w]

    # Query with a known perturbed vector
    var query = tokenToHypervector("SymbolIdentifier_42")
    # Flip 5 bits in word 0
    query[0] = query[0] xor 0b11111'u64

    var oracleQuery: array[128, uint64]
    for w in 0 ..< 128:
      oracleQuery[w] = query[w]

    for k in [1, 5, 10, 50]:
      let prodTopK = searchLinearTopK(query, records, k)
      let oracleTopKRes = oracleTopK(oracleQuery, oracleDataset, k)

      check prodTopK.len == oracleTopKRes.len
      for idx in 0 ..< prodTopK.len:
        check prodTopK[idx].id == oracleTopKRes[idx].id
        check prodTopK[idx].distance == oracleTopKRes[idx].distance

  test "MMap Search Engine returns identical Top-K":
    let tmpPath = getTempDir() / "test_mmap_search.hdc"
    var container: HdcContainer
    container.records = newSeq[HdcRecord](200)
    for i in 0 ..< 200:
      container.records[i] = HdcRecord(
        id: uint64(i + 1000),
        symbolId: uint32(i),
        flags: 0,
        vector: tokenToHypervector("MMapToken_" & $i)
      )

    var fs = newFileStream(tmpPath, fmWrite)
    serializeContainer(container, fs)
    fs.close()

    var engine = openMMapEngine(tmpPath)
    let query = tokenToHypervector("MMapToken_77")
    let results = engine.searchMMapTopK(query, 5)

    check results.len == 5
    # Exact match must be candidate 77 with distance 0
    check results[0].id == 1077'u64
    check results[0].distance == 0

    engine.close()
    removeFile(tmpPath)
