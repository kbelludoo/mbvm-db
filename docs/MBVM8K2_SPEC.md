# MBVM8K2 v2.0 — binary container specification (DRAFT)

Status: **design proposal, not an implemented codec**. The normative terms
MUST, MUST NOT and SHOULD define acceptance conditions for a future Go
reader/writer. Keep this document versioned alongside golden test vectors.

## 1. Scope and non-goals

MBVM8K2 v2.0 persists a **snapshot of a typed module impact graph**:
stable fully-qualified symbol identities, unique strings, call-site edges,
forward and reverse CSR, and optional opaque bytecode. It does **not**
serialize AST objects, `go/types` objects, Go compiler importers, or the
in-process `ModuleCache`. It does not prove runtime reachability,
reflection-based call targets, or mathematical theorem correctness.

Current legacy `pkg/mbvmdb/storage.go` writes `MBVM` + version 1.0,
big-endian metadata, an interned string pool, symbol entries, zlib-based
bytecode and payload CRC32. The in-memory CSR is reconstructed on read.
Its public `SaveToFile`/`LoadFromFile` behavior MUST remain unchanged.

MBVM8K2 is a **distinct format** with a distinct magic and separate
`SaveV2`/`LoadV2` APIs; a v2 reader MUST NOT reinterpret a v1 file.
Migration is an explicit conversion action, never an implicit overwrite.

## 2. Fixed wire rules

- Magic: 8 bytes `4D 42 56 4D 38 4B 32 00` (`MBVM8K2\0`).
- Version: major `2`, minor `0`. Integers are **unsigned little-endian**
  with fixed widths; no compiler-native struct layout or padding is used.
- All absolute file offsets are `uint64` and measured from byte zero.
- All section starts are 8-byte aligned; padding bytes MUST be zero.
- The header is exactly 128 bytes; the section directory immediately
  follows at offset 128. Each directory entry is exactly 48 bytes.
- Sections are ordered by ascending numeric section ID; each is unique.
- No encryption or compression in v2.0. Readers MUST reject unknown
  required flags/algorithms rather than guessing.
- Names and paths in the string table are UTF-8, unnormalized and compared
  by exact bytes. The writer's canonical order is lexical byte order.
  IDs in sections refer to positions in this canonical table.

## 3. Header (128 bytes)

| Offset | Bytes | Field | Rule |
|---:|---:|---|---|
| 0 | 8 | magic | Exact bytes above |
| 8 | 2 | major | `2` |
| 10 | 2 | minor | `0` |
| 12 | 4 | header_size | `128` |
| 16 | 4 | flags | `0` for v2.0 |
| 20 | 4 | section_count | 6..64 |
| 24 | 8 | directory_offset | `128` |
| 32 | 8 | file_length | Exact physical length |
| 40 | 8 | symbol_count | Must equal SYMBOLS count |
| 48 | 32 | file_sha256 | SHA-256 of entire file with these 32 bytes set to zero |
| 80 | 32 | producer_fingerprint | Optional, zero if unknown; informational |
| 112 | 16 | reserved | All zero |

`producer_fingerprint` may record the digest of a separately documented
build provenance key. It is **not** itself proof that cache inputs are
unchanged and MUST NOT grant a cache hit without independently validating
inputs under the relevant caching policy.

`file_sha256` detects corruption but is **not an authentication signature**.
The digest includes header, directory, padding, and all sections.

## 4. Section directory entry (48 bytes)

| Entry offset | Bytes | Field | Rule |
|---:|---:|---|---|
| 0 | 4 | id | Unique section type |
| 4 | 4 | flags | Bit 0 = optional; other bits MUST be zero |
| 8 | 8 | offset | 8-byte-aligned absolute file offset |
| 16 | 8 | length | Section byte length |
| 24 | 8 | item_count | Logical record count |
| 32 | 4 | item_width | Fixed record width, or 0 for variable |
| 36 | 4 | reserved_a | Zero |
| 40 | 4 | crc32c | CRC-32C (Castagnoli) over section bytes |
| 44 | 4 | reserved_b | Zero |

