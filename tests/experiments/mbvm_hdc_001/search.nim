## MBVM-HDC-001: Linear Scan and Top-K Search with Deterministic Tie-Breaking
## Uses CPU popcount with memfiles (mmap) for zero-copy memory access.

import std/[bitops, memfiles, algorithm]
import ./encoder
import ./storage

type
  SearchResult* = object
    id*: uint64
    symbolId*: uint32
    distance*: int

proc hammingDistance*(a, b: Hypervector): int {.inline.} =
  var d = 0
  for i in 0 ..< HdcWords:
    d += popcount(a[i] xor b[i])
  return d

proc searchLinearTopK*(query: Hypervector, records: openArray[HdcRecord], k: int): seq[SearchResult] =
  if records.len == 0 or k <= 0:
    return @[]

  result = newSeq[SearchResult](records.len)
  for i in 0 ..< records.len:
    result[i] = SearchResult(
      id: records[i].id,
      symbolId: records[i].symbolId,
      distance: hammingDistance(query, records[i].vector)
    )

  # Deterministic tie-breaking: distance ASC, then id ASC
  result.sort proc(x, y: SearchResult): int =
    if x.distance != y.distance:
      return cmp(x.distance, y.distance)
    return cmp(x.id, y.id)

  let count = min(k, result.len)
  result.setLen(count)

# --- MMap-based Direct Search Engine ---

type
  MMapHdcEngine* = object
    mf*: MemFile
    header*: ptr HdcStoreHeader
    recordCount*: int
    recordsPtr*: ptr UncheckedArray[byte]

proc openMMapEngine*(filePath: string): MMapHdcEngine =
  result.mf = memfiles.open(filePath, mode = fmRead)
  if result.mf.size < HdcStoreHeaderSize:
    result.mf.close()
    raise newException(HdcCorruptError, "File smaller than HDC header")

  result.header = cast[ptr HdcStoreHeader](result.mf.mem)
  if result.header.magic != HdcStoreMagic:
    result.mf.close()
    raise newException(HdcCorruptError, "Invalid magic in mmap file")

  result.recordCount = result.header.recordCount.int
  let expectedSize = HdcStoreHeaderSize + (result.recordCount * HdcRecordSize)
  if result.mf.size < expectedSize:
    result.mf.close()
    raise newException(HdcCorruptError, "File truncated for specified record count")

  result.recordsPtr = cast[ptr UncheckedArray[byte]](cast[uint](result.mf.mem) + HdcStoreHeaderSize.uint)

proc close*(e: var MMapHdcEngine) =
  if e.mf.mem != nil:
    e.mf.close()
    e.mf.mem = nil

proc searchMMapTopK*(e: MMapHdcEngine, query: Hypervector, k: int): seq[SearchResult] =
  if e.recordCount == 0 or k <= 0:
    return @[]

  result = newSeq[SearchResult](e.recordCount)
  let recSize = HdcRecordSize

  for i in 0 ..< e.recordCount:
    let offset = i * recSize
    let idPtr = cast[ptr uint64](addr e.recordsPtr[offset])
    let symPtr = cast[ptr uint32](addr e.recordsPtr[offset + 8])
    let vecPtr = cast[ptr Hypervector](addr e.recordsPtr[offset + 16])

    let dist = hammingDistance(query, vecPtr[])
    result[i] = SearchResult(id: idPtr[], symbolId: symPtr[], distance: dist)

  result.sort proc(x, y: SearchResult): int =
    if x.distance != y.distance:
      return cmp(x.distance, y.distance)
    return cmp(x.id, y.id)

  let count = min(k, result.len)
  result.setLen(count)
