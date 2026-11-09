import std/unittest
import ../src/hdc

suite "HDC 8K Engine Tests":
  test "Deterministic zero vector identity":
    var a: Hypervector
    var b: Hypervector
    check hammingDistance(a, b) == 0
    check cosineSimilarityEquivalent(a, b) == 1.0

  test "Hamming distance calculation":
    var a: Hypervector
    var b: Hypervector

    # Flip 1 bit in word 0
    a[0] = 0b01
    b[0] = 0b10
    # Distance in word 0 is 2, rest is 0
    check hammingDistance(a, b) == 2

  test "Cosine similarity equivalent":
    var a: Hypervector
    var b: Hypervector

    check cosineSimilarityEquivalent(a, a) == 1.0

    # Flip exactly 4096 bits (orthogonal in 8K)
    for i in 0 ..< 64:
      b[i] = high(uint64)
    for i in 64 ..< 128:
      b[i] = 0

    check hammingDistance(a, b) == 4096
    # 1.0 - (2.0 * 4096 / 8192) == 0.0
    check cosineSimilarityEquivalent(a, b) == 0.0

  test "Superpose operation":
    var acc: Hypervector
    var other: Hypervector
    acc[0] = 0xAA
    other[0] = 0x55
    superpose(acc, other)
    check acc[0] == 0xFF
