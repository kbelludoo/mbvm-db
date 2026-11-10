## MBVM-HDC-002 Inverted File Index (IVF) for HDC 8K Vectors
##
## Mathematical Principles:
## - Centroids: K clusters represented by binary hypervectors (8,192 bits).
## - Deterministic Initialization: K centroids generated deterministically from seed.
## - Centroid Update: Majority bundling of assigned member vectors.
## - Index Structure:
##   * Centroid Array: array[K, Hypervector]
##   * Posting Lists: Table[int, seq[uint32]] (list of record indices in container)
## - Query Routing:
##   * Step 1: Distance between Query and all K Centroids -> Select nprobe closest centroids.
##   * Step 2: Pool candidate record indices from the nprobe posting lists.
##   * Step 3: Exact Hamming reranking only over candidate pool.
##   * Step 4: Top-K selection with deterministic tie-breaking (distance ASC, id ASC).

import std/[bitops, algorithm, streams, tables]
import ../mbvm_hdc_001/encoder
import ../mbvm_hdc_001/storage
import ../mbvm_hdc_001/search

const
  IvfMagic* = ['M', 'B', 'V', 'M', 'I', 'V', 'F', '1']
  IvfHeaderSize* = 64

type
  IvfHeader* {.packed.} = object
    magic*: array[8, char]
    versionMajor*: uint16
    versionMinor*: uint16
    headerSize*: uint32
    clusterCount*: uint32
    vectorDim*: uint32
    totalIndexed*: uint64
    nprobeDefault*: uint32
    reserved*: array[32, uint8]

  IvfIndex* = object
    header*: IvfHeader
    centroids*: seq[Hypervector]
    postingLists*: seq[seq[uint32]] # clusterIdx -> list of container record indices

# --- Deterministic K-Means for Binary Hypervectors (K-Medoids / Majority Centroids) ---

proc initDeterministicCentroids*(clusterCount: int, seed: uint64 = DefaultSeed): seq[Hypervector] =
  result = newSeq[Hypervector](clusterCount)
  for c in 0 ..< clusterCount:
    let tok = "mbvm_ivf_cluster_seed_" & $c
    result[c] = tokenToHypervector(tok, seed xor (c.uint64 shl 32))

proc buildIvfIndex*(
  records: openArray[HdcRecord],
  clusterCount: int = 64,
  maxIters: int = 5,
  seed: uint64 = DefaultSeed
): IvfIndex =
  if records.len == 0 or clusterCount <= 0:
    return

  let k = min(clusterCount, records.len)
  var centroids = initDeterministicCentroids(k, seed)
  var assignments = newSeq[int](records.len)

  # Clustering iterations
  for iter in 0 ..< maxIters:
    # 1. Assignment step: find nearest centroid for each record
    var clusterMembers = newSeq[seq[int]](k)
    for i in 0 ..< records.len:
      var bestCluster = 0
      var bestDist = 8193
      for c in 0 ..< k:
        let d = hammingDistance(records[i].vector, centroids[c])
        if d < bestDist:
          bestDist = d
          bestCluster = c
      assignments[i] = bestCluster
      clusterMembers[bestCluster].add(i)

    # 2. Update step: recalculate centroid as majority bundle of its members
    var changed = 0
    for c in 0 ..< k:
      if clusterMembers[c].len > 0:
        var memberVecs = newSeq[Hypervector](clusterMembers[c].len)
        for idx, recIdx in clusterMembers[c]:
          memberVecs[idx] = records[recIdx].vector
        let newCentroid = bundleMajority(memberVecs)
        if newCentroid != centroids[c]:
          centroids[c] = newCentroid
          inc changed

    if changed == 0:
      break

  # Build final posting lists
  var postingLists = newSeq[seq[uint32]](k)
  for i in 0 ..< records.len:
    let c = assignments[i]
    postingLists[c].add(i.uint32)

  result.header = IvfHeader(
    magic: IvfMagic,
    versionMajor: 1,
    versionMinor: 0,
    headerSize: IvfHeaderSize.uint32,
    clusterCount: k.uint32,
    vectorDim: HdcDimension.uint32,
    totalIndexed: records.len.uint64,
    nprobeDefault: max(1, k div 8).uint32
  )
  result.centroids = centroids
  result.postingLists = postingLists

