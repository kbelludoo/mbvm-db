# MBVM-DB impact-diff: compare two REAL Go package versions

Version 1 of the stage-2 comparison is built on the production
`internal/impact` typed call graph. It supports:

- **Two existing directories**, such as two separate Git checkouts.
- **Two local Git commit/tag/branch refs** within one repository, read
  by `git archive` into bounded temporary snapshots, without changing
  the checkout, index, stash, branches, or tracked source files.

## Usage

```sh
# Two directory snapshots (same Go package/import path):
go run ./cmd/mbvm-db impact-diff \
  -old ../old-checkout/pkg/mbvmdb \
  -new ./pkg/mbvmdb -json

# Two local revisions of the SAME Git repository:
go run ./cmd/mbvm-db impact-diff \
  -repo . -old-ref HEAD~1 -new-ref HEAD \
  -package pkg/mbvmdb -json
```

The refs must already exist locally; this command does not fetch
remotes. Go Module imports must resolve in each snapshot, which may
require module cache access. Git snapshots are limited to 256 MiB
of regular-file contents per version, reject symlinks and unsafe paths,
and are discarded when analysis completes. This allocates temporary
disk space; it is not a zero-disk-allocation operation.

### Windows Git archive I/O (post-326fb0f)

Git for Windows may emit a POSIX PAX global header called
`pax_global_header` (TAR type `'g'`, 103). The extractor now explicitly
accepts `tar.TypeXGlobalHeader` and `tar.TypeXHeader` as metadata,
skipping their extraction while continuing to reject links and special
files. `TestExtractGitArchivePAXMetadata` verifies extraction using a
real `git archive` and verifies that the metadata entry never appears
as a file in the snapshot.

`TestExtractArchiveDeterministicPAXAndRejectLinks` additionally creates
an actual global PAX TAR header via Go's standard `archive/tar.Writer`,
checks that a regular Go file is extracted, and checks that TAR symlinks
remain rejected. It does not depend on whether the installed Git
happens to emit a PAX global header.

The Git archive reader no longer mixes an application-owned
`StdoutPipe` with `cmd.Wait`. It now uses `exec.CommandContext` and
`cmd.Run` to write a bounded TAR stream to a **temporary regular file**,
then parses that archive after the process has finished. Both the
on-disk archive and the extracted regular contents are restricted to
256 MiB per revision; the command is cancelled after two minutes.

This avoids unbounded RAM buffering while preventing the specific
pipe-consumption/Wait synchronization problem on Windows. The
temporary TAR is removed even after a failed run.

The cross-platform regression test commits a TAR payload larger than
an ordinary anonymous pipe buffer. A dedicated Windows GitHub Actions
workflow (`.github/workflows/impact_windows.yml`) runs these tests on
`windows-latest`. The fix should not be marked Windows-validated until
that workflow passes, and a single passing runner does not cover
every installed Git for Windows version.

The emitted schema is **`mbvm-version-impact-v1`**, not
`mbvm-impact-diff-v1`.


## Evidence contract

The JSON schema is `mbvm-version-impact-v1`, with before/after
SHA-256 hashes, healthy old/new package snapshots, unchanged original
file verification, and sorted changes:

- `added` or `removed`: declaration appeared/disappeared.
- `signature_changed`: a typed function parameter/result contract
  changed. Names of parameters are ignored in the contract fingerprint.
- `body_changed`: a formatted body changed without a typed callable
  signature change. It does **not** mean different runtime behavior.
- Unchanged function declarations are omitted from the change list.

Each changed/removed symbol includes *old* direct and transitive
caller candidates from the typed local CSR.

For supported `signature_changed` cases, the checker constructs a
controlled **HYBRID**: old package source and consumers with the new
function/method signature transplanted into the old AST. The Go type
checker then checks whether those **old same-package consumers** still
typecheck. This is an actual Go type-checking observation for that
hybrid, **not a build of a mixed full repository**. A consumer
confirmed by a diagnostic appears under
`old_consumer_compatibility.broken_old_direct_callers`.

A compatibility result with `supported=false` is **unknown**.
In particular, generic function signatures, or cases in which the
transplanted declaration/body produces errors, must not be reported
as confirmed caller breakages. Diagnostics from other locations may
also be unrelated to the changed declaration, so the tool reports
only direct callers with errors as observed candidate breakages
under this hybrid.

## Limits to preserve scientific accuracy

- Both snapshots must independently pass `go/types`, have identical
  package import paths and be valid for the selected GOOS/GOARCH and
  build tags. If either snapshot fails to typecheck, comparison fails.
- Only statically resolved local direct calls are linked.
  Function values, interface dispatch, reflection, cross-package
  consumers and alternate build configurations are not covered.
- A removed symbol currently records old callers but does not perform
  a removal-based compiler validation.
- No AST canonical equivalence or runtime test is implied by
  formatted source differences.
- Hybrid errors at the target declaration or outside attributed
  function bodies cause a conservative inconclusive outcome.
- Go Modules export-cache resolution (`go list`) and Git archive
  extraction cost are INCLUDED in the end-to-end command, so nanosecond
  CSR traversal microbenchmarks cannot be used as CLI latency numbers.

## Tests

```sh
CGO_ENABLED=0 go test -count=1 -v ./internal/impact ./cmd/mbvm-db
```

The tests exercise two independently well-typed snapshots with a
breaking signature change and adjusted new callers, verified old
caller diagnostic, added and removed functions, body-only changes,
method name collisions, formatting-only changes, and two actual
committed Git revisions with an unchanged dirty worktree.

## Real-version module compatibility mode

The `impact-diff` CLI also supports `-module -symbol <fully-qualified>`
with either `-old/-new` **module roots** or `-old-ref/-new-ref`
local Git revisions. This invokes the REAL provider code from the new
snapshot and overlays the original old consumer source files onto the
new snapshot. The mode does not create a synthetic signature and does
not alter the old consumers to accommodate it.

The separate JSON schema is `mbvm-real-module-diff-v1` and preserves
the original `mbvm-version-impact-v1` output for existing users.
The first release checks one explicitly named symbol and confirms only
compiler diagnostics for insufficient arguments at old typed call sites;
other real ABI/API incompatibilities remain inconclusive.

See [REAL_DIFF.md](REAL_DIFF.md) for the exact evidence procedure,
Git CLI examples, safety checks and acceptance tests.
