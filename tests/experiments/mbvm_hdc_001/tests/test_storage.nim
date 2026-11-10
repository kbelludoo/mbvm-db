import std/[unittest, streams]
import ../storage
import ../encoder
import ./test_oracle

suite "MBVM-HDC-001: P1/P2 Binary Storage & Corruption Rejection":

  test "Exact bitwise roundtrip serialization":
    var orig: HdcContainer
    orig.records = @[
      HdcRecord(id: 101, symbolId: 1, flags: 0, vector: tokenToHypervector("AlphaSymbol")),
      HdcRecord(id: 102, symbolId: 2, flags: 1, vector: tokenToHypervector("BetaSymbol")),
      HdcRecord(id: 103, symbolId: 3, flags: 2, vector: tokenToHypervector("GammaSymbol"))
    ]

    var ss = newStringStream()
    serializeContainer(orig, ss)
    ss.setPosition(0)

    let loaded = deserializeContainer(ss)

    check loaded.header.recordCount == 3
    check loaded.header.vectorDim == 8192
    check loaded.header.recordSize == 1040
    check loaded.records.len == 3

    for i in 0 ..< 3:
      check loaded.records[i].id == orig.records[i].id
      check loaded.records[i].symbolId == orig.records[i].symbolId
      check loaded.records[i].flags == orig.records[i].flags
      check loaded.records[i].vector == orig.records[i].vector

  test "Rejection of truncated header":
    var raw = "MBVMHDC1_TRUNCATED"
    var ss = newStringStream(raw)
    expect HdcCorruptError:
      discard deserializeContainer(ss)

  test "Rejection of invalid magic":
    var orig: HdcContainer
    orig.records = @[HdcRecord(id: 1, symbolId: 1, flags: 0, vector: tokenToHypervector("Test"))]
    var ss = newStringStream()
    serializeContainer(orig, ss)
    var data = ss.data
    # Corrupt magic byte 0
    data[0] = 'X'
    var corrupted = newStringStream(data)
    expect HdcCorruptError:
      discard deserializeContainer(corrupted)

  test "Rejection of header CRC32C corruption":
    var orig: HdcContainer
    orig.records = @[HdcRecord(id: 1, symbolId: 1, flags: 0, vector: tokenToHypervector("Test"))]
    var ss = newStringStream()
    serializeContainer(orig, ss)
    var data = ss.data
    # Corrupt vector count field inside header without updating CRC
    data[16] = 0xFF.char
    var corrupted = newStringStream(data)
    expect HdcCorruptError:
      discard deserializeContainer(corrupted)

  test "Rejection of truncated record payload":
    var orig: HdcContainer
    orig.records = @[
      HdcRecord(id: 1, symbolId: 1, flags: 0, vector: tokenToHypervector("Test1")),
      HdcRecord(id: 2, symbolId: 2, flags: 0, vector: tokenToHypervector("Test2"))
    ]
    var ss = newStringStream()
    serializeContainer(orig, ss)
    # Truncate by cutting last 200 bytes
    var truncatedData = ss.data[0 ..< (ss.data.len - 200)]
    var corrupted = newStringStream(truncatedData)
    expect HdcCorruptError:
      discard deserializeContainer(corrupted)
