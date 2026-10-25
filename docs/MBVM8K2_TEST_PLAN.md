# MBVM8K2 — acceptance matrix and migration plan (DRAFT)

The normative wire contract is `docs/MBVM8K2_SPEC.md`. This file
describes release gates, not tests that are already implemented.

## Phase A: codec-only, no effect on v1

- Create an isolated Go package such as `internal/mbvm8k2`.
- Encode and decode fixed-width little-endian fields using
  `encoding/binary`; never serialize a Go struct with native padding.
- Enforce version, count, offset, CRC32C and SHA256 validation before
  constructing exposed graph slices.
- Expose explicit `Encode(io.Writer, Snapshot)` and
  `Decode(io.ReaderAt, length int64, Limits)` APIs.
- Keep the existing `pkg/mbvmdb.SaveToFile`, `LoadFromFile`,
  and CLI commands behavior-identical.

### Mandatory codec test matrix

| Test | Expected result |
|---|---|
| Exact empty graph (N=0, E=0, empty string ID 0) | Roundtrip |
| One symbol, no edges | Roundtrip |
| Multiple typed packages and receiver methods | Stable qualified identities |
| Repeated call-site edges | Duplicates preserved |
| Bidirectional CSR | Each reverse index references the correct forward edge |
| Serialize the same data 100 times | Byte-identical results |
| Corrupt SHA-256, section CRC, magic or major version | Reject |
| Truncate at every byte boundary in a small fixture | Reject except full file |
| Corrupt directory offsets or section lengths | Reject without panic |
| Overlapping, duplicated or out-of-order sections | Reject |
| Invalid string IDs, invalid UTF-8, duplicate strings | Reject |
| CSR malformed offsets or reverse permutation | Reject |
| Fuzzed file with restrictive Limits | No panic/unbounded allocation |
| Unknown optional section | Skip safely |
| Unknown required section | Reject |
| v1 file handed to v2 decoder | Reject without changing file |
| v2 file handed to v1 decoder | Reject without changing file |
| Existing target on Windows during publish | Never remove prior valid file |

## Phase B: typed module graph adapter

- Map `ModuleGraph.Symbols` to deterministic full-qualified IDs.
- Preserve all `ModuleEdge` call sites, not merely distinct (caller,callee).
- Preserve positions and cross-package metadata; reconstruct both CSR
  directions and prove exact impact report equivalence to the in-memory
  typed graph on the same source snapshot.
- Do not persist AST/type-checker internals.
- Keep this adapter independent from `pkg/mbvmdb.SymbolRecord` legacy
  uint16 IDs, which can collide at scale or omit qualified identities.

## Phase C: usability and performance gates

- Explicit opt-in export/import commands using **new output filenames**.
- Benchmark cold parse, warm impact queries, RSS, B/op and allocs/op.
- Compare same-source snapshots with the existing
  `ModuleCache.Impact`; never claim that disk parsing is faster than a
  cache hit without evidence.
- Document actual limits for files and graphs on Windows and Linux.
- Test publication to a new path and safe failure on overwrite, crash
  simulation where feasible, and no source tree modification.
- Only consider persistent caching when source fingerprint validation is
  demonstrably complete for the selected Go toolchain/build inputs.

## Release gate

No `MBVM8K2` writer is enabled by default before:
1. A golden format fixture is committed and reviewed independently.
2. Fuzz/corruption and overflow tests pass.
3. Graph reconstruction roundtrips pass without missing call sites.
4. Legacy format and existing CLI regression tests remain unchanged.
5. Windows safe-publish behavior is actually tested.

Security scope: SHA-256 and CRC32C detect accidental corruption only.
This protocol does not authenticate the original producer or the source
repository; consumers requiring authenticity need a separately signed
manifest and trust policy.
