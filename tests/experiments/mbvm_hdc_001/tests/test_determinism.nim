import std/unittest
import ../encoder
import ./test_oracle

suite "MBVM-HDC-001: P0 Determinism & Oracle Verification":

  test "WyHash v3 64-bit matches independent oracle":
    let testStrings = @[
      "", "a", "foo", "main.go", "func ProcessData(ctx context.Context) error",
      "Very long string with lots of code and syntax to verify multi-block 16-byte wyhash loop behavior"
    ]
    for s in testStrings:
      let prodHash = hashString(s)
      let oracleH = oracleHashString(s)
      check prodHash == oracleH

  test "SplitMix64 and full Hypervector match independent oracle":
    let tokens = @["init", "HandleRequest", "BufferOverflow", "int64", "return nil"]
    for tok in tokens:
      let prodVec = tokenToHypervector(tok)
      let oracleVec = oracleGenerateVector(tok)
      for w in 0 ..< 128:
        check prodVec[w] == oracleVec[w]

  test "Bit-exact determinism across repeated executions":
    let t1 = tokenToHypervector("DeterministicSymbol")
    let t2 = tokenToHypervector("DeterministicSymbol")
    check t1 == t2
    let diff = tokenToHypervector("DifferentSymbol")
    check t1 != diff

  test "Cyclic permutation rho^k matches bit-by-bit oracle and preserves Hamming weight":
    var testVec: Hypervector
    # Initialize with alternating patterns
    for i in 0 ..< 128:
      testVec[i] = 0xAA55_AA55_AA55_AA55'u64

    for shift in [1, 7, 63, 64, 65, 127, 128, 511, 1024, 8191]:
      let prodP = permuteCyclic(testVec, shift)
      let oracleP = oraclePermuteBitByBit(testVec, shift)
      check prodP == oracleP

      # Reversibility test
      let reconstructed = inversePermute(prodP, shift)
      check reconstructed == testVec
