import std/[unittest, streams]
import ../ivf3
import ../../../../src/csr
import ../../mbvm_hdc_001/encoder
import ../../mbvm_hdc_001/storage
import ../../mbvm_hdc_001/search

suite "MBVM-HDC-003: P0 Clustering Consistency & P2 Security Audit":

  test "P0: Post-update final assignment pass consistency":
    # Construct 300 diverse vectors
    var records = newSeq[HdcRecord](300)
    for i in 0 ..< 300:
      records[i] = HdcRecord(
        id: uint64(i),
        symbolId: uint32(i),
        flags: 0,
        vector: tokenToHypervector("cluster_consistency_test_" & $i)
      )

    let k = 8
    let index = buildIvf3Index(records, clusterCount = k, maxIters = 5)

    # AUDIT VERIFICATION:
    # Verify that every single record in postingLists[c] actually has centroid[c] as its strictly nearest centroid!
    var correctlyAssigned = 0
    var totalRecords = 0

    for c in 0 ..< k:
      for recIdx in index.postingLists[c]:
        inc totalRecords
        let vec = records[recIdx.int].vector
        let distAssigned = hammingDistance(vec, index.centroids[c])

        # Check all other centroids
        var isNearest = true
        for otherC in 0 ..< k:
          let distOther = hammingDistance(vec, index.centroids[otherC])
          if distOther < distAssigned:
            isNearest = false
            break

        if isNearest:
          inc correctlyAssigned

    check totalRecords == 300
    # In HDC-002, this was NOT 100% because centroids moved on the last iteration without a reassignment pass.
    # In HDC-003, with the P0 fix, it MUST be 100%!
    check correctlyAssigned == 300

  test "P2: Binary serialization security & corruption resistance":
    var records = newSeq[HdcRecord](50)
    for i in 0 ..< 50:
      records[i] = HdcRecord(
        id: uint64(i),
        symbolId: uint32(i),
        flags: 0,
        vector: tokenToHypervector("sec_token_" & $i)
      )

    let index = buildIvf3Index(records, clusterCount = 4, maxIters = 2)
    var ss = newStringStream()
    serializeIvf3(index, ss)
    let validBytes = ss.data

    # 1. Truncated header
    expect Ivf3CorruptError:
      discard deserializeIvf3(newStringStream("SHORT"))

    # 2. Invalid magic
    var badMagic = validBytes
    badMagic[0] = 'X'
    expect Ivf3CorruptError:
      discard deserializeIvf3(newStringStream(badMagic))

    # 3. Checksum mismatch
    var badCrc = validBytes
    badCrc[16] = 0xEE.char # Tamper with cluster count without updating CRC
    expect Ivf3CorruptError:
      discard deserializeIvf3(newStringStream(badCrc))

    # 4. Trailing unexpected bytes (tamper attempt)
    var trailing = validBytes & "MALICIOUS_EXTRA_PAYLOAD"
    expect Ivf3CorruptError:
      discard deserializeIvf3(newStringStream(trailing))

    # 5. Exact roundtrip with legitimate bytes
    ss.setPosition(0)
    let restored = deserializeIvf3(ss)
    check restored.header.clusterCount == 4
    check restored.header.totalIndexed == 50
    check restored.centroids == index.centroids
    check restored.postingLists == index.postingLists