# --- IVF Search with Exact Candidate Reranking ---

proc searchIvfTopK*(
  index: IvfIndex,
  records: openArray[HdcRecord],
  query: Hypervector,
  k: int,
  nprobe: int = 4
): seq[SearchResult] =
  if index.centroids.len == 0 or records.len == 0 or k <= 0:
    return @[]

  let actualNprobe = clamp(nprobe, 1, index.centroids.len)

  # 1. Rank centroids by distance to query
  type CentroidDist = object
    clusterIdx: int
    dist: int

  var cDists = newSeq[CentroidDist](index.centroids.len)
  for c in 0 ..< index.centroids.len:
    cDists[c] = CentroidDist(clusterIdx: c, dist: hammingDistance(query, index.centroids[c]))

  cDists.sort proc(x, y: CentroidDist): int =
    if x.dist != y.dist:
      return cmp(x.dist, y.dist)
    return cmp(x.clusterIdx, y.clusterIdx)

  # 2. Gather candidate record indices from top nprobe clusters
  var candidateIndices = newSeq[uint32]()
  for i in 0 ..< actualNprobe:
    let c = cDists[i].clusterIdx
    for recIdx in index.postingLists[c]:
      candidateIndices.add(recIdx)

  if candidateIndices.len == 0:
    return @[]

  # 3. Exact Hamming reranking over candidates
  result = newSeq[SearchResult](candidateIndices.len)
  for i in 0 ..< candidateIndices.len:
    let recIdx = candidateIndices[i].int
    let dist = hammingDistance(query, records[recIdx].vector)
    result[i] = SearchResult(
      id: records[recIdx].id,
      symbolId: records[recIdx].symbolId,
      distance: dist
    )

  # Deterministic tie-breaking (distance ASC, id ASC)
  result.sort proc(x, y: SearchResult): int =
    if x.distance != y.distance:
      return cmp(x.distance, y.distance)
    return cmp(x.id, y.id)

  let finalCount = min(k, result.len)
  result.setLen(finalCount)

# --- Binary Serialization of IVF Index ---

proc serializeIvf*(index: IvfIndex, s: Stream) =
  s.writeData(unsafeAddr index.header, sizeof(index.header))
  for c in index.centroids:
    s.writeData(unsafeAddr c[0], 1024)
  for pList in index.postingLists:
    var count = pList.len.uint32
    s.writeData(addr count, sizeof(count))
    if count > 0:
      s.writeData(unsafeAddr pList[0], count.int * sizeof(uint32))

proc deserializeIvf*(s: Stream): IvfIndex =
  var h: IvfHeader
  let readBytes = s.readData(addr h, sizeof(h))
  if readBytes < sizeof(h):
    raise newException(HdcCorruptError, "Truncated IVF header")
  if h.magic != IvfMagic:
    raise newException(HdcCorruptError, "Invalid IVF magic")

  result.header = h
  let k = h.clusterCount.int
  result.centroids = newSeq[Hypervector](k)
  for c in 0 ..< k:
    let r = s.readData(addr result.centroids[c][0], 1024)
    if r < 1024:
      raise newException(HdcCorruptError, "Truncated IVF centroid at cluster " & $c)

  result.postingLists = newSeq[seq[uint32]](k)
  for c in 0 ..< k:
    var count: uint32
    let r = s.readData(addr count, sizeof(count))
    if r < sizeof(count):
      raise newException(HdcCorruptError, "Truncated posting list length at cluster " & $c)
    result.postingLists[c] = newSeq[uint32](count)
    if count > 0:
      let rList = s.readData(addr result.postingLists[c][0], count.int * sizeof(uint32))
      if rList < (count.int * sizeof(uint32)):
        raise newException(HdcCorruptError, "Truncated posting list payload at cluster " & $c)
