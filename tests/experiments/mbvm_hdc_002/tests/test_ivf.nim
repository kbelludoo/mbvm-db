import std/[unittest, streams]
import ../ivf
import ../../mbvm_hdc_001/encoder
import ../../mbvm_hdc_001/storage
import ../../mbvm_hdc_001/search

suite "MBVM-HDC-002: Phase 1 & 2 IVF Index Correctness & Audit":

  test "Deterministic centroid generation":
    let c1 = initDeterministicCentroids(16, DefaultSeed)
    let c2 = initDeterministicCentroids(16, DefaultSeed)
    check c1.len == 16
    check c2.len == 16
    for i in 0 ..< 16:
      check c1[i] == c2[i]

  test "IVF index build & serialization roundtrip":
    var records = newSeq[HdcRecord](200)
    for i in 0 ..< 200:
      records[i] = HdcRecord(
        id: uint64(i),
        symbolId: uint32(i mod 20),
        flags: 0,
        vector: tokenToHypervector("record_symbol_" & $i)
      )

    let index = buildIvfIndex(records, clusterCount = 8, maxIters = 3)
    check index.centroids.len == 8
    check index.postingLists.len == 8

    var totalMembers = 0
    for p in index.postingLists:
      totalMembers += p.len
    check totalMembers == 200

    # Serialization roundtrip
    var ss = newStringStream()
    serializeIvf(index, ss)
    ss.setPosition(0)

    let loaded = deserializeIvf(ss)
    check loaded.header.clusterCount == 8
    check loaded.header.totalIndexed == 200
    check loaded.centroids == index.centroids
    check loaded.postingLists == index.postingLists

  test "IVF Recall vs Exact Linear Oracle":
    # Build a database of 1000 items
    var records = newSeq[HdcRecord](1000)
    for i in 0 ..< 1000:
      records[i] = HdcRecord(
        id: uint64(i),
        symbolId: uint32(i),
        flags: 0,
        vector: tokenToHypervector("ivf_eval_tok_" & $i)
      )

    let index = buildIvfIndex(records, clusterCount = 16, maxIters = 4)

    # Test 20 queries with nprobe = 4 (probing 25% of clusters)
    var top1Hits = 0
    var top5Hits = 0

    for qIdx in 0 ..< 20:
      let targetId = uint64(qIdx * 45)
      var query = records[targetId.int].vector
      # Add small perturbation (flip 12 bits)
      query[0] = query[0] xor 0xFFF'u64

      let exactResults = searchLinearTopK(query, records, 5)
      let ivfResults = searchIvfTopK(index, records, query, 5, nprobe = 4)

      # Check if exact Top-1 was found by IVF
      if ivfResults.len > 0 and ivfResults[0].id == exactResults[0].id:
        inc top1Hits

      var ivfIds: seq[uint64] = @[]
      for r in ivfResults: ivfIds.add(r.id)

      if exactResults[0].id in ivfIds:
        inc top5Hits

    let r1 = top1Hits.float / 20.0
    let r5 = top5Hits.float / 20.0
    echo "IVF Correctness Test (1000 vectors, 16 clusters, nprobe=4):"
    echo "  Top-1 Match Rate: ", r1 * 100.0, "%"
    echo "  Top-5 Recall of Oracle Best: ", r5 * 100.0, "%"

    # With nprobe = 4 (25% probe), top5 recall must be >= 85%
    check r5 >= 0.85