Required sections MUST have flags=0. Unknown section IDs may be skipped
ONLY when the optional bit is set. Duplicate IDs, overlaps, offsets that
overflow `offset+length`, nonzero padding, or trailing undisclosed bytes
are invalid. Every section MUST lie after the directory.

## 5. Section types (v2.0)

| ID | Section | Required | Width |
|---:|---|---|---:|
| 1 | STRINGS | Yes | variable |
| 2 | SYMBOLS | Yes | 48 |
| 3 | OUT_OFFSETS | Yes | 4 |
| 4 | OUT_EDGES | Yes | 16 |
| 5 | IN_OFFSETS | Yes | 4 |
| 6 | IN_EDGE_IDS | Yes | 4 |
| 7 | BYTECODE | No | variable |

### 5.1 STRINGS (ID=1)

Payload is `uint32 offsets[S+1]` followed by the concatenated UTF-8
string bytes. Offsets are relative to the first byte of the string blob;
`offsets[0]=0`, offsets are nondecreasing, and `offsets[S]` equals the
blob length. Directory `item_count=S` and `item_width=0`.

String ID 0 MUST be the empty string and be the first entry. All other
strings are **unique** and sorted lexicographically by their raw UTF-8
bytes. Embedded NUL is disallowed in v2.0. IDs are `uint32`; all
references MUST be in range `[0,S)`.

### 5.2 SYMBOLS (ID=2)

Records are 48 bytes each, sorted by the full-qualified identity string.
Directory `item_count=N` MUST match `symbol_count` in the header.

| Record offset | Bytes | Field |
|---:|---:|---|
| 0 | 4 | qualified_str_id |
| 4 | 4 | name_str_id |
| 8 | 4 | package_str_id |
| 12 | 4 | receiver_str_id (0 when absent) |
| 16 | 4 | kind_str_id |
| 20 | 4 | signature_str_id (0 when absent) |
| 24 | 4 | flags (zero in v2.0) |
| 28 | 4 | reserved (zero) |
| 32 | 8 | bytecode_offset (relative to BYTECODE, or 0) |
| 40 | 8 | bytecode_length (0 when absent) |

The full qualified name is the **cross-version symbol identity**;
the ordinal `uint32` symbol index is **local to this snapshot** and
can change on rebuild. Fully qualified identities MUST be nonempty and
unique. The serializer MUST reject overflow, duplicate identities, and
ambiguous package/receiver encodings rather than truncate to `uint16`.

### 5.3 CSR of call-site edges (IDs 3–6)

A call-site edge is preserved even if another edge has the same caller
and callee. `OUT_OFFSETS` has exactly N+1 little-endian `uint32` values,
starting at zero, nondecreasing, with final value E.

`OUT_EDGES` contains E fixed 16-byte records:

| Offset | Bytes | Field |
|---:|---:|---|
| 0 | 4 | caller_symbol_index |
| 4 | 4 | callee_symbol_index |
| 8 | 4 | position_str_id (0 = unknown) |
| 12 | 4 | flags (bit 0 = cross-package; other bits zero) |

All symbol indices must be within `[0,N)`. In source row i, every
record's caller index MUST equal i. Edges within each row are sorted
by `(callee_symbol_index, position_str_id, flags)`, preserving duplicates.
A position is a UTF-8 canonical source locator, not a trusted OS path.

`IN_OFFSETS` similarly has N+1 `uint32` values ending at E.
`IN_EDGE_IDS` contains E `uint32` values: for reverse row j, each
index references an `OUT_EDGES` entry whose callee is j. All E reverse
indices form an **exact permutation of [0,E)** — no missing or repeated
forward edge references. Reverse entries are sorted by their referenced
forward edge index. Readers validate these invariants before exposing
the CSR.

This layout allows impact queries to follow reverse call relationships
without recomputing the graph or duplicating call-site metadata.

