import std/unittest
import ../encoder
import ./test_oracle

suite "MBVM-HDC-001: P0 Binding, Permutation & Bundling":

  test "Structural binding properties":
    let roleDecl = RoleFuncDecl
    let roleCall = RoleFuncCall
    let idFoo = "ProcessData"
    let idBar = "HandleRequest"

    let b1 = bindRole(roleDecl, idFoo)
    let b2 = bindRole(roleDecl, idFoo)
    let bDifferentRole = bindRole(roleCall, idFoo)
    let bDifferentId = bindRole(roleDecl, idBar)

    # 1. Identical role and id produce identical vectors
    check b1 == b2

    # 2. Different roles produce different vectors
    check b1 != bDifferentRole

    # 3. Different identifiers produce different vectors
    check b1 != bDifferentId

    # 4. Measure Hamming distances (expecting quasi-orthogonality ~4096 bits)
    let distRole = oracleHammingDistance(b1, bDifferentRole)
    let distId = oracleHammingDistance(b1, bDifferentId)

    check distRole > 3800 and distRole < 4396
    check distId > 3800 and distId < 4396

  test "Bundling behavior: single, odd, even, ties":
    let v1 = tokenToHypervector("vec_alpha")
    let v2 = tokenToHypervector("vec_beta")
    let v3 = tokenToHypervector("vec_gamma")

    # One vector bundling returns itself
    let bSingle = bundleMajority(@[v1])
    check bSingle == v1

    # Repeated identical vector returns itself
    let bRepeat = bundleMajority(@[v1, v1, v1])
    check bRepeat == v1

    # Odd bundle: bits agreeing in 2 of 3 must be present
    let bOdd = bundleMajority(@[v1, v2, v3])
    for w in 0 ..< 128:
      let w1 = v1[w]
      let w2 = v2[w]
      let w3 = v3[w]
      let maj = (w1 and w2) or (w1 and w3) or (w2 and w3)
      check bOdd[w] == maj

  test "AST-aware snippet encoding discriminates structure vs identifier":
    let codeDeclFoo = "func Foo() { return }"
    let codeDeclBar = "func Bar() { return }"
    let codeCallFoo = "Foo()"
    let codeCallBar = "Bar()"

    let vDeclFoo = encodeSnippet(codeDeclFoo)
    let vDeclBar = encodeSnippet(codeDeclBar)
    let vCallFoo = encodeSnippet(codeCallFoo)
    let vCallBar = encodeSnippet(codeCallBar)

    # Declarations with different names
    let distDecl = oracleHammingDistance(vDeclFoo, vDeclBar)
    # Declaration vs Call with the SAME name
    let distDeclVsCall = oracleHammingDistance(vDeclFoo, vCallFoo)
    # Exact same code snippet produces distance 0
    let distSelf = oracleHammingDistance(vDeclFoo, encodeSnippet(codeDeclFoo))

    check distSelf == 0
    check distDecl > 2000
    check distDeclVsCall > 2000
