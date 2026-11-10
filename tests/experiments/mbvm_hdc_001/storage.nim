## MBVM-HDC-001 Binary Storage (MBVM8K2 HDC Section Extension)
##
## File Layout:
## - 64-byte Container Header:
##   magic: array[8, char] = ['M','B','V','M','H','D','C','1']
##   versionMajor: uint16 = 1
##   versionMinor: uint16 = 0
##   headerSize: uint32 = 64
##   vectorDim: uint32 = 8192
##   recordCount: uint64
##   recordSize: uint32 = 1040 (16-byte metadata + 1024-byte vector)
##   encoderVersion: uint32 = 1
##   hashAlgoId: uint32 = 3 # WyHash v3 64-bit
##   seed: uint64 = 0x243F_6A88_85A3_08D3
##   crc32c: uint32
##   reserved: array[12, uint8]
##
## - Array of Records [recordCount]:
##   id: uint64
##   symbolId: uint32
##   flags: uint32
##   vector: array[128, uint64] (1024 bytes)

import std/[streams, endians]
import ./encoder

const
  HdcStoreMagic* = ['M', 'B', 'V', 'M', 'H', 'D', 'C', '1']
  HdcStoreHeaderSize* = 64
  HdcRecordSize* = 1040 # 8 + 4 + 4 + 1024

type
  HdcStoreHeader* {.packed.} = object
    magic*: array[8, char]
    versionMajor*: uint16
    versionMinor*: uint16
    headerSize*: uint32
    vectorDim*: uint32
    recordCount*: uint64
    recordSize*: uint32
    encoderVersion*: uint32
    hashAlgoId*: uint32
    seed*: uint64
    crc32c*: uint32
    reserved*: array[12, uint8]

  HdcRecord* = object
    id*: uint64
    symbolId*: uint32
    flags*: uint32
    vector*: Hypervector

  HdcContainer* = object
    header*: HdcStoreHeader
    records*: seq[HdcRecord]

# --- Deterministic CRC32C (Castagnoli 0x82F63B78) ---

proc crc32cUpdate*(crc: uint32, data: openArray[byte]): uint32 =
  var c = crc xor 0xFFFF_FFFF'u32
  for b in data:
    c = c xor b.uint32
    for _ in 0 ..< 8:
      if (c and 1'u32) != 0:
        c = (c shr 1) xor 0x82F6_3B78'u32
      else:
        c = c shr 1
  return c xor 0xFFFF_FFFF'u32

proc calculateHeaderCRC*(h: HdcStoreHeader): uint32 =
  var copyH = h
  copyH.crc32c = 0
  let p = cast[ptr UncheckedArray[byte]](addr copyH)
  var b = newSeq[byte](HdcStoreHeaderSize)
  for i in 0 ..< HdcStoreHeaderSize:
    b[i] = p[i]
  return crc32cUpdate(0, b)

# --- Serialization ---

proc serializeContainer*(c: HdcContainer, s: Stream) =
  var h = c.header
  h.magic = HdcStoreMagic
  h.versionMajor = 1
  h.versionMinor = 0
  h.headerSize = HdcStoreHeaderSize.uint32
  h.vectorDim = HdcDimension.uint32
  h.recordCount = c.records.len.uint64
  h.recordSize = HdcRecordSize.uint32
  h.encoderVersion = 1
  h.hashAlgoId = 3 # WyHash v3
  h.seed = DefaultSeed
  h.crc32c = calculateHeaderCRC(h)

  # Write Header
  s.writeData(addr h, sizeof(h))

  # Write Records deterministically in Little-Endian
  for r in c.records:
    var idLe = r.id
    var symLe = r.symbolId
    var flagsLe = r.flags
    s.writeData(addr idLe, sizeof(idLe))
    s.writeData(addr symLe, sizeof(symLe))
    s.writeData(addr flagsLe, sizeof(flagsLe))
    s.writeData(unsafeAddr r.vector[0], 1024)

# --- Deserialization with Rigorous Validation ---

type
  HdcCorruptError* = object of CatchableError

proc deserializeContainer*(s: Stream): HdcContainer =
  if s.isNil:
    raise newException(HdcCorruptError, "Stream is nil")

  var h: HdcStoreHeader
  let readBytes = s.readData(addr h, sizeof(h))
  if readBytes < sizeof(h):
    raise newException(HdcCorruptError, "Truncated header: expected " & $sizeof(h) & " bytes, got " & $readBytes)

  if h.magic != HdcStoreMagic:
    raise newException(HdcCorruptError, "Invalid magic bytes in HDC store")

  if h.versionMajor != 1:
    raise newException(HdcCorruptError, "Unsupported major version: " & $h.versionMajor)

  if h.vectorDim != HdcDimension.uint32:
    raise newException(HdcCorruptError, "Invalid vector dimension: expected " & $HdcDimension & ", got " & $h.vectorDim)

  if h.recordSize != HdcRecordSize.uint32:
    raise newException(HdcCorruptError, "Invalid record size: expected " & $HdcRecordSize & ", got " & $h.recordSize)

  let expectedCrc = calculateHeaderCRC(h)
  if h.crc32c != expectedCrc:
    raise newException(HdcCorruptError, "CRC32C header mismatch: expected " & $expectedCrc & ", got " & $h.crc32c)

  result.header = h
  result.records = newSeq[HdcRecord](h.recordCount)

  for i in 0 ..< h.recordCount.int:
    var rec: HdcRecord
    let rId = s.readData(addr rec.id, sizeof(rec.id))
    let rSym = s.readData(addr rec.symbolId, sizeof(rec.symbolId))
    let rFlags = s.readData(addr rec.flags, sizeof(rec.flags))
    let rVec = s.readData(addr rec.vector[0], 1024)

    let totalRecRead = rId + rSym + rFlags + rVec
    if totalRecRead < HdcRecordSize:
      raise newException(HdcCorruptError, "Truncated payload at record index " & $i)

    result.records[i] = rec