### 5.4 BYTECODE (ID=7; optional)

A raw, uncompressed opaque byte sequence. A symbol's
`bytecode_offset + bytecode_length` MUST be checked against this section
with overflow-safe arithmetic. Without the section, every symbol MUST
have a zero bytecode length. Existing v1 zlib streams MUST NOT be copied
blindly and called v2 raw bytecode; a future converter must decompress
and validate or explicitly omit them.

## 6. Reader correctness and security

Readers MUST validate header, full-file digest, directory ordering,
per-section CRC32C and all structural invariants **before** providing
any query interface or unsafe/mmap-backed view. Avoid unchecked
`int` conversions and arithmetic overflows. Do not trust counts
supplied by a corrupt or adversarial file.

Recommended initial default reader limits (caller may lower them):
- File size <= 256 MiB; no unrestricted `io.ReadAll`.
- Symbols <= 1,000,000; call-site edges <= 8,000,000.
- Strings <= 4,000,000; sections <= 64.
- Fail early when sizes or offset arrays imply excessive allocations.

No symlinks are followed on behalf of section paths; source paths are
informational metadata only. A reader does not execute bytecode or
source code. A hash protects against accidental corruption but not
malicious substitution; signed manifests are out of scope.

## 7. Deterministic writer and Windows publication

Given the same normalized snapshot, the writer MUST output
byte-for-byte identical files: sort strings, symbols and edges;
zero reserved bytes and padding; fixed section order; no timestamps
or map iteration order in the encoded format.

Write to a unique temporary file **in the destination directory**,
flush and close it, and only then publish. NEVER delete an existing
valid destination before attempting rename. On platforms where
replacement rename is unavailable (notably some Windows cases), the
writer MUST return a non-destructive error, or publish an immutable
content-addressed/versioned filename. Do not claim cross-platform
atomic replacement until a tested, OS-specific implementation exists.

## 8. Compatibility and acceptance gates

- v1 `MBVM` header remains readable by the existing v1 loader; never
  re-label a v1 artifact as MBVM8K2.
- v2 loader requires major=2; v2 writer emits minor=0.
- Unknown mandatory sections and flags fail closed; known required
  sections must all be present; unknown optional sections may be skipped.
- Test golden byte fixtures with an **independent second decoder**.
- Roundtrip 0, 1, many symbols; duplicate call sites; receiver methods;
  cross-package edges; zero-byte bytecode; UTF-8 identities.
- Corrupt each header field, digest, CRC, directory range, section
  alignment, duplicate ID, bogus offset/length, invalid string index,
  invalid UTF-8, CSR nonmonotonicity and reverse-edge permutation.
- Fuzz parsing with allocation limits and truncated files.
- Compare impact queries after legacy/in-memory graph -> v2 -> in-memory
  graph roundtrips for the same typed module fixture.
- Benchmark cold parse, hot query, allocations and RSS **separately**
  from fresh Go typing, the ModuleCache hash path and v1 serialization.
- Never claim cache-validity merely from stored `producer_fingerprint`.

## 9. Implementation milestones (no rollout implied)

1. Codec-only package with constants, checked header/section parser,
   a deterministic writer, golden fixtures and fuzz tests. Keep it out
   of the existing v1 API until these gates pass.
2. Adapter for `internal/impact.ModuleGraph` using typed, fully qualified
   identities and original call-site positions. Verify reverse CSR.
3. Optional explicit command (e.g. `impact-export-v2`) that writes a
   **new** file; do not replace `build` or `verify` implicitly.
4. Test legacy compatibility, bounded memory, corruption resilience,
   Windows publication behavior, and real-repository benchmarks.
5. Only after these checks consider persistent source fingerprinting
   and integration with the in-process ModuleCache.

**Open design decision:** whether symbol bytecode will be supported in
the first writer. The proposed safe default is *omitted BYTECODE* for
typed impact graphs, which do not need it.
