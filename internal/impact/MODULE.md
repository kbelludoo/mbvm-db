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
packages, the self-consistent verification proceeds as follows:

1. `PrepareContractOverlay` reparses only selected provider Go sources
   with `parser.ParseComments` (preserving build tags and compiler
   directives), creates an in-memory signature mutation, and appends a
   synthetic integer `0` **only** to local call sites already proven
   by `types.Func` identity in the original typed graph. Changed
   provider files are formatted to temporary replacements and declared
   in one Go overlay JSON; original source/AST is never modified.
2. Compile the original **provider baseline** using
   `go build -mod=readonly`. If it fails, every external consumer is
   marked inconclusive.
3. Compile the **changed virtual provider** with `-overlay`. If this
   self-consistent provider does not compile, all consumers stay
   inconclusive; no downstream breakages are invented.
4. For each cross-package direct consumer, compile its unmodified
   baseline followed by its unmodified sources against the **same
   changed provider overlay**. Promote to confirmed breakage only when
   baseline passed and Go reports `not enough arguments in call to
   <target>` at a file/line previously linked as that target's actual
   call site. Unrelated compiler errors remain inconclusive.
5. Hash all selected Go sources across the module before and after,
   remove temporary files and never touch original Git content.

The JSON reports `provider_baseline_build_passed`,
`provider_overlay_build_passed`,
`adapted_internal_call_sites`, optional
`provider_diagnostics` and `provider_reason`, in addition to the
existing per-consumer evidence. Compiler baselines and overlays may
update the Go build cache but do not execute application/test code.


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
  error. A consumer is marked confirmed only for the known target-call
  source line and matching callee name; all other errors remain unknown.
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

The offline fixture covers two packages in one module, with methods
of identical spelling, and callers across package boundaries, a transitive caller,
interface dispatch exclusion, baseline compilation, a hypothetical
changed-API compilation failure, and exact source-file integrity.
The real-module regression verifies that the existing MBVM-DB CLI has
statically resolved calls to its own library package.

## Acceptance regression for a provider with internal calls

```bash
CGO_ENABLED=0 go test -count=1 -timeout=180s -v \\
  -run 'Test(PrepareContractOverlay|SelfConsistentProvider|ProviderOverlayFailure|KnownTargetCallLine)' \\
  ./internal/impact
```

The test fixture gives the provider an internal caller in a separate
file and an external client in another package. The provider must
compile after its type-resolved internal calls are adapted virtually;
its original client must fail at the known call site. Another fixture
keeps a typed function-value assignment incompatible with the changed
signature in the provider, to verify that the external result remains
inconclusive rather than falsely confirmed. This is a hypothetical
signature mutation, not a real Git version diff.

### Opt-in real MBVM-DB acceptance gate

The core fixture tests run by default. To test the full repository
and require compiler witnesses for **every known direct consumer** of
`pkg/mbvmdb.New`, run this separate, slower test on a matching Go
toolchain and populated module cache:

```powershell
$env:CGO_ENABLED = "0"
$env:MBVM_RUN_REAL_OVERLAY = "1"
go test -count=1 -timeout=10m -v -run '^TestRealModuleSelfConsistentNewOverlay$' ./internal/impact
```

The test fails if the virtual provider does not compile or any
known consumer remains inconclusive. The expected number is derived
from the actual module graph, **not hardcoded as five**. This real
gate is opt-in to keep the default unit/CI run bounded. The source
hash assertions cover all selected packages.
