## MBVM-HDC-001: Deterministic 8,192-bit Hyperdimensional Computing Engine
## Mathematical specifications:
## - Vector representation: array[128, uint64] = 8,192 bits exactly
## - Hash: WyHash v3 64-bit (seed: 0x243F_6A88_85A3_08D3)
## - PRNG: SplitMix64 (deterministic pseudo-random projection)
## - Structural Binding: XOR (a xor b)
## - Sequential Permutation: rho^k(v) - cyclic bit rotation over full 8,192 bits
## - Majority Bundling: Tie-breaking deterministic rule (>= ceiling((N+1)/2))

import std/[bitops, strutils]

const
  HdcDimension* = 8192
  HdcWords* = 128
  DefaultSeed* = 0x243F_6A88_85A3_08D3'u64 # pi fractional constant seed

type
  Hypervector* = array[HdcWords, uint64]

  StructuralRole* = enum
    RoleNone = 0
    RoleFuncDecl = 1
    RoleFuncCall = 2
    RoleVarDecl = 3
    RoleVarRef = 4
    RoleTypeDecl = 5
    RoleFieldAccess = 6
    RoleParam = 7
    RoleReturn = 8

# --- WyHash v3 64-bit Core ---

proc wymix(a, b: uint64): uint64 {.inline.} =
  # 128-bit multiplication fold
  # Since Nim handles 64-bit natively, we do 32-bit high/low cross-multiplication
  let
    aLo = a and 0xFFFF_FFFF'u64
    aHi = a shr 32
    bLo = b and 0xFFFF_FFFF'u64
    bHi = b shr 32
    
    loLo = aLo * bLo
    hiLo = aHi * bLo
    loHi = aLo * bHi
    hiHi = aHi * bHi

    cross = (loLo shr 32) + (hiLo and 0xFFFF_FFFF'u64) + loHi
    lo128 = (cross shl 32) or (loLo and 0xFFFF_FFFF'u64)
    hi128 = hiHi + (hiLo shr 32) + (cross shr 32)

  return lo128 xor hi128

proc wyr8(p: ptr UncheckedArray[uint8], offset: int): uint64 {.inline.} =
  var v = 0'u64
  for i in 0 ..< 8:
    v = v or (p[offset + i].uint64 shl (i * 8))
  return v

proc wyr4(p: ptr UncheckedArray[uint8], offset: int): uint64 {.inline.} =
  var v = 0'u64
  for i in 0 ..< 4:
    v = v or (p[offset + i].uint64 shl (i * 8))
  return v

proc wyr3(p: ptr UncheckedArray[uint8], offset: int, k: int): uint64 {.inline.} =
  return (p[offset].uint64 shl 16) or (p[offset + (k shr 1)].uint64 shl 8) or p[offset + k - 1].uint64

proc wyhash64*(data: openArray[byte], seed: uint64 = DefaultSeed): uint64 =
  let len = data.len
  if len == 0:
    return seed
  let p = cast[ptr UncheckedArray[uint8]](unsafeAddr data[0])
  var
    s = seed xor 0xA076_1D64_78BD_642F'u64
    see1 = seed
    i = len
    offset = 0

  if i <= 16:
    if i >= 4:
      let a = (wyr4(p, 0) shl 32) or wyr4(p, ((i shr 3) shl 2))
      let b = (wyr4(p, i - 4) shl 32) or wyr4(p, i - 4 - ((i shr 3) shl 2))
      return wymix(a xor 0xA076_1D64_78BD_642F'u64, b xor s)
    elif i > 0:
      return wymix(wyr3(p, 0, i) xor 0xA076_1D64_78BD_642F'u64, s)
    else:
      return wymix(0xA076_1D64_78BD_642F'u64, s)

  if i > 16:
    while i > 16:
      s = wymix(wyr8(p, offset) xor 0xA076_1D64_78BD_642F'u64, wyr8(p, offset + 8) xor s)
      offset += 16
      i -= 16
  let a = wyr8(p, len - 16)
  let b = wyr8(p, len - 8)
  return wymix(a xor 0xA076_1D64_78BD_642F'u64, b xor s)

proc hashString*(s: string, seed: uint64 = DefaultSeed): uint64 {.inline.} =
  if s.len == 0:
    return wyhash64(@[], seed)
  var b = newSeq[byte](s.len)
  for i in 0 ..< s.len:
    b[i] = byte(s[i])
  return wyhash64(b, seed)

# --- SplitMix64 PRNG Core ---

proc splitmix64Next*(state: var uint64): uint64 {.inline.} =
  state += 0x9E37_79B9_7F4A_7C15'u64
  var z = state
  z = (z xor (z shr 30)) * 0xBF58_476D_1CE4_E5B9'u64
  z = (z xor (z shr 27)) * 0x94D0_49BB_1331_11EB'u64
  return z xor (z shr 31)

# --- Deterministic Hypervector Projection ---

proc tokenToHypervector*(token: string, seed: uint64 = DefaultSeed): Hypervector =
  var h = hashString(token, seed)
  for i in 0 ..< HdcWords:
    result[i] = splitmix64Next(h)

proc roleToHypervector*(role: StructuralRole): Hypervector =
  let roleStr = "mbvm_role_internal:" & $role
  return tokenToHypervector(roleStr, DefaultSeed xor 0x517C_C1B7_2722_0A95'u64)

# --- VSA Operators: Binding, Permutation, Bundling ---

proc bindVectors*(a, b: Hypervector): Hypervector {.inline.} =
  for i in 0 ..< HdcWords:
    result[i] = a[i] xor b[i]

proc bindRole*(role: StructuralRole, identifier: string): Hypervector =
  let vRole = roleToHypervector(role)
  let vId = tokenToHypervector(identifier)
  return bindVectors(vRole, vId)

proc permuteCyclic*(v: Hypervector, shiftBits: int): Hypervector =
  ## Exact bijective reversible permutation over all 8,192 bit positions.
  ## shiftBits can be positive (left/forward) or negative (right/inverse).
  let totalBits = HdcDimension
  var k = shiftBits mod totalBits
  if k < 0:
    k += totalBits
  if k == 0:
    return v

  let wordShift = k div 64
  let bitShift = k mod 64
  let invBitShift = 64 - bitShift

  for i in 0 ..< HdcWords:
    let srcWordIdx = (i - wordShift + HdcWords) mod HdcWords
    let prevWordIdx = (srcWordIdx - 1 + HdcWords) mod HdcWords

    if bitShift == 0:
      result[i] = v[srcWordIdx]
    else:
      let partA = v[srcWordIdx] shl bitShift
      let partB = v[prevWordIdx] shr invBitShift
      result[i] = partA or partB

proc inversePermute*(v: Hypervector, shiftBits: int): Hypervector {.inline.} =
  return permuteCyclic(v, -shiftBits)

proc bundleMajority*(vectors: openArray[Hypervector]): Hypervector =
  ## Majority rule with deterministic tie-breaking.
  ## For N vectors:
  ## If ones * 2 > N -> 1
  ## If ones * 2 < N -> 0
  ## If ones * 2 == N (exact tie when N is even):
  ##   deterministic parity tie-break rule: (pos mod 2) == 0 -> 1 else 0
  let n = vectors.len
  if n == 0:
    return
  if n == 1:
    return vectors[0]

  var counts: array[HdcDimension, int32]
  for v in vectors:
    for w in 0 ..< HdcWords:
      let word = v[w]
      let baseBit = w * 64
      for b in 0 ..< 64:
        if ((word shr b) and 1'u64) == 1'u64:
          inc counts[baseBit + b]

  for w in 0 ..< HdcWords:
    var wordVal = 0'u64
    let baseBit = w * 64
    for b in 0 ..< 64:
      let bitIdx = baseBit + b
      let c2 = counts[bitIdx] * 2
      var bitResult = 0'u64
      if c2 > n:
        bitResult = 1'u64
      elif c2 < n:
        bitResult = 0'u64
      else:
        # Tie condition only occurs when n is even
        bitResult = if (bitIdx mod 2 == 0): 1'u64 else: 0'u64
      wordVal = wordVal or (bitResult shl b)
    result[w] = wordVal

# --- AST-aware Structural Tokenizer and Encoder ---

type
  StructuralToken* = object
    role*: StructuralRole
    value*: string

proc tokenizeStructuralGo*(source: string): seq[StructuralToken] =
  ## Deterministic lexical and AST-role extractor for code snippets.
  ## Distinguishes declarations, calls, parameters, types, and returns.
  result = @[]
  var i = 0
  let n = source.len

  proc skipWhitespace() =
    while i < n and (source[i] in {' ', '\t', '\r', '\n'}):
      inc i

  proc readIdent(): string =
    var s = ""
    while i < n and (source[i] in {'a'..'z', 'A'..'Z', '0'..'9', '_'}):
      s.add(source[i])
      inc i
    return s

  while i < n:
    skipWhitespace()
    if i >= n: break

    if source[i] == '/' and i + 1 < n and source[i+1] == '/':
      while i < n and source[i] != '\n': inc i
      continue

    if source[i] in {'a'..'z', 'A'..'Z', '_'}:
      let word = readIdent()
      case word
      of "func":
        skipWhitespace()
        let fnName = readIdent()
        if fnName.len > 0:
          result.add(StructuralToken(role: RoleFuncDecl, value: fnName))
      of "type":
        skipWhitespace()
        let typeName = readIdent()
        if typeName.len > 0:
          result.add(StructuralToken(role: RoleTypeDecl, value: typeName))
      of "var":
        skipWhitespace()
        let varName = readIdent()
        if varName.len > 0:
          result.add(StructuralToken(role: RoleVarDecl, value: varName))
      of "return":
        result.add(StructuralToken(role: RoleReturn, value: "return"))
      else:
        skipWhitespace()
        if i < n and source[i] == '(':
          # Identified as a function call
          result.add(StructuralToken(role: RoleFuncCall, value: word))
        elif i < n and source[i] == '.':
          # Field access / method access
          inc i # skip '.'
          let field = readIdent()
          result.add(StructuralToken(role: RoleFieldAccess, value: word & "." & field))
        else:
          result.add(StructuralToken(role: RoleVarRef, value: word))
    else:
      inc i

proc encodeSnippet*(source: string): Hypervector =
  ## Encodes an entire code snippet into a single 8,192-bit hypervector:
  ## 1. Extract structural tokens.
  ## 2. Bind each token with its structural role.
  ## 3. Permute sequentially according to position: rho^k(V_bound).
  ## 4. Bundle all tokens with majority rule.
  let tokens = tokenizeStructuralGo(source)
  if tokens.len == 0:
    return tokenToHypervector("empty_source_fallback")

  var boundSeq = newSeq[Hypervector](tokens.len)
  for k in 0 ..< tokens.len:
    let bound = bindRole(tokens[k].role, tokens[k].value)
    boundSeq[k] = permuteCyclic(bound, k + 1)

  return bundleMajority(boundSeq)
