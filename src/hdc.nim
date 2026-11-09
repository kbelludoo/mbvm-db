## MBVM-DB HDC (Hyperdimensional Computing / 8K 1-bit) Vector Engine
## 8,192 bits = 1,024 bytes = 128 x uint64.
## Uses native CPU popcount for sub-microsecond Hamming distance.

import std/bitops

const
  HdcDimension* = 8192
  HdcWords* = HdcDimension div 64 # 128 x 64-bit words = 1,024 bytes

type
  Hypervector* = array[HdcWords, uint64]

proc hammingDistance*(a, b: Hypervector): int {.inline.} =
  var dist = 0
  for i in 0 ..< HdcWords:
    dist += popcount(a[i] xor b[i])
  return dist

proc cosineSimilarityEquivalent*(a, b: Hypervector): float64 {.inline.} =
  let dist = hammingDistance(a, b)
  return 1.0 - (2.0 * dist.float64 / HdcDimension.float64)

proc superpose*(acc: var Hypervector, other: Hypervector) {.inline.} =
  # Majority rule / bitwise superposition step for binary HDC
  for i in 0 ..< HdcWords:
    acc[i] = acc[i] xor other[i] # XOR unbinding or OR superposition
