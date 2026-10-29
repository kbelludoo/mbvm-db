# MBVM-DB — Real Git contract compatibility (`impact-diff -module`)

The existing `impact-diff` command preserves its v1 **single-package**
behavior without `-module`. With `-module -symbol` it checks a **real
contract from the new committed source**, not a hypothetical
`-simulate-add-param int` mutation.

## Compare two local commits

```powershell
go run ./cmd/mbvm-db impact-diff `
  -module -repo . `
  -old-ref HEAD~1 -new-ref HEAD `
  -symbol 'github.com/kbelludoo/mbvm-db/pkg/mbvmdb.New' `
  -json
```

Or compare two independent, existing module root directories:

```powershell
go run ./cmd/mbvm-db impact-diff `
  -module -old C:\code\old -new C:\code\new `
  -symbol 'example.test/project/api.Run' -json
```

The symbol must be fully qualified. Both refs must be **available in the
local Git repository**; the command does not fetch, checkout, stash or
reset. Each revision is extracted by the bounded and safe
`git archive` infrastructure (PAX metadata accepted; links rejected).
Snapshots are discarded after comparison.

## Methodology and evidence boundary

1. Ingest/typecheck all packages from the old module; discover packages
   in the new module, and typecheck the actual new provider package.
2. Require the same fully qualified provider symbol in both snapshots
   and compare its *typed* callable contract, ignoring parameter names.
   If the contract is unchanged, no consumer build is attempted.
3. Identify packages with **direct, statically linked old call sites**
   to that symbol. Interface dispatch, reflection and function-valued
   indirect calls are not assigned to concrete callees.
4. Build the **real new provider** unchanged. If it fails, all old
   consumers are inconclusive.
5. For each old consumer, require its old original source to build and
   the native new consumer to build. Require exactly matching sets of
   build-selected source filenames across revisions.
6. Create a Go `-overlay` whose **new consumer file paths** resolve to
   exact bytes from the **old consumer source**. The provider and other
   packages remain exactly as they were in the real new revision.
   Retain the exact reverse map from **physical replacement paths** to
   **logical Go source paths** for diagnostic attribution. Depending on
   the Go toolchain and OS, compiler messages may name either path.
   Windows-relative physical paths are recognized only when their
   suffix includes the unique temporary run directory, overlay folder
   and replacement filename, not a bare matching filename.
7. Run `go build -mod=readonly -overlay ...` in the new module.
   A consumer is **broken** only if the build fails and a diagnostic
   of `not enough arguments in call to <function>` corresponds to a
   known old, type-resolved direct call site, after mapping its
   relative filename/line into the new checkout.
   A passing build is **compatible_in_test**, not a general proof of
   behavioral compatibility or interface compatibility. All other
   failures are **inconclusive** and preserve the actual diagnostics.
8. Verify SHA-256 of all `.go`, `go.mod`, `go.sum`, `go.work`
   and `go.work.sum` files under both original snapshot roots before
   and after analysis. Any change is an error.

Output schema: `mbvm-real-module-diff-v1`. Its
`confirmed_broken_consumers`, `compatible_in_test`, and
`inconclusive_consumers` arrays refer to **packages**. It also
includes `old_contract`, `new_contract`, baseline build evidence,
per-consumer diagnostics and both tree digests.

## Scope

- This first release analyzes **one explicit changed symbol** at a
  time. It does not yet auto-enumerate every changed exported API.
- Both versions must use the same Go import paths; deletions, renames,
  filename-set drift, moved packages and missing dependencies are
  unsupported or yield inconclusive/error.
- Compiler diagnostic confirmation currently supports the **missing
  arguments** error at a known old call site. Other real breaks
  (changed result types, type constraints, removed symbols, method
  sets, functions used as values, etc.) may remain inconclusive. That
  conservative boundary avoids a misleading universal accuracy claim.
- No source files or Git state are mutated. Snapshot TARs, overlays,
  build outputs and compiler cache may use disk space. Builds are
  compile-only and never execute application binaries or unit tests.
- The old graph is loaded from the entire old module, so startup is
  substantially more expensive than a nanosecond CSR hot-path lookup.
- `-package` is not supported together with `-module`.
  With `-module`, `-old` and `-new` mean *module roots*.
- The new result schema does **not** replace the existing
  `mbvm-version-impact-v1` single-package JSON contract.

## Acceptance tests

```powershell
$env:CGO_ENABLED = "0"
go test -count=1 -timeout=240s -v `
  -run 'Test(RealModule|ImpactDiffModule)' `
  ./internal/impact ./cmd/mbvm-db
```

Tests cover real changed signature causing an old-client compile error,
reverse mapping of physical, logical and Windows-relative overlay
locations, rejection of unrelated file/callee/line diagnostics,
a changed signature still accepting an old caller, divergent source
file sets treated as inconclusive, comparison between two actual local
Git commits without touching dirty working-tree files, and CLI JSON
with flag validation.
