# Module-wide impact and cross-package compile witnesses

This is the first cross-package stage of MBVM-DB, using only the Go
standard library and the Go toolchain.

## Module-wide static graph

```bash
go run ./cmd/mbvm-db impact-module \
  -root . \
  -symbol 'github.com/kbelludoo/mbvm-db/pkg/mbvmdb.New' \
  -json
```

The engine invokes `go list -mod=readonly -json ./...`, then performs a
type-checked ingestion of each build-selected package. Imported
`types.Func` references are joined to declarations using the complete
package path, receiver type and function name. The result is a single
module-wide reverse CSR containing both within-package and resolved
cross-package call sites.

For example, an edge from `cmd/mbvm-db.main` to
`pkg/mbvmdb.New` can be included. Calls through an interface or
function value are deliberately not reinterpreted as concrete method
calls. Calls targeting stdlib or dependencies outside the loaded
module remain unlinked and are counted separately.

The result schema is `mbvm-module-impact-v1`. The `direct_callers`
and `transitive_candidates` arrays list qualified *functions*, not
individual call sites; many call sites may belong to one caller.

## Cross-package compiler evidence

```bash
go run ./cmd/mbvm-db impact-module \
  -root . \
  -symbol 'github.com/kbelludoo/mbvm-db/pkg/mbvmdb.New' \
  -simulate-add-param int -json
```

For a target function with statically linked consumers in other
packages, the tool:

1. Creates a Go source replacement in a temporary folder, adding one
   required `__mbvm_cross_package_probe int` parameter to the target
   declaration **in a virtual source file**, never touching checked-in
   Go source.
2. Uses the standard Go `-overlay` mechanism and executes a
   baseline `go build -mod=readonly` for each direct consumer package
   before trying a mutated build. Both commands run with
   `CGO_ENABLED=0`.
3. Reports a confirmed consumer compilation break only when the
   baseline compiled, the overlay build failed, and the resulting
   compiler diagnostics point to the consumer's source directory.
   Failures limited to the callee package (or a failed baseline) are
   marked **inconclusive** rather than misattributed.
4. Checks the original analyzed Go source files' SHA-256 after
   processing, removes all temporary overlays/binaries, and never
   checks out Git revisions.

The output envelope includes `impact` and the optional
`cross_package_breakage` field using the
`mbvm-cross-package-breakage-v1` schema.

This check is an explicit **hypothetical required-parameter
mutation**, not automatic proof of the breakage of a real Git commit.
It can update the Go build cache and use temporary disk space; it
does not claim zero OS filesystem writes. It does not execute tests or
program startup code. Each compiler subprocess has a two-minute
deadline.

## Accuracy boundaries

- Build configurations not selected by the current OS/architecture,
  cgo, external dependent repositories, dynamically invoked symbols,
  reflection and interface dispatch remain outside the scope.
- The supported mutation is a required trailing `int` argument for
  a nonvariadic function. Target signatures must be parseable and
  the original module packages must type-check.
- A failed overlay compilation is evidence of an observed compiler
  error, not necessarily a full proof of which semantic change caused
  an error, unless it is localized to the consumer source.
- A missing compiler diagnostic or baseline build failure is
  **unknown**, not compatible or incompatible.
- Loading/checking entire modules and compiler overlay builds cost
  much more than the cached ~80 ns single-package CSR traversal.
- The module loader currently requires `go list ./...` to succeed,
  and caps discovery at 2048 packages; this is not a multi-module
  workspace analysis.
- The module-level graph does not yet persist into MBVM8K2.

## Tests

```bash
CGO_ENABLED=0 go test -count=1 -timeout=180s -v \
  -run 'Test(ModuleGraph|ModuleImpact|CrossPackage|ImpactModule)' \
  ./internal/impact ./cmd/mbvm-db

CGO_ENABLED=0 go test -count=1 -timeout=180s \
  ./internal/impact ./cmd/mbvm-db
```

The offline fixture covers two modules' methods with identical
spelling, a caller across package boundaries, a transitive caller,
interface dispatch exclusion, baseline compilation, a hypothetical
changed-API compilation failure, and exact source-file integrity.
The real-module regression verifies that the existing MBVM-DB CLI has
statically resolved calls to its own library package.
