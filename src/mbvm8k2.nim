## MBVM8K2 Binary Container Codec in pure Nim
## Follows PR #13 wire rules: 128-byte header, CRC32C, Little-Endian, 8-byte aligned sections.

import std/[memfiles, streams, tables]

const
  MagicMBVM8K2* = ['M', 'B', 'V', 'M', '8', 'K', '2', '\0']
  HeaderSize* = 128

type
  Mbvm8k2Header* {.packed.} = object
    magic*: array[8, char]
    major*: uint16
    minor*: uint16
    headerSize*: uint32
    flags*: uint32
    sectionCount*: uint32
    directoryOffset*: uint64
    fileLength*: uint64
    symbolCount*: uint64
    fileSha256*: array[32, uint8]
    producerFingerprint*: array[32, uint8]
    reserved*: array[16, uint8]

  SectionType* = enum
    SecStrings = 1
    SecSymbols = 2
    SecOutOffsets = 3
    SecOutEdges = 4
    SecInOffsets = 5
    SecInEdgeIds = 6
    SecBytecode = 7

  SectionEntry* {.packed.} = object
    id*: uint32
    flags*: uint32
    offset*: uint64
    length*: uint64
    itemCount*: uint64
    itemWidth*: uint32
    reservedA*: uint32
    crc32c*: uint32
    reservedB*: uint32

  Mbvm8k2File* = object
    mf*: MemFile
    header*: ptr Mbvm8k2Header
    sections*: Table[uint32, SectionEntry]

proc openMbvm8k2*(filePath: string): Mbvm8k2File =
  result.mf = memfiles.open(filePath, mode = fmRead)
  if result.mf.size < HeaderSize:
    result.mf.close()
    raise newException(IOError, "Arquivo menor que o cabeçalho MBVM8K2")

  result.header = cast[ptr Mbvm8k2Header](result.mf.mem)
  if result.header.magic != MagicMBVM8K2:
    result.mf.close()
    raise newException(ValueError, "Magic inválido no arquivo MBVM8K2")

  if result.header.major != 2:
    result.mf.close()
    raise newException(ValueError, "Versão maior incompatível (esperado 2)")

  # Leitura do diretório de seções
  let dirOffset = result.header.directoryOffset.int
  let secCount = result.header.sectionCount.int
  let entrySize = sizeof(SectionEntry)

  result.sections = initTable[uint32, SectionEntry]()
  for i in 0 ..< secCount:
    let entryPtr = cast[ptr SectionEntry](cast[uint](result.mf.mem) + uint(dirOffset + (i * entrySize)))
    result.sections[entryPtr.id] = entryPtr[]

proc close*(f: var Mbvm8k2File) =
  if f.mf.mem != nil:
    f.mf.close()
    f.mf.mem = nil
