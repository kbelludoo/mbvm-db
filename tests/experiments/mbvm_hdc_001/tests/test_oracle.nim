## Independent Oracle implementation for HDC 8K verification.
## MUST NOT call production procedures in encoder.nim or storage.nim.
## Implements isolated Bit-level and Token calculations from pure specifications.

import std/[strutils]

const
  OracleWords* = 128
  OracleBits* = 8192
  OracleDefaultSeed* = 0x243F_6A88_85A3_08D3'u64

# --- Independent WyHash v3 64-bit ---

proc oracleWymix(a, b: uint64): uint64 =
  let
    a0 = a and 0xFFFF_FFFF'u64
    a1 = a shr 32
    b0 = b and 0xFFFF_FFFF'u64
    b1 = b shr 32
    p00 = a0 * b0
    p01 = a0 * b1
    p10 = a1 * b0
    p11 = a1 * b1
    middle = p01 + (p00 shr 32)
    mLo = middle and 0xFFFF_FFFF'u64
    mHi = middle shr 32
    p10_shifted = p10 + mLo
    finalHi = p11 + mHi + (p10_shifted shr 32)
    finalLo = (p10_shifted shl 32) or (p00 and 0xFFFF_FFFF'u64)
  return finalLo xor finalHi

proc oracleWyhash(data: seq[byte], seed: uint64): uint64 =
  let len = data.len
  if len == 0:
    return seed
  var
    s = seed xor 0xA076_1D64_78BD_642F'u64
    i = len
    offset = 0

  proc r8(off: int): uint64 =
    var v = 0'u64
    for b in 0 ..< 8:
      v = v or (data[off + b].uint64 shl (b * 8))
    return v

  proc r4(off: int): uint64 =
    var v = 0'u64
    for b in 0 ..< 4:
      v = v or (data[off + b].uint64 shl (b * 8))
    return v

  proc r3(off: int, k: int): uint64 =
    return (data[off].uint64 shl 16) or (data[off + (k shr 1)].uint64 shl 8) or data[off + k - 1].uint64

  if i <= 16:
    if i >= 4:
      let a = (r4(0) shl 32) or r4((i shr 3) shl 2)
      let b = (r4(i - 4) shl 32) or r4(i - 4 - ((i shr 3) shl 2))
      return oracleWymix(a xor 0xA076_1D64_78BD_642F'u64, b xor s)
    elif i > 0:
      return oracleWymix(r3(0, i) xor 0xA076_1D64_78BD_642F'u64, s)
    else:
      return oracleWymix(0xA076_1D64_78BD_642F'u64, s)

  while i > 16:
    s = oracleWymix(r8(offset) xor 0xA076_1D64_78BD_642F'u64, r8(offset + 8) xor s)
    offset += 16
    i -= 16

  let a = r8(len - 16)
  let b = r8(len - 8)
  return oracleWymix(a xor 0xA076_1D64_78BD_642F'u64, b xor s)

proc oracleHashString*(s: string, seed: uint64 = OracleDefaultSeed): uint64 =
  var b = newSeq[byte](s.len)
  for i in 0 ..< s.len:
    b[i] = byte(s[i])
  return oracleWyhash(b, seed)

# --- Independent SplitMix64 ---

proc oracleSplitmix64(state: var uint64): uint64 =
  state += 0x9E37_79B9_7F4A_7C15'u64
  var z = state
  z = (z xor (z shr 30)) * 0xBF58_476D_1CE4_E5B9'u64
  z = (z xor (z shr 27)) * 0x94D0_49BB_1331_11EB'u64
  return z xor (z shr 31)

proc oracleGenerateVector*(token: string, seed: uint64 = OracleDefaultSeed): array[128, uint64] =
  var st = oracleHashString(token, seed)
  for i in 0 ..< 128:
    result[i] = oracleSplitmix64(st)

# --- Independent Hamming Distance ---

proc oracleHammingDistance*(a, b: array[128, uint64]): int =
  var cnt = 0
  for i in 0 ..< 128:
    var diff = a[i] xor b[i]
    while diff != 0:
      diff = diff and (diff - 1'u64)
      inc cnt
  return cnt

# --- Independent Permutation (Bit-by-Bit ground truth) ---

proc oraclePermuteBitByBit*(v: array[128, uint64], shift: int): array[128, uint64] =
  var s = shift mod 8192
  if s < 0: s += 8192
  var bits: array[8192, bool]
  for w in 0 ..< 128:
    for b in 0 ..< 64:
      bits[w * 64 + b] = ((v[w] shr b) and 1'u64) == 1'u64

  var permBits: array[8192, bool]
  for i in 0 ..< 8192:
    let target = (i + s) mod 8192
    permBits[target] = bits[i]

  for w in 0 ..< 128:
    var word = 0'u64
    for b in 0 ..< 64:
      if permBits[w * 64 + b]:
        word = word or (1'u64 shl b)
    result[w] = word

# --- Independent Top-K Linear Oracle ---

type
  OracleCandidate* = object
    id*: uint64
    distance*: int

proc oracleTopK*(query: array[128, uint64], dataset: openArray[array[128, uint64]], k: int): seq[OracleCandidate] =
  var candidates = newSeq[OracleCandidate](dataset.len)
  for i in 0 ..< dataset.len:
    candidates[i] = OracleCandidate(id: uint64(i), distance: oracleHammingDistance(query, dataset[i]))

  # Deterministic bubble sort / selection sort by (distance ASC, id ASC)
  for i in 0 ..< candidates.len:
    for j in i + 1 ..< candidates.len:
      if candidates[j].distance < candidates[i].distance or
         (candidates[j].distance == candidates[i].distance and candidates[j].id < candidates[i].id):
        let tmp = candidates[i]
        candidates[i] = candidates[j]
        candidates[j] = tmp

  let count = min(k, candidates.len)
  result = candidates[0 ..< count]
